package wiki

import (
	"path/filepath"
	"strings"
	"testing"
)

// maintainDoc has two sections so a dangling entry has somewhere right to go.
const maintainDoc = "# Target\n\n## Section one\n\nOriginal body.\n\n## Section two\n\nAnother body.\n\n## Section three\n\nWhere the lost entry belongs.\n"

// maintainSet is authored the way this repository's sets are: frontmatter, a
// title, an intro, and entries whose summary sits on a continuation line.
const maintainSet = "---\n" +
	"type: knowledge_set\n" +
	"description: 원래 카탈로그 줄\n" +
	"tags: [ops]\n" +
	"roles: []\n" +
	"mode: live\n" +
	"---\n" +
	"\n" +
	"# 운영\n" +
	"\n" +
	"본문은 손대지 않는다.\n" +
	"\n" +
	"## 규칙\n" +
	"\n" +
	"- [one](../../target.md#section-one) —\n" +
	"  첫째 절의 요약이다.\n" +
	"- [gone](../../target.md#section-nope) —\n" +
	"  사라진 절을 가리킨다.\n" +
	"- [inline](../../target.md#section-two) — 한 줄에 쓴 요약.\n"

func maintainWiki(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "docs", "target.md"), maintainDoc)
	mustWrite(t, filepath.Join(root, DirName, setsSubdir, "ops.md"), maintainSet)
	return root
}

func TestEntryLinesAreFileLines(t *testing.T) {
	s, err := ParseSet("docs/wiki/sets/ops.md", []byte(maintainSet))
	if err != nil {
		t.Fatal(err)
	}
	// Seven header lines, then the body. A diagnostic that reports the body
	// line sends a reader to the wrong place, and the editor cuts there.
	if got := s.Entries()[0].Line; got != 15 {
		t.Fatalf("first entry Line = %d, want 15 (the file line)", got)
	}
}

func TestRepointFixesADanglingEntry(t *testing.T) {
	root := maintainWiki(t)
	res, v, err := EditSetEntry(root, "ops", "docs/target.md#section-nope",
		EntryEdit{NodeID: "docs/target.md#section-three"})
	if err != nil {
		t.Fatal(err)
	}
	if v.Blocked() {
		t.Fatalf("blocked: %v", v.Findings)
	}
	got := readFile(t, filepath.Join(root, DirName, setsSubdir, "ops.md"))
	// The link moved and nothing else on the line did; the summary the
	// author wrote is still there, still on its own line.
	if !strings.Contains(got, "- [gone](../../target.md#section-three) —\n  사라진 절을 가리킨다.\n") {
		t.Errorf("entry not repointed surgically:\n%s", got)
	}
	// The write is marked for a person, and the mark is the only header change.
	if !strings.Contains(got, "reviewed: false") || !strings.Contains(got, "description: 원래 카탈로그 줄") {
		t.Errorf("mark missing or header rewritten:\n%s", got)
	}
	// Pins follow: the new node is pinned, the old one is forgotten.
	store, _ := Load(root)
	if _, ok := store.Pins.Get("ops", "docs/target.md#section-three"); !ok || !res.Repinned {
		t.Error("repointed entry was not pinned")
	}
	if _, ok := store.Pins.Get("ops", "docs/target.md#section-nope"); ok {
		t.Error("pin for the abandoned node survived")
	}
	if !store.Sets["ops"].Unreviewed {
		t.Error("set does not read as unreviewed after an agent edit")
	}
	if probs := Check(root, store.SetList(), store.Pins); len(probs) != 2 {
		// The two untouched entries are unpinned; nothing dangles.
		for _, p := range probs {
			if p.Kind == ProblemDangling {
				t.Errorf("still dangling: %s", p)
			}
		}
	}
}

