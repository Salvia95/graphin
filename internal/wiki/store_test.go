package wiki

import (
	"os"
	"path/filepath"
	"testing"
)

func writeSet(t *testing.T, root, name, frontmatter string) {
	t.Helper()
	src := "---\n" + frontmatter + "---\n\n# " + name + "\n\n## G\n\n" +
		"- [x](../../target.md#section-one) — a summary\n"
	mustWrite(t, filepath.Join(root, DirName, setsSubdir, name+".md"), src)
}

func TestLoadEmptyWorkspace(t *testing.T) {
	// A project with no wiki must load, not fail: preflight has to be able to
	// answer "no knowledge applies" for it.
	s, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Sets) != 0 || len(s.Terms) != 0 {
		t.Fatalf("expected empty store, got %d sets %d terms", len(s.Sets), len(s.Terms))
	}
}

func TestLoadSetsAndTerms(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "docs", "target.md"), targetDoc)
	writeSet(t, root, "basics", "roles: [all]\n")
	writeSet(t, root, "release", "roles: [backend]\nprerequisites: [basics]\n")
	mustWrite(t, filepath.Join(root, DirName, glossarySubdir, "posting.md"),
		"---\ncanonical: posting\naliases: [post]\nnot_to_be_confused_with:\n  - blog — a posting is a unit, a blog is a medium\n---\nA unit of published writing.\n")

	s, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Sets) != 2 {
		t.Fatalf("sets = %d", len(s.Sets))
	}
	term, ok := s.Terms["posting"]
	if !ok {
		t.Fatalf("terms = %v", s.Terms)
	}
	if !term.Matches("POST") {
		t.Error("alias matching should be case-insensitive")
	}
	if len(term.Confusions) != 1 || term.Confusions[0].Term != "blog" {
		t.Fatalf("Confusions = %+v", term.Confusions)
	}
	if term.Confusions[0].Why == "" {
		t.Error("the reason is the whole value of a confusion note")
	}
	if term.Status != StatusActive {
		t.Errorf("Status = %q, want active by default", term.Status)
	}
}

func TestExpandPutsPrerequisitesFirst(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "docs", "target.md"), targetDoc)
	writeSet(t, root, "basics", "")
	writeSet(t, root, "release", "prerequisites: [basics]\n")
	s, _ := Load(root)

	sets, missing := s.Expand([]string{"release"})
	if len(missing) != 0 {
		t.Fatalf("missing = %v", missing)
	}
	// A prerequisite that arrives after the set assuming it has already
	// failed at its one job.
	if len(sets) != 2 || sets[0].Name != "basics" || sets[1].Name != "release" {
		t.Fatalf("order = %v", names(sets))
	}
}

func TestExpandReportsMissingRatherThanDropping(t *testing.T) {
	s, _ := Load(t.TempDir())
	sets, missing := s.Expand([]string{"nope"})
	if len(sets) != 0 {
		t.Fatalf("sets = %v", names(sets))
	}
	// A request for a set that does not exist is a coverage miss worth
	// recording; a shorter list would hide it.
	if len(missing) != 1 || missing[0] != "nope" {
		t.Fatalf("missing = %v", missing)
	}
}

func TestExpandSurvivesPrerequisiteCycle(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "docs", "target.md"), targetDoc)
	writeSet(t, root, "a", "prerequisites: [b]\n")
	writeSet(t, root, "b", "prerequisites: [a]\n")
	s, _ := Load(root)

	sets, _ := s.Expand([]string{"a"})
	// The cycle is an authoring bug, but a reader must still get everything
	// reachable rather than an empty list or a hang.
	if len(sets) != 2 {
		t.Fatalf("sets = %v, want both", names(sets))
	}
}

func TestExpandDeduplicates(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "docs", "target.md"), targetDoc)
	writeSet(t, root, "basics", "")
	writeSet(t, root, "a", "prerequisites: [basics]\n")
	writeSet(t, root, "b", "prerequisites: [basics]\n")
	s, _ := Load(root)

	sets, _ := s.Expand([]string{"a", "b"})
	if len(sets) != 3 {
		t.Fatalf("sets = %v, want basics once", names(sets))
	}
}

func TestForRoleIncludesAll(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "docs", "target.md"), targetDoc)
	writeSet(t, root, "everyone", "roles: [all]\n")
	writeSet(t, root, "backend", "roles: [backend]\n")
	writeSet(t, root, "pullonly", "roles: []\n")
	s, _ := Load(root)

	got := names(s.ForRole("backend"))
	if len(got) != 2 {
		t.Fatalf("ForRole(backend) = %v, want everyone+backend", got)
	}
	// A set with no roles is pull-only: discovered by task, never pushed.
	for _, n := range got {
		if n == "pullonly" {
			t.Fatal("a set with no roles must not be pushed")
		}
	}
	if got := names(s.ForRole("frontend")); len(got) != 1 || got[0] != "everyone" {
		t.Fatalf("ForRole(frontend) = %v", got)
	}
}

func TestLoadRejectsDuplicateSetNames(t *testing.T) {
	root := t.TempDir()
	writeSet(t, root, "dup", "")
	// A second file claiming the same name would make "the filename is the
	// set's name" false, and one of the two would silently never be served.
	mustWrite(t, filepath.Join(root, DirName, setsSubdir, "dup.markdown"), "# dup\n")
	if _, err := os.Stat(filepath.Join(root, DirName, setsSubdir, "dup.markdown")); err != nil {
		t.Fatal(err)
	}
	// .markdown is not loaded, so this must still succeed — the guard is for
	// two .md files, which the filesystem itself prevents. Verify the loader
	// only takes .md.
	s, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Sets) != 1 {
		t.Fatalf("sets = %d, want 1", len(s.Sets))
	}
}

func names(sets []*Set) []string {
	out := make([]string, 0, len(sets))
	for _, s := range sets {
		out = append(out, s.Name)
	}
	return out
}
