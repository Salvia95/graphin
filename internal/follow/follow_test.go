package follow

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Salvia95/graphin/internal/obs"
)

func echo(_ context.Context, tool string, args json.RawMessage) (string, bool, bool) {
	switch tool {
	case "echo":
		return string(args), false, true
	case "fail":
		return "<error/>", true, true
	}
	return "", false, false
}

func TestCallsReachTheLeader(t *testing.T) {
	dir := t.TempDir()
	l, err := Listen(dir, "0.5.0", echo, obs.Nop())
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	c, err := Dial(dir, "0.5.3") // patch differs: still the same contract
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if c.LeaderPID != os.Getpid() || c.LeaderVer != "0.5.0" {
		t.Fatalf("handshake identity: pid=%d ver=%q", c.LeaderPID, c.LeaderVer)
	}

	// concurrent calls come back to their own callers
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			want, _ := json.Marshal(map[string]int{"n": i})
			got, isErr, known, err := c.Call(context.Background(), "echo", want)
			if err != nil || isErr || !known || got != string(want) {
				t.Errorf("call %d: got %q isErr=%v known=%v err=%v", i, got, isErr, known, err)
			}
		}(i)
	}
	wg.Wait()

	if _, isErr, known, err := c.Call(context.Background(), "fail", nil); err != nil || !isErr || !known {
		t.Fatalf("tool error must travel as a result: isErr=%v known=%v err=%v", isErr, known, err)
	}
	if _, _, known, err := c.Call(context.Background(), "nope", nil); err != nil || known {
		t.Fatalf("unknown tool: known=%v err=%v", known, err)
	}
}

func TestVersionSkewIsRefused(t *testing.T) {
	dir := t.TempDir()
	l, err := Listen(dir, "0.5.0", echo, obs.Nop())
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	_, err = Dial(dir, "0.6.0")
	var inc *IncompatibleError
	if !errors.As(err, &inc) {
		t.Fatalf("expected IncompatibleError, got %v", err)
	}
	if inc.LeaderVersion != "0.5.0" || inc.LeaderPID != os.Getpid() {
		t.Fatalf("refusal must name the leader: %+v", inc)
	}
}

func TestCompatible(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want bool
	}{
		{"0.4.15", "0.4.14", true},
		{"v0.4.15", "0.4.2-rc1", true},
		{"0.5.0", "0.4.15", false},
		{"1.4.0", "0.4.0", false},
		{"dev", "dev", true},
		{"dev", "0.4.15", false},
		{"dev-abc", "dev-def", false},
	} {
		if got := Compatible(tc.a, tc.b); got != tc.want {
			t.Errorf("Compatible(%q,%q)=%v want %v", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestNoLeader(t *testing.T) {
	dir := t.TempDir()
	if _, err := Dial(dir, "0.5.0"); !errors.Is(err, ErrNoLeader) {
		t.Fatalf("no addr file: %v", err)
	}
	// a crashed leader leaves the addr file and a dead socket behind
	l, err := Listen(dir, "0.5.0", echo, obs.Nop())
	if err != nil {
		t.Fatal(err)
	}
	addr, _ := os.ReadFile(filepath.Join(dir, addrName))
	l.Close()
	if err := os.WriteFile(filepath.Join(dir, addrName), addr, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Dial(dir, "0.5.0"); !errors.Is(err, ErrNoLeader) {
		t.Fatalf("dead socket: %v", err)
	}
	// and the next leader listens over the leftovers
	l2, err := Listen(dir, "0.5.0", echo, obs.Nop())
	if err != nil {
		t.Fatalf("listen over leftovers: %v", err)
	}
	l2.Close()
}

func TestLeaderGoingAwayFailsCallsAndSignals(t *testing.T) {
	dir := t.TempDir()
	started := make(chan struct{})
	slow := func(ctx context.Context, _ string, _ json.RawMessage) (string, bool, bool) {
		close(started)
		<-ctx.Done() // the leader shutting down cancels what it was running
		return "", true, true
	}
	l, err := Listen(dir, "0.5.0", slow, obs.Nop())
	if err != nil {
		t.Fatal(err)
	}
	c, err := Dial(dir, "0.5.0")
	if err != nil {
		t.Fatal(err)
	}
	errc := make(chan error, 1)
	go func() {
		_, _, _, err := c.Call(context.Background(), "x", nil)
		errc <- err
	}()
	<-started
	l.Close()

	select {
	case <-c.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("Done never closed")
	}
	// The answer may have raced the hang-up; what must not happen is a hang.
	select {
	case <-errc:
	case <-time.After(2 * time.Second):
		t.Fatal("in-flight call hung")
	}
	if _, _, _, err := c.Call(context.Background(), "x", nil); !errors.Is(err, ErrLeaderGone) {
		t.Fatalf("call after the leader left: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, addrName)); !os.IsNotExist(err) {
		t.Fatalf("addr file survived Close: %v", err)
	}
}

func TestCancelReachesTheLeader(t *testing.T) {
	dir := t.TempDir()
	started, stopped := make(chan struct{}), make(chan struct{})
	h := func(ctx context.Context, _ string, _ json.RawMessage) (string, bool, bool) {
		close(started)
		<-ctx.Done()
		close(stopped)
		return "", true, true
	}
	l, err := Listen(dir, "0.5.0", h, obs.Nop())
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	c, err := Dial(dir, "0.5.0")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	ctx, cancel := context.WithCancel(context.Background())
	go func() { <-started; cancel() }()
	if _, _, _, err := c.Call(ctx, "x", nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("the leader kept running a call its follower abandoned")
	}
}

func TestLongWorkspacePathFallsBack(t *testing.T) {
	run := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", run)
	dir := filepath.Join(t.TempDir(), strings.Repeat("d", 60), strings.Repeat("e", 60))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	l, err := Listen(dir, "0.5.0", echo, obs.Nop())
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	if !strings.HasPrefix(l.path, run) {
		t.Fatalf("socket %q should live under the runtime dir", l.path)
	}
	c, err := Dial(dir, "0.5.0")
	if err != nil {
		t.Fatalf("dial through addr file: %v", err)
	}
	c.Close()
}
