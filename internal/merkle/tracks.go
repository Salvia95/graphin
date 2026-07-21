package merkle

import (
	"sort"

	"github.com/llls2542/graphin/internal/parse"
)

// FileDiff classifies each node of a freshly reparsed file (§2.1.2 2-Track):
//
//	OffsetOnly — subtree hash unchanged. Track A: refresh Start/End byte
//	             metadata (they shift when code above moves) and nothing else.
//	Changed    — new or content-changed. Track B: re-embed + re-extract edges.
//	Removed    — node IDs that existed before and are gone now.
type FileDiff struct {
	RelPath    string
	OffsetOnly []parse.Node
	Changed    []parse.Node
	Removed    []string
}

// Diff compares the recorded hashes for res.RelPath against a fresh parse.
func Diff(prev *Tree, res *parse.FileResult) FileDiff {
	var prevNodes map[string]string
	if prev != nil {
		prevNodes = prev.Files[res.RelPath].Nodes
	}
	d := FileDiff{RelPath: res.RelPath}
	seen := make(map[string]bool, len(res.Nodes))
	for _, n := range res.Nodes {
		seen[n.ID] = true
		if prevNodes != nil && prevNodes[n.ID] == Hex(n.Hash) {
			d.OffsetOnly = append(d.OffsetOnly, n)
		} else {
			d.Changed = append(d.Changed, n)
		}
	}
	for id := range prevNodes {
		if !seen[id] {
			d.Removed = append(d.Removed, id)
		}
	}
	sort.Strings(d.Removed)
	return d
}

// Embedder receives Track-B embedding work. The ONNX engine implements this
// in Phase 4; tests use counters.
type Embedder interface {
	EmbedNode(n parse.Node)
}

// EdgeSink receives Track-B graph work. The graph engine implements this in
// Phase 3; tests use counters.
type EdgeSink interface {
	UpdateEdges(n parse.Node)
	RemoveNode(id string)
}

// Apply routes Track-B work to the consumers. OffsetOnly nodes bypass both —
// that skip is the entire point of the pipeline (§7-P2-②).
func Apply(d FileDiff, emb Embedder, edges EdgeSink) {
	for _, n := range d.Changed {
		if emb != nil {
			emb.EmbedNode(n)
		}
		if edges != nil {
			edges.UpdateEdges(n)
		}
	}
	for _, id := range d.Removed {
		if edges != nil {
			edges.RemoveNode(id)
		}
	}
}
