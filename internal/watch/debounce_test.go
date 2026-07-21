package watch

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

// Test100EventsCoalesceToSingleBatch proves §7-P1-②: a burst of 100 events
// yields exactly one batch with deduplicated paths.
func Test100EventsCoalesceToSingleBatch(t *testing.T) {
	d := NewDebouncer(80*time.Millisecond, 5*time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go d.Run(ctx)

	for i := 0; i < 100; i++ {
		d.In() <- Change{Path: fmt.Sprintf("/src/f%d.kt", i%10), Kind: Modified}
	}

	var first Batch
	select {
	case first = <-d.Out():
	case <-time.After(2 * time.Second):
		t.Fatal("no batch within 2s")
	}
	if len(first) != 10 {
		t.Fatalf("batch has %d paths, want 10 deduplicated", len(first))
	}

	select {
	case b := <-d.Out():
		t.Fatalf("unexpected second batch with %d paths", len(b))
	case <-time.After(300 * time.Millisecond):
	}
}

// TestMaxLatencyCapFlushesUnderContinuousEvents: a stream that never goes
// quiet still flushes at the max-latency cap, so consumers are not starved.
func TestMaxLatencyCapFlushesUnderContinuousEvents(t *testing.T) {
	d := NewDebouncer(100*time.Millisecond, 300*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go d.Run(ctx)

	var (
		mu      sync.Mutex
		batches int
	)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-d.Out():
				mu.Lock()
				batches++
				mu.Unlock()
			}
		}
	}()

	stop := time.Now().Add(1 * time.Second)
	i := 0
	for time.Now().Before(stop) {
		d.In() <- Change{Path: fmt.Sprintf("/src/g%d.py", i%5), Kind: Modified}
		i++
		time.Sleep(30 * time.Millisecond) // always within the quiet window
	}

	mu.Lock()
	got := batches
	mu.Unlock()
	if got < 2 {
		t.Fatalf("expected >=2 cap-forced flushes during 1s of continuous events, got %d", got)
	}
}

func TestLatestEventWinsPerPath(t *testing.T) {
	d := NewDebouncer(60*time.Millisecond, time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go d.Run(ctx)

	d.In() <- Change{Path: "/src/a.java", Kind: Modified}
	d.In() <- Change{Path: "/src/a.java", Kind: Removed}

	select {
	case b := <-d.Out():
		if got := b["/src/a.java"].Kind; got != Removed {
			t.Fatalf("latest event should win, got kind %v", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no batch")
	}
}
