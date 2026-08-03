package graph

import (
	"sort"
	"strings"

	"github.com/Salvia95/graphin/internal/graph/fbsgen"
)

// 진단용 읽기 API (admin 페이지): 열거·통계·dangling 검출. readUses와 같은
// mmap 규율을 따른다 — RLock으로 핸들 스냅샷, RUnlock 전 Acquire, 문자열은
// 전부 복사, 순회가 끝나면 Release.

// NodeInfo is a copied-out snapshot of one node on the read path.
type NodeInfo struct {
	ID          string
	DisplayName string
	Kind        string
	FilePath    string
	Pkg         string
	Start, End  uint32
	Partial     bool
	Uses        []Edge
}

// pinnedShard pairs a shard handle with its package key while the mapping is
// pinned by Acquire.
type pinnedShard struct {
	pkg string
	h   *shardHandle
}

// acquireShards snapshots the current shard set in deterministic package
// order; every returned mapping stays valid across concurrent shard swaps
// until releaseShards.
func (e *Engine) acquireShards() []pinnedShard {
	e.mu.RLock()
	ps := make([]pinnedShard, 0, len(e.shards))
	for pkg, h := range e.shards {
		h.ref.Acquire()
		ps = append(ps, pinnedShard{pkg: pkg, h: h})
	}
	e.mu.RUnlock()
	sort.Slice(ps, func(i, j int) bool { return ps[i].pkg < ps[j].pkg })
	return ps
}

func releaseShards(ps []pinnedShard) {
	for _, p := range ps {
		p.h.ref.Release()
	}
}

// copyNode copies every field of one FlatBuffers node out of the mmap.
func copyNode(node *fbsgen.Node, pkg string, edge *fbsgen.Edge) NodeInfo {
	info := NodeInfo{
		ID:          string(node.Id()),
		DisplayName: string(node.DisplayName()),
		Kind:        string(node.Kind()),
		FilePath:    string(node.FilePath()),
		Pkg:         pkg,
		Start:       node.StartByte(),
		End:         node.EndByte(),
		Partial:     node.Partial(),
	}
	if n := node.UsesLength(); n > 0 {
		info.Uses = make([]Edge, 0, n)
		for j := 0; j < n; j++ {
			if node.Uses(edge, j) {
				info.Uses = append(info.Uses, Edge{
					TargetID:   string(edge.TargetId()),
					Type:       edge.Type(),
					Confidence: edge.Confidence(),
				})
			}
		}
	}
	return info
}

// ForEachNode visits every node currently visible on the read path in
// deterministic order (package, then shard vector order). fn returning false
// stops the walk. Nodes applied but not yet flushed are not visible — the
// same contract as Explore.
func (e *Engine) ForEachNode(fn func(NodeInfo) bool) {
	ps := e.acquireShards()
	defer releaseShards(ps)
	var node fbsgen.Node
	var edge fbsgen.Edge
	for _, p := range ps {
		sh := fbsgen.GetRootAsShard(p.h.ref.Data, 0)
		for i := 0; i < sh.NodesLength(); i++ {
			if !sh.Nodes(&node, i) {
				continue
			}
			if !fn(copyNode(&node, p.pkg, &edge)) {
				return
			}
		}
	}
}

// Info returns the copied-out snapshot of one visible node.
func (e *Engine) Info(id string) (NodeInfo, bool) {
	e.mu.RLock()
	loc, ok := e.nodeLoc[id]
	if !ok {
		e.mu.RUnlock()
		return NodeInfo{}, false
	}
	h := e.shards[loc.pkg]
	if h == nil {
		e.mu.RUnlock()
		return NodeInfo{}, false
	}
	h.ref.Acquire()
	e.mu.RUnlock()
	defer h.ref.Release()

	sh := fbsgen.GetRootAsShard(h.ref.Data, 0)
	var node fbsgen.Node
	if !sh.Nodes(&node, loc.idx) {
		return NodeInfo{}, false
	}
	var edge fbsgen.Edge
	return copyNode(&node, loc.pkg, &edge), true
}

// ShardStat summarizes one visible shard.
type ShardStat struct {
	Pkg   string
	Gen   uint64
	Nodes int
	Edges int
}

// Stats aggregates node/edge counts over every visible shard.
type Stats struct {
	Nodes  int
	Edges  int
	Shards []ShardStat
}

// Stats counts the read path; indexing in flight keeps the numbers moving
// until the next Flush lands.
func (e *Engine) Stats() Stats {
	ps := e.acquireShards()
	defer releaseShards(ps)
	var st Stats
	var node fbsgen.Node
	for _, p := range ps {
		sh := fbsgen.GetRootAsShard(p.h.ref.Data, 0)
		s := ShardStat{Pkg: p.pkg, Gen: p.h.gen, Nodes: sh.NodesLength()}
		for i := 0; i < sh.NodesLength(); i++ {
			if sh.Nodes(&node, i) {
				s.Edges += node.UsesLength()
			}
		}
		st.Nodes += s.Nodes
		st.Edges += s.Edges
		st.Shards = append(st.Shards, s)
	}
	return st
}

// Dangling is an edge whose target has no visible node. DB-domain targets can
// dangle by design — snapshots never grow stub nodes (§7a).
type Dangling struct {
	SourceID string
	Edge     Edge
	DBDomain bool
}

// DanglingEdges scans every visible edge and reports those whose target is
// not on the read path. At most max entries are returned (0 = count only);
// total always counts everything.
func (e *Engine) DanglingEdges(max int) (out []Dangling, total int) {
	e.ForEachNode(func(n NodeInfo) bool {
		for _, u := range n.Uses {
			if e.HasNode(u.TargetID) {
				continue
			}
			total++
			if len(out) < max {
				out = append(out, Dangling{
					SourceID: n.ID,
					Edge:     u,
					DBDomain: strings.HasPrefix(u.TargetID, "db."),
				})
			}
		}
		return true
	})
	return out, total
}

// ReverseStats summarizes the used_by index and its delta log.
type ReverseStats struct {
	Targets       int
	Edges         int
	LogRecords    int
	LogTombstones int
}

func (e *Engine) ReverseStats() ReverseStats {
	r := e.rev
	r.mu.RLock()
	defer r.mu.RUnlock()
	st := ReverseStats{Targets: len(r.m), LogRecords: r.records, LogTombstones: r.tombstones}
	for _, edges := range r.m {
		st.Edges += len(edges)
	}
	return st
}
