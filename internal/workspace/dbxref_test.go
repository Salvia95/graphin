package workspace

// Phase 7b: cross-domain edge invalidation when the DB registry moves after
// bootstrap (docs/phase7-spec.md §1.6). Batches are driven synchronously via
// handleBatch — no watcher timing in these tests.

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Salvia95/graphin/internal/obs"
	"github.com/Salvia95/graphin/internal/watch"
)

const xrefFixtureDir = "../../testdata/fixtures/dbxref"

func bootstrapXrefWorkspace(t *testing.T, withSnapshot bool) *Workspace {
	t.Helper()
	root := t.TempDir()
	copyTree(t, filepath.Join(xrefFixtureDir, "src"), filepath.Join(root, "src"))
	if withSnapshot {
		copyTree(t, filepath.Join(xrefFixtureDir, "db"), filepath.Join(root, "db"))
	}
	w := New(Config{Root: root, Log: obs.Nop(), OrtLib: "/nonexistent-ort"})
	t.Cleanup(w.Close)
	if _, err := w.Bootstrap(context.Background(), "", false); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) && w.FSM.Phase() < PhaseLexicalReady {
		time.Sleep(20 * time.Millisecond)
	}
	if w.FSM.Phase() < PhaseLexicalReady {
		t.Fatal("initial scan never finished")
	}
	return w
}

func xrefEdge(t *testing.T, w *Workspace, id, target string) bool {
	t.Helper()
	p, err := w.graph.Explore(id, "uses", "", 0.5)
	if err != nil {
		t.Fatalf("explore %s: %v", id, err)
	}
	for _, u := range p.Uses {
		if u.NodeID == target {
			return true
		}
	}
	return false
}

const (
	xrefEntity = "com.acme.JobPosting"
	xrefTable  = "db.main.public.job_posting"
)

// TestSnapshotAddInvalidatesXrefs: 부트스트랩 이후 첫 스냅샷 커밋 시나리오 —
// 기존 코드 노드의 크로스 엣지가 재해석으로 나타나야 한다.
func TestSnapshotAddInvalidatesXrefs(t *testing.T) {
	w := bootstrapXrefWorkspace(t, false)
	if xrefEdge(t, w, xrefEntity, xrefTable) {
		t.Fatal("no snapshot yet — edge must not exist")
	}

	src, err := os.ReadFile(filepath.Join(xrefFixtureDir, "db", "main.graphindb.json"))
	if err != nil {
		t.Fatal(err)
	}
	abs := filepath.Join(w.Root, "db", "main.graphindb.json")
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, src, 0o644); err != nil {
		t.Fatal(err)
	}
	w.handleBatch(watch.Batch{abs: watch.Change{Path: abs, Kind: watch.Modified}})

	if !xrefEdge(t, w, xrefEntity, xrefTable) {
		t.Fatal("snapshot add must re-resolve existing code nodes")
	}
}

// TestSnapshotRemoveInvalidatesXrefs: 스냅샷 삭제 시 코드 쪽 stale 엣지가
// 역인덱스 소스 추적으로 정리된다.
func TestSnapshotRemoveInvalidatesXrefs(t *testing.T) {
	w := bootstrapXrefWorkspace(t, true)
	if !xrefEdge(t, w, xrefEntity, xrefTable) {
		t.Fatal("bootstrap with snapshot must resolve the edge")
	}

	abs := filepath.Join(w.Root, "db", "main.graphindb.json")
	if err := os.Remove(abs); err != nil {
		t.Fatal(err)
	}
	w.handleBatch(watch.Batch{abs: watch.Change{Path: abs, Kind: watch.Removed}})

	if xrefEdge(t, w, xrefEntity, xrefTable) {
		t.Fatal("snapshot removal must clear stale cross-domain edges")
	}
}
