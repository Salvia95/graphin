// Package workspace owns the index lifecycle: the bootstrap sequence, the
// watcher pipeline, and (from Phase 2 on) the single-writer indexer that
// serializes every state mutation.
package workspace

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/llls2542/graphin/internal/lexical"
	"github.com/llls2542/graphin/internal/lock"
	"github.com/llls2542/graphin/internal/mcp"
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
	}
}

// Bootstrapped reports whether Bootstrap has completed successfully.
func (w *Workspace) Bootstrapped() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.bootstrapped
}

// Bootstrap acquires the workspace lock, starts the watcher pipeline and
// kicks off indexing in the background. It returns quickly (§3.1): progress
// is reported through Status on subsequent tool calls. Calling it again is a
// no-op returning current status.
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

// initialScan performs the first full index pass. Phase 1 has no parser yet:
// it only opens lexical availability; Phase 2+ fill in scanning, parsing,
// merkle hashing and graph building.
func (w *Workspace) initialScan(ctx context.Context) {
	_ = ctx
	w.FSM.SetProgress(100)
	w.FSM.Set(PhaseLexicalReady)
	w.Log.Event("initial_scan_done", map[string]any{"nodes": w.Sym.Len()})
}

// consumeBatches drains debounced watcher batches. Phase 2 wires this into
// the indexer job queue; for now batches are only logged.
func (w *Workspace) consumeBatches(ctx context.Context, batches <-chan watch.Batch) {
	for {
		select {
		case <-ctx.Done():
			return
		case b := <-batches:
			w.Log.Event("watch_batch", map[string]any{"files": len(b)})
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
