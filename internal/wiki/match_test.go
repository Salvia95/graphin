package wiki

import (
	"path/filepath"
	"testing"
)

func TestWordKeysBridgeKoreanParticles(t *testing.T) {
	// Nothing separates a Korean word from the particle glued to it, so a
	// whitespace tokenizer files "릴리스를" and "릴리스" as unrelated. That
	// made every Korean task match nothing in a repository whose docs are
	// Korean — the matcher was working and useless at the same time.
	keys := keySet("릴리스 노트를 적는다")
	if n := countMatchingWords("릴리스를 내야 한다", keys); n == 0 {
		t.Fatal("a shared Korean word must match across its particles")
	}
}

func TestCountsWordsNotKeys(t *testing.T) {
	// One CJK word expands to several bigrams. Counting keys would let a
	// single shared word clear a bar meant to require two.
	keys := keySet("릴리스 절차")
	if n := countMatchingWords("릴리스", keys); n != 1 {
		t.Fatalf("countMatchingWords = %d, want 1 for one shared word", n)
	}
}

func TestShortWordsAreIgnored(t *testing.T) {
	if got := wordKeys("a"); got != nil {
		t.Errorf("wordKeys(a) = %v, want nil", got)
	}
	if got := wordKeys("의"); got != nil {
		t.Errorf("wordKeys(의) = %v, want nil", got)
	}
}

func TestLatinMatchingUnchangedByBigrams(t *testing.T) {
	// Latin text must keep meeting the way search does: camelCase, snake_case
	// and the joined form are the same term.
	keys := keySet("cancelPayment handler")
	if n := countMatchingWords("cancel_payment", keys); n != 1 {
		t.Fatalf("countMatchingWords = %d, want the identifier forms to meet", n)
	}
}

func TestSelectMatchesKoreanTask(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "docs", "target.md"), targetDoc)
	mustWrite(t, filepath.Join(root, DirName, setsSubdir, "release.md"),
		"---\nroles: []\n---\n\n# 릴리스\n\n릴리스를 낼 때 필요한 지식.\n\n"+
			"## 버전 자리 고르기\n\n- [규칙](../../target.md#section-one) — 요약.\n")
	store, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}

	if sel := store.Select("", "릴리스를 내야 한다"); sel.Empty() {
		t.Fatal("a Korean task must reach a Korean set")
	}
	// Conservatism must survive the bridge: unrelated work still matches
	// nothing, or the catalogue teaches readers to skip it.
	if sel := store.Select("", "css 색을 바꾼다"); !sel.Empty() {
		t.Fatalf("unrelated task matched %v", names(sel.Sets))
	}
}

// Counting concepts must not depend on the order the query says them in: the
// same task worded two ways must pull the same sets.
func TestCountMatchingWordsIsOrderIndependent(t *testing.T) {
	keys := keySet("에이전트 이전 버전 호환")
	a := countMatchingWords("에이전트 이전", keys)
	b := countMatchingWords("이전 에이전트", keys)
	if a != b {
		t.Fatalf("order changed the count: %d vs %d", a, b)
	}
	// And two words that are one concept still count once.
	if n := countMatchingWords("서브에이전트를 에이전트", keySet("에이전트 표")); n != 1 {
		t.Fatalf("one concept counted %d times", n)
	}
}
