// Package workspace owns the index lifecycle: the bootstrap sequence, the
// watcher pipeline, and the single-writer indexer that serializes every
// state mutation (§4 동시성 모델).
package workspace

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/llls2542/graphin/internal/ignore"
	"github.com/llls2542/graphin/internal/lexical"
	"github.com/llls2542/graphin/internal/lock"
	"github.com/llls2542/graphin/internal/mcp"
	"github.com/llls2542/graphin/internal/merkle"
	"github.com/llls2542/graphin/internal/obs"
	"github.com/llls2542/graphin/internal/search"
	"github.com/llls2542/graphin/internal/watch"
)

// DataDirName is the per-workspace data directory (§2.5).
const DataDirName = ".graphin"

// ErrLockHeld re-exports the lock error for tool handlers.
var ErrLockHeld = lock.ErrHeld

// Config carries startup flags (§6.4) into the workspace.
type Config struct {
	Root      string
	Workers   int
	ModelType string // default for bootstrap_workspace's model_type
	Offline   bool
	ModelDir  string
	OrtLib    string
	Log       *obs.Logger
}

// Workspace holds all engines for one indexed source tree.
type Workspace struct {
	Root string
	Dir  string // <root>/.graphin
	FSM  *FSM
	Log  *obs.Logger

	Sym    *lexical.SymbolTable
	Lex    *lexical.Index
	Router *search.Router

	cfg Config

	// indexMu serializes every index mutation (merkle, lexical, symtab,
	// node metadata): the single-writer discipline with a synchronous
	// escape hatch for read_code's inline reparse.
	indexMu sync.Mutex
	merkle  *merkle.Tree

	nodesMu sync.RWMutex
	nodes   map[string]NodeMeta

	matcherMu sync.Mutex
	matcher   *ignore.Matcher

	// Track-B consumers; nil until the graph (P3) and semantic (P4) engines
	// are wired in.
	embedder merkle.Embedder
	edgeSink merkle.EdgeSink

	mu           sync.Mutex
	bootstrapped bool
	lk           *lock.Lock
	cancel       context.CancelFunc
}

func New(cfg Config) *Workspace {
	if cfg.Workers <= 0 {
		cfg.Workers = runtime.NumCPU()
	}
	sym := lexical.NewSymbolTable()
	ix := lexical.NewIndex()
	return &Workspace{
		Root:   cfg.Root,
		Dir:    filepath.Join(cfg.Root, DataDirName),
		FSM:    &FSM{},
		Log:    cfg.Log,
		Sym:    sym,
		Lex:    ix,
		Router: &search.Router{Sym: sym, Lex: ix},
		cfg:    cfg,
		merkle: merkle.NewTree(),
		nodes:  map[string]NodeMeta{},
	}
}

// Bootstrapped reports whether Bootstrap has completed successfully.
func (w *Workspace) Bootstrapped() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.bootstrapped
}

// Bootstrap acquires the workspace lock, restores persisted indexes, starts
// the watcher pipeline and kicks off indexing in the background. It returns
// quickly (§3.1); progress is reported through Status on later tool calls.
func (w *Workspace) Bootstrap(ctx context.Context, modelType string, offline bool) (mcp.Status, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.bootstrapped {
		return w.FSM.Status(), nil
	}
	if modelType == "" {
		modelType = w.cfg.ModelType
	}

	if err := os.MkdirAll(w.Dir, 0o755); err != nil {
		return w.FSM.Status(), err
	}
	lk, err := lock.Acquire(filepath.Join(w.Dir, "lockfile"), lock.Options{}, w.Log)
	if err != nil {
		return w.FSM.Status(), err
	}

	// Restore prior state so the initial scan only pays for what changed.
	// The lexical snapshot and merkle tree must move together: without the
	// lexical docs, unchanged (OffsetOnly) nodes would never re-enter the
	// symbol table, so a missing/corrupt snapshot forces a full re-index.
	lex, sym, lerr := lexical.Load(filepath.Join(w.Dir, "search", "lexical.idx"))
	mt, merr := merkle.Load(filepath.Join(w.Dir, "merkle.json"))
	if lerr == nil && merr == nil && lex.Len() > 0 {
		w.Lex, w.Sym = lex, sym
		w.Router.Lex, w.Router.Sym = lex, sym
		w.merkle = mt
	}

	runCtx, cancel := context.WithCancel(context.Background())
	deb := watch.NewDebouncer(0, 0) // spec defaults: 500ms quiet, 2s cap
	watcher, err := watch.NewWatcher(w.Root, deb, w.Log)
	if err != nil {
		cancel()
		_ = lk.Release()
		return w.FSM.Status(), err
	}

	w.lk = lk
	w.cancel = cancel
	go deb.Run(runCtx)
	go watcher.Run(runCtx)
	go w.consumeBatches(runCtx, deb.Out())

	w.FSM.Set(PhaseIndexing)
	w.bootstrapped = true
	w.Log.Event("bootstrap", map[string]any{
		"root": w.Root, "model_type": modelType, "offline": offline || w.cfg.Offline,
	})

	go w.initialScan(runCtx)
	return w.FSM.Status(), nil
}

// consumeBatches drains debounced watcher batches into the indexer.
func (w *Workspace) consumeBatches(ctx context.Context, batches <-chan watch.Batch) {
	for {
		select {
		case <-ctx.Done():
			return
		case b := <-batches:
			w.handleBatch(b)
		}
	}
}

// Close stops background goroutines and releases the lock.
func (w *Workspace) Close() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.cancel != nil {
		w.cancel()
		w.cancel = nil
	}
	if w.lk != nil {
		_ = w.lk.Release()
		w.lk = nil
	}
}
