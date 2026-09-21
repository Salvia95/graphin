package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Salvia95/graphin/internal/mcp"
	"github.com/Salvia95/graphin/internal/obs"
	"github.com/Salvia95/graphin/internal/workspace"
)

// session is one server over root, as cmd/graphin assembles it.
type session struct {
	ws  *workspace.Workspace
	reg *mcp.Registry
}

func newSession(root, version string) *session {
	ws := workspace.New(workspace.Config{Root: root, Log: obs.Nop(), OrtLib: "/nonexistent-ort"})
	reg := mcp.NewRegistry()
	Register(reg, ws)
	Follow(reg, ws, version)
	return &session{ws: ws, reg: reg}
}

func (s *session) call(t *testing.T, tool, args string) (string, bool) {
	t.Helper()
	tl, ok := s.reg.Get(tool)
	if !ok {
		t.Fatalf("no tool %q", tool)
	}
	return tl.Handler(context.Background(), json.RawMessage(args))
}

func indexedRoot(t *testing.T) string {
	t.Helper()
	// Short on purpose: t.TempDir() names are long enough to push the socket
	// out of the data dir, and both placements deserve a run.
	root, err := os.MkdirTemp("", "gf")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(root) })
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	if err := os.WriteFile(filepath.Join(root, "A.java"), []byte("class A { void hello(){} }"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func waitLexical(t *testing.T, s *session) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if s.ws.FSM.Status().LexicalReady {
			if _, err := os.Stat(filepath.Join(s.ws.Dir, "merkle.json")); err == nil {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("index never became ready")
}

// TestSecondSessionAnswersThroughTheFirst is the whole point: two servers in
// one directory, one index, the same answer from both.
func TestSecondSessionAnswersThroughTheFirst(t *testing.T) {
	root := indexedRoot(t)
	a := newSession(root, "0.5.0")
	defer a.ws.Close()
	if out, isErr := a.call(t, "bootstrap_workspace", `{}`); isErr {
		t.Fatalf("leader bootstrap: %s", out)
	}
	waitLexical(t, a)

	b := newSession(root, "0.5.1")
	defer b.ws.Close()
	q := `{"query":"hello"}`
	want, _ := a.call(t, "search_hybrid", q)
	got, isErr := b.call(t, "search_hybrid", q)
	if isErr || !strings.Contains(got, `id="A.hello()"`) {
		t.Fatalf("follower search: isErr=%v\n%s", isErr, got)
	}
	if got != want {
		t.Fatalf("follower and leader disagree:\n--- leader\n%s\n--- follower\n%s", want, got)
	}
	if b.ws.Bootstrapped() {
		t.Fatal("the follower opened its own index")
	}
	// the ritual still works from a follower: it reports the leader's state
	if out, isErr := b.call(t, "bootstrap_workspace", `{}`); isErr || strings.Contains(out, "LOCK_HELD") {
		t.Fatalf("bootstrap through the leader: %s", out)
	}
}

// TestFollowerTakesOverWhenTheLeaderLeaves: the next call after the leader is
// gone is answered — by this server, which now holds the lock and listens.
func TestFollowerTakesOverWhenTheLeaderLeaves(t *testing.T) {
	root := indexedRoot(t)
	a := newSession(root, "0.5.0")
	a.call(t, "bootstrap_workspace", `{}`)
	waitLexical(t, a)

	b := newSession(root, "0.5.0")
	defer b.ws.Close()
	if out, isErr := b.call(t, "search_hybrid", `{"query":"hello"}`); isErr {
		t.Fatalf("follower search: %s", out)
	}
	a.ws.Close()

	out, isErr := b.call(t, "search_hybrid", `{"query":"hello"}`)
	if isErr || !strings.Contains(out, `id="A.hello()"`) {
		t.Fatalf("search after the leader left: isErr=%v\n%s", isErr, out)
	}
	if !b.ws.Bootstrapped() {
		t.Fatal("the survivor did not take the lock")
	}
	// and the survivor is a leader in full: a third session follows it
	c := newSession(root, "0.5.0")
	defer c.ws.Close()
	if out, isErr := c.call(t, "search_hybrid", `{"query":"hello"}`); isErr || c.ws.Bootstrapped() {
		t.Fatalf("third session: isErr=%v bootstrapped=%v\n%s", isErr, c.ws.Bootstrapped(), out)
	}
}

// TestSuccessionNeedsNoToolCall: the index must not sit unwatched just
// because the surviving session is idle.
func TestSuccessionNeedsNoToolCall(t *testing.T) {
	root := indexedRoot(t)
	a := newSession(root, "0.5.0")
	a.call(t, "bootstrap_workspace", `{}`)
	waitLexical(t, a)
	b := newSession(root, "0.5.0")
	defer b.ws.Close()
	b.call(t, "search_hybrid", `{"query":"hello"}`)
	a.ws.Close()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && !b.ws.Bootstrapped() {
		time.Sleep(20 * time.Millisecond)
	}
	if !b.ws.Bootstrapped() {
		t.Fatal("idle follower never took over")
	}
}

// TestVersionSkewSaysWhatToDo: a refused handshake must not look like a
// missing index, and must name the session to restart.
func TestVersionSkewSaysWhatToDo(t *testing.T) {
	root := indexedRoot(t)
	a := newSession(root, "0.5.0")
	defer a.ws.Close()
	a.call(t, "bootstrap_workspace", `{}`)
	waitLexical(t, a)

	b := newSession(root, "0.6.0")
	defer b.ws.Close()
	out, isErr := b.call(t, "search_hybrid", `{"query":"hello"}`)
	if !isErr || !strings.Contains(out, `code="LOCK_HELD"`) ||
		!strings.Contains(out, "0.5.0") || !strings.Contains(out, "Restart") {
		t.Fatalf("skew response: isErr=%v\n%s", isErr, out)
	}
}

// TestNeverIndexedStillWaitsForBootstrap: following must not change the
// first-index contract.
func TestNeverIndexedStillWaitsForBootstrap(t *testing.T) {
	root := indexedRoot(t)
	s := newSession(root, "0.5.0")
	defer s.ws.Close()
	out, isErr := s.call(t, "search_hybrid", `{"query":"hello"}`)
	if !isErr || !strings.Contains(out, `code="NOT_BOOTSTRAPPED"`) {
		t.Fatalf("fresh tree: isErr=%v\n%s", isErr, out)
	}
}