func TestJudgeEntryBlocksWhatFilesCanDecide(t *testing.T) {
	root := maintainWiki(t)
	store, _ := Load(root)
	s := store.Sets["ops"]
	cases := []struct {
		name string
		to   string
		want RuleID
	}{
		{"missing heading", "docs/target.md#section-four", RuleAnchor},
		{"code node", "internal/x.go#Foo", RuleStructure},
		{"already listed", "docs/target.md#section-one", RuleDuplicate},
	}
	for _, tc := range cases {
		v := JudgeEntry(root, s, "docs/target.md#section-nope", tc.to, "제목", "요약")
		found := false
		for _, f := range v.Findings {
			if f.Rule == tc.want {
				found = true
			}
		}
		if !found {
			t.Errorf("%s: want %s, got %v", tc.name, tc.want, v.Findings)
		}
	}
	// A repoint onto the node the entry already has is not a duplicate of
	// itself, and a blank summary is its own rule.
	if v := JudgeEntry(root, s, "docs/target.md#section-one", "docs/target.md#section-one", "제목", ""); len(v.Findings) != 1 || v.Findings[0].Rule != RuleSummary {
		t.Errorf("self-repoint with empty summary: %v", v.Findings)
	}
	// Text the file format cannot carry is refused before it is written: a
	// title with "]" or a node id with a space would render an entry line the
	// parser no longer reads, and the entry would vanish with its pin in place.
	if v := JudgeEntry(root, s, "docs/target.md#section-one", "docs/target.md#section-one", "제목]", "요약"); !hasRule(v, RuleFormat) {
		t.Errorf("']' in a title passed: %v", v.Findings)
	}
	if v := JudgeEntry(root, s, "docs/target.md#section-nope", "docs/target md#section-two", "제목", "요약"); !hasRule(v, RuleFormat) {
		t.Errorf("a space in a node id passed: %v", v.Findings)
	}
	// A blocked edit writes nothing.
	before := readFile(t, filepath.Join(root, DirName, setsSubdir, "ops.md"))
	if _, v, err := EditSetEntry(root, "ops", "docs/target.md#section-nope",
		EntryEdit{NodeID: "docs/target.md#section-four"}); err != nil || !v.Blocked() {
		t.Fatalf("expected a blocked verdict, got err=%v v=%v", err, v)
	}
	if after := readFile(t, filepath.Join(root, DirName, setsSubdir, "ops.md")); after != before {
		t.Error("a blocked edit touched the file")
	}
}

func TestSummarizeKeepsTheAuthorsShape(t *testing.T) {
	root := maintainWiki(t)
	// Continuation-line entry stays continuation-line; inline stays inline.
	if _, v, err := EditSetEntry(root, "ops", "docs/target.md#section-one",
		EntryEdit{Summary: "다시 읽고 새로 쓴 요약이다."}); err != nil || v.Blocked() {
		t.Fatalf("err=%v v=%v", err, v)
	}
	if _, v, err := EditSetEntry(root, "ops", "docs/target.md#section-two",
		EntryEdit{Summary: "한 줄로 다시 쓴 요약."}); err != nil || v.Blocked() {
		t.Fatalf("err=%v v=%v", err, v)
	}
	got := readFile(t, filepath.Join(root, DirName, setsSubdir, "ops.md"))
	for _, want := range []string{
		"- [one](../../target.md#section-one) —\n  다시 읽고 새로 쓴 요약이다.\n",
		"- [inline](../../target.md#section-two) — 한 줄로 다시 쓴 요약.\n",
		"- [gone](../../target.md#section-nope) —\n  사라진 절을 가리킨다.\n", // untouched
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "첫째 절의 요약이다") {
		t.Error("old summary survived")
	}
	store, _ := Load(root)
	if e := store.Sets["ops"].Entries()[0]; e.Summary != "다시 읽고 새로 쓴 요약이다." {
		t.Errorf("reparsed summary = %q", e.Summary)
	}
}

func TestWrapIndentedCountsEastAsianWidth(t *testing.T) {
	// Twelve Hangul words of four syllables are 8 columns each; at width 40
	// with a 2-column indent, four fit per line.
	text := strings.Repeat("가나다라 ", 12)
	lines := wrapIndented(text, "  ", 40)
	if len(lines) != 3 {
		t.Fatalf("lines = %d, want 3: %q", len(lines), lines)
	}
	for _, l := range lines {
		if !strings.HasPrefix(l, "  ") || displayWidth(strings.TrimSpace(l)) > 38 {
			t.Errorf("bad line %q", l)
		}
	}
}

func TestConfirmRepinsAndMarks(t *testing.T) {
	root := maintainWiki(t)
	res, v, err := ConfirmEntry(root, "ops", "docs/target.md#section-one")
	if err != nil || v.Blocked() {
		t.Fatalf("err=%v v=%v", err, v)
	}
	if !res.Repinned {
		t.Error("confirm did not pin")
	}
	store, _ := Load(root)
	if !store.Sets["ops"].Unreviewed {
		t.Error("confirm did not mark the set")
	}
	if _, _, err := ConfirmEntry(root, "ops", "docs/target.md#nowhere"); err == nil {
		t.Error("confirming an entry the set does not list must fail")
	}
}

// A dangling entry has no text to have re-read. Confirming it is a verdict,
// not a quiet no-op that tells the agent "applied".
func TestConfirmRefusesADanglingEntry(t *testing.T) {
	root := maintainWiki(t)
	res, v, err := ConfirmEntry(root, "ops", "docs/target.md#section-nope")
	if err != nil {
		t.Fatal(err)
	}
	if !hasRule(v, RuleAnchor) || res.Repinned {
		t.Fatalf("dangling confirm: v=%v res=%+v", v, res)
	}
	store, _ := Load(root)
	if store.Sets["ops"].Unreviewed {
		t.Error("a refused confirm must not mark the set")
	}
}

