package search

import (
	"slices"
	"testing"

	"github.com/llls2542/graphin/internal/lexical"
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

func TestNoTier0ForMultiWordQuery(t *testing.T) {
	r := fixtureRouter()
	res := r.Search("cancel payment logic", 5)
	for _, got := range res {
		if got.Match == MatchExact {
			t.Fatalf("multi-word query must not hit Tier-0: %+v", got)
		}
	}
}
