package lock

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Salvia95/graphin/internal/obs"
)

func lockPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "lockfile")
}

// deadPID returns the PID of an already-exited process.
func deadPID(t *testing.T) int {
	t.Helper()
	cmd := exec.Command("/bin/true")
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	return cmd.Process.Pid
}

// TestOrphanLockStolen proves §7-P1-③: a lockfile whose owner is dead and
// whose heartbeat is stale gets stolen automatically.
func TestOrphanLockStolen(t *testing.T) {
	path := lockPath(t)
	if err := os.WriteFile(path, []byte(fmt.Sprintf("%d\n", deadPID(t))), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-11 * time.Second)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}

	lk, err := Acquire(path, Options{Heartbeat: time.Hour}, obs.Nop())
	if err != nil {
		t.Fatalf("expected steal, got %v", err)
	}
	defer lk.Release()

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(b)) != strconv.Itoa(os.Getpid()) {
		t.Fatalf("lockfile PID = %q, want ours %d", strings.TrimSpace(string(b)), os.Getpid())
	}
}

// TestDeadOwnerFreshMtimeStolen: a dead PID is an orphan even when the mtime
// is still fresh (owner crashed moments ago).
func TestDeadOwnerFreshMtimeStolen(t *testing.T) {
	path := lockPath(t)
	if err := os.WriteFile(path, []byte(fmt.Sprintf("%d\n", deadPID(t))), 0o644); err != nil {
		t.Fatal(err)
	}
	lk, err := Acquire(path, Options{Heartbeat: time.Hour}, obs.Nop())
	if err != nil {
		t.Fatalf("expected steal of dead owner, got %v", err)
	}
	lk.Release()
}

// TestLiveLockRefused proves a live owner (alive PID + fresh mtime) keeps the
// lock: the second acquirer gets ErrHeld.
func TestLiveLockRefused(t *testing.T) {
	path := lockPath(t)
	if err := os.WriteFile(path, []byte(fmt.Sprintf("%d\n", os.Getpid())), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Acquire(path, Options{Heartbeat: time.Hour}, obs.Nop())
	if !errors.Is(err, ErrHeld) {
		t.Fatalf("expected ErrHeld, got %v", err)
	}
}

// TestHeartbeatUpdatesMtime proves the 3s(here compressed) mtime heartbeat.
func TestHeartbeatUpdatesMtime(t *testing.T) {
	path := lockPath(t)
	lk, err := Acquire(path, Options{Heartbeat: 50 * time.Millisecond}, obs.Nop())
	if err != nil {
		t.Fatal(err)
	}
	defer lk.Release()

	old := time.Now().Add(-time.Minute)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)

	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(fi.ModTime()) > 5*time.Second {
		t.Fatalf("heartbeat did not refresh mtime: %v", fi.ModTime())
	}
}

func TestReleaseRemovesLockfile(t *testing.T) {
	path := lockPath(t)
	lk, err := Acquire(path, Options{Heartbeat: time.Hour}, obs.Nop())
	if err != nil {
		t.Fatal(err)
	}
	if err := lk.Release(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("lockfile still present after Release")
	}
	if err := lk.Release(); err != nil { // second release must be safe
		t.Fatalf("double release errored: %v", err)
	}
}
