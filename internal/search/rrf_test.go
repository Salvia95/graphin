package search

import (
	"testing"

	"github.com/Salvia95/graphin/internal/lexical"
)

type fakeSem struct {
	ready bool
	ids   []string
}

func (f *fakeSem) Ready() bool { return f.ready }

func (f *fakeSem) Search(_ string, topK int) []string {
	if len(f.ids) > topK {
		return f.ids[:topK]
	}
	return f.ids
}

// TestModelNotLoadedReturnsLexicalOnly proves §7-P4-①: before warmup the
// router serves BM25 results and reports semantic_ready=false — no blocking.
func TestModelNotLoadedReturnsLexicalOnly(t *testing.T) {
	sym := lexical.NewSymbolTable()
	ix := lexical.NewIndex()
	ix.Upsert("com.a.PayService.charge(long)", lexical.BuildDocTokens("charge", "com.a.PayService.charge(long)", "long"))
	r := &Router{Sym: sym, Lex: ix, Sem: &fakeSem{ready: false, ids: []string{"should.not.appear"}}}

	if r.SemanticReady() {
		t.Fatal("semantic must not be ready")
	}
	res := r.Search("charge", 5)
	if len(res) != 1 || res[0].Match != MatchLexical {
		t.Fatalf("expected lexical-only results, got %+v", res)
	}
}

// TestRRFMergeAndMatchTypes: a doc ranked by both engines fuses to the top
// with match_type "both"; single-engine docs keep their engine tag.
func TestRRFMergeAndMatchTypes(t *testing.T) {
	sym := lexical.NewSymbolTable()
	ix := lexical.NewIndex()
	// Lexical ranking: lexTop (strong), shared (weaker).
	ix.Upsert("pkg.LexTop.f()", []string{"cancel", "cancel", "cancel", "payment"})
	ix.Upsert("pkg.Shared.g()", []string{"cancel", "payment"})
	sem := &fakeSem{ready: true, ids: []string{"pkg.Shared.g()", "pkg.SemOnly.h()"}}
	r := &Router{Sym: sym, Lex: ix, Sem: sem}

	res := r.Search("cancel payment", 5)
	if len(res) != 3 {
		t.Fatalf("got %+v", res)
	}
	byID := map[string]Result{}
	for i, x := range res {
		if x.Rank != i+1 {
			t.Fatalf("ranks must be contiguous 1..n: %+v", res)
		}
		byID[x.NodeID] = x
	}
	// Shared appears in both lists → RRF sum wins and match_type=both.
	if res[0].NodeID != "pkg.Shared.g()" || res[0].Match != MatchBoth {
		t.Fatalf("dual-engine doc must fuse to top as 'both': %+v", res)
	}
	if byID["pkg.LexTop.f()"].Match != MatchLexical {
		t.Fatalf("lexical-only tag wrong: %+v", res)
	}
	if byID["pkg.SemOnly.h()"].Match != MatchSemantic {
		t.Fatalf("semantic-only tag wrong: %+v", res)
	}
}

// TestTier0StillWinsOverRRF: exact matches stay pinned above fused results.
func TestTier0StillWinsOverRRF(t *testing.T) {
	sym := lexical.NewSymbolTable()
	ix := lexical.NewIndex()
	target := "pkg.Exact.hit()"
	sym.Put(target, "hit")
	ix.Upsert("pkg.Other.f()", []string{"hit", "hit", "hit"})
	sem := &fakeSem{ready: true, ids: []string{"pkg.Other.f()"}}
	r := &Router{Sym: sym, Lex: ix, Sem: sem}

	res := r.Search("hit", 5)
	if res[0].NodeID != target || res[0].Match != MatchExact {
		t.Fatalf("Tier-0 must stay rank 1: %+v", res)
	}
}
