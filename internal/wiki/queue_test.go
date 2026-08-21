package wiki

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// TestQueueReportMarshalsEmptyListsNotNull guards the wire shape, not the Go
// shape.
//
// An empty queue is the normal state — most days there is nothing to approve —
// so this is the payload the console receives most often. A nil Go slice
// marshals to `null`, and `null.map()` is a crash in the one view that has to
// keep working when there is nothing to show.
func TestQueueReportMarshalsEmptyListsNotNull(t *testing.T) {
	q, err := BuildQueueReport(t.TempDir())
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	raw, err := json.Marshal(q)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, key := range []string{"awaiting_review", "misses", "unread_sets", "drifted"} {
		if strings.Contains(string(raw), `"`+key+`":null`) {
			t.Errorf("%s marshalled as null:\n%s", key, raw)
		}
	}
}

// TestQueueReportCarriesTheProposalFile pins the field a consumer needs to act
// rather than only to display. Approving a candidate is moving this file into
// the glossary; a queue that names candidates without locating them can be
// read and not used.
func TestQueueReportCarriesTheProposalFile(t *testing.T) {
	root := t.TempDir()
	st, err := Load(root)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	st.Root = root
	if _, err := st.Propose(&Term{
		Canonical: "posting",
		Body:      "A unit of published writing.",
		Evidence:  []string{"pkg.a", "pkg.b"},
	}); err != nil {
		t.Fatalf("propose: %v", err)
	}

	q, err := BuildQueueReport(root)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(q.Awaiting) != 1 {
		t.Fatalf("awaiting = %d, want 1", len(q.Awaiting))
	}
	got := q.Awaiting[0]
	if want := ProposalPath(root, "posting"); got.File != want {
		t.Errorf("File = %q, want %q", got.File, want)
	}
	if got.Canonical != "posting" {
		t.Errorf("Canonical = %q, want %q", got.Canonical, "posting")
	}
	if len(got.Evidence) != 2 {
		t.Errorf("Evidence = %v, want 2 entries", got.Evidence)
	}
}

// TestRenderQueueTruncatesOnlyTheMisses fixes where the limit applies. Misses
// grow without bound because every unanswered task appends one; the other
// sections are bounded by the wiki's own size, and silently cutting them would
// hide work rather than shorten a list.
func TestRenderQueueTruncatesOnlyTheMisses(t *testing.T) {
	q := QueueReport{
		Glossary: GlossaryUsage{Count: 2, Cap: GlossaryCap},
		Awaiting: []QueuedProposal{
			{Canonical: "posting", Seen: 2, Evidence: []string{"a", "b"}},
			{Canonical: "gate", Seen: 1, Evidence: []string{"c"}},
		},
		Misses: []FrictionEvent{
			{Task: "one", Role: "backend"},
			{Task: "two"},
			{Task: "three"},
		},
		Unread:  []UnreadSet{{Set: "release", Offered: 4}},
		Drifted: []DriftedNode{{Node: "docs/a.md#x", Served: 3}},
	}

	var buf bytes.Buffer
	RenderQueue(&buf, q, 2)
	out := buf.String()

	for _, want := range []string{
		"glossary: 2 of 30",
		"awaiting review (2)",
		"posting", "gate",
		"work the wiki had no answer for (3, newest first)",
		"[backend] one",
		"[-] two",
		"… 1 more",
		"offered but never opened (1)",
		"release", "offered 4 times",
		"served with a stale pin (1)",
		"docs/a.md#x (3)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	// The third miss is past the limit; both proposals are not.
	if strings.Contains(out, "three") {
		t.Errorf("miss past the limit was printed:\n%s", out)
	}
}

// TestRenderQueueOmitsSectionsWithNothingInThem keeps the empty queue short.
// The two standing sections say "(nothing)" because their absence would read
// as a broken command; the optional two say nothing at all.
func TestRenderQueueOmitsSectionsWithNothingInThem(t *testing.T) {
	var buf bytes.Buffer
	RenderQueue(&buf, QueueReport{Glossary: GlossaryUsage{Cap: GlossaryCap}}, 10)
	out := buf.String()

	if n := strings.Count(out, "(nothing)"); n != 2 {
		t.Errorf("(nothing) appeared %d times, want 2:\n%s", n, out)
	}
	for _, absent := range []string{"offered but never opened", "served with a stale pin"} {
		if strings.Contains(out, absent) {
			t.Errorf("empty section %q was printed:\n%s", absent, out)
		}
	}
}
