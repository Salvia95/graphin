// Package workspace owns the index lifecycle: the bootstrap sequence, the
// watcher pipeline, and the single-writer indexer that serializes every
// state mutation (§4 동시성 모델).
package workspace

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Salvia95/graphin/internal/graph"
	"github.com/Salvia95/graphin/internal/ignore"
	"github.com/Salvia95/graphin/internal/lexical"
	"github.com/Salvia95/graphin/internal/lock"
	"github.com/Salvia95/graphin/internal/mcp"
	"github.com/Salvia95/graphin/internal/merkle"
	"github.com/Salvia95/graphin/internal/obs"
	"github.com/Salvia95/graphin/internal/provision"
	"github.com/Salvia95/graphin/internal/search"
	"github.com/Salvia95/graphin/internal/semantic"
	"github.com/Salvia95/graphin/internal/watch"
)

// DataDirName is the per-workspace data directory (§2.5).
const DataDirName = ".graphin"

// ErrLockHeld re-exports the lock error for tool handlers.
var ErrLockHeld = lock.ErrHeld

// SemanticSink receives Track-B embedding work (implemented by
// *semantic.Engine; tests substitute recorders).
type SemanticSink interface {
	Enqueue(id, summary string)
	Remove(id string)
}

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
	// node metadata, graph): the single-writer discipline with a synchronous
	// escape hatch for read_code's inline reparse.
	indexMu sync.Mutex
	merkle  *merkle.Tree
	graph   *graph.Engine

	nodesMu sync.RWMutex
	nodes   map[string]NodeMeta

	matcherMu sync.Mutex
	matcher   *ignore.Matcher

	// Semantic engine (Track-B embedding consumer). semSink is the narrow
	// interface applyFileResult talks to, so tests can substitute recorders.
	sem        *semantic.Engine
	semSink    SemanticSink
	forceEmbed map[string]bool // files stale vs vectors.bin header (§2.2)

	mu           sync.Mutex
	bootstrapped bool
	lk           *lock.Lock
	cancel       context.CancelFunc
	bg           sync.WaitGroup // goroutines touching engines; Close waits

	semErr atomic.Pointer[string] // permanent semantic warmup failure
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
	eng, err := graph.Open(filepath.Join(w.Dir, "graph"), w.Log)
	if err != nil {
		return w.FSM.Status(), err
	}
	w.graph = eng

	// Semantic engine: restore vectors.bin, then mark files whose hash moved
	// since that export as force-embed — the crash-recovery diff (§2.2).
	if err := os.MkdirAll(filepath.Join(w.Dir, "search"), 0o755); err != nil {
		return w.FSM.Status(), err
	}
	w.sem = semantic.New(filepath.Join(w.Dir, "search", "vectors.bin"), w.merkleSnapshot, w.Log)
	w.semSink = w.sem
	w.Router.Sem = w.sem
	current := make(map[string]string, len(w.merkle.Files))
	for rel, fe := range w.merkle.Files {
		current[rel] = fe.Hash
	}
	w.forceEmbed = w.sem.StaleFiles(current)

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

	w.FSM.Set(PhaseIndexing)
	w.bootstrapped = true
	w.Log.Event("bootstrap", map[string]any{
		"root": w.Root, "model_type": modelType, "offline": offline || w.cfg.Offline,
	})

	w.bg.Add(2)
	go func() { defer w.bg.Done(); w.consumeBatches(runCtx, deb.Out()) }()
	go func() { defer w.bg.Done(); w.initialScan(runCtx) }()
	// warmup is not waited on (provisioning may be mid-download at shutdown);
	// it works on captured pointers, never on torn-down fields.
	go w.warmupSemantic(runCtx, w.sem, modelType, offline || w.cfg.Offline)
	return w.FSM.Status(), nil
}

// merkleSnapshot feeds the vectors.bin export header (§2.2).
func (w *Workspace) merkleSnapshot() (string, map[string]string) {
	w.indexMu.Lock()
	defer w.indexMu.Unlock()
	files := make(map[string]string, len(w.merkle.Files))
	for rel, fe := range w.merkle.Files {
		files[rel] = fe.Hash
	}
	root := w.merkle.Root()
	return merkle.Hex(root), files
}

// warmupSemantic provisions and loads the embedding stack in the background
// (§2.1.1 단계적 가용성): failures leave the server on lexical fallback with
// semantic_ready=false, never blocking tools.
func (w *Workspace) warmupSemantic(ctx context.Context, sem *semantic.Engine, modelType string, offline bool) {
	cacheDir := ""
	if ucd, err := os.UserCacheDir(); err == nil {
		cacheDir = filepath.Join(ucd, "graphin", "artifacts")
	}
	paths, err := provision.Resolve(modelType, provision.Options{
		RuntimeDir: filepath.Join(w.Dir, "runtime"),
		CacheDir:   cacheDir,
		Offline:    offline,
		ModelDir:   w.cfg.ModelDir,
		OrtLib:     w.cfg.OrtLib,
		Log:        w.Log,
	})
	if err != nil {
		w.Log.Event("semantic_unavailable", map[string]any{"error": err.Error()})
		msg := err.Error()
		w.semErr.Store(&msg)
		return
	}
	if err := sem.Warmup(paths.OrtLib, paths.Model, paths.Tokenizer,
		paths.Spec.ID, paths.Spec.Dim, paths.Spec.QueryPrefix, paths.Spec.PassagePrefix); err != nil {
		w.Log.Event("semantic_unavailable", map[string]any{"error": err.Error()})
		msg := err.Error()
		w.semErr.Store(&msg)
		return
	}
	w.watchSemanticReady(ctx, sem)
}

// watchSemanticReady flips the FSM once both lexical indexing and the model
// warmup are done.
func (w *Workspace) watchSemanticReady(ctx context.Context, sem *semantic.Engine) {
	t := time.NewTicker(100 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if w.FSM.Phase() >= PhaseLexicalReady && sem.Ready() {
				w.FSM.Set(PhaseSemanticReady)
				return
			}
		}
	}
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
	w.bg.Wait() // engines must outlive every background consumer
	if w.sem != nil {
		w.sem.Close()
		w.sem = nil
	}
	if w.graph != nil {
		w.graph.Close()
		w.graph = nil
	}
	if w.lk != nil {
		_ = w.lk.Release()
		w.lk = nil
	}
}

// Explore proxies to the graph engine (§3.3).
func (w *Workspace) Explore(nodeID, direction, cursor string, minConf float32) (*graph.Page, error) {
	w.mu.Lock()
	eng := w.graph
	w.mu.Unlock()
	if eng == nil {
		return nil, ErrNodeNotFound
	}
	page, err := eng.Explore(nodeID, direction, cursor, minConf)
	if err != nil {
		if errors.Is(err, graph.ErrNodeNotFound) {
			return nil, ErrNodeNotFound
		}
		return nil, err
	}
	return page, nil
}

// SemUnavailable reports a permanent semantic warmup failure ("" if none):
// surfaced as MODEL_UNAVAILABLE with a lexical-fallback hint (§3.0).
func (w *Workspace) SemUnavailable() string {
	if p := w.semErr.Load(); p != nil {
		return *p
	}
	return ""
}
