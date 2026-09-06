package tools

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Salvia95/graphin/internal/search"
)

// The absent-identifier case is checked first on purpose: it is the one that
// fires. A query naming a symbol that is not indexed still comes back full,
// because the tokenizer keeps the query's common words.
func TestSearchHintPrefersTheAbsentIdentifier(t *testing.T) {
	st := search.Stats{LexicalMatched: 900, AbsentIdents: []string{"RETRY_BUDGET"}}
	h := searchHint(st, 5, 1000, "")
	if !strings.Contains(h, "RETRY_BUDGET") || !strings.Contains(h, "search_keyword") {
		t.Fatalf("hint = %q, want it to name the identifier and the retriever that can find it", h)
	}
}

func TestSearchHintOnEmptyNamesTheFilter(t *testing.T) {
	h := searchHint(search.Stats{}, 0, 1000, "code")
	if !strings.Contains(h, `target="code"`) {
		t.Fatalf("an empty filtered result must suggest dropping the filter: %q", h)
	}
	if strings.Contains(searchHint(search.Stats{}, 0, 1000, ""), "target=") {
		t.Fatal("unfiltered search must not suggest dropping a filter it never had")
	}
}

// Breadth needs both a share and a count: the share alone fires on every query
// in a ten-node workspace.
func TestSearchHintBreadthNeedsShareAndCount(t *testing.T) {
	cases := []struct {
		name             string
		matched, indexed int
		want             bool
	}{
		{"broad", 900, 2000, true},
		{"small workspace, high share", 8, 10, false},
		{"large index, low share", 150, 10000, false},
	}
	for _, c := range cases {
		got := searchHint(search.Stats{LexicalMatched: c.matched}, 5, c.indexed, "") != ""
		if got != c.want {
			t.Errorf("%s: hint=%v, want %v", c.name, got, c.want)
		}
	}
}

// The reported cost includes the element that reports it — the agent sums
// these to track a budget the stateless server cannot hold for it.
func TestCostLineCountsItself(t *testing.T) {
	for _, bodyLen := range []int{0, 9, 94, 995, 9_996, 99_997} {
		line := costLine(bodyLen)
		var n int
		if _, err := fmt.Sscanf(strings.TrimPrefix(line, "\n<cost bytes=\""), "%d", &n); err != nil {
			t.Fatalf("cannot read the cost back out of %q", line)
		}
		if total := bodyLen + len(line); n != total {
			t.Errorf("body %d: reported %d, actual %d", bodyLen, n, total)
		}
	}
}

// The two identifier states are different instructions. "Not here at all" ends
// the search; "here as text but not as a symbol" redirects it.
func TestSearchHintSeparatesAbsentFromUnnamed(t *testing.T) {
	absent := searchHint(search.Stats{AbsentIdents: []string{"zzz_nope"}}, 5, 1000, "")
	if !strings.Contains(absent, "not in this workspace") {
		t.Fatalf("absent identifier hint = %q", absent)
	}
	unnamed := searchHint(search.Stats{UnnamedIdents: []string{"RETRY_BUDGET"}}, 5, 1000, "")
	if !strings.Contains(unnamed, "no indexed symbol is named it") {
		t.Fatalf("unnamed identifier hint = %q", unnamed)
	}
	if strings.Contains(unnamed, "not in this workspace") {
		t.Fatal("an identifier that is present as text must not be reported as missing")
	}
}

// A quoted query is an instruction to find text, so it is said first — even
// when the ranking returned a full list of word-matches.
func TestSearchHintQuotedQueryWinsFirst(t *testing.T) {
	st := search.Stats{Quoted: true, LexicalMatched: 900, UnnamedIdents: []string{"RETRY_BUDGET"}}
	h := searchHint(st, 5, 1000, "")
	if !strings.Contains(h, "quoted") || !strings.Contains(h, "search_keyword") {
		t.Fatalf("hint = %q, want the quoted-string redirect", h)
	}
}

// The vocabulary rule needs a majority of at least two content words, and it
// yields to the identifier rules, which say something more specific.
func TestSearchHintAbsentTermsMajority(t *testing.T) {
	fires := searchHint(search.Stats{ContentTerms: 3, AbsentTerms: []string{"kubernetes", "ingress"}}, 5, 1000, "")
	if !strings.Contains(fires, "2 of the query's 3 words") || !strings.Contains(fires, "search_keyword") {
		t.Fatalf("2/3 absent must fire: %q", fires)
	}
	if h := searchHint(search.Stats{ContentTerms: 3, AbsentTerms: []string{"kubernetes"}}, 5, 1000, ""); h != "" {
		t.Fatalf("1/3 absent must not fire: %q", h)
	}
	if h := searchHint(search.Stats{ContentTerms: 1, AbsentTerms: []string{"kubernetes"}}, 5, 1000, ""); h != "" {
		t.Fatalf("a one-word query must not fire: %q", h)
	}
	ident := searchHint(search.Stats{ContentTerms: 2, AbsentTerms: []string{"zzz_nope", "thing"},
		AbsentIdents: []string{"zzz_nope"}}, 5, 1000, "")
	if !strings.Contains(ident, "no indexed symbol spells") {
		t.Fatalf("identifier rule must win over the vocabulary rule: %q", ident)
	}
	// Empty result with a majority absent: the vocabulary hint is the more
	// specific one and comes first.
	empty := searchHint(search.Stats{ContentTerms: 2, AbsentTerms: []string{"kubernetes", "ingress"}}, 0, 1000, "")
	if !strings.Contains(empty, "in no indexed document") {
		t.Fatalf("vocabulary hint must precede the plain empty hint: %q", empty)
	}
}

func TestKeywordEmptyHintMentionsRegexOnlyForLiterals(t *testing.T) {
	if h := keywordEmptyHint(false); !strings.Contains(h, "regex=true") || !strings.Contains(h, "search_hybrid") {
		t.Fatalf("literal miss: %q", h)
	}
	if h := keywordEmptyHint(true); strings.Contains(h, "regex=true") {
		t.Fatalf("a regex miss must not suggest regex: %q", h)
	}
}
