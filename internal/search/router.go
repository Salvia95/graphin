// Package search routes queries per §2.1.1: Tier-0 exact matches
// short-circuit to the top; remaining slots come from BM25 and, once the
// vector engine is warm, RRF-merged semantic hits (Phase 4).
package search

import (
	"github.com/llls2542/graphin/internal/lexical"
)

// MatchType tags how a result matched. Raw scores are never exposed.
type MatchType string

const (
	MatchExact    MatchType = "exact"
	MatchLexical  MatchType = "lexical"
	MatchSemantic MatchType = "semantic"
	MatchBoth     MatchType = "both"
)

// Result is one ranked search result.
type Result struct {
	NodeID string
	Rank   int
	Match  MatchType
}

// Semantic is implemented by the vector engine once it is warmed up.
type Semantic interface {
	Ready() bool
	// Search returns ranked node IDs, best first.
	Search(query string, topK int) []string
}

// Router combines Tier-0, lexical and (later) semantic search.
type Router struct {
	Sym *lexical.SymbolTable
	Lex *lexical.Index
	Sem Semantic // nil until Phase 4 wires the vector engine
}

// SemanticReady reports whether vector search is available.
func (r *Router) SemanticReady() bool { return r.Sem != nil && r.Sem.Ready() }

// Search returns up to topK results. Tier-0 hits are pinned to the top with
// match_type "exact"; the rest are filled from BM25 ranking.
func (r *Router) Search(query string, topK int) []Result {
	if topK <= 0 {
		topK = 5
	}
	var out []Result
	used := map[string]bool{}
	add := func(id string, mt MatchType) {
		if used[id] || len(out) >= topK {
			return
		}
		used[id] = true
		out = append(out, Result{NodeID: id, Rank: len(out) + 1, Match: mt})
	}

	for _, id := range r.Sym.Lookup(query) {
		add(id, MatchExact)
	}
	for _, h := range r.Lex.Search(lexical.Tokenize(query), topK+len(out)) {
		add(h.DocID, MatchLexical)
	}
	return out
}
