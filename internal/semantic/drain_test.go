package semantic

// Phase 7c 후속 ①: Drained()는 큐 잔량뿐 아니라 in-flight op까지 반영해야
// 한다 — QueueDepth()==0만으로는 마지막 임베딩이 끝났음을 보장하지 못한다.

import (
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Salvia95/graphin/internal/obs"
)

// gateVec blocks each Embed until the test releases the gate.
type gateVec struct{ gate chan struct{} }

func (g *gateVec) Embed(string) ([]float32, error) {
	<-g.gate
	return make([]float32, 4), nil
}

// closeVec records whether the engine released it on shutdown.
type closeVec struct{ closed atomic.Bool }

func (c *closeVec) Embed(string) ([]float32, error) { return make([]float32, 4), nil }
func (c *closeVec) Close()                          { c.closed.Store(true) }

// TestShutdownReleasesVectorizer: 종료 시 vectorizer의 네이티브 자원(ORT
// 세션)이 해제되어야 한다. 해제 누락은 부트스트랩 반복 시 세션당 수백 MB가
// 새어 VM 전체를 OOM으로 몰고 간다.
func TestShutdownReleasesVectorizer(t *testing.T) {
	for _, tc := range []struct {
		name string
		stop func(*Engine)
	}{
		{"Close", func(e *Engine) { e.Close() }},
		{"Abandon", func(e *Engine) { e.Abandon() }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := New(filepath.Join(t.TempDir(), "vectors.bin"), nil, obs.Nop())
			v := &closeVec{}
			e.WarmupWith(v, "m", "q: ", "p: ")
			tc.stop(e)
			if !v.closed.Load() {
				t.Fatal("vectorizer must be closed on shutdown")
			}
			if got := e.Search("anything", 3); got != nil {
				t.Fatalf("search after shutdown must be inert, got %v", got)
			}
		})
	}
}

func TestDrainedTracksInFlightOps(t *testing.T) {
	e := New(filepath.Join(t.TempDir(), "vectors.bin"), nil, obs.Nop())
	defer e.Abandon()
	g := &gateVec{gate: make(chan struct{})}
	e.WarmupWith(g, "m", "q: ", "p: ")

	if !e.Drained() {
		t.Fatal("fresh engine must report drained")
	}
	e.Enqueue("node.a", "alpha")
	e.Enqueue("node.b", "beta")
	if e.Drained() {
		t.Fatal("queued ops must unset drained")
	}
	g.gate <- struct{}{} // release first embed; second still pending
	if e.Drained() {
		t.Fatal("one op still queued/in-flight")
	}
	g.gate <- struct{}{} // release second
	deadline := time.Now().Add(5 * time.Second)
	for !e.Drained() {
		if time.Now().After(deadline) {
			t.Fatal("never drained after all embeds completed")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if e.Dropped() != 0 {
		t.Fatalf("no backpressure expected, dropped=%d", e.Dropped())
	}
}

// TestNoDropUnderColdBurst: 워밍업 전 대량 버스트가 와도(콜드 부트스트랩)
// 백로그가 전부 담아 드랍이 0이어야 한다. 이전 구현은 유계 채널이라 큐 상한
// 초과분을 조용히 버려 벡터 인덱스를 반쯤 비웠다 (§7c 후속 ①).
func TestNoDropUnderColdBurst(t *testing.T) {
	e := New(filepath.Join(t.TempDir(), "vectors.bin"), nil, obs.Nop())
	const burst = 50000 // 옛 상한(16384)의 3배 — 유계 채널이라면 드랍 발생
	for i := 0; i < burst; i++ {
		e.Enqueue("n", "summary text")
	}
	if e.Dropped() != 0 {
		t.Fatalf("backlog must never drop, dropped=%d", e.Dropped())
	}
	// 워커는 웨이크 즉시 백로그 전체를 배치로 가져가 첫 op의 readyCh에서
	// 파킹한다 → QueueDepth는 0~burst로 요동. 의미 있는 불변식은 "아직 하나도
	// 적용되지 않아 드레인되지 않았다"이다.
	if e.Drained() {
		t.Fatal("un-warmed engine holds the full backlog, not drained")
	}

	// 워밍업 후 전부 임베딩되고 드레인된다 — 드랍 0 유지.
	v := &countVec{}
	e.WarmupWith(v, "m", "q: ", "p: ")
	deadline := time.Now().Add(10 * time.Second)
	for !e.Drained() {
		if time.Now().After(deadline) {
			t.Fatalf("backlog never drained (pending, calls=%d)", v.calls.Load())
		}
		time.Sleep(5 * time.Millisecond)
	}
	if e.Dropped() != 0 {
		t.Fatalf("still zero drops expected, dropped=%d", e.Dropped())
	}
	if got := v.calls.Load(); got != burst {
		t.Fatalf("embedded %d, want all %d (last-writer id is same, but every op runs)", got, burst)
	}
	e.Abandon()
}

// countVec counts Embed calls; unblocked (fast) so drain completes.
type countVec struct{ calls atomic.Int64 }

func (c *countVec) Embed(string) ([]float32, error) {
	c.calls.Add(1)
	return make([]float32, 4), nil
}
