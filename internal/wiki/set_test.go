package wiki

import "testing"

const sampleSet = "---\n" +
	"roles: [backend]\n" +
	"prerequisites: [basics]\n" +
	"aliases: [versioning, gate tier]\n" +
	"mode: pinned\n" +
	"---\n" +
	"\n" +
	"# Release\n" +
	"\n" +
	"Intro prose that is not an entry.\n" +
	"\n" +
	"## Picking the version\n" +
	"\n" +
	"- [§13.3 the rule](../../plugin-distribution.md#133-the-rule) —\n" +
	"  In 0.x, minor means the user has to fix something.\n" +
	"- [§13.2 breakage](../../plugin-distribution.md#132-breakage) — Only five surfaces break.\n" +
	"\n" +
	"## Why the workflow\n" +
	"\n" +
	"```markdown\n" +
	"- [not an entry](../../nope.md#nope) — lives inside a fence\n" +
	"```\n" +
	"\n" +
	"- [§5.2 workflow](../../plugin-distribution.md#52-workflow) — Dispatch-driven, not tag-driven.\n"

func TestParseSet(t *testing.T) {
	s, err := ParseSet("docs/wiki/sets/release.md", []byte(sampleSet))
	if err != nil {
		t.Fatal(err)
	}
	if s.Name != "release" {
		t.Errorf("Name = %q", s.Name)
	}
	if s.Mode != ModePinned {
		t.Errorf("Mode = %q", s.Mode)
	}
	if len(s.Roles) != 1 || s.Roles[0] != "backend" {
		t.Errorf("Roles = %v", s.Roles)
	}
	// A multi-word alias stays one item: it is a phrase, not two words.
	if len(s.Aliases) != 2 || s.Aliases[1] != "gate tier" {
		t.Errorf("Aliases = %v", s.Aliases)
	}
	if len(s.Groups) != 2 {
		t.Fatalf("Groups = %d, want 2", len(s.Groups))
	}

	// A fenced example must not become a real entry: this repository's own
	// documents quote sample sets.
	if got := len(s.Entries()); got != 3 {
		t.Fatalf("entries = %d, want 3 (fenced sample must not count)", got)
	}

	e := s.Groups[0].Entries[0]
	// The node ID is the link target resolved against the set's directory,
	// which is what makes a set point at a paragraph rather than a file.
	if e.NodeID != "docs/plugin-distribution.md#133-the-rule" {
		t.Errorf("NodeID = %q", e.NodeID)
	}
	// A summary on the following line is the common shape in real sets;
	// treating continuations as noise would drop most summaries.
	if e.Summary != "In 0.x, minor means the user has to fix something." {
		t.Errorf("Summary = %q", e.Summary)
	}

	// Group IDs come from the parser so they are the same strings the index
	// assigns, letting an agent read one group instead of the whole set.
	if s.Groups[0].NodeID != "docs/wiki/sets/release.md#picking-the-version" {
		t.Errorf("group NodeID = %q", s.Groups[0].NodeID)
	}
	if s.Intro != "Intro prose that is not an entry." {
		t.Errorf("Intro = %q", s.Intro)
	}
	// Line is the file line, header included: six header lines then the
	// body, so the first entry sits on line 14 of the file.
	if e.Line != 14 {
		t.Errorf("Line = %d, want 14", e.Line)
	}
}

func TestParseSetDefaultsToLive(t *testing.T) {
	s, _ := ParseSet("docs/wiki/sets/x.md", []byte("# X\n"))
	if s.Mode != ModeLive {
		t.Errorf("Mode = %q, want live", s.Mode)
	}
}

func TestSetNodeIDsDeduplicates(t *testing.T) {
	src := "# X\n\n## G\n\n" +
		"- [a](../../d.md#s) — one\n" +
		"- [b](../../d.md#s) — same node, different framing\n"
	s, _ := ParseSet("docs/wiki/sets/x.md", []byte(src))
	if got := len(s.Entries()); got != 2 {
		t.Fatalf("entries = %d, want 2", got)
	}
	// Two entries may legitimately cite one section; its pin is still one pin.
	if got := s.NodeIDs(); len(got) != 1 || got[0] != "docs/d.md#s" {
		t.Fatalf("NodeIDs = %v, want one id", got)
	}
}

func TestParseSetKeepsEntriesBeforeAnyGroup(t *testing.T) {
	// Losing an entry silently is the one outcome this parser cannot have.
	src := "# X\n\n- [a](../../d.md#s) — before any heading\n"
	s, _ := ParseSet("docs/wiki/sets/x.md", []byte(src))
	if got := len(s.Entries()); got != 1 {
		t.Fatalf("entries = %d, want 1", got)
	}
}
