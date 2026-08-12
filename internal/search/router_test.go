package search

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/Salvia95/graphin/internal/lexical"
)

const (
	targetID = "com.example.order.domain.OrderService.cancelPayment(long,String)"
	noisyID  = "com.example.refund.RefundService.cancelPaymentAndRefundEverything(String)"
)

func fixtureRouter() *Router {
	sym := lexical.NewSymbolTable()
	ix := lexical.NewIndex()

	// The noisy doc is stuffed with the query tokens so plain BM25 prefers it.
	noisyTokens := append(
		lexical.BuildDocTokens("cancelPaymentAndRefundEverything", noisyID, "refund"),
		slices.Repeat([]string{"cancel", "payment", "cancelpayment"}, 8)...)
	ix.Upsert(noisyID, noisyTokens)
	ix.Upsert(targetID, lexical.BuildDocTokens(
		"cancelPayment", targetID, "long orderId, String reason"))

	sym.Put(noisyID, "cancelPaymentAndRefundEverything")
	sym.Put(targetID, "cancelPayment")

	return &Router{Sym: sym, Lex: ix}
}

// TestTier0ExactBeatsLexical proves §7-P1-④: an exact simple-name match is
// pinned to rank 1 even when BM25 would rank another document higher.
func TestTier0ExactBeatsLexical(t *testing.T) {
	r := fixtureRouter()

	// Sanity: BM25 alone must prefer the noisy doc for this to prove anything.
	hits := r.Lex.Search(lexical.Tokenize("cancelPayment"), 5)
	if len(hits) == 0 || hits[0].DocID != noisyID {
		t.Fatalf("test premise broken: BM25 top is %+v, want noisy doc", hits)
	}

	res := r.Search("cancelPayment", 5)
	if len(res) == 0 {
		t.Fatal("no results")
	}
	if res[0].NodeID != targetID {
		t.Fatalf("rank 1 = %s, want Tier-0 target %s", res[0].NodeID, targetID)
	}
	if res[0].Match != MatchExact {
		t.Fatalf("match_type = %s, want exact", res[0].Match)
	}
}

// TestTier0SimpleNameReturnsAllIDs: simple name → many IDs is allowed and all
// matches surface (§2.3).
func TestTier0SimpleNameReturnsAllIDs(t *testing.T) {
	sym := lexical.NewSymbolTable()
	ix := lexical.NewIndex()
	a := "com.example.OrderService.process(ProcessRequest)"
	b := "com.example.OrderService.process(ProcessRequest,boolean)"
	sym.Put(a, "process")
	sym.Put(b, "process")
	r := &Router{Sym: sym, Lex: ix}

	res := r.Search("process", 5)
	if len(res) != 2 {
		t.Fatalf("got %d results, want both overloads", len(res))
	}
	for i, want := range []string{a, b} { // lexicographic, deterministic
		if res[i].NodeID != want || res[i].Match != MatchExact {
			t.Fatalf("res[%d] = %+v, want %s exact", i, res[i], want)
		}
	}
}

// Prose stays prose: lowercase words are indistinguishable from English, so a
// sentence made only of them never pins anything.
func TestNoTier0ForMultiWordQuery(t *testing.T) {
	r := fixtureRouter()
	res := r.Search("cancel payment logic", 5)
	for _, got := range res {
		if got.Match == MatchExact {
			t.Fatalf("multi-word query must not hit Tier-0: %+v", got)
		}
	}
}

// …but naming the symbol inside a sentence is knowledge of the code, and it
// must be worth what naming it alone is worth. Without this the definition
// loses to its own call sites, which repeat the sentence's prose words
// (docs/eval/2026-08-07-adoption-diagnosis/findings.md §원인 ③).
func TestTier0PinsIdentifierTokenInSentence(t *testing.T) {
	r := fixtureRouter()
	for _, q := range []string{
		"where is cancelPayment called from",
		"cancelPayment, and what breaks if I change it?",
	} {
		res := r.Search(q, 5)
		if len(res) == 0 || res[0].NodeID != targetID || res[0].Match != MatchExact {
			t.Fatalf("%q: rank 1 = %+v, want %s exact", q, res, targetID)
		}
	}
}

// A token hit is one word of a sentence, not the whole request, so it may not
// take the entire list — otherwise an overloaded name would bury everything
// the ranking engines found.
func TestTier0TokenPinningIsCapped(t *testing.T) {
	sym := lexical.NewSymbolTable()
	ix := lexical.NewIndex()
	var ids []string
	for _, sig := range []string{"(int)", "(long)", "(String)", "(Object)", "(byte[])"} {
		id := "com.example.Service.doWork" + sig
		sym.Put(id, "doWork")
		ix.Upsert(id, lexical.BuildDocTokens("doWork", id, "work"))
		ids = append(ids, id)
	}
	other := "com.example.Other.helper()"
	ix.Upsert(other, lexical.BuildDocTokens("helper", other, "work queue drain"))
	r := &Router{Sym: sym, Lex: ix}

	res := r.Search("how does doWork drain the work queue", 6)
	exact := 0
	for _, got := range res {
		if got.Match == MatchExact {
			exact++
		}
	}
	if want := tier0Cap(6); exact != want {
		t.Fatalf("pinned %d exact of %d overloads, want cap %d: %+v", exact, len(ids), want, res)
	}
	// The whole-query form is a different claim — the caller typed the symbol
	// and nothing else — so the cap does not apply and every overload is
	// pinned before BM25 fills what is left.
	res = r.Search("doWork", 6)
	if len(res) < len(ids) {
		t.Fatalf("bare symbol query got %d results, want at least %d overloads", len(res), len(ids))
	}
	for i := range ids {
		if res[i].Match != MatchExact {
			t.Fatalf("bare symbol query: res[%d] = %+v, want exact", i, res[i])
		}
	}
}

