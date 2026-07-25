package workspace

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Salvia95/graphin/internal/obs"
)

func waitLexical(t *testing.T, ws *Workspace) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && ws.FSM.Phase() < PhaseLexicalReady {
		time.Sleep(10 * time.Millisecond)
	}
	if !ws.FSM.Status().LexicalReady {
		t.Fatal("lexical never became ready")
	}
}

// TestSemanticNodeGate: a tree past SemanticMaxNodes trips the gate mid-scan —
// semantic goes dark (router lexical-only), status says so, and a marker makes
// the next bootstrap skip semantic setup entirely. The gate check reads the
// prior-files node count, so several files are needed to cross a ceiling of 1.
func TestSemanticNodeGate(t *testing.T) {
	root := t.TempDir()
	for _, n := range []string{"A", "B", "C", "D", "E"} {
		if err := os.WriteFile(filepath.Join(root, n+".java"),
			[]byte("class "+n+" {}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	ws := New(Config{Root: root, Log: obs.Nop(), OrtLib: "/nonexistent-ort", SemanticMaxNodes: 1})
	if _, err := ws.Bootstrap(context.Background(), "", false); err != nil {
		t.Fatal(err)
	}
	waitLexical(t, ws)

	if !ws.semGated.Load() {
		t.Fatal("semantic was not gated despite nodes > SemanticMaxNodes")
	}
	if ws.Router.SemanticReady() {
		t.Fatal("router reports semantic ready while gated")
	}
	st := ws.statusWithDB()
	if !st.SemanticGated || st.State != "ready" || st.SemanticNote == "" {
		t.Fatalf("gated status wrong: %+v", st)
	}
	marker := filepath.Join(root, DataDirName, "semantic-gated.json")
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("gate marker not written: %v", err)
	}
	ws.Close()

	// Restart: the marker must gate before any engine setup (w.sem stays nil).
	ws2 := New(Config{Root: root, Log: obs.Nop(), OrtLib: "/nonexistent-ort", SemanticMaxNodes: 1})
	defer ws2.Close()
	if _, err := ws2.Bootstrap(context.Background(), "", false); err != nil {
		t.Fatal(err)
	}
	waitLexical(t, ws2)
	if !ws2.semGated.Load() {
		t.Fatal("restart did not re-gate from marker")
	}
	if ws2.sem != nil {
		t.Fatal("marker gate should skip engine setup, but w.sem is set")
	}
}

// TestSemanticNotGatedUnderCeiling: below the ceiling (and with the limit off),
// semantic is wired normally and nothing is gated.
func TestSemanticNotGatedUnderCeiling(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "A.java"), []byte("class A {}"), 0o644); err != nil {
		t.Fatal(err)
	}
	ws := New(Config{Root: root, Log: obs.Nop(), OrtLib: "/nonexistent-ort", SemanticMaxNodes: 1000})
	defer ws.Close()
	if _, err := ws.Bootstrap(context.Background(), "", false); err != nil {
		t.Fatal(err)
	}
	waitLexical(t, ws)
	if ws.semGated.Load() {
		t.Fatal("semantic gated below the ceiling")
	}
	if _, err := os.Stat(filepath.Join(root, DataDirName, "semantic-gated.json")); err == nil {
		t.Fatal("marker written when not gated")
	}
}
