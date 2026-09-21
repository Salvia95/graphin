package workspace

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Salvia95/graphin/internal/obs"
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

// TestEnsureBootstrappedOnlyRestores pins the line the auto-bootstrap draws:
// a tree never indexed waits for the explicit call (the first index is the
// user's decision), a tree indexed before comes up on its own, a held lock
// leaves the explicit path open, and nothing bootstraps after Close.
func TestEnsureBootstrappedOnlyRestores(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "A.java"), []byte("class A {}"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	cfg := Config{Root: root, Log: obs.Nop(), OrtLib: "/nonexistent-ort"}

	fresh := New(cfg)
	if fresh.EnsureBootstrapped(ctx, "test") {
		t.Fatal("never-indexed workspace bootstrapped itself")
	}
	if _, err := fresh.Bootstrap(ctx, "", false); err != nil {
		t.Fatal(err)
	}
	merklePath := filepath.Join(root, DataDirName, "merkle.json")
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(merklePath); err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := os.Stat(merklePath); err != nil {
		t.Fatalf("merkle.json never saved: %v", err)
	}

	// lock still held by the first server: fail quietly, stay unbootstrapped
	blocked := New(cfg)
	if blocked.EnsureBootstrapped(ctx, "test") {
		t.Fatal("bootstrapped under a held lock")
	}
	blocked.Close()
	fresh.Close()

	restored := New(cfg)
	if !restored.EnsureBootstrapped(ctx, "test") {
		t.Fatal("indexed workspace did not restore itself")
	}
	restored.Close()

	closed := New(cfg)
	closed.Close()
	if closed.EnsureBootstrapped(ctx, "test") {
		t.Fatal("bootstrapped after Close")
	}
}

// closerFunc adapts a func to io.Closer.
type closerFunc func() error

func (f closerFunc) Close() error { return f() }

// TestLeaderHookLifecycle: the hook runs once the lock is held, never for a
// server that lost it, and its Closer is closed without w.mu held — closing
// the follower socket drains forwarded calls, and handlers take w.mu.
func TestLeaderHookLifecycle(t *testing.T) {
	root := t.TempDir()
	cfg := Config{Root: root, Log: obs.Nop(), OrtLib: "/nonexistent-ort"}
	ctx := context.Background()

	ws := New(cfg)
	started, closed := 0, 0
	ws.OnLeader(func(dir string) (io.Closer, error) {
		if dir != ws.Dir {
			t.Errorf("hook dir = %q, want %q", dir, ws.Dir)
		}
		started++
		return closerFunc(func() error {
			closed++
			ws.Bootstrapped() // takes w.mu: deadlocks if Close still holds it
			return nil
		}), nil
	})
	if _, err := ws.Bootstrap(ctx, "", false); err != nil {
		t.Fatal(err)
	}
	if _, err := ws.Bootstrap(ctx, "", false); err != nil { // idempotent: no second listener
		t.Fatal(err)
	}

	loser := New(cfg)
	loserStarted := false
	loser.OnLeader(func(string) (io.Closer, error) {
		loserStarted = true
		return closerFunc(func() error { return nil }), nil
	})
	if _, err := loser.Bootstrap(ctx, "", false); !errors.Is(err, ErrLockHeld) {
		t.Fatalf("expected ErrLockHeld, got %v", err)
	}
	loser.Close()
	if loserStarted {
		t.Fatal("a server without the lock started listening")
	}

	done := make(chan struct{})
	go func() { ws.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Close deadlocked closing the leader")
	}
	if started != 1 || closed != 1 {
		t.Fatalf("started=%d closed=%d, want 1/1", started, closed)
	}
}
