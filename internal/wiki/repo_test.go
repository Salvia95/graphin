package wiki

import "testing"

// TestRepoWikiIsHonest is the guard that used to live in a Python script: it
// re-derives every entry's target from the documents themselves and compares.
//
// It runs here rather than as a separate CI step because nothing about it
// needs an index or a server — a section's hash is BLAKE3 over its source
// slice, so parsing the document reproduces exactly what the indexer stored.
// That property is what lets the check run anywhere, and it is worth keeping.
//
// Renaming a heading breaks every set pointing at it and leaves no trace in
// the set file. Without this, a set rots invisibly.
func TestRepoWikiIsHonest(t *testing.T) {
	const root = "../.."

	store, err := Load(root)
	if err != nil {
		t.Fatalf("load wiki: %v", err)
	}
	sets := store.SetList()
	if len(sets) == 0 {
		t.Skip("no knowledge sets in this tree")
	}

	// Generated blocks are checked here rather than in a CI step of their own:
	// it is the same question — does what we ship still match what we wrote —
	// and a step that no-ops until someone adds a role tag is noise nobody
	// reads when it finally fires.
	for _, name := range store.StaleSkills(root + "/.claude/skills") {
		t.Errorf("generated skill %s is stale — run `graphin wiki skills` and commit the result", name)
	}

	for _, p := range Check(root, sets, store.Pins) {
		switch p.Kind {
		case ProblemDangling:
			t.Errorf("%s\n    the heading was renamed or the file moved; fix the link", p)
		case ProblemDrift:
			t.Errorf("%s\n    re-read the section, confirm the summary still holds, then `graphin wiki repin`", p)
		default:
			t.Errorf("%s", p)
		}
	}
}
