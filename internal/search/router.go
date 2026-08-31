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

// Filter reports whether a node may appear in results. A nil Filter accepts
// everything.
type Filter func(nodeID string) bool

func (f Filter) ok(id string) bool { return f == nil || f(id) }

// Candidate pool sizes. Unfiltered, three per requested slot is enough slack
// for the RRF merge to reorder within. A filter draws from that same pool, so
// leaving it unwidened would answer "five code hits" with two whenever the top
// of the ranking is documentation — which is exactly the query shape the
// filter exists for.
//
// Widening is close to free. The semantic side pays one query embedding
// regardless of topK, and BM25 only grows the heap it already scores into.
const (
	fetchPerSlot         = 3
	filteredFetchPerSlot = 24
	maxFetch             = 600
)

func fetchSize(topK int, filter Filter) int {
	per := fetchPerSlot
	if filter != nil {
		per = filteredFetchPerSlot
	}
	if n := topK * per; n < maxFetch {
		return n
	}
	return maxFetch
}

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
	return r.SearchFilteredK(query, topK, k, nil)
}

// SearchFiltered runs Search admitting only nodes the filter accepts.
func (r *Router) SearchFiltered(query string, topK int, filter Filter) []Result {
	return r.SearchFilteredK(query, topK, rrfK, filter)
}

// SearchFilteredStats is SearchFiltered plus Stats, at the shipped RRF
// constant — the shape a tool handler wants.
func (r *Router) SearchFilteredStats(query string, topK int, filter Filter) ([]Result, Stats) {
	return r.SearchStats(query, topK, rrfK, filter)
}

// Stats is what the response can say about the retrieval itself, without
// exposing a score. Match types already rank the evidence — exact beats a
// both-engine agreement beats a single engine — but they say nothing about how
// hard the query was, and an RRF score cannot fill that gap either: with k=60
// the first and tenth results differ by 1/61 against 1/70, so any band derived
// from them would call everything strong.
//
// LexicalMatched does say it. It is the size of the pool the ranking chose
// from, which is the difference between "the query named something" and "the
// query touched a third of the repository".
type Stats struct {
	LexicalMatched int
	SemanticReady  bool
	// AbsentIdents are identifier-shaped words of the query that no indexed
	// symbol spells. They matter because an empty result list is not the dead
	// end a search loop actually hits: BM25 splits `zzz_no_such_symbol_zzz`
	// into `zzz`, `no`, `such`, `symbol` and answers with five plausible hits
	// built entirely out of the common words. Saying which identifier is
	// simply not here is the difference between "look harder" and "look
	// elsewhere".
	AbsentIdents []string
	// UnnamedIdents are identifier-shaped words the index does hold as text
	// but that name no symbol. `RETRY_BUDGET` is the shape: a package-level
	// constant is a real thing an agent asks for, and it is spelled inside the
	// bodies that use it, but no node is named it — so the ranking can only
	// ever answer with its callers. Saying so sends the caller to the
	// retriever that can point at the declaration itself.
	UnnamedIdents []string
}

// codeShaped narrows identTokens for hint purposes: a bare acronym like HTTP
// passes the identifier shape test but is ordinary prose in a question, and
// reporting it every time would make the hint noise.
func codeShaped(w string) bool {
	if strings.Contains(w, "_") {
		return true
	}
	hasLower, hasUpper := false, false
	for _, r := range w {
		switch {
		case r >= 'a' && r <= 'z':
			hasLower = true
		case r >= 'A' && r <= 'Z':
			hasUpper = true
		}
	}
	return hasLower && hasUpper
}

// absentIdents keeps the identifier-shaped words whose joined form no document
// carries. The joined form is the test that works: `stemFinalE` indexes as
// `stemfinale` beside its parts, so its absence means the symbol itself is
// missing rather than one of its syllables being rare.
func (r *Router) identStates(query string) (absent, unnamed []string) {
	for _, w := range identTokens(query) {
		term := strings.ToLower(w)
		if parts := lexical.SplitIdentifier(w); len(parts) > 1 {
			term = strings.Join(parts, "")
		}
		switch {
		case !r.Lex.HasTerm(term):
			absent = append(absent, w)
		case len(r.Sym.Lookup(w)) == 0 && codeShaped(w):
			unnamed = append(unnamed, w)
		}
	}
	return absent, unnamed
}

// SearchFilteredK is Search with both knobs. The filter is applied to every
// candidate stream — Tier-0 included — so an exact match outside the requested
// population never takes a slot it would then have to be excused from.
func (r *Router) SearchFilteredK(query string, topK, k int, filter Filter) []Result {
	hits, _ := r.SearchStats(query, topK, k, filter)
	return hits
}

// SearchStats is SearchFilteredK plus what the caller needs to decide whether
// to ask again differently.
func (r *Router) SearchStats(query string, topK, k int, filter Filter) ([]Result, Stats) {
	if topK <= 0 {
		topK = 5
	}
	var out []Result
	used := map[string]bool{}
	add := func(id string, mt MatchType) {
		if used[id] || len(out) >= topK || !filter.ok(id) {
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
		// Unfiltered, topK+len(out) is exactly enough — and it is the pool the
		// lexical-only sweeps in docs/eval were measured on, so it stays put.
		// A filter needs the wider pool for the same reason the RRF path does.
		n := topK + len(out)
		if filter != nil {
			n = fetchSize(topK, filter)
		}
		hits, matched := r.Lex.SearchMatched(lexical.Tokenize(query), n)
		for _, h := range hits {
			add(h.DocID, MatchLexical)
		}
		absent, unnamed := r.identStates(query)
		return out, Stats{LexicalMatched: matched, AbsentIdents: absent, UnnamedIdents: unnamed}
	}

	// RRF merge: Score(d) = Σ 1/(k + rank), rank 1-based per engine.
	fetch := fetchSize(topK, filter)
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
	lexHits, matched := r.Lex.SearchMatched(lexical.Tokenize(query), fetch)
	for i, h := range lexHits {
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
	absent, unnamed := r.identStates(query)
	return out, Stats{LexicalMatched: matched, SemanticReady: true, AbsentIdents: absent, UnnamedIdents: unnamed}
}
