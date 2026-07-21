package workspace

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/llls2542/graphin/internal/obs"
)

func TestBootstrapAcquiresLockAndOpensLexical(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "A.java"), []byte("class A {}"), 0o644); err != nil {
		t.Fatal(err)
	}

	ws := New(Config{Root: root, Log: obs.Nop(), OrtLib: "/nonexistent-ort"})
	defer ws.Close()

	st, err := ws.Bootstrap(context.Background(), "", false)
	if err != nil {
		t.Fatal(err)
	}
	if st.State == "not_bootstrapped" {
		t.Fatalf("status after bootstrap: %+v", st)
	}
	if _, err := os.Stat(filepath.Join(root, DataDirName, "lockfile")); err != nil {
		t.Fatalf("lockfile missing: %v", err)
	}

	// Phase 1's initial scan flips to lexical-ready almost immediately.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && ws.FSM.Phase() < PhaseLexicalReady {
		time.Sleep(10 * time.Millisecond)
	}
	if got := ws.FSM.Status(); !got.LexicalReady {
		t.Fatalf("lexical never became ready: %+v", got)
	}
}

// TestSecondBootstrapOnSameRootIsLockHeld: a second process (simulated by a
// second Workspace) must observe LOCK_HELD while the first heartbeat is live.
func TestSecondBootstrapOnSameRootIsLockHeld(t *testing.T) {
	root := t.TempDir()

	first := New(Config{Root: root, Log: obs.Nop(), OrtLib: "/nonexistent-ort"})
	defer first.Close()
	if _, err := first.Bootstrap(context.Background(), "", false); err != nil {
		t.Fatal(err)
	}

	second := New(Config{Root: root, Log: obs.Nop(), OrtLib: "/nonexistent-ort"})
	defer second.Close()
	_, err := second.Bootstrap(context.Background(), "", false)
	if !errors.Is(err, ErrLockHeld) {
		t.Fatalf("expected ErrLockHeld, got %v", err)
	}
}
