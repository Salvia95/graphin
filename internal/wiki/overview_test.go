package wiki

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// overviewRoot builds a workspace with one document holding three sections and
// one set citing all three plus a heading that does not exist.
func overviewRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "docs", "target.md"),
		"# Target\n\n## Section one\n\nFirst body.\n\n## Section two\n\nSecond body.\n\n## Section three\n\nThird body.\n")
	mustWrite(t, filepath.Join(root, DirName, setsSubdir, "s.md"),
		"---\ntype: knowledge_set\ndescription: three sections\nroles: []\nmode: live\n---\n\n"+
			"# S\n\n## G\n\n"+
			"- [one](../../target.md#section-one) — first summary\n"+
			"- [two](../../target.md#section-two) — second summary\n"+
			"- [three](../../target.md#section-three) — third summary\n"+
			"- [gone](../../target.md#section-four) — a summary for nothing\n")
	return root
}

func overviewOf(t *testing.T, root, skillDir string) Overview {
	t.Helper()
	o, err := BuildOverview(root, skillDir)
	if err != nil {
		t.Fatal(err)
	}
	return o
}

// A set-level count says three entries are wrong; it cannot say which three,
// and the fix is always a specific line of a specific file.
func TestSetViewCarriesEachEntrysOwnState(t *testing.T) {
	root := overviewRoot(t)
	store, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	pins, _ := Repin(root, store.SetList())
	if err := pins.Save(store.PinsPath()); err != nil {
		t.Fatal(err)
	}
	// Move one section's content out from under its pin.
	mustWrite(t, filepath.Join(root, "docs", "target.md"),
		"# Target\n\n## Section one\n\nFirst body.\n\n## Section two\n\nRewritten body.\n\n## Section three\n\nThird body.\n")

	o := overviewOf(t, root, "")
	if len(o.Sets) != 1 {
		t.Fatalf("sets = %d, want 1", len(o.Sets))
	}
	got := map[string]EntryStatus{}
	for _, it := range o.Sets[0].Items {
		got[it.NodeID] = it.Status
	}
	want := map[string]EntryStatus{
		"docs/target.md#section-one":   EntryOK,
		"docs/target.md#section-two":   EntryDrift,
		"docs/target.md#section-three": EntryOK,
		"docs/target.md#section-four":  EntryDangling,
	}
	for id, w := range want {
		if got[id] != w {
			t.Errorf("%s = %q, want %q", id, got[id], w)
		}
	}
	// Entries is the length of the list, not a second count that could disagree.
	if o.Sets[0].Entries != len(o.Sets[0].Items) {
		t.Errorf("Entries = %d, Items = %d", o.Sets[0].Entries, len(o.Sets[0].Items))
	}
	// The line number travels because fixing a dangling entry means editing
	// that line of the set file.
	for _, it := range o.Sets[0].Items {
		if it.Line == 0 {
			t.Errorf("entry %s has no line", it.NodeID)
		}
	}
}

// Every other decision kind ends when the state it describes ends. This one was
// derived from an append-only log, so writing the answer used to leave the card
// in place — the one kind a person could not finish.
func TestUncoveredRetiresOnceSomethingAnswersIt(t *testing.T) {
	root := overviewRoot(t)
	// The first task now matches the set's own labels ("three sections"), which
	// is the same predicate the miss was recorded from; the second matches
	// nothing in the wiki.
	mustWrite(t, FrictionPath(root),
		`{"v":1,"ts":"2026-08-01T00:00:00Z","kind":"coverage_miss","task":"walk the three sections"}`+"\n"+
			`{"v":1,"ts":"2026-08-02T00:00:00Z","kind":"coverage_miss","task":"walk the three sections"}`+"\n"+
			`{"v":1,"ts":"2026-08-03T00:00:00Z","kind":"coverage_miss","task":"완전히 다른 주제 폐기물 처리"}`+"\n")

	o := overviewOf(t, root, "")
	var uncovered []Decision
	for _, d := range o.Decisions {
		if d.Kind == DecisionUncovered {
			uncovered = append(uncovered, d)
		}
	}
	if len(uncovered) != 1 {
		t.Fatalf("uncovered = %d, want 1: %+v", len(uncovered), uncovered)
	}
	if uncovered[0].Title != "완전히 다른 주제 폐기물 처리" {
		t.Errorf("survivor = %q, want the one nothing matches", uncovered[0].Title)
	}
	if o.Health.Answered != 1 {
		t.Errorf("Answered = %d, want 1 — a backlog that shrinks with no trace is not believable",
			o.Health.Answered)
	}
}