// A set with no frontmatter still gets its mark: an agent write may not land
// unmarked, so the header is added rather than the write refused halfway.
func TestConfirmAndDescribeOnAHeaderlessSet(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "docs", "target.md"), maintainDoc)
	mustWrite(t, filepath.Join(root, DirName, setsSubdir, "bare.md"),
		"# 맨몸\n\n- [one](../../target.md#section-one) — 요약\n")
	if _, v, err := ConfirmEntry(root, "bare", "docs/target.md#section-one"); err != nil || v.Blocked() {
		t.Fatalf("confirm: err=%v v=%v", err, v)
	}
	if _, v, err := DescribeSet(root, "bare", "맨몸 세트를 읽는 이유."); err != nil || v.Blocked() {
		t.Fatalf("describe: err=%v v=%v", err, v)
	}
	store, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	s := store.Sets["bare"]
	if !s.Unreviewed || s.Description != "맨몸 세트를 읽는 이유." || len(s.Entries()) != 1 {
		t.Errorf("headerless set after agent writes: %+v", s)
	}
	if _, ok := store.Pins.Get("bare", "docs/target.md#section-one"); !ok {
		t.Error("confirm did not pin")
	}
}

func TestDescribeSetRewritesTheCatalogueLine(t *testing.T) {
	root := maintainWiki(t)
	if _, v, _ := DescribeSet(root, "ops", "두 줄\n짜리"); !v.Blocked() {
		t.Error("a two-line description must be blocked")
	}
	res, v, err := DescribeSet(root, "ops", "운영 규칙을 고칠 때 읽는다.")
	if err != nil || v.Blocked() {
		t.Fatalf("err=%v v=%v", err, v)
	}
	got := readFile(t, filepath.Join(root, res.File))
	if !strings.Contains(got, "description: 운영 규칙을 고칠 때 읽는다.") || !strings.Contains(got, "reviewed: false") {
		t.Errorf("description or mark missing:\n%s", got)
	}
}

func TestMarkAddsAHeaderWhereThereIsNone(t *testing.T) {
	out, err := markUnreviewed("# S\n\n- [one](../../target.md#section-one) — 요약\n")
	if err != nil || !strings.HasPrefix(out, "---\nreviewed: false\n---\n# S\n") {
		t.Fatalf("out=%q err=%v", out, err)
	}
	// A person's reviewed: true is flipped, not duplicated.
	out, _ = markUnreviewed("---\nreviewed: true\nmode: live\n---\n# S\n")
	if strings.Count(out, "reviewed:") != 1 || !strings.Contains(out, "reviewed: false") {
		t.Fatalf("flip went wrong: %q", out)
	}
}

func TestUnreviewedReachesEveryReader(t *testing.T) {
	root := maintainWiki(t)
	if _, _, err := EditSetEntry(root, "ops", "docs/target.md#section-nope",
		EntryEdit{NodeID: "docs/target.md#section-three"}); err != nil {
		t.Fatal(err)
	}
	store, _ := Load(root)

	// The catalogue says so before the set is opened.
	man := store.Manifest(store.Select("", "운영 규칙"), []byte("k"))
	if len(man.Sets) != 1 || !man.Sets[0].Unreviewed {
		t.Errorf("manifest does not carry the flag: %+v", man.Sets)
	}
	// Every served section says so.
	for _, e := range store.Resolve(fileReader{}, []string{"ops"}).Sets[0].Entries {
		if !e.Unreviewed {
			t.Errorf("served section %s lacks the flag", e.NodeID)
		}
	}
	if got := store.ResolveNodes(fileReader{}, []string{"docs/target.md#section-three"}); !got[0].Unreviewed {
		t.Error("a section fetched by id lacks the flag")
	}
	// The queue lists it as a decision, and the overview ranks it.
	q, err := BuildQueueReport(root)
	if err != nil || len(q.Unreviewed) != 1 || q.Unreviewed[0].Set != "ops" {
		t.Errorf("queue: err=%v unreviewed=%v", err, q.Unreviewed)
	}
	o, err := BuildOverview(root, "")
	if err != nil {
		t.Fatal(err)
	}
	if countKind(o.Decisions, DecisionUnreviewed) != 1 || o.Health.Unreviewed != 1 {
		t.Errorf("overview: decisions=%v health=%+v", o.Decisions, o.Health)
	}
	// A person ends it, and the flag leaves every surface at once.
	yes := true
	if _, err := EditSetFront(root, "ops", SetFrontEdits{Reviewed: &yes}); err != nil {
		t.Fatal(err)
	}
	store, _ = Load(root)
	if store.Sets["ops"].Unreviewed {
		t.Error("reviewed: true did not clear the flag")
	}
}
