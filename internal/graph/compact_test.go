package graph

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/llls2542/graphin/internal/obs"
)

func TestCompactionRewritesBaseAndEmptiesLog(t *testing.T) {
	dir := t.TempDir()
	r, err := OpenReverse(dir, obs.Nop())
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 100; i++ {
		r.Upsert("t", fmt.Sprintf("s%03d", i), EdgeCall, 0.9)
	}
	for i := 0; i < 50; i++ {
		r.Tombstone("t", fmt.Sprintf("s%03d", i), EdgeCall)
	}
	r.Compact()

	if len(r.Query("t", 0)) != 50 {
		t.Fatalf("post-compact query: %d edges", len(r.Query("t", 0)))
	}
	if fi, err := os.Stat(filepath.Join(dir, baseName)); err != nil || fi.Size() == 0 {
		t.Fatalf("base not written: %v", err)
	}
	if fi, _ := os.Stat(filepath.Join(dir, deltaName)); fi.Size() != 0 {
		t.Fatalf("delta log not rotated: %d bytes", fi.Size())
	}
	if _, err := os.Stat(filepath.Join(dir, compactingName)); !os.IsNotExist(err) {
		t.Fatal(".compacting leftover after successful compaction")
	}
	r.Close()

	// Reopen: state comes purely from the compacted base.
	r2, err := OpenReverse(dir, obs.Nop())
	if err != nil {
		t.Fatal(err)
	}
	defer r2.Close()
	if len(r2.Query("t", 0)) != 50 {
		t.Fatalf("reload after compaction: %d edges", len(r2.Query("t", 0)))
	}
}

// TestConcurrentReadsDuringCompaction proves §7-P3-②: readers stay
// consistent while compaction and writes run (exercised under -race).
func TestConcurrentReadsDuringCompaction(t *testing.T) {
	dir := t.TempDir()
	r, err := OpenReverse(dir, obs.Nop())
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	// Stable population that must always be visible.
	for i := 0; i < 200; i++ {
		r.Upsert("stable", fmt.Sprintf("s%04d", i), EdgeCall, 0.9)
	}

	stop := make(chan struct{})
	var wg sync.WaitGroup
	errs := make(chan string, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				if got := len(r.Query("stable", 0)); got != 200 {
					errs <- fmt.Sprintf("reader saw %d stable edges", got)
					return
				}
			}
		}()
	}

	for round := 0; round < 5; round++ {
		for i := 0; i < 100; i++ {
			r.Upsert("churn", fmt.Sprintf("r%d-%03d", round, i), EdgeCall, 0.8)
		}
		for i := 0; i < 60; i++ {
			r.Tombstone("churn", fmt.Sprintf("r%d-%03d", round, i), EdgeCall)
		}
		r.Compact()
	}
	close(stop)
	wg.Wait()
	select {
	case msg := <-errs:
		t.Fatal(msg)
	default:
	}

	if got := len(r.Query("churn", 0)); got != 5*40 {
		t.Fatalf("churn edges = %d, want 200", got)
	}
}

func TestCompactionTriggerConditions(t *testing.T) {
	dir := t.TempDir()
	r, err := OpenReverse(dir, obs.Nop())
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	// >30% tombstones trips the ratio trigger.
	for i := 0; i < 10; i++ {
		r.Upsert("x", fmt.Sprintf("s%d", i), EdgeCall, 0.9)
	}
	for i := 0; i < 6; i++ {
		r.Tombstone("x", fmt.Sprintf("s%d", i), EdgeCall)
	}
	r.mu.Lock()
	ratio := float64(r.tombstones) / float64(r.records)
	r.mu.Unlock()
	if ratio <= compactTombstoneFrac {
		t.Fatalf("test premise: ratio %.2f should exceed %.2f", ratio, compactTombstoneFrac)
	}
	r.Sync() // fires async compaction
	// Compact() is serialized by compactMu; call it directly to wait for a
	// deterministic post-state instead of sleeping.
	r.Compact()
	r.mu.Lock()
	recs := r.records
	r.mu.Unlock()
	if recs != 0 {
		t.Fatalf("records counter not reset after compaction: %d", recs)
	}
}

// Crash between rotation and base write: the .compacting file must be
// replayed on the next open.
func TestCompactingLeftoverReplayedOnOpen(t *testing.T) {
	dir := t.TempDir()
	var buf []byte
	buf = encodeRecord(buf, record{Op: opUpsert, TargetID: "t", SourceID: "a", Type: EdgeCall, Confidence: 0.9})
	buf = encodeRecord(buf, record{Op: opUpsert, TargetID: "t", SourceID: "b", Type: EdgeCall, Confidence: 0.9})
	if err := os.WriteFile(filepath.Join(dir, compactingName), buf, 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := OpenReverse(dir, obs.Nop())
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if len(r.Query("t", 0)) != 2 {
		t.Fatalf("leftover .compacting not replayed: %d edges", len(r.Query("t", 0)))
	}
}
