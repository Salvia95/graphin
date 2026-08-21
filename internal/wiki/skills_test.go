package wiki

import (
	"path/filepath"
	"strings"
	"testing"
)

func roleStore(t *testing.T) *Store {
	t.Helper()
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "docs", "target.md"), targetDoc)
	writeSet(t, root, "layering", "roles: [backend]\n")
	writeSet(t, root, "everywhere", "roles: [all]\n")
	writeSet(t, root, "pullonly", "roles: []\n")
	mustWrite(t, filepath.Join(root, DirName, glossarySubdir, "posting.md"),
		"---\ncanonical: posting\naliases: [post]\nscope: [backend]\n"+
			"not_to_be_confused_with:\n  - blog — a posting is a unit, a blog is a medium\n"+
			"---\nA unit of published writing. Longer prose follows.\n")
	mustWrite(t, filepath.Join(root, DirName, "agents.md"),
		"---\nagents:\n  - backend-dev — backend\n  - lint-bot — exempt\n---\n")

	s, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestRolesExcludeTheWildcard(t *testing.T) {
	// "all" lands in every role's block, so a skill named for it would
	// duplicate the same text under a name no agent is assigned.
	got := roleStore(t).Roles()
	if len(got) != 1 || got[0] != "backend" {
		t.Fatalf("Roles = %v", got)
	}
}

func TestGeneratedSkillReferencesSetsWithoutCopyingThem(t *testing.T) {
	g := roleStore(t).GenerateSkill("backend")

	if !strings.Contains(g.Body, "`layering`") || !strings.Contains(g.Body, "`everywhere`") {
		t.Fatalf("role and wildcard sets should both appear:\n%s", g.Body)
	}
	if strings.Contains(g.Body, "pullonly") {
		t.Fatalf("a set with no roles is pull-only and must not be pushed:\n%s", g.Body)
	}
	// A skill is procedural and a set is declarative. Copying the second into
	// the first makes two copies of one fact, and the copy here is the one
	// that goes stale unseen.
	if strings.Contains(g.Body, "a summary") {
		t.Fatalf("entry text was copied into the push block:\n%s", g.Body)
	}
	if !strings.Contains(g.Body, "wiki_resolve") {
		t.Fatalf("no way to load the referenced sets:\n%s", g.Body)
	}
}

func TestGeneratedSkillInlinesDefinitions(t *testing.T) {
	g := roleStore(t).GenerateSkill("backend")
	// Terms are the exception to reference-don't-copy: a reader who does not
	// know they are using the wrong word will not go looking for the right one.
	if !strings.Contains(g.Body, "A unit of published writing.") {
		t.Fatalf("definition not inlined:\n%s", g.Body)
	}
	if !strings.Contains(g.Body, "Not **blog**") {
		t.Fatalf("confusion note lost:\n%s", g.Body)
	}
	// One sentence, not the whole page.
	if strings.Contains(g.Body, "Longer prose follows") {
		t.Fatalf("the whole body was inlined:\n%s", g.Body)
	}
}

func TestGeneratedSkillIsDeterministic(t *testing.T) {
	st := roleStore(t)
	// Regeneration must produce no diff when nothing changed, or the check
	// below becomes noise people learn to ignore.
	if st.GenerateSkill("backend").Body != st.GenerateSkill("backend").Body {
		t.Fatal("generation is not deterministic")
	}
}

func TestPushCapIsReportedInTheArtifact(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "docs", "target.md"), targetDoc)
	for i := 0; i < PushCap+3; i++ {
		writeSet(t, root, "set"+string(rune('a'+i)), "roles: [backend]\n")
	}
	st, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	g := st.GenerateSkill("backend")

	if len(g.Dropped) != 3 {
		t.Fatalf("dropped = %v, want 3", g.Dropped)
	}
	// Whoever reads this block later is the one who needs to know it is
	// partial, so it has to say so in the artifact, not only in a log line.
	if !strings.Contains(g.Body, "capped") || !strings.Contains(g.Body, "did not fit") {
		t.Fatalf("the block does not admit it is partial:\n%s", g.Body)
	}
}

func TestStaleSkillsDetectsDrift(t *testing.T) {
	st := roleStore(t)
	dir := filepath.Join(st.Root, ".claude", "skills")

	if stale := st.StaleSkills(dir); len(stale) != 1 {
		t.Fatalf("a missing file must read as stale: %v", stale)
	}
	if _, err := st.WriteSkills(dir); err != nil {
		t.Fatal(err)
	}
	if stale := st.StaleSkills(dir); len(stale) != 0 {
		t.Fatalf("freshly written skills are stale: %v", stale)
	}

	mustWrite(t, filepath.Join(dir, "backend-conventions", "SKILL.md"), "hand-edited\n")
	// A generated artifact that drifts from its source still reads as
	// authoritative, which is worse than it being absent.
	if stale := st.StaleSkills(dir); len(stale) != 1 {
		t.Fatalf("hand edits must read as stale: %v", stale)
	}
}

func TestAgentsNeedingReportsRatherThanEdits(t *testing.T) {
	got := roleStore(t).AgentsNeeding()
	agents := got["backend-conventions"]
	if len(agents) != 1 || agents[0] != "backend-dev" {
		t.Fatalf("mapping = %v", got)
	}
	// An exempt agent carries no role and therefore no block.
	for _, list := range got {
		for _, a := range list {
			if a == "lint-bot" {
				t.Fatal("an exempt agent must not be assigned a convention skill")
			}
		}
	}
}
