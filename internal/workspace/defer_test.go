package workspace

// Phase 7c 후속 ②: watcher batches must be deferred until lexical-ready so a
// live edit during the initial scan cannot be clobbered by the scan's own
// in-flight stale read of the same file. These tests drive consumeBatches
// directly with a manual batch channel and a manually-closed lexReady.

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Salvia95/graphin/internal/obs"
	"github.com/Salvia95/graphin/internal/scan"
	"github.com/Salvia95/graphin/internal/watch"
)

func minimalWorkspace(t *testing.T, root string) *Workspace {
	t.Helper()
	w := New(Config{Root: root, Log: obs.Nop(), OrtLib: "/nonexistent-ort"})
	res, err := scan.Walk(root, obs.Nop())
	if err != nil {
		t.Fatal(err)
	}
	w.matcher = res.Matcher // graph/sem stay nil — plain-file indexing needs neither
	return w
}

func batchFor(root, rel string) watch.Batch {
	abs := filepath.Join(root, rel)
	return watch.Batch{abs: watch.Change{Path: abs, Kind: watch.Modified}}
}

// runConsumer starts consumeBatches on its own goroutine and joins it at test
// end BEFORE t.TempDir's cleanup runs. t.Cleanup is LIFO and TempDir registers
// its RemoveAll first, so this cleanup runs earlier: cancel stops the loop,
// then <-done waits for its final persistIndexes to finish — otherwise an
// in-flight index write into .graphin/search races RemoveAll ("directory not
// empty"). consumeBatches is the only file-writing goroutine here
// (minimalWorkspace skips Bootstrap, so no watcher/scan/engine run).
func runConsumer(t *testing.T, w *Workspace, batches chan watch.Batch) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); w.consumeBatches(ctx, batches) }()
	t.Cleanup(func() { cancel(); <-done })
}

func waitIndexed(t *testing.T, w *Workspace, id string, want time.Duration) NodeMeta {
	t.Helper()
	deadline := time.Now().Add(want)
	for time.Now().Before(deadline) {
		if m, ok := w.nodeMeta(id); ok {
			return m
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("node %q never indexed within %s", id, want)
	return NodeMeta{}
}

// TestWatcherBatchDeferredUntilLexicalReady: a batch delivered before
// lexical-ready is held, then applied once ready — never before.
func TestWatcherBatchDeferredUntilLexicalReady(t *testing.T) {
	root := t.TempDir()
	rel := "config/app.yml"
	if err := os.MkdirAll(filepath.Join(root, "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, rel), []byte("db: v1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	w := minimalWorkspace(t, root)

	batches := make(chan watch.Batch)
	runConsumer(t, w, batches)

	batches <- batchFor(root, rel) // arrives during "scan" (lexReady still open)

	// The batch must NOT be applied while lexical-ready is pending. Give the
	// consumer a real chance to (wrongly) apply it.
	time.Sleep(150 * time.Millisecond)
	if _, ok := w.nodeMeta(rel); ok {
		t.Fatal("watcher batch applied before lexical-ready — deferral broken")
	}

	w.markLexicalReady() // scan finished: release deferred batches
	waitIndexed(t, w, rel, 2*time.Second)
}

// TestDeferredBatchesReplayInArrivalOrder: two batches touching the same file
// before ready must replay in order, so the last edit wins.
func TestDeferredBatchesReplayInArrivalOrder(t *testing.T) {
	root := t.TempDir()
	rel := "notes.md"
	write := func(s string) {
		if err := os.WriteFile(filepath.Join(root, rel), []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("first content\n")
	w := minimalWorkspace(t, root)

	batches := make(chan watch.Batch)
	runConsumer(t, w, batches)

	// Two deferred batches; the on-disk file ends at the final content. Order
	// is preserved but each handleBatch re-reads disk, so the end state must
	// reflect the latest bytes regardless — assert the node exists and reads
	// the final content after release.
	batches <- batchFor(root, rel)
	write("second content with searchable_token\n")
	batches <- batchFor(root, rel)

	if _, ok := w.nodeMeta(rel); ok {
		t.Fatal("batches applied before ready")
	}
	w.markLexicalReady()
	waitIndexed(t, w, rel, 2*time.Second)

	cb, err := w.ReadCode(rel)
	if err != nil {
		t.Fatal(err)
	}
	if cb.Code != "second content with searchable_token\n" {
		t.Fatalf("final content not reflected: %q", cb.Code)
	}
}

// TestLiveBatchesAppliedAfterReady: once ready, batches apply immediately
// (no deferral), preserving steady-state behavior.
func TestLiveBatchesAppliedAfterReady(t *testing.T) {
	root := t.TempDir()
	rel := "live.yml"
	if err := os.WriteFile(filepath.Join(root, rel), []byte("k: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	w := minimalWorkspace(t, root)
	w.markLexicalReady() // already ready before any batch

	batches := make(chan watch.Batch)
	runConsumer(t, w, batches)

	batches <- batchFor(root, rel)
	waitIndexed(t, w, rel, 2*time.Second)
}
