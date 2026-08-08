package workspace

import (
	"context"
	"testing"
	"time"

	"github.com/Salvia95/graphin/internal/obs"
	"github.com/Salvia95/graphin/internal/search"
)

const rankingFixtureDir = "../../testdata/fixtures/ranking"

// headRefDef is the one node in the fixture that defines the symbol; every
// other node that mentions it is a call site.
const headRefDef = "src.runtime.session.headRef"

func rankingWorkspace(t *testing.T) *Workspace {
	t.Helper()
	root := t.TempDir()
	copyTree(t, rankingFixtureDir, root)
	w := New(Config{Root: root, Log: obs.Nop(), OrtLib: "/nonexistent-ort"})
	t.Cleanup(w.Close)
	if _, err := w.Bootstrap(context.Background(), "", false); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) && w.FSM.Phase() < PhaseLexicalReady {
		time.Sleep(20 * time.Millisecond)
	}
	if w.FSM.Phase() < PhaseLexicalReady {
		t.Fatal("initial scan never finished")
	}
	return w
}

// TestRankingHeadRefPair pins the one same-intent fallback the 2026-08-07
// adoption log caught: the agent asked search_hybrid a sentence containing the
// symbol, got five results, ignored them, and grepped the same token twice
// more (findings §원인 ③).
//
// The fixture reproduces why. A symbol is used far more often than it is
// defined, and every caller repeats the sentence's prose words, so BM25 alone
// ranks the definition below its own call sites — measured at rank 3 here, and
// off a five-result list for the prose-only phrasing. Naming a symbol in a
// sentence must not be worth less than naming it alone.
func TestRankingHeadRefPair(t *testing.T) {
	w := rankingWorkspace(t)

	for _, tc := range []struct {
		name  string
		query string
	}{
		{"bare symbol", "headRef"},
		{"symbol named inside a sentence", "runtime start session worktree headRef"},
		{"symbol among other prose", "which function returns the headRef for a worktree"},
	} {
		res := w.Router.Search(tc.query, 5)
		if len(res) == 0 {
			t.Fatalf("%s: no results", tc.name)
		}
		if res[0].NodeID != headRefDef || res[0].Match != search.MatchExact {
			t.Errorf("%s: got %s (%s) at rank 1, want %s (exact)\nfull: %+v",
				tc.name, res[0].NodeID, res[0].Match, headRefDef, res)
		}
	}
}

// A sentence that never names the symbol must not be pinned to anything. This
// is the other half of the rule: Tier-0 fires on knowledge the caller
// demonstrated ("headRef"), not on a topic they described ("head ref"). Those
// queries belong to the semantic engine, which this lexical-only workspace
// does not have.
func TestRankingProseQueryPinsNothing(t *testing.T) {
	w := rankingWorkspace(t)
	res := w.Router.Search("where does the runtime resolve the head ref of a worktree", 5)
	for _, r := range res {
		if r.Match == search.MatchExact {
			t.Fatalf("prose query pinned an exact match: %+v", res)
		}
	}
}
