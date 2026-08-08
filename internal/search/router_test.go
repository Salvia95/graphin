package search

import (
	"slices"
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
