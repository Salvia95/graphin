package wiki

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// authorWiki is a workspace with one human set and a document with a section
// that set does not cover, plus a recorded miss for a task about it.
func authorWiki(t *testing.T) string {
	t.Helper()
	root := maintainWiki(t)
	mustWrite(t, filepath.Join(root, "docs", "release.md"),
		"# 릴리스\n\n## 버전 자리\n\n0.x에서 마이너는 사용자가 고칠 일이 있을 때다.\n\n## 노트\n\n프렐류드는 마이너 이상에만 쓴다.\n")
	AppendFriction(root, FrictionEvent{Kind: FrictionMiss, Task: "릴리스 버전 자리를 고른다"})
	return root
}

func goodDraft() *SetDraft {
	return &SetDraft{
		Name: "release", Title: "릴리스", Description: "릴리스를 낼 때 읽는다.",
		Aliases: []string{"versioning"}, Tags: []string{"release"},
		Intro: "버전 자리와 노트 규칙.",
		Groups: []DraftGroup{{Title: "먼저", Entries: []DraftEntry{
			{Title: "버전 자리", NodeID: "docs/release.md#버전-자리", Summary: "마이너는 사용자가 고칠 일이 있을 때다."},
			{Title: "노트", NodeID: "docs/release.md#노트", Summary: "프렐류드는 마이너 이상에만 쓴다."},
		}}},
		Task: "릴리스 버전 자리를 고른다",
	}
}

