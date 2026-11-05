package search

import (
	"testing"

	"github.com/Salvia95/graphin/internal/lexical"
	"github.com/Salvia95/graphin/internal/nodeid"
)

func chunkRouter() (*Router, string) {
	sym, ix := lexical.NewSymbolTable(), lexical.NewIndex()
	sec := "docs/ops.md#운영-메모"
	ix.Upsert(sec, lexical.Tokenize("운영 메모 홈 피드 개요"))
	ix.Upsert(nodeid.ChunkID(sec, 2), lexical.Tokenize("운영 메모 무효화 웹훅 트리거"))
	ix.Upsert(nodeid.ChunkID(sec, 3), lexical.Tokenize("운영 메모 무효화 재시도 백오프"))
	ix.Upsert("docs/other.md#기타", lexical.Tokenize("기타 무효화 언급"))
	return &Router{Sym: sym, Lex: ix}, sec
}

// A hit on a chunk is reported as its section, once, with the chunk as Via.
func TestChunksFoldIntoTheirSection(t *testing.T) {
	r, sec := chunkRouter()
	res := r.Search("무효화 웹훅 재시도", 5)
	if len(res) != 2 || res[0].NodeID != sec {
		t.Fatalf("results: %+v", res)
	}
	if res[0].Via != nodeid.ChunkID(sec, 2) && res[0].Via != nodeid.ChunkID(sec, 3) {
		t.Fatalf("Via = %q, want one of the section's chunks", res[0].Via)
	}
	for _, x := range res {
		if _, _, ok := nodeid.ChunkParent(x.NodeID); ok {
			t.Fatalf("a chunk id leaked into the results: %+v", res)
		}
	}
	// The section's own entry winning leaves Via empty.
	if res := r.Search("홈 피드 개요", 5); len(res) == 0 || res[0].NodeID != sec || res[0].Via != "" {
		t.Fatalf("head query: %+v", res)
	}
}

// With semantic on, both engines fold, and a chunk in either still names
// its section.
func TestChunksFoldUnderRRF(t *testing.T) {
	r, sec := chunkRouter()
	r.Sem = &fakeSem{ready: true, ids: []string{nodeid.ChunkID(sec, 3), "docs/other.md#기타", nodeid.ChunkID(sec, 2)}}
	res := r.Search("백오프", 5)
	if len(res) != 2 || res[0].NodeID != sec || res[0].Match != MatchBoth {
		t.Fatalf("results: %+v", res)
	}
	if res[0].Via != nodeid.ChunkID(sec, 3) {
		t.Fatalf("Via = %q", res[0].Via)
	}
}

func TestFoldChunksKeepsPlainListsIntact(t *testing.T) {
	ids := []string{"a", "b", "c", "d"}
	got, via := foldChunks(ids, 3)
	if len(got) != 3 || got[0] != "a" || got[2] != "c" || len(via) != 0 {
		t.Fatalf("got %v via %v", got, via)
	}
}
