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

// Session is one indexed task repo held open across sweep configs. Indexing
// is the expensive part and is identical for every config — only the
// search/explore parameters differ — so a sweep bootstraps once and replays
// in memory. Re-bootstrapping per config also multiplied the ORT session
// footprint, which is what drove the eval host out of memory.
type Session struct {
	w     *workspace.Workspace
	root  string
	keep  bool
	Stats ExploreStats
}

// Open indexes one task repo and waits until it is queryable: lexical ready,
// and in semantic mode the model warm AND the embedding queue drained
// (SemanticReady alone would measure a near-empty vector index).
func Open(ctx context.Context, repoDir string, o Options) (*Session, error) {
	ortLib := o.OrtLib
	if !o.Semantic && ortLib == "" {
		// lexical-only: poison the ORT path so no provisioning/download runs.
		ortLib = filepath.Join(os.TempDir(), "graphin-eval-no-ort")
	}
	s := &Session{
		w: workspace.New(workspace.Config{
			Root:      repoDir,
			ModelType: o.ModelType,
			Offline:   o.Offline,
			ModelDir:  o.ModelDir,
			OrtLib:    ortLib,
			Log:       obs.Nop(),
		}),
		root: repoDir,
		keep: o.KeepIndex,
	}

	if _, err := s.w.Bootstrap(ctx, o.ModelType, o.Offline); err != nil {
		s.Close()
		return nil, err
	}
	deadline := time.Now().Add(o.WaitTimeout)
	for {
		st := s.w.FSM.Status()
		if st.LexicalReady && (!o.Semantic || (st.SemanticReady && s.w.SemanticDrained())) {
			break
		}
		var err error
		switch {
		case o.Semantic && s.w.SemUnavailable() != "":
			err = fmt.Errorf("semantic engine unavailable: %s", s.w.SemUnavailable())
		case time.Now().After(deadline) && o.Semantic && st.LexicalReady:
			err = fmt.Errorf("semantic warmup/drain did not finish within %s (ready=%t drained=%t)",
				o.WaitTimeout, st.SemanticReady, s.w.SemanticDrained())
		case time.Now().After(deadline):
			err = fmt.Errorf("indexing did not finish within %s", o.WaitTimeout)
		}
		if err != nil {
			s.Close()
			return nil, err
		}
		select {
		case <-ctx.Done():
			s.Close()
			return nil, ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
	s.Stats.EmbedDropped = s.w.SemanticEmbedDropped()
	return s, nil
}

// Explore replays the policy against the open index: derived queries →
// SearchK seeds → one-hop explore expansion → node spans as ranked regions.
// Every step is deterministic (engine tie-breaks are lexicographic), so the
// same session and options always yield the same regions.
func (s *Session) Explore(issue string, o Options) []Region {
	return collectRegions(s.w, issue, o)
}

// Close releases the workspace and, unless KeepIndex was set, the on-disk
// index it built.
func (s *Session) Close() {
	s.w.Close()
	if !s.keep {
		os.RemoveAll(filepath.Join(s.root, workspace.DataDirName))
	}
}

// ExploreRepo is the single-config convenience wrapper around Open+Explore.
func ExploreRepo(ctx context.Context, repoDir, issue string, o Options) ([]Region, ExploreStats, error) {
	s, err := Open(ctx, repoDir, o)
	if err != nil {
		return nil, ExploreStats{}, err
	}
	defer s.Close()
	return s.Explore(issue, o), s.Stats, nil
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
