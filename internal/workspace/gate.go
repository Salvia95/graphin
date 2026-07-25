package workspace

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// gateMarker records a prior semantic node-count gate so a restart stays
// lexical-only without re-paying the embedding cost. Persisted in
// <root>/.graphin so it survives across sessions; cleared when the ceiling is
// raised above the recorded count.
type gateMarker struct {
	Nodes int `json:"nodes"`
	Max   int `json:"max"`
}

func gateMarkerPath(dir string) string { return filepath.Join(dir, "semantic-gated.json") }

func readGateMarker(dir string) (gateMarker, bool) {
	b, err := os.ReadFile(gateMarkerPath(dir))
	if err != nil {
		return gateMarker{}, false
	}
	var m gateMarker
	if json.Unmarshal(b, &m) != nil || m.Nodes <= 0 {
		return gateMarker{}, false
	}
	return m, true
}

func writeGateMarker(dir string, m gateMarker) {
	if b, err := json.Marshal(m); err == nil {
		_ = os.WriteFile(gateMarkerPath(dir), b, 0o644)
	}
}

func removeGateMarker(dir string) { _ = os.Remove(gateMarkerPath(dir)) }

func gateNote(nodes, max int) string {
	return fmt.Sprintf("nodes %d exceed semantic_max_nodes %d; lexical-only "+
		"(raise --semantic-max-nodes to enable semantic)", nodes, max)
}

// gateSemanticOff disables semantic search when the live node count crosses
// the configured ceiling mid-scan. Called from applyFileResult, which holds
// w.indexMu, so the w.semSink write races nothing. Idempotent: the first call
// wins, later files skip. The partial vector index built so far is never
// queried (engine Disable → Ready() false → router lexical-only) and a marker
// makes the next bootstrap skip semantic before any embedding.
func (w *Workspace) gateSemanticOff(nodes int) {
	if !w.semGated.CompareAndSwap(false, true) {
		return
	}
	note := gateNote(nodes, w.semMaxNodes)
	w.semErr.Store(&note)
	w.semSink = nil // stop further enqueues (w.indexMu held by caller)
	if w.sem != nil {
		w.sem.Disable() // Ready()→false: router + Search fall back to lexical
	}
	writeGateMarker(w.Dir, gateMarker{Nodes: nodes, Max: w.semMaxNodes})
	w.Log.Event("semantic_gated", map[string]any{"nodes": nodes, "max": w.semMaxNodes})
}
