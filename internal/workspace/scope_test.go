package workspace

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Salvia95/graphin/internal/obs"
)

// scopeWS boots a workspace on a temp root seeded with `files` and waits for
// the initial scan to land.
func scopeWS(t *testing.T, files map[string]string) (*Workspace, string) {
	t.Helper()
	root := t.TempDir()
	writeAll(t, root, files)
	ws := New(Config{Root: root, Log: obs.Nop(), OrtLib: "/nonexistent-ort"})
	t.Cleanup(ws.Close)
	if _, err := ws.Bootstrap(context.Background(), "", false); err != nil {
		t.Fatal(err)
	}
	waitLexical(t, ws)
	return ws, root
}

func writeAll(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, body := range files {
		abs := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(abs, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// rescan reruns the first-pass scan the way a fresh bootstrap would, which is
// the moment exclusions are supposed to take effect.
func rescan(t *testing.T, ws *Workspace) {
	t.Helper()
	ws.initialScan(context.Background())
}

func TestScopeCountsFilesAndNodesPerGroup(t *testing.T) {
	ws, _ := scopeWS(t, map[string]string{
		"A.java":        "class A { void a() {} }",
		"pkg/B.java":    "class B { void b() {} }",
		"docs/guide.md": "# One\ntext\n## Two\nmore\n",
	})
	rep, err := ws.Scope()
	if err != nil {
		t.Fatal(err)
	}
	if rep.Estimated {
		t.Fatal("a bootstrapped workspace must report exact counts, not an estimate")
	}
	if rep.Files != 3 {
		t.Fatalf("files = %d, want 3 (%+v)", rep.Files, rep.Dirs)
	}
	if rep.Nodes == 0 {
		t.Fatal("nodes = 0 after indexing three files")
	}
	byName := map[string]ScopeEntry{}
	for _, e := range rep.Dirs {
		byName[e.Name] = e
	}
	if got := byName["docs"].Files; got != 1 {
		t.Fatalf("docs files = %d, want 1", got)
	}
	if byName["docs"].MDHeadings == 0 {
		t.Fatal("markdown headings must be broken out: they are what makes node counts jump")
	}
	if got := byName[rootGroup].Files; got != 1 {
		t.Fatalf("root-level files = %d, want 1 (A.java)", got)
	}
}

func TestScopePreviewBeforeBootstrapIsEstimated(t *testing.T) {
	root := t.TempDir()
	writeAll(t, root, map[string]string{
		"A.java":        "class A {}",
		"docs/guide.md": "# One\n## Two\n### Three\n",
	})
	ws := New(Config{Root: root, Log: obs.Nop(), OrtLib: "/nonexistent-ort"})
	t.Cleanup(ws.Close)

	rep, err := ws.Scope() // no bootstrap: falls through to the preview
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Estimated {
		t.Fatal("a scan-based report must say so; otherwise it reads as a measured index")
	}
	if rep.Files != 2 {
		t.Fatalf("files = %d, want 2", rep.Files)
	}
	if rep.Headings != 3 {
		t.Fatalf("headings = %d, want 3", rep.Headings)
	}
}

// Headings inside a fenced block are shell comments, not sections.
func TestPreviewHeadingCountIgnoresFencedBlocks(t *testing.T) {
	root := t.TempDir()
	writeAll(t, root, map[string]string{
		"doc.md": "# Real\n\n```sh\n# not a heading\n## also not\n```\n\n## Also real\n",
	})
	ws := New(Config{Root: root, Log: obs.Nop(), OrtLib: "/nonexistent-ort"})
	t.Cleanup(ws.Close)
	rep, err := ws.ScopePreview()
	if err != nil {
		t.Fatal(err)
	}
	if rep.Headings != 2 {
		t.Fatalf("headings = %d, want 2 (fenced lines are not sections)", rep.Headings)
	}
}

func TestScopeImpactCountsWhatWouldGo(t *testing.T) {
	ws, _ := scopeWS(t, map[string]string{
		"keep/A.java": "class A {}",
		"tmp/B.java":  "class B {}",
		"tmp/C.java":  "class C {}",
	})
	imp, err := ws.ScopeImpactOf([]string{"tmp/"})
	if err != nil {
		t.Fatal(err)
	}
	if imp.Files != 2 {
		t.Fatalf("files = %d, want 2 (%+v)", imp.Files, imp.Sample)
	}
	if imp.Nodes == 0 {
		t.Fatal("nodes = 0: a directory of indexed classes must carry nodes")
	}
	for _, s := range imp.Sample {
		if !strings.HasPrefix(s, "tmp/") {
			t.Fatalf("sample leaked a file outside the pattern: %q", s)
		}
	}
}

// Extension patterns are the case this tool exists for: scratch text files
// that are indexed as whole-file nodes.
func TestScopeImpactMatchesExtensionPattern(t *testing.T) {
	ws, _ := scopeWS(t, map[string]string{
		"A.java":     "class A {}",
		"notes.txt":  "scratch",
		"other.txt":  "scratch too",
		"keep/D.txt": "nested scratch",
	})
	imp, err := ws.ScopeImpactOf([]string{"*.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if imp.Files != 3 {
		t.Fatalf("files = %d, want 3 (%+v)", imp.Files, imp.Sample)
	}
}

func TestScopeAddWritesPatternAndReason(t *testing.T) {
	ws, root := scopeWS(t, map[string]string{
		"A.java":     "class A {}",
		"tmp/B.java": "class B {}",
	})
	if _, err := ws.ScopeAdd([]string{"tmp/"}, "빌드 스크래치"); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(root, DataDirName, "ignore"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	if !strings.Contains(got, "tmp/") {
		t.Fatalf("pattern not written:\n%s", got)
	}
	if !strings.Contains(got, "빌드 스크래치") {
		t.Fatalf("reason not recorded — the file must explain itself:\n%s", got)
	}
	if pats := ws.excludedPatterns(); len(pats) != 1 || pats[0] != "tmp/" {
		t.Fatalf("excludedPatterns = %v, want [tmp/]", pats)
	}
}

// The contract: writing a pattern changes nothing until the next scan.
func TestExclusionTakesEffectOnlyAtNextBootstrap(t *testing.T) {
	ws, _ := scopeWS(t, map[string]string{
		"keep/A.java": "class A {}",
		"tmp/B.java":  "class B {}",
	})
	before, err := ws.Scope()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ws.ScopeAdd([]string{"tmp/"}, "scratch"); err != nil {
		t.Fatal(err)
	}
	mid, err := ws.Scope()
	if err != nil {
		t.Fatal(err)
	}
	if mid.Files != before.Files || mid.Nodes != before.Nodes {
		t.Fatalf("index changed before a bootstrap: %d/%d → %d/%d",
			before.Files, before.Nodes, mid.Files, mid.Nodes)
	}

	rescan(t, ws)

	after, err := ws.Scope()
	if err != nil {
		t.Fatal(err)
	}
	if after.Files != 1 {
		t.Fatalf("files after rescan = %d, want 1 (tmp/B.java should be gone)", after.Files)
	}
	if after.Nodes >= before.Nodes {
		t.Fatalf("nodes did not drop: %d → %d", before.Nodes, after.Nodes)
	}
}

// The same reconciliation covers a plain deletion that no watcher saw, which
// is the bug that made a removed file's nodes survive a restart.
func TestRescanPrunesDeletedFile(t *testing.T) {
	ws, root := scopeWS(t, map[string]string{
		"A.java": "class A {}",
		"B.java": "class B {}",
	})
	if err := os.Remove(filepath.Join(root, "B.java")); err != nil {
		t.Fatal(err)
	}
	rescan(t, ws)

	rep, err := ws.Scope()
	if err != nil {
		t.Fatal(err)
	}
	if rep.Files != 1 {
		t.Fatalf("files = %d, want 1 after deleting B.java", rep.Files)
	}
	ws.indexMu.Lock()
	_, still := ws.merkle.Files["B.java"]
	ws.indexMu.Unlock()
	if still {
		t.Fatal("merkle still records a file the walk no longer reaches")
	}
}

func TestScopeRemoveRestoresAtNextBootstrap(t *testing.T) {
	ws, _ := scopeWS(t, map[string]string{
		"keep/A.java": "class A {}",
		"tmp/B.java":  "class B {}",
	})
	if _, err := ws.ScopeAdd([]string{"tmp/"}, "scratch"); err != nil {
		t.Fatal(err)
	}
	rescan(t, ws)
	if rep, _ := ws.Scope(); rep.Files != 1 {
		t.Fatalf("setup: files = %d, want 1", rep.Files)
	}

	if _, err := ws.ScopeRemove([]string{"tmp/"}); err != nil {
		t.Fatal(err)
	}
	if pats := ws.excludedPatterns(); len(pats) != 0 {
		t.Fatalf("pattern still listed after removal: %v", pats)
	}
	rescan(t, ws)

	rep, err := ws.Scope()
	if err != nil {
		t.Fatal(err)
	}
	if rep.Files != 2 {
		t.Fatalf("files = %d, want 2 after un-excluding", rep.Files)
	}
}

func TestScopeRemoveRejectsUnknownPattern(t *testing.T) {
	ws, _ := scopeWS(t, map[string]string{"A.java": "class A {}"})
	if _, err := ws.ScopeAdd([]string{"tmp/"}, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := ws.ScopeRemove([]string{"never-added/"}); err == nil {
		t.Fatal("removing a pattern that is not there must fail rather than silently rewrite the file")
	}
	if pats := ws.excludedPatterns(); len(pats) != 1 {
		t.Fatalf("a failed removal must leave the file alone, got %v", pats)
	}
}

func TestScopeWarnsOnPinnedWikiSections(t *testing.T) {
	ws, _ := scopeWS(t, map[string]string{
		"A.java":                "class A {}",
		"docs/subsystem.md":     "# Rules\nbody\n",
		"docs/wiki/pins.lock":   `{"v":1,"pins":{"set":{"docs/subsystem.md#rules":{"hash":"b3:x"}}}}`,
		"docs/wiki/sets/set.md": "# Set\n- entry\n",
	})
	imp, err := ws.ScopeImpactOf([]string{"docs/subsystem.md"})
	if err != nil {
		t.Fatal(err)
	}
	if len(imp.Warnings) == 0 {
		t.Fatal("cutting a pinned section must warn: the sets that cite it break")
	}
	joined := strings.Join(imp.Warnings, " ")
	if !strings.Contains(joined, "핀한 문서") {
		t.Fatalf("warning does not name the pinned documents: %q", joined)
	}
}

func TestScopeWarnsOnLargeShare(t *testing.T) {
	ws, _ := scopeWS(t, map[string]string{
		"big/A.java": "class A { void a() {} void b() {} void c() {} }",
		"big/B.java": "class B { void d() {} void e() {} void f() {} }",
		"small.java": "class S {}",
	})
	imp, err := ws.ScopeImpactOf([]string{"big/"})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(imp.Warnings, " ")
	if !strings.Contains(joined, "%") {
		t.Fatalf("cutting most of the index must state the share, got %q", joined)
	}
}

// A pattern that matches nothing is a typo far more often than an intention,
// and the impact report is where that shows.
func TestScopeImpactOfUnmatchedPatternIsEmpty(t *testing.T) {
	ws, _ := scopeWS(t, map[string]string{"A.java": "class A {}"})
	imp, err := ws.ScopeImpactOf([]string{"no/such/path/"})
	if err != nil {
		t.Fatal(err)
	}
	if imp.Files != 0 || imp.Nodes != 0 {
		t.Fatalf("unmatched pattern reported %d files / %d nodes", imp.Files, imp.Nodes)
	}
}
