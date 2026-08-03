// admin.go — admin 페이지가 쓰는 읽기 전용 파사드. 엔진 포인터를 밖으로
// 내보내지 않고 프록시로 감싼다(sweexplore 선례의 캡슐화 유지). 그래프
// 프록시는 부트스트랩 전이면 zero 값을 반환한다.
package workspace

import (
	"github.com/Salvia95/graphin/internal/graph"
	"github.com/Salvia95/graphin/internal/semantic"
)

// Meta exposes one node's location metadata (read_code's backing record).
func (w *Workspace) Meta(id string) (NodeMeta, bool) { return w.nodeMeta(id) }

// engineForRead returns the graph engine once Bootstrap installed it; w.graph
// is written under w.mu, so reads take the same lock.
func (w *Workspace) engineForRead() *graph.Engine {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.bootstrapped {
		return nil
	}
	return w.graph
}

// GraphStats aggregates node/edge counts per shard (zero before bootstrap).
func (w *Workspace) GraphStats() graph.Stats {
	e := w.engineForRead()
	if e == nil {
		return graph.Stats{}
	}
	return e.Stats()
}

// GraphInfo returns one node's read-path snapshot including its edges.
func (w *Workspace) GraphInfo(id string) (graph.NodeInfo, bool) {
	e := w.engineForRead()
	if e == nil {
		return graph.NodeInfo{}, false
	}
	return e.Info(id)
}

// GraphForEach visits every visible node (no-op before bootstrap).
func (w *Workspace) GraphForEach(fn func(graph.NodeInfo) bool) {
	if e := w.engineForRead(); e != nil {
		e.ForEachNode(fn)
	}
}

// GraphDangling lists edges whose target is not on the read path.
func (w *Workspace) GraphDangling(max int) ([]graph.Dangling, int) {
	e := w.engineForRead()
	if e == nil {
		return nil, 0
	}
	return e.DanglingEdges(max)
}

// GraphReverseStats summarizes the used_by index and its delta log.
func (w *Workspace) GraphReverseStats() graph.ReverseStats {
	e := w.engineForRead()
	if e == nil {
		return graph.ReverseStats{}
	}
	return e.ReverseStats()
}

// SemanticHeader returns the vectors.bin header restored at bootstrap (nil
// when semantic never loaded, e.g. gated or not bootstrapped).
func (w *Workspace) SemanticHeader() *semantic.Header {
	w.mu.Lock()
	sem := w.sem
	w.mu.Unlock()
	if sem == nil {
		return nil
	}
	return sem.LoadedHeader()
}

// SemanticQueue reports the live embedding backlog (drained=true when
// semantic is absent).
func (w *Workspace) SemanticQueue() (pending int64, depth int, drained bool) {
	w.mu.Lock()
	sem := w.sem
	w.mu.Unlock()
	if sem == nil {
		return 0, 0, true
	}
	return sem.Pending(), sem.QueueDepth(), sem.Drained()
}

// ConfigView is the effective startup configuration, for display only.
type ConfigView struct {
	Root             string
	ModelType        string
	ModelDir         string
	OrtLib           string
	Workers          int
	SemanticMaxNodes int
	Offline          bool
}

// EffectiveConfig snapshots the startup flags (immutable after New).
func (w *Workspace) EffectiveConfig() ConfigView {
	return ConfigView{
		Root:             w.cfg.Root,
		ModelType:        w.cfg.ModelType,
		ModelDir:         w.cfg.ModelDir,
		OrtLib:           w.cfg.OrtLib,
		Workers:          w.cfg.Workers,
		SemanticMaxNodes: w.cfg.SemanticMaxNodes,
		Offline:          w.cfg.Offline,
	}
}

// GateInfo reads the persisted semantic node-count gate marker.
func (w *Workspace) GateInfo() (nodes, max int, gated bool) {
	m, ok := readGateMarker(w.Dir)
	if !ok {
		return 0, 0, false
	}
	return m.Nodes, m.Max, true
}
