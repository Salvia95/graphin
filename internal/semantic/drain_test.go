package semantic

// Phase 7c 후속 ①: Drained()는 큐 잔량뿐 아니라 in-flight op까지 반영해야
// 한다 — QueueDepth()==0만으로는 마지막 임베딩이 끝났음을 보장하지 못한다.

import (
	"path/filepath"
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

func TestDroppedCountsBackpressure(t *testing.T) {
	e := New(filepath.Join(t.TempDir(), "vectors.bin"), nil, obs.Nop())
	defer e.Abandon()
	// 워밍업 없음: worker는 첫 op에서 readyCh를 기다리며 파킹된다 → 큐가
	// cap까지 차면 이후 Enqueue는 드랍된다.
	total := int64(queueCap + 10)
	for i := int64(0); i < total; i++ {
		e.Enqueue("n", "s")
	}
	// worker가 첫 op를 이미 집었으면 흡수량 = cap+1 → 드랍 9, 아직이면
	// cap → 드랍 10.
	if d := e.Dropped(); d < 9 || d > 10 {
		t.Fatalf("drop count off: dropped=%d cap=%d total=%d", d, queueCap, total)
	}
	if e.Drained() {
		t.Fatal("parked queue must not report drained")
	}
}
