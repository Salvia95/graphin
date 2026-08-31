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
