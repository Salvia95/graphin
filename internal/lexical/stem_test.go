package lexical

import "testing"

// The point of stemming is that a prose inflection and the identifier it
// refers to land on the same term. Each pair here is a question shape that
// missed before: "acquired" for `Acquire`, "re-embedded" for `Embed`.
func TestStemMeetsIdentifierForm(t *testing.T) {
	for _, pair := range [][2]string{
		{"acquired", "acquire"},
		{"released", "release"},
		{"releases", "release"},
		{"embeddings", "embedded"},
		{"embedded", "embed"},
		{"files", "file"},
		{"watching", "watch"},
		{"truncated", "truncate"},
		{"classes", "class"},
		{"indexes", "index"},
		{"stopped", "stop"},
	} {
		if a, b := Stem(pair[0]), Stem(pair[1]); a != b {
			t.Errorf("%q → %q but %q → %q; they must meet", pair[0], a, pair[1], b)
		}
	}
}

// Guards. Over-stemming costs precision silently, so the words that must
// survive intact are pinned.
func TestStemLeavesShortAndNonWordsAlone(t *testing.T) {
	for _, w := range []string{
		"go", "id", "err", "utf8", "v2", "sha256", // short or digit-bearing
		"class", "status", "analysis", // ss/us/is are not plurals
		"file", "node", "code", "type", // four letters keep their "e"
		"degree", "call", "pass", // "ee" and doubled l/s
		"embed", "used", // stripping would leave a stub
	} {
		if got := Stem(w); got != w {
			t.Errorf("Stem(%q) = %q, want it unchanged", w, got)
		}
	}
}

// The index stems on write and Search stems on read; a caller that never
// heard of stemming must still find an inflected document.
func TestIndexMatchesAcrossInflection(t *testing.T) {
	ix := NewIndex()
	ix.Upsert("lock.Acquire", Tokenize("Acquire the workspace lock"))
	ix.Upsert("noise.Other", Tokenize("something entirely unrelated"))

	hits := ix.Search(Tokenize("where is the lock acquired"), 5)
	if len(hits) == 0 || hits[0].DocID != "lock.Acquire" {
		t.Fatalf("hits = %v, want lock.Acquire first", hits)
	}
}

// Korean questions arrive with particles glued on. The noun has to meet the
// bare form it is written as in the document.
func TestStemDropsKoreanParticles(t *testing.T) {
	for _, pair := range [][2]string{
		{"이벤트를", "이벤트"},
		{"델타로그가", "델타로그"},
		{"워크스페이스의", "워크스페이스"},
		{"인제스트하지", "인제스트"},
		{"집계하는", "집계"},
		{"graphin을", "graphin"}, // mixed script: one token, Korean tail
	} {
		if a, b := Stem(pair[0]), Stem(pair[1]); a != b {
			t.Errorf("%q → %q but %q → %q; they must meet", pair[0], a, pair[1], b)
		}
	}
}

// The two-rune floor is the whole safety margin: these are nouns that merely
// end in a particle-shaped syllable.
func TestStemKeepsShortKoreanWordsWhole(t *testing.T) {
	for _, w := range []string{"회의", "결과", "국가", "대로", "정의", "락을"} {
		if got := Stem(w); got != w {
			t.Errorf("Stem(%q) = %q, want it unchanged", w, got)
		}
	}
}
