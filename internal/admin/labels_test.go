package admin

import (
	"testing"

	"github.com/Salvia95/graphin/internal/graph"
	"github.com/Salvia95/graphin/internal/nodeid"
	"github.com/Salvia95/graphin/internal/search"
)

// 용어 사전의 완전성: 소스 enum이 늘어나면 여기서 먼저 실패해야 한다.

func TestKindLabelsComplete(t *testing.T) {
	kinds := []string{
		nodeid.KindClass, nodeid.KindInterface, nodeid.KindMethod,
		nodeid.KindFunction, nodeid.KindFile,
		nodeid.KindTable, nodeid.KindView, nodeid.KindDBFunction,
		nodeid.KindProcedure, nodeid.KindRLSPolicy, nodeid.KindTrigger,
	}
	for _, k := range kinds {
		if kindLabel(k) == k {
			t.Errorf("kind %q has no Korean label", k)
		}
	}
}

func TestTypeLabelsComplete(t *testing.T) {
	types := []graph.EdgeType{
		graph.EdgeImport, graph.EdgeExtends, graph.EdgeImplements,
		graph.EdgeCall, graph.EdgeReference, graph.EdgeForeignKey,
	}
	for _, et := range types {
		name := graph.EdgeTypeName(et)
		if typeLabel(name) == name {
			t.Errorf("edge type %q has no Korean label", name)
		}
	}
}

func TestStateLabelsComplete(t *testing.T) {
	for _, s := range []string{"not_bootstrapped", "indexing", "ready"} {
		if stateLabel(s) == s {
			t.Errorf("state %q has no Korean label", s)
		}
	}
}

func TestMatchLabelsComplete(t *testing.T) {
	for _, m := range []search.MatchType{
		search.MatchExact, search.MatchLexical, search.MatchSemantic, search.MatchBoth,
	} {
		if matchLabel(m) == string(m) {
			t.Errorf("match %q has no Korean label", m)
		}
	}
}

func TestLabelFallbackKeepsRaw(t *testing.T) {
	if kindLabel("mystery") != "mystery" || minConfLabel("0.42") != "0.42" {
		t.Fatal("unknown values must fall back to raw")
	}
}

func TestMinConfLabelFormat(t *testing.T) {
	if got := minConfLabel("0.85"); got != "0.85 — 기본값" {
		t.Fatalf("minConfLabel: %q", got)
	}
}