func TestIdentTokens(t *testing.T) {
	got := identTokens("where is cancelPayment, HTTPServer and drain_queue in Foo?")
	want := []string{"cancelPayment", "HTTPServer", "drain_queue", "Foo"}
	if !slices.Equal(got, want) {
		t.Fatalf("identTokens = %v, want %v", got, want)
	}
	// Bare lowercase words are prose; digits-first is not an identifier.
	if got := identTokens("start the session for 3rd worktree"); len(got) != 0 {
		t.Fatalf("prose produced identifiers: %v", got)
	}
}

// proseHeavyRouter builds a workspace shaped like this repository at the time
// the target filter was added: a pile of eval notes whose prose matches a
// sentence query better than the function the sentence is about. Measured
// then: "cross-file edge invalidation when caller file changes" returned three
// markdown files and two tests, and the implementation was nowhere.
func proseHeavyRouter(nDocs int) (r *Router, codeID string, isDoc func(string) bool) {
	sym := lexical.NewSymbolTable()
	ix := lexical.NewIndex()

	prose := []string{"cross", "file", "edge", "invalidation", "caller", "changes"}
	for i := range nDocs {
		id := fmt.Sprintf("docs/eval/note-%02d.md", i)
		ix.Upsert(id, slices.Repeat(prose, 6))
	}

	codeID = "internal.graph.Engine.refreshCrossFileEdges"
	ix.Upsert(codeID, lexical.BuildDocTokens("refreshCrossFileEdges", codeID, "caller string"))
	sym.Put(codeID, "refreshCrossFileEdges")

	return &Router{Sym: sym, Lex: ix}, codeID,
		func(id string) bool { return strings.HasSuffix(id, ".md") }
}

const proseQuery = "cross file edge invalidation when caller changes"

// TestTargetFilterRecoversCodeBuriedUnderProse is the defect this filter was
// built for: without it the whole result list is documentation.
func TestTargetFilterRecoversCodeBuriedUnderProse(t *testing.T) {
	r, codeID, isDoc := proseHeavyRouter(20)

	// Sanity: unfiltered, prose must own every slot, or this proves nothing.
	for _, res := range r.Search(proseQuery, 5) {
		if !isDoc(res.NodeID) {
			t.Fatalf("precondition broken: %q is not prose; the fixture no longer reproduces the defect", res.NodeID)
		}
	}

	got := r.SearchFiltered(proseQuery, 5, func(id string) bool { return !isDoc(id) })
	if len(got) != 1 || got[0].NodeID != codeID {
		t.Fatalf("filtered search = %+v, want exactly the implementation %q", got, codeID)
	}
	if got[0].Rank != 1 {
		t.Errorf("rank = %d, want 1: ranks are positions in the returned list", got[0].Rank)
	}
}

// TestTargetFilterWidensCandidatePool guards the subtle half. Filtering the
// unwidened pool would find nothing here: the implementation sits below more
// prose documents than topK*fetchPerSlot reaches.
func TestTargetFilterWidensCandidatePool(t *testing.T) {
	buried := 5 * fetchPerSlot * 2 // deeper than the unfiltered pool ever looks
	r, codeID, isDoc := proseHeavyRouter(buried)

	// Precondition, stated against the real narrow pool rather than a
	// stand-in: at the unfiltered candidate count the implementation is not
	// among the candidates at all, so no amount of filtering could surface it.
	narrow := r.Lex.Search(lexical.Tokenize(proseQuery), fetchSize(5, nil))
	if slices.ContainsFunc(narrow, func(h lexical.Hit) bool { return h.DocID == codeID }) {
		t.Fatalf("fixture too small: %q is already inside the narrow pool of %d", codeID, len(narrow))
	}
	got := r.SearchFiltered(proseQuery, 5, func(id string) bool { return !isDoc(id) })
	if len(got) != 1 || got[0].NodeID != codeID {
		t.Fatalf("filtered search = %+v, want the implementation %q found past the narrow pool", got, codeID)
	}
}

// TestTargetFilterAppliesToTier0 proves an exact match cannot smuggle itself
// past the filter. A caller asking for code only must not be handed a document
// just because its name matched exactly.
func TestTargetFilterAppliesToTier0(t *testing.T) {
	sym := lexical.NewSymbolTable()
	ix := lexical.NewIndex()
	const docID = "docs/adr/headRef.md"
	sym.Put(docID, "headRef")
	ix.Upsert(docID, lexical.BuildDocTokens("headRef", docID, ""))

	r := &Router{Sym: sym, Lex: ix}
	if got := r.Search("headRef", 5); len(got) == 0 || got[0].Match != MatchExact {
		t.Fatalf("precondition: unfiltered search must return the exact doc hit, got %+v", got)
	}
	if got := r.SearchFiltered("headRef", 5, func(id string) bool { return !strings.HasSuffix(id, ".md") }); len(got) != 0 {
		t.Fatalf("filtered search = %+v, want empty: the only exact match is excluded", got)
	}
}

// TestNilFilterMatchesUnfilteredSearch pins the promise that adding the
// parameter changed nothing for callers that omit it.
func TestNilFilterMatchesUnfilteredSearch(t *testing.T) {
	r, _, _ := proseHeavyRouter(20)
	for _, q := range []string{proseQuery, "refreshCrossFileEdges", "caller"} {
		want, got := r.Search(q, 5), r.SearchFiltered(q, 5, nil)
		if !slices.Equal(want, got) {
			t.Errorf("query %q: nil filter = %+v, want identical to Search %+v", q, got, want)
		}
	}
}
