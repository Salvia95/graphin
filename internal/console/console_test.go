package console

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

// TestRootServesSomethingEitherWay covers both legitimate binaries: one built
// after `make ui` and one built by `go build` alone. Which of the two a test
// run sees depends on whether dist/ happens to be built on this machine, so
// the assertion is what must hold in both — a page, not a 404.
func TestRootServesSomethingEitherWay(t *testing.T) {
	rec := httptest.NewRecorder()
	NewMux(t.TempDir(), "").ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("GET / = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q, want HTML", ct)
	}
}

// TestExplicitUIDirectoryWins pins the precedence development depends on:
// --ui serves from disk without rebuilding the binary between edits, which is
// only useful if it beats whatever was compiled in.
func TestExplicitUIDirectoryWins(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<h1>from disk</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	NewMux(t.TempDir(), dir).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if !strings.Contains(rec.Body.String(), "from disk") {
		t.Errorf("--ui did not win:\n%s", rec.Body)
	}
}

// writeFile is the fixture helper for the endpoints that read a real wiki.
func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// wikiRoot is a workspace with one document, one set citing two of its
// sections, and one queued candidate.
func wikiRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "docs", "target.md"),
		"# Target\n\n## Section one\n\nFirst body.\n\n## Section two\n\nSecond body.\n")
	writeFile(t, filepath.Join(root, "docs", "wiki", "sets", "s.md"),
		"---\ntype: knowledge_set\ndescription: two sections\nroles: []\nmode: live\n---\n\n"+
			"# S\n\n## G\n\n"+
			"- [one](../../target.md#section-one) — first summary\n"+
			"- [two](../../target.md#section-two) — second summary\n")
	writeFile(t, filepath.Join(root, "docs", "wiki", "propose", "만료_기한.md"),
		"---\ntype: glossary\ncanonical: 만료 기한\ndescription: 스스로 만료를 선언하는 기간\n"+
			"tags: []\naliases:\n  - stale_after\nscope: []\nevidence:\n  - a.go:1\n  - b.go:2\n"+
			"status: draft\nseen: 2\nlast_verified: 2026-08-01\n---\n\n본문이다.\n")
	return root
}

// TestCandidateEndpointCarriesWhatTheFormFills. The queue list does not carry a
// proposal's body or aliases, so a form built from it would arrive blank and a
// reviewer would retype what the proposer already wrote.
func TestCandidateEndpointCarriesWhatTheFormFills(t *testing.T) {
	root := wikiRoot(t)
	rec := httptest.NewRecorder()
	NewMux(root, "").ServeHTTP(rec,
		httptest.NewRequest(http.MethodGet, "/api/queue/"+url.PathEscape("만료 기한"), nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("GET candidate = %d: %s", rec.Code, rec.Body)
	}
	var got struct {
		Canonical string   `json:"canonical"`
		Body      string   `json:"body"`
		Aliases   []string `json:"aliases"`
		Evidence  []string `json:"evidence"`
		Seen      int      `json:"seen"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Canonical != "만료 기한" || got.Body == "" || len(got.Aliases) != 1 || len(got.Evidence) != 2 {
		t.Errorf("candidate = %+v, want the proposal's own fields", got)
	}
	if got.Seen != 2 {
		t.Errorf("seen = %d, want 2", got.Seen)
	}
}

func TestCandidateEndpointIs404ForAnUnqueuedName(t *testing.T) {
	rec := httptest.NewRecorder()
	NewMux(wikiRoot(t), "").ServeHTTP(rec,
		httptest.NewRequest(http.MethodGet, "/api/queue/nothing", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

// TestRepinScopesToOneEntry guards the difference the drift card promises:
// re-read this section, confirm this summary, record that. Repinning
// everything would clear warnings nobody looked at.
func TestRepinScopesToOneEntry(t *testing.T) {
	root := wikiRoot(t)
	mux := NewMux(root, "")

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/wiki/repin", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("repin all = %d: %s", rec.Code, rec.Body)
	}
	// Both sections move; only one is verified.
	writeFile(t, filepath.Join(root, "docs", "target.md"),
		"# Target\n\n## Section one\n\nRewritten one.\n\n## Section two\n\nRewritten two.\n")

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/wiki/repin",
		strings.NewReader(`{"set":"s","node_id":"docs/target.md#section-one"}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("repin one = %d: %s", rec.Code, rec.Body)
	}

	o, err := wiki.BuildOverview(root, "")
	if err != nil {
		t.Fatal(err)
	}
	if o.Health.Drifted != 1 {
		t.Fatalf("drifted = %d, want 1 — the unverified entry must keep its warning", o.Health.Drifted)
	}
}

func TestRepinRefusesHalfAPair(t *testing.T) {
	rec := httptest.NewRecorder()
	NewMux(wikiRoot(t), "").ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/wiki/repin",
		strings.NewReader(`{"set":"s"}`)))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 — a set with no node is not a repin-everything request", rec.Code)
	}
}

func TestWorkspaceEndpointNamesTheRepository(t *testing.T) {
	root := wikiRoot(t)
	rec := httptest.NewRecorder()
	NewMux(root, "").ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/workspace", nil))

	var got workspace
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Name != filepath.Base(root) || !filepath.IsAbs(got.Root) {
		t.Errorf("workspace = %+v, want the absolute root and its base name", got)
	}
}

// TestEventsAnnouncesAChange is the whole point of the stream: every action a
// card names happens in an editor, so the page has to hear about it.
func TestEventsAnnouncesAChange(t *testing.T) {
	root := wikiRoot(t)
	srv := httptest.NewServer(NewMux(root, ""))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/api/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if ct := res.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("Content-Type = %q, want an event stream", ct)
	}

	sc := bufio.NewScanner(res.Body)
	// The greeting proves the first digest was taken before anything moved, so
	// the change below cannot be the stream noticing its own start-up.
	for sc.Scan() {
		if strings.HasPrefix(sc.Text(), "event: hello") {
			break
		}
	}

	go func() {
		time.Sleep(200 * time.Millisecond)
		writeFile(t, filepath.Join(root, "docs", "target.md"),
			"# Target\n\n## Section one\n\nEdited in an editor.\n\n## Section two\n\nSecond body.\n")
	}()

	for sc.Scan() {
		if strings.HasPrefix(sc.Text(), "event: change") {
			return
		}
	}
	t.Fatalf("no change announced: %v", sc.Err())
}
