package wiki

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeDefiner stands in for the code index.
type fakeDefiner map[string]string // word → kind

func (d fakeDefiner) Defines(word string) (string, string, bool) {
	kind, ok := d[word]
	return "pkg." + word, kind, ok
}

func proposeStore(t *testing.T) *Store {
	t.Helper()
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "docs", "target.md"), targetDoc)
	s, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	s.Root = root
	return s
}

func candidate(evidence ...string) *Term {
	return &Term{Canonical: "posting", Body: "A unit of published writing.", Evidence: evidence}
}

func TestJudgeRejectsAnIdentifier(t *testing.T) {
	st := proposeStore(t)
	// The whole boundary, as a function: if the index resolves it, search
	// answers it and a second definition will drift from the first.
	v := st.Judge(&Term{Canonical: "OrderService", Evidence: []string{"a.md#x", "b.md#y"}},
		fakeDefiner{"OrderService": "class"})
	if !v.Blocked() {
		t.Fatal("an indexed identifier must not reach the queue")
	}
	if v.Findings[0].Rule != RuleIdentifier {
		t.Fatalf("rule = %q", v.Findings[0].Rule)
	}
}

func TestJudgeRejectsAnIdentifierMatchedByAlias(t *testing.T) {
	st := proposeStore(t)
	v := st.Judge(&Term{
		Canonical: "주문서비스", Aliases: []string{"OrderService"},
		Evidence: []string{"a.md#x", "b.md#y"},
	}, fakeDefiner{"OrderService": "class"})
	if !v.Blocked() {
		t.Fatal("an alias that is an identifier must be caught too")
	}
}

func TestJudgeRequiresCrossContextEvidence(t *testing.T) {
	st := proposeStore(t)
	// Two citations in one file are one context: that is an author's usage,
	// not a vocabulary the project speaks.
	v := st.Judge(candidate("docs/a.md#one", "docs/a.md#two"), nil)
	if !v.Blocked() || v.Findings[0].Rule != RuleEvidence {
		t.Fatalf("verdict = %v", v)
	}
}

func TestJudgeGradesStatusByCorroboration(t *testing.T) {
	st := proposeStore(t)

	v := st.Judge(candidate("docs/a.md#x", "docs/b.md#y"), nil)
	if v.Blocked() {
		t.Fatalf("two contexts should pass: %v", v)
	}
	if v.Status != StatusUnverified {
		t.Errorf("status = %q, want unverified at the floor", v.Status)
	}

	v = st.Judge(candidate("docs/a.md#x", "docs/b.md#y", "docs/c.md#z"), nil)
	if v.Status != StatusActive {
		t.Errorf("status = %q, want active when corroborated past the floor", v.Status)
	}
}

func TestJudgeReportsTheCapRatherThanEvicting(t *testing.T) {
	st := proposeStore(t)
	for i := 0; i < GlossaryCap; i++ {
		name := "term" + string(rune('a'+i%26)) + string(rune('a'+i/26))
		st.Terms[name] = &Term{Canonical: name}
	}
	v := st.Judge(candidate("docs/a.md#x", "docs/b.md#y"), nil)
	if !v.Blocked() || v.Findings[0].Rule != RuleCap {
		t.Fatalf("verdict = %v", v)
	}
	// Displacing an entry is a judgement about which knowledge matters more,
	// and that is exactly what the write separation reserves for a person.
	if !strings.Contains(v.Findings[0].Detail, "person") {
		t.Errorf("cap finding should say who decides: %q", v.Findings[0].Detail)
	}
}

func TestJudgeSkipsIdentifierRuleWithoutAnIndex(t *testing.T) {
	st := proposeStore(t)
	// "No index available" and "not an identifier" are different states to
	// approve on, so the rule is skipped, never silently passed.
	if v := st.Judge(candidate("docs/a.md#x", "docs/b.md#y"), nil); v.Blocked() {
		t.Fatalf("verdict = %v", v)
	}
}

func TestProposeWritesSomethingThatParsesBack(t *testing.T) {
	st := proposeStore(t)
	t0 := candidate("docs/a.md#x", "docs/b.md#y")
	t0.Aliases = []string{"post"}
	t0.Confusions = []Confusion{{Term: "blog", Why: "a posting is a unit, a blog is a medium"}}

	p, err := st.Propose(t0)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(p.File)
	if err != nil {
		t.Fatal(err)
	}
	back, err := ParseTerm(p.File, raw)
	if err != nil {
		t.Fatal(err)
	}
	// Approving is a file move, so what is written has to be the authored
	// format — not a serialization that a human then has to translate.
	if back.Canonical != "posting" || back.Status != StatusProposed {
		t.Fatalf("round trip = %+v", back)
	}
	if len(back.Confusions) != 1 || back.Confusions[0].Why == "" {
		t.Fatalf("confusions lost: %+v", back.Confusions)
	}
	if len(back.Evidence) != 2 {
		t.Fatalf("evidence = %v", back.Evidence)
	}
}

func TestProposeMergesOnResubmission(t *testing.T) {
	st := proposeStore(t)
	if _, err := st.Propose(candidate("docs/a.md#x", "docs/b.md#y")); err != nil {
		t.Fatal(err)
	}
	p, err := st.Propose(candidate("docs/c.md#z"))
	if err != nil {
		t.Fatal(err)
	}
	// A term that keeps coming back is evidence about the term, not a repeat
	// of it — which is why resubmission merges instead of overwriting.
	if p.Seen != 2 {
		t.Errorf("Seen = %d, want 2", p.Seen)
	}
	if len(p.Evidence) != 3 {
		t.Errorf("evidence = %v, want all three merged", p.Evidence)
	}
}

func TestQueueListsProposals(t *testing.T) {
	st := proposeStore(t)
	if _, err := st.Propose(candidate("docs/a.md#x", "docs/b.md#y")); err != nil {
		t.Fatal(err)
	}
	q, err := st.Queue()
	if err != nil {
		t.Fatal(err)
	}
	if len(q) != 1 || q[0].Canonical != "posting" {
		t.Fatalf("queue = %+v", q)
	}
}

func TestQueueIsEmptyWithoutADirectory(t *testing.T) {
	st := proposeStore(t)
	q, err := st.Queue()
	if err != nil || len(q) != 0 {
		t.Fatalf("queue = %v err = %v", q, err)
	}
}