func TestCreateSetWritesAReachableMarkedSet(t *testing.T) {
	root := authorWiki(t)
	res, v, err := CreateSet(root, goodDraft())
	if err != nil {
		t.Fatal(err)
	}
	if v.Blocked() {
		t.Fatalf("blocked: %v", v.Findings)
	}
	got := readFile(t, filepath.Join(root, res.File))
	for _, want := range []string{
		"origin: agent\n", "reviewed: false\n", "aliases: [versioning]\n", "roles: []\n",
		"# 릴리스\n", "## 먼저\n", "- [버전 자리](../../release.md#버전-자리) —\n  마이너는 사용자가 고칠 일이 있을 때다.\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	store, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	s := store.Sets["release"]
	if s == nil || s.Origin != OriginAgent || !s.Unreviewed || len(s.Entries()) != 2 {
		t.Fatalf("reparsed set = %+v", s)
	}
	// Pinned on the way in, so the checker is green and drift can be seen.
	for _, p := range Check(root, []*Set{s}, store.Pins) {
		t.Errorf("fresh set has a problem: %s", p)
	}
	if !res.Repinned {
		t.Error("Repinned false for a fully pinned set")
	}
	// The miss it was written for is closed: the same question now matches.
	if sel := store.Select("", "릴리스 버전 자리를 고른다"); len(sel.Matched) != 1 || sel.Matched[0] != "release" {
		t.Errorf("the task still does not reach the set: %v", sel.Matched)
	}
	o, err := BuildOverview(root, "")
	if err != nil {
		t.Fatal(err)
	}
	if countKind(o.Decisions, DecisionUncovered) != 0 || o.Health.Answered != 1 {
		t.Errorf("miss not retired: decisions=%v answered=%d", o.Decisions, o.Health.Answered)
	}
	if countKind(o.Decisions, DecisionUnreviewed) != 1 {
		t.Error("an agent-written set must wait for a person")
	}
}

func TestJudgeSetRules(t *testing.T) {
	root := authorWiki(t)
	store, _ := Load(root)
	cases := []struct {
		name string
		mod  func(d *SetDraft)
		want RuleID
	}{
		{"bad name", func(d *SetDraft) { d.Name = "릴리스/노트" }, RuleName},
		{"taken name", func(d *SetDraft) { d.Name = "ops" }, RuleName},
		{"no description", func(d *SetDraft) { d.Description = " " }, RuleSummary},
		{"no entries", func(d *SetDraft) { d.Groups = nil }, RuleSummary},
		{"code node", func(d *SetDraft) { d.Groups[0].Entries[0].NodeID = "internal/x.go#Foo" }, RuleStructure},
		{"missing heading", func(d *SetDraft) { d.Groups[0].Entries[0].NodeID = "docs/release.md#없다" }, RuleAnchor},
		{"listed twice", func(d *SetDraft) { d.Groups[0].Entries[1].NodeID = d.Groups[0].Entries[0].NodeID }, RuleDuplicate},
		{"empty summary", func(d *SetDraft) { d.Groups[0].Entries[1].Summary = "" }, RuleSummary},
		{"subset of an existing set", func(d *SetDraft) {
			d.Groups[0].Entries = []DraftEntry{{Title: "one", NodeID: "docs/target.md#section-one", Summary: "s"}}
		}, RuleDuplicate},
		{"never missed", func(d *SetDraft) { d.Task = "아무도 묻지 않은 일" }, RuleUnasked},
		{"alias with a comma", func(d *SetDraft) { d.Aliases = []string{"a, b"} }, RuleFormat},
		{"title with a bracket", func(d *SetDraft) { d.Groups[0].Entries[0].Title = "닫는] 괄호" }, RuleFormat},
		{"unreachable", func(d *SetDraft) {
			d.Name, d.Title, d.Aliases = "notes", "메모", nil
			d.Groups[0].Title = "규칙"
			d.Groups[0].Entries[0].Title, d.Groups[0].Entries[1].Title = "첫째", "둘째"
		}, RuleUnreachable},
	}
	for _, tc := range cases {
		d := goodDraft()
		tc.mod(d)
		v := store.JudgeSet(d)
		if !hasRule(v, tc.want) {
			t.Errorf("%s: want %s, got %v", tc.name, tc.want, v.Findings)
		}
	}
	if v := store.JudgeSet(goodDraft()); v.Blocked() {
		t.Errorf("a good draft was blocked: %v", v.Findings)
	}
	// A draft of code nodes has nothing to compare against existing sets and
	// must fail on structure alone, not read as a duplicate of one.
	d := goodDraft()
	for i := range d.Groups[0].Entries {
		d.Groups[0].Entries[i].NodeID = "internal/x.go#F" + itoa(i)
	}
	if v := store.JudgeSet(d); hasRule(v, RuleDuplicate) || !hasRule(v, RuleStructure) {
		t.Errorf("code-only draft: %v", v.Findings)
	}
	// A blocked draft writes nothing.
	d = goodDraft()
	d.Task = "아무도 묻지 않은 일"
	if _, v, err := CreateSet(root, d); err != nil || !v.Blocked() {
		t.Fatalf("err=%v v=%v", err, v)
	}
	if _, err := Load(root); err != nil {
		t.Fatal(err)
	}
	if store, _ := Load(root); store.Sets["release"] != nil {
		t.Error("a blocked draft reached the disk")
	}
}

// Creating docs/wiki is how a project opts in and what arms the gate, so a
// set is never the thing that creates it.
func TestCreateSetRefusesWithoutAWiki(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "docs", "release.md"), "# 릴리스\n\n## 버전 자리\n\n본문.\n")
	AppendFriction(root, FrictionEvent{Kind: FrictionMiss, Task: "릴리스 버전 자리를 고른다"})
	if _, _, err := CreateSet(root, goodDraft()); err != ErrNoWiki {
		t.Fatalf("err = %v, want ErrNoWiki", err)
	}
	if _, err := os.Stat(filepath.Join(root, DirName)); !os.IsNotExist(err) {
		t.Error("docs/wiki was created as a side effect")
	}
}

func TestJudgeSetHonoursTheCap(t *testing.T) {
	root := authorWiki(t)
	for i := 0; i < SetCap; i++ {
		mustWrite(t, filepath.Join(root, DirName, setsSubdir, "s"+itoa(i)+".md"),
			"# S"+itoa(i)+"\n\n- [one](../../target.md#section-one) — 요약\n")
	}
	store, _ := Load(root)
	if v := store.JudgeSet(goodDraft()); !hasRule(v, RuleCap) {
		t.Errorf("cap not enforced: %v", v.Findings)
	}
}

func TestRenderSetRoundTrips(t *testing.T) {
	d := goodDraft()
	d.Description = "#으로 시작하는 줄은 주석이 아니다"
	s, err := ParseSet("docs/wiki/sets/release.md", []byte(renderSet(d)))
	if err != nil {
		t.Fatal(err)
	}
	if s.Description != d.Description || s.Title != "릴리스" || len(s.Entries()) != 2 ||
		s.Entries()[1].Summary != "프렐류드는 마이너 이상에만 쓴다." || s.Groups[0].Title != "먼저" {
		t.Errorf("round trip lost something: %+v", s)
	}
}
