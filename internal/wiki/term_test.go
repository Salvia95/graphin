package wiki

import (
	"path/filepath"
	"testing"
)

func TestTermsReachTheManifest(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "docs", "target.md"), targetDoc)
	mustWrite(t, filepath.Join(root, DirName, glossarySubdir, "posting.md"),
		"---\ncanonical: posting\naliases: [post]\nnot_to_be_confused_with:\n  - blog — a posting is a unit, a blog is a medium\n---\nA unit of published writing. More prose after.\n")
	store, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}

	sel := store.Select("", "fix the posting pipeline")
	if len(sel.Terms) != 1 {
		t.Fatalf("terms = %+v", sel.Terms)
	}
	// A task that used a project word and got its definition was answered,
	// even with no set matching.
	if sel.Empty() {
		t.Fatal("a term match is not a coverage miss")
	}
	m := store.Manifest(sel, []byte("k"))
	if len(m.Terms) != 1 || m.Terms[0].Definition != "A unit of published writing." {
		t.Fatalf("manifest terms = %+v", m.Terms)
	}
	if len(m.Terms[0].Confusions) != 1 {
		t.Fatalf("confusions dropped: %+v", m.Terms[0])
	}
}

func TestTermsMatchOnNamesNotProse(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "docs", "target.md"), targetDoc)
	mustWrite(t, filepath.Join(root, DirName, glossarySubdir, "posting.md"),
		"---\ncanonical: posting\n---\nA unit of published writing about pipelines.\n")
	store, _ := Load(root)

	// Offering a definition because the task shared a word with its body is
	// noise, and noise in a glossary teaches readers to skip the block.
	if sel := store.Select("", "fix the pipeline"); len(sel.Terms) != 0 {
		t.Fatalf("terms = %+v, want none", sel.Terms)
	}
}

func TestProposedTermsAreNotServed(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "docs", "target.md"), targetDoc)
	mustWrite(t, filepath.Join(root, DirName, glossarySubdir, "draft.md"),
		"---\ncanonical: draft\nstatus: proposed\n---\nNot approved yet.\n")
	store, _ := Load(root)

	// The queue is not the glossary. Serving from it would make the human
	// gate decorative.
	if sel := store.Select("", "look at the draft"); len(sel.Terms) != 0 {
		t.Fatalf("terms = %+v, want none", sel.Terms)
	}
}
