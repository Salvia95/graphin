package lexical

import "testing"

func TestBM25RankingSanity(t *testing.T) {
	ix := NewIndex()
	ix.Upsert("a", []string{"payment", "cancel", "payment", "refund"})
	ix.Upsert("b", []string{"order", "create", "list"})
	ix.Upsert("c", []string{"payment", "charge"})

	hits := ix.Search([]string{"payment", "cancel"}, 10)
	if len(hits) == 0 || hits[0].DocID != "a" {
		t.Fatalf("doc a should rank first, got %+v", hits)
	}
}

func TestBM25DeterministicTieBreak(t *testing.T) {
	ix := NewIndex()
	ix.Upsert("z", []string{"charge"})
	ix.Upsert("y", []string{"charge"})

	hits := ix.Search([]string{"charge"}, 10)
	if len(hits) != 2 || hits[0].DocID != "y" || hits[1].DocID != "z" {
		t.Fatalf("ties must break by DocID, got %+v", hits)
	}
}

func TestUpsertReplacesAndDeleteRemoves(t *testing.T) {
	ix := NewIndex()
	ix.Upsert("a", []string{"old", "tokens"})
	ix.Upsert("a", []string{"fresh"})
	if hits := ix.Search([]string{"old"}, 5); len(hits) != 0 {
		t.Fatalf("stale postings survived upsert: %+v", hits)
	}
	if hits := ix.Search([]string{"fresh"}, 5); len(hits) != 1 {
		t.Fatalf("fresh posting missing: %+v", hits)
	}
	ix.Delete("a")
	if ix.Len() != 0 {
		t.Fatal("delete left the doc behind")
	}
	if hits := ix.Search([]string{"fresh"}, 5); len(hits) != 0 {
		t.Fatalf("postings survived delete: %+v", hits)
	}
}
