package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Salvia95/graphin/internal/obs"
	"github.com/Salvia95/graphin/internal/workspace"
)

func scopeTestWS(t *testing.T) (*workspace.Workspace, string) {
	t.Helper()
	root := t.TempDir()
	for rel, body := range map[string]string{
		"A.java":     "class A {}",
		"tmp/B.java": "class B {}",
	} {
		abs := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(abs, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	ws := workspace.New(workspace.Config{Root: root, Log: obs.Nop(), OrtLib: "/nonexistent-ort"})
	t.Cleanup(ws.Close)
	if _, err := ws.Bootstrap(context.Background(), "", false); err != nil {
		t.Fatal(err)
	}
	// Bootstrapped() flips before the first scan lands; the composition is
	// only meaningful once lexical is ready.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && !ws.FSM.Status().LexicalReady {
		time.Sleep(10 * time.Millisecond)
	}
	return ws, root
}

// The second return value is isError, not ok — a successful call must report
// false or every response is flagged as a failure to the client.
func callScope(t *testing.T, ws *workspace.Workspace, args string) string {
	t.Helper()
	out, isErr := scopeHandler(ws)(context.Background(), json.RawMessage(args))
	if isErr {
		t.Fatalf("successful call marked as an error: %s", out)
	}
	return out
}

// The whole point of the two-step: a call without confirm must not touch the
// file, so a person can read the impact before anything is decided.
func TestScopeToolPreviewDoesNotWrite(t *testing.T) {
	ws, root := scopeTestWS(t)
	ignorePath := filepath.Join(root, workspace.DataDirName, "ignore")

	out := callScope(t, ws, `{"add":["tmp/"],"reason":"scratch"}`)
	if !strings.Contains(out, `action="preview"`) {
		t.Fatalf("not reported as a preview:\n%s", out)
	}
	if !strings.Contains(out, "confirm=true") {
		t.Fatalf("preview must say how to confirm:\n%s", out)
	}
	if _, err := os.Stat(ignorePath); !os.IsNotExist(err) {
		t.Fatalf("preview wrote the ignore file (err=%v)", err)
	}

	out = callScope(t, ws, `{"add":["tmp/"],"reason":"scratch","confirm":true}`)
	if !strings.Contains(out, `action="excluded"`) {
		t.Fatalf("confirmed call not reported as a write:\n%s", out)
	}
	b, err := os.ReadFile(ignorePath)
	if err != nil {
		t.Fatalf("confirmed call did not write: %v", err)
	}
	if !strings.Contains(string(b), "tmp/") || !strings.Contains(string(b), "scratch") {
		t.Fatalf("pattern or reason missing:\n%s", b)
	}
}

func TestScopeToolReportsComposition(t *testing.T) {
	ws, _ := scopeTestWS(t)
	out := callScope(t, ws, `{}`)
	if !strings.Contains(out, "<index_scope ") {
		t.Fatalf("no composition element:\n%s", out)
	}
	if !strings.Contains(out, `<dir name=`) {
		t.Fatalf("no per-directory rows:\n%s", out)
	}
	if !strings.Contains(out, `<excluded patterns="0"`) {
		t.Fatalf("exclusions must be reported even when there are none:\n%s", out)
	}
}

// An empty argument object and a null both mean "just report".
func TestScopeToolAcceptsNoArguments(t *testing.T) {
	ws, _ := scopeTestWS(t)
	for _, raw := range []string{``, `{}`, `{"add":[],"remove":[]}`} {
		out, isErr := scopeHandler(ws)(context.Background(), json.RawMessage(raw))
		if isErr || !strings.Contains(out, "<index_scope ") {
			t.Fatalf("raw %q did not produce a report: isError=%v\n%s", raw, isErr, out)
		}
	}
}

func TestScopeToolRejectsAddAndRemoveTogether(t *testing.T) {
	ws, _ := scopeTestWS(t)
	out, isErr := scopeHandler(ws)(context.Background(),
		json.RawMessage(`{"add":["a/"],"remove":["b/"],"confirm":true}`))
	if !isErr {
		t.Fatalf("a refusal must set isError so the client sees it as one:\n%s", out)
	}
	if !strings.Contains(out, "<error") {
		t.Fatalf("mixed add/remove must be refused:\n%s", out)
	}
}

// A pattern that matches nothing is nearly always a typo, and the preview is
// where that has to show.
func TestScopeToolWarnsOnUnmatchedPattern(t *testing.T) {
	ws, _ := scopeTestWS(t)
	out := callScope(t, ws, `{"add":["no/such/dir/"]}`)
	if !strings.Contains(out, "<warning>") {
		t.Fatalf("unmatched pattern produced no warning:\n%s", out)
	}
}
