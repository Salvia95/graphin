package console

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Salvia95/graphin/internal/wiki"
)

// TestConsoleDoesNotReachTheIndex turns the package comment into a check.
//
// The rule is easy to break by accident and expensive when broken: importing
// anything that reaches internal/graph means opening the engine, and opening
// it truncates the reverse delta log of a workspace another process may be
// holding. Nothing about that failure is loud — the console would work, and
// the server would lose its log.
//
// -deps rather than direct imports, because the mistake arrives through a
// helper three packages down, not through an import line someone would notice.
func TestConsoleDoesNotReachTheIndex(t *testing.T) {
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go toolchain not on PATH")
	}
	out, err := exec.Command(goBin, "list", "-deps", ".").Output()
	if err != nil {
		t.Fatalf("go list -deps: %v", err)
	}
	forbidden := []string{
		"github.com/Salvia95/graphin/internal/graph",
		"github.com/Salvia95/graphin/internal/workspace",
	}
	for _, dep := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		for _, bad := range forbidden {
			if dep == bad {
				t.Errorf("console depends on %s — it must never open the index", bad)
			}
		}
	}
}

// TestLoopbackOnly checks the assumption the design spends: authentication and
// CORS are omitted *because* the socket is unreachable from another machine.
// If the address is not enforced, that sentence stops being true and nothing
// else in the design compensates.
func TestLoopbackOnly(t *testing.T) {
	for _, addr := range []string{"127.0.0.1:7673", "localhost:7673", "[::1]:7673"} {
		if err := checkLoopback(addr); err != nil {
			t.Errorf("checkLoopback(%q) = %v, want nil", addr, err)
		}
	}
	for _, addr := range []string{"0.0.0.0:7673", ":7673", "192.168.1.10:7673"} {
		if err := checkLoopback(addr); err == nil {
			t.Errorf("checkLoopback(%q) = nil, want a refusal", addr)
		}
	}
}

// TestQueueEndpointIsTheCommandsAnswer pins the shell to the function. If the
// handler ever grows its own filtering or shaping, the console and the command
// start answering the same question differently, which is the failure the
// split existed to prevent.
func TestQueueEndpointIsTheCommandsAnswer(t *testing.T) {
	root := t.TempDir()
	rec := httptest.NewRecorder()
	NewMux(root, "").ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/queue", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body)
	}
	want, err := wiki.BuildQueueReport(root)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	var got wiki.QueueReport
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("response is not a QueueReport: %v\n%s", err, rec.Body)
	}
	a, _ := json.Marshal(got)
	b, _ := json.Marshal(want)
	if string(a) != string(b) {
		t.Errorf("endpoint and command disagree:\n endpoint %s\n command  %s", a, b)
	}
}

// TestConsoleIsReadOnly guards the step boundary. Writing is a decided feature
// with a decided limit (the working tree, never a commit) and it arrives with
// its own path validation; until then answering a POST would be inventing that
// design by accident.
func TestConsoleIsReadOnly(t *testing.T) {
	rec := httptest.NewRecorder()
	NewMux(t.TempDir(), "").ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/queue", nil))

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST status = %d, want 405", rec.Code)
	}
	if allow := rec.Header().Get("Allow"); allow != "GET, HEAD" {
		t.Errorf("Allow = %q, want %q", allow, "GET, HEAD")
	}
}

// TestFailuresStayJSON matters because the caller is a fetch(), not a person.
// A workspace with no usage log is an ordinary state — the hook may simply
// never have fired — and an HTML error page there turns a legible message into
// a parse error in the browser console.
func TestFailuresStayJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	NewMux(t.TempDir(), "").ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/usage", nil))

	if rec.Code == http.StatusOK {
		t.Skip("a usage log resolved unexpectedly; nothing to assert")
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want JSON", ct)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("error body is not JSON: %v\n%s", err, rec.Body)
	}
	if body["error"] == "" {
		t.Errorf("error body has no message: %s", rec.Body)
	}
}

func queueCandidate(t *testing.T, root string) {
	t.Helper()
	st, err := wiki.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Propose(&wiki.Term{
		Canonical: "posting",
		Body:      "A unit of published writing.",
		Evidence:  []string{"pkg.a", "pkg.b"},
	}); err != nil {
		t.Fatal(err)
	}
}

// TestApproveEndpointStopsAtTheWorkingTree checks the write and, just as
// importantly, that the response says where it stopped. A UI rendering this
// cannot leave someone believing the term is published when what actually
// happened is an uncommitted file move.
func TestApproveEndpointStopsAtTheWorkingTree(t *testing.T) {
	root := t.TempDir()
	queueCandidate(t, root)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/queue/posting/approve",
		strings.NewReader(`{"title":"Posting","description":"A unit of published writing."}`))
	NewMux(root, "").ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body)
	}
	var got struct {
		Term *wiki.Term `json:"term"`
		File string     `json:"file"`
		Note string     `json:"note"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("response: %v\n%s", err, rec.Body)
	}
	if got.Term == nil || got.Term.Title != "Posting" {
		t.Errorf("edits were not applied: %+v", got.Term)
	}
	if got.File != wiki.GlossaryPath(root, "posting") {
		t.Errorf("file = %q, want the glossary path", got.File)
	}
	if !strings.Contains(got.Note, "not committed") {
		t.Errorf("note does not state the boundary: %q", got.Note)
	}
	if _, err := os.Stat(wiki.GlossaryPath(root, "posting")); err != nil {
		t.Errorf("glossary entry missing: %v", err)
	}
	if _, err := os.Stat(wiki.ProposalPath(root, "posting")); !errors.Is(err, os.ErrNotExist) {
		t.Error("candidate still queued")
	}
}

// TestApproveEndpointMapsRefusals keeps the failures a reviewer can cause
// distinguishable at the wire. A form that cannot tell "no such candidate"
// from "the glossary is full" has to say "something went wrong", which is the
// friction this surface exists to remove.
func TestApproveEndpointMapsRefusals(t *testing.T) {
	root := t.TempDir()
	mux := NewMux(root, "")

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/queue/nothing/approve", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("unknown candidate = %d, want 404: %s", rec.Code, rec.Body)
	}

	queueCandidate(t, root)
	live := wiki.GlossaryPath(root, "posting")
	if err := os.MkdirAll(filepath.Dir(live), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(live,
		[]byte("---\ntype: glossary\ncanonical: posting\n---\n\nHere.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/queue/posting/approve", nil))
	if rec.Code != http.StatusConflict {
		t.Errorf("clobber = %d, want 409: %s", rec.Code, rec.Body)
	}
}

func TestDiscardEndpoint(t *testing.T) {
	root := t.TempDir()
	queueCandidate(t, root)
	mux := NewMux(root, "")

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/queue/posting/discard", nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204: %s", rec.Code, rec.Body)
	}
	if _, err := os.Stat(wiki.ProposalPath(root, "posting")); !errors.Is(err, os.ErrNotExist) {
		t.Error("candidate survived discard")
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/queue/posting/discard", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("second discard = %d, want 404", rec.Code)
	}
}
