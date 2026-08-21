package wiki

import (
	"path/filepath"
	"testing"
)

func TestTrustIsDerivedNotDeclared(t *testing.T) {
	// Deriving the tier from confirmations means nobody can assert it.
	cases := []struct {
		name     string
		reviewed []Review
		want     Trust
	}{
		{"nothing", nil, TrustUnverified},
		{"machine only", []Review{{By: "reference_agent/gemini-2.5-pro", At: "2026-08-21"}}, TrustMachine},
		{"human present", []Review{
			{By: "process:nightly", At: "2026-08-20"},
			{By: "human:tipa", At: "2026-08-21"},
		}, TrustHuman},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			term := &Term{Reviewed: tc.reviewed}
			if got := term.Trust(); got != tc.want {
				t.Fatalf("Trust = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestStaleIsADifferentQuestionFromDrift(t *testing.T) {
	term := &Term{StaleAfter: "2026-08-21"}
	if term.Stale("2026-08-20") {
		t.Error("not stale before the date")
	}
	// Content can be byte-for-byte what it was and still describe a world
	// that is gone. No hash will ever say so.
	if !term.Stale("2026-08-21") {
		t.Error("stale on the date itself")
	}
	if (&Term{}).Stale("2099-01-01") {
		t.Error("an entry with no expiry never expires")
	}
}

func TestReviewedParsesFlatPairs(t *testing.T) {
	// The Open Knowledge Format spells `verified` as {by, at} mappings, which
	// this parser cannot read. Writing strings under their key would hand a
	// conforming consumer the wrong type, so the authored key is ours and the
	// exporter translates it.
	src := "---\ncanonical: posting\nreviewed:\n  - human:tipa — 2026-08-21\n  - process:nightly — 2026-08-20\n---\nBody.\n"
	term, err := ParseTerm("docs/wiki/glossary/posting.md", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(term.Reviewed) != 2 {
		t.Fatalf("Reviewed = %+v", term.Reviewed)
	}
	if term.Reviewed[0].By != "human:tipa" || term.Reviewed[0].At != "2026-08-21" {
		t.Fatalf("first = %+v", term.Reviewed[0])
	}
	if term.Trust() != TrustHuman {
		t.Errorf("Trust = %q", term.Trust())
	}
}

func TestOKFFlatFieldsParse(t *testing.T) {
	src := "---\ncanonical: posting\ntitle: Posting\ndescription: A unit of published writing.\n" +
		"tags: [content, editorial]\nstale_after: 2027-01-01\nstatus: deprecated\n---\nBody.\n"
	term, err := ParseTerm("docs/wiki/glossary/posting.md", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if term.Title != "Posting" || term.Description != "A unit of published writing." {
		t.Fatalf("term = %+v", term)
	}
	if len(term.Tags) != 2 || term.StaleAfter != "2027-01-01" {
		t.Fatalf("term = %+v", term)
	}
	if term.Status != StatusDeprecated {
		t.Fatalf("Status = %q", term.Status)
	}
}

func TestTitleDefaultsToTheFilename(t *testing.T) {
	term, _ := ParseTerm("docs/wiki/glossary/posting.md", []byte("Body only.\n"))
	// The filename is the identity everywhere else in the system, so it is
	// what a missing title falls back to.
	if term.Canonical != "posting" || term.Title != "posting" {
		t.Fatalf("term = %+v", term)
	}
}

func TestSetDescriptionFallsBackToIntro(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "docs", "target.md"), targetDoc)

	src := "---\ntype: knowledge_set\n---\n\n# S\n\nThe opening prose.\n\n## G\n\n" +
		"- [x](../../target.md#section-one) — a summary\n"
	s, _ := ParseSet("docs/wiki/sets/s.md", []byte(src))
	if s.Summary() != "The opening prose." {
		t.Fatalf("Summary = %q", s.Summary())
	}

	src = "---\ntype: knowledge_set\ndescription: Declared instead.\n---\n\n# S\n\nThe opening prose.\n"
	s, _ = ParseSet("docs/wiki/sets/s.md", []byte(src))
	// The heading and opening are prose an author may rewrite; the field is
	// what other documents were told.
	if s.Summary() != "Declared instead." {
		t.Fatalf("Summary = %q", s.Summary())
	}
}

func TestDeprecatedTermsAreStillServed(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "docs", "target.md"), targetDoc)
	mustWrite(t, filepath.Join(root, DirName, glossarySubdir, "old.md"),
		"---\ncanonical: shard\nstatus: deprecated\n---\nWe now say partition.\n")
	store, _ := Load(root)

	// A reader who arrives with the old word needs to be told it is the old
	// word. Withholding it just leaves them using it.
	if sel := store.Select("", "rebalance the shard"); len(sel.Terms) != 1 {
		t.Fatalf("terms = %+v, want the deprecated one served", sel.Terms)
	}
}
