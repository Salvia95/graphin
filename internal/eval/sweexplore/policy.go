package sweexplore

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Salvia95/graphin/internal/nodeid"
	"github.com/Salvia95/graphin/internal/obs"
	"github.com/Salvia95/graphin/internal/workspace"
)

// Options parameterizes one policy configuration — exactly the §3.3 sweep
// axes plus mechanical caps.
type Options struct {
	TopK    int     // search_hybrid top_k per query
	RRFK    int     // RRF constant (SearchK)
	MinConf float32 // explore_graph min_confidence
	Queries int     // derived queries per issue

	MaxRegions     int // ranked list cap (scorer applies line budgets)
	MaxRegionLines int // skip nodes spanning more lines (whole-file noise)

	Semantic  bool // wait for the vector engine (hybrid mode)
	ModelType string
	ModelDir  string
	OrtLib    string
	Offline   bool
	KeepIndex bool // leave <repo>/.graphin behind for inspection

	WaitTimeout time.Duration
}

// Defaults returns the shipping tool defaults (top_k 5 / RRF 60 / min_conf
// 0.5) — the H2 null hypothesis configuration.
func Defaults() Options {
	return Options{
		TopK: 5, RRFK: 60, MinConf: 0.5, Queries: 3,
		MaxRegions: 100, MaxRegionLines: 400,
		WaitTimeout: 10 * time.Minute,
	}
}

// ExploreStats carries per-task measurement caveats.
type ExploreStats struct {
	// EmbedDropped: 임베딩 큐 백프레셔 드랍 수 — 0이 아니면 벡터 인덱스가
	// 그만큼 비어 있는 채 측정된 것이다 (semantic 모드 커버리지 캐비앳).
	EmbedDropped int64
}

// ExploreRepo runs the graphin policy over one task repo: bootstrap →
// derived queries → SearchK seeds → one-hop explore expansion → node spans
// as ranked regions. Every step is deterministic (engine tie-breaks are
// lexicographic), so identical inputs yield identical submissions. Semantic
// mode additionally waits for the embedding queue to drain — SemanticReady
// alone would measure a near-empty vector index.
func ExploreRepo(ctx context.Context, repoDir, issue string, o Options) ([]Region, ExploreStats, error) {
	ortLib := o.OrtLib
	if !o.Semantic && ortLib == "" {
		// lexical-only: poison the ORT path so no provisioning/download runs.
		ortLib = filepath.Join(os.TempDir(), "graphin-eval-no-ort")
	}
	w := workspace.New(workspace.Config{
		Root:      repoDir,
		ModelType: o.ModelType,
		Offline:   o.Offline,
		ModelDir:  o.ModelDir,
		OrtLib:    ortLib,
		Log:       obs.Nop(),
	})
	defer w.Close()
	if !o.KeepIndex {
		defer os.RemoveAll(filepath.Join(repoDir, workspace.DataDirName))
	}

	var stats ExploreStats
	if _, err := w.Bootstrap(ctx, o.ModelType, o.Offline); err != nil {
		return nil, stats, err
	}
	deadline := time.Now().Add(o.WaitTimeout)
	for {
		st := w.FSM.Status()
		if st.LexicalReady && (!o.Semantic || (st.SemanticReady && w.SemanticDrained())) {
			break
		}
		if o.Semantic {
			if msg := w.SemUnavailable(); msg != "" {
				return nil, stats, fmt.Errorf("semantic engine unavailable: %s", msg)
			}
		}
		if time.Now().After(deadline) {
			if o.Semantic && st.LexicalReady {
				return nil, stats, fmt.Errorf("semantic warmup/drain did not finish within %s (ready=%t drained=%t)",
					o.WaitTimeout, st.SemanticReady, w.SemanticDrained())
			}
			return nil, stats, fmt.Errorf("indexing did not finish within %s", o.WaitTimeout)
		}
		select {
		case <-ctx.Done():
			return nil, stats, ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
	stats.EmbedDropped = w.SemanticEmbedDropped()

	return collectRegions(w, issue, o), stats, nil
}

func collectRegions(w *workspace.Workspace, issue string, o Options) []Region {
	var order []string
	seen := map[string]bool{}
	visit := func(id string) {
		if !seen[id] {
			seen[id] = true
			order = append(order, id)
		}
	}

	for _, q := range DeriveQueries(issue, o.Queries) {
		for _, r := range w.Router.SearchK(q, o.TopK, o.RRFK) {
			visit(r.NodeID)
		}
	}
	// one-hop expansion, single page per seed (20 edges, confidence-sorted):
	// CodeCompass §0 — bounded depth beats exhaustive graphs.
	for _, id := range append([]string(nil), order...) {
		p, err := w.Explore(id, "both", "", o.MinConf)
		if err != nil {
			continue
		}
		for _, e := range p.Uses {
			visit(e.NodeID)
		}
		for _, e := range p.UsedBy {
			visit(e.NodeID)
		}
	}

	var regions []Region
	dedup := map[Region]bool{}
	for _, id := range order {
		if len(regions) >= o.MaxRegions {
			break
		}
		if w.NodeKind(id) == nodeid.KindFile {
			continue // whole-file text nodes flood line budgets
		}
		cb, err := w.ReadCode(id)
		if err != nil {
			continue
		}
		r := Region{Path: cb.RelPath, Start: cb.StartLine, End: cb.EndLine}
		if o.MaxRegionLines > 0 && r.End-r.Start+1 > o.MaxRegionLines {
			continue
		}
		if !dedup[r] {
			dedup[r] = true
			regions = append(regions, r)
		}
	}
	return regions
}