// A count on its own cannot tell three misses last week from three misses in
// March, and those are not the same decision.
func TestUncoveredCarriesItsDatesAndSaysHowOften(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, DirName, setsSubdir, "s.md"), "# S\n")
	mustWrite(t, FrictionPath(root),
		`{"v":1,"ts":"2026-03-01T00:00:00Z","kind":"coverage_miss","task":"zzz qqq"}`+"\n"+
			`{"v":1,"ts":"2026-08-04T00:00:00Z","kind":"coverage_miss","task":"zzz qqq"}`+"\n")

	o := overviewOf(t, root, "")
	if len(o.Decisions) != 1 {
		t.Fatalf("decisions = %+v", o.Decisions)
	}
	d := o.Decisions[0]
	if d.Count != 2 || d.FirstSeen != "2026-03-01" || d.LastSeen != "2026-08-04" {
		t.Errorf("count/first/last = %d/%q/%q, want 2/2026-03-01/2026-08-04",
			d.Count, d.FirstSeen, d.LastSeen)
	}
	if !strings.Contains(d.Detail, "twice") || !strings.Contains(d.Detail, "2026-08-04") {
		t.Errorf("detail = %q, want the recurrence and the date in it", d.Detail)
	}
}

// Telling someone a skill drifted when they have simply never run the generator
// sends them looking for a change nobody made.
func TestMissingSkillIsNotReportedAsDrift(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, DirName, setsSubdir, "s.md"),
		"---\ntype: knowledge_set\ndescription: backend things\nroles: [backend]\nmode: live\n---\n\n# S\n")

	o := overviewOf(t, root, filepath.Join(root, ".claude", "skills"))
	var d *Decision
	for i := range o.Decisions {
		if o.Decisions[i].Kind == DecisionStaleSkill {
			d = &o.Decisions[i]
		}
	}
	if d == nil {
		t.Fatal("no stale_skill decision for a role with no generated skill")
	}
	if !strings.Contains(d.Detail, "never been generated") {
		t.Errorf("detail = %q, want it to say the skill was never generated", d.Detail)
	}
	if d.Role != "backend" {
		t.Errorf("role = %q, want backend", d.Role)
	}
}

// Repinning everything after verifying one entry vouches for the entries nobody
// opened, and does it by making their warnings disappear.
func TestRepinEntryLeavesTheOtherDriftAlone(t *testing.T) {
	root := overviewRoot(t)
	store, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	pins, _ := Repin(root, store.SetList())
	if err := pins.Save(store.PinsPath()); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(root, "docs", "target.md"),
		"# Target\n\n## Section one\n\nRewritten one.\n\n## Section two\n\nRewritten two.\n\n## Section three\n\nThird body.\n")

	before := overviewOf(t, root, "")
	if got := before.Health.Drifted; got != 2 {
		t.Fatalf("drifted before = %d, want 2", got)
	}

	res, err := RepinEntry(root, "s", "docs/target.md#section-one")
	if err != nil {
		t.Fatal(err)
	}
	if res.Updated != 1 || !res.Wrote {
		t.Fatalf("result = %+v, want one update written", res)
	}

	after := overviewOf(t, root, "")
	if got := after.Health.Drifted; got != 1 {
		t.Fatalf("drifted after = %d, want 1 — only the verified entry should clear", got)
	}
	for _, d := range after.Decisions {
		if d.Kind == DecisionDrift && d.NodeID != "docs/target.md#section-two" {
			t.Errorf("wrong survivor: %q", d.NodeID)
		}
	}
}

func TestRepinEntryRefusesAPairNoSetLists(t *testing.T) {
	root := overviewRoot(t)
	if _, err := RepinEntry(root, "s", "docs/target.md#not-listed"); err == nil {
		t.Fatal("want an error for an entry the set does not list")
	}
	if _, err := RepinEntry(root, "nope", "docs/target.md#section-one"); err == nil {
		t.Fatal("want an error for a set that does not exist")
	}
}

// The whole surface is a JSON contract, and empty is the state the console sees
// most often. `null.map()` is exactly how a healthy wiki breaks its own screen.
func TestOverviewMarshalsEmptyListsAsArrays(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, DirName, setsSubdir, ".keep"), "")

	b, err := json.Marshal(overviewOf(t, root, ""))
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{`"decisions":[]`, `"sets":[]`, `"terms":[]`} {
		if !strings.Contains(string(b), key) {
			t.Errorf("missing %s in %s", key, b)
		}
	}
}
