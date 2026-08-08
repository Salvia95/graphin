// Package search routes queries per §2.1.1: Tier-0 exact matches
// short-circuit to the top; remaining slots come from BM25 and, once the
// vector engine is warm, RRF-merged semantic hits (Phase 4).
package search

import (
	"sort"
	"strings"

	"github.com/Salvia95/graphin/internal/lexical"
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

// rrfK is the reciprocal-rank-fusion constant (§2.1.1; §8 lists it as a
// benchmark-tunable).
const rrfK = 60

// tier0Cap bounds how much of the result list per-token exact matches may
// take. A whole-query Tier-0 hit means the caller typed the symbol and nothing
// else, so it earns the whole list; a token hit is one word of a sentence, and
// an overloaded name could otherwise pin every overload above everything the
// ranking engines found.
func tier0Cap(topK int) int {
	if topK < 2 {
		return 1
	}
	return topK / 2
}

// identTokens returns the whitespace-separated words of q that look like code
// identifiers rather than prose: snake_case, camelCase/PascalCase, or
// SCREAMING_CAPS. Surrounding punctuation is trimmed so "the headRef," works.
//
// Deliberately not lexical.Tokenize: that splits camelCase into words, which
// is right for BM25 and useless here — `headRef` must survive as `headRef` to
// match a symbol name.
func identTokens(q string) []string {
	var out []string
	for _, w := range strings.Fields(q) {
		w = strings.Trim(w, `.,;:!?()[]{}"'`+"`")
		if isIdentShaped(w) {
			out = append(out, w)
		}
	}
	return out
}

// isIdentShaped mirrors the shape rule usage.PatternShape applies to grep
// patterns (usage-spec §4.2.1), narrowed to a single token: a bare lowercase
// word is indistinguishable from prose, so it does not qualify.
func isIdentShaped(t string) bool {
	if len(t) < 2 {
		return false
	}
	upper, lower, underscore := false, false, false
	for i, r := range t {
		switch {
		case r == '_':
			underscore = true
		case r >= 'a' && r <= 'z':
			lower = true
		case r >= 'A' && r <= 'Z':
			upper = true
		case r >= '0' && r <= '9':
			if i == 0 {
				return false
			}
		default:
			return false
		}
	}
	return underscore || (upper && lower) || upper
}

// Search returns up to topK results. Tier-0 hits are pinned to the top with
// match_type "exact"; remaining slots come from BM25 alone before semantic
// warmup, or from the RRF merge of both rankings after (§2.1.1). Raw scores
// never leave this function.
func (r *Router) Search(query string, topK int) []Result {
	return r.SearchK(query, topK, rrfK)
}

// SearchK runs Search with an explicit RRF constant — the §8 benchmark
// sweeps k ∈ {20, 60, 100}.
func (r *Router) SearchK(query string, topK, k int) []Result {
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

	// Tier-0 short-circuit: exact matches never depend on ranking engines.
	for _, id := range r.Sym.Lookup(query) {
		add(id, MatchExact)
	}
	// A question can name a symbol without being one. "where does the runtime
	// resolve the headRef of a worktree" knows the symbol exactly, but the
	// whole-string lookup above refuses multi-word queries, so that knowledge
	// was thrown away and BM25 decided alone — and BM25 ranks a definition
	// below its own call sites, because callers repeat the prose tokens too.
	// Measured on testdata/fixtures/ranking: rank 3, then off the list.
	//
	// So pin identifier-shaped tokens as well. Only identifier-shaped ones:
	// "start" and "session" are query prose that happens to be spellable as a
	// symbol, while `headRef`/`CONTEXT_TYPES` are things only someone who read
	// the code would type.
	if len(out) == 0 {
		for _, tok := range identTokens(query) {
			if len(out) >= tier0Cap(topK) {
				break
			}
			for _, id := range r.Sym.Lookup(tok) {
				if len(out) >= tier0Cap(topK) {
					break
				}
				add(id, MatchExact)
			}
		}
	}

	if !r.SemanticReady() {
		for _, h := range r.Lex.Search(lexical.Tokenize(query), topK+len(out)) {
			add(h.DocID, MatchLexical)
		}
		return out
	}

	// RRF merge: Score(d) = Σ 1/(k + rank), rank 1-based per engine.
	fetch := topK * 3
	type fused struct {
		score    float64
		lex, sem bool
	}
	scores := map[string]*fused{}
	at := func(id string) *fused {
		f := scores[id]
		if f == nil {
			f = &fused{}
			scores[id] = f
		}
		return f
	}
	for i, h := range r.Lex.Search(lexical.Tokenize(query), fetch) {
		f := at(h.DocID)
		f.score += 1.0 / float64(k+i+1)
		f.lex = true
	}
	for i, id := range r.Sem.Search(query, fetch) {
		f := at(id)
		f.score += 1.0 / float64(k+i+1)
		f.sem = true
	}

	ids := make([]string, 0, len(scores))
	for id := range scores {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		a, b := scores[ids[i]], scores[ids[j]]
		if a.score != b.score {
			return a.score > b.score
		}
		return ids[i] < ids[j]
	})
	for _, id := range ids {
		f := scores[id]
		switch {
		case f.lex && f.sem:
			add(id, MatchBoth)
		case f.sem:
			add(id, MatchSemantic)
		default:
			add(id, MatchLexical)
		}
	}
	return out
}
