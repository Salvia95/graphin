// Package tools binds the five §3 MCP tools to the workspace engines.
// Handlers gain real behavior phase by phase; anything not yet backed by an
// index reports a spec error code instead of failing the protocol.
package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Salvia95/graphin/internal/bench"
	"github.com/Salvia95/graphin/internal/graph"
	"github.com/Salvia95/graphin/internal/mcp"
	"github.com/Salvia95/graphin/internal/workspace"
)

// Register wires all tools into the registry.
func Register(reg *mcp.Registry, ws *workspace.Workspace) {
	reg.Register(&mcp.Tool{
		Name:        "bootstrap_workspace",
		Description: "Index the workspace and start the file watcher. Returns immediately; progress is embedded in every subsequent tool response. Also detects RDB schema snapshots (*.graphindb.json, see schema/graphindb.md): tables/views/functions and their foreign keys index as graph nodes. When database traces exist without a snapshot, the response carries a <hint> explaining how to generate one.",
		InputSchema: objSchema(map[string]any{
			"model_type": map[string]any{
				"type": "string", "enum": []string{"english_optimal", "multilingual_cjk"},
				"description": "Embedding model family.",
			},
			"offline": map[string]any{
				"type": "boolean", "description": "Never download; use local runtime/ artifacts only.",
			},
		}, nil),
		Handler: bootstrapHandler(ws),
	})

	reg.Register(&mcp.Tool{
		Name:        "search_hybrid",
		Description: "Hybrid (exact/lexical/semantic) symbol search. Returns entry-point node IDs without code bodies.",
		InputSchema: objSchema(map[string]any{
			"query": map[string]any{"type": "string", "description": "Natural language or symbol query."},
			"top_k": map[string]any{"type": "integer", "default": 5, "maximum": 20, "description": "Max results."},
		}, []string{"query"}),
		Handler: searchHandler(ws),
	})

	reg.Register(&mcp.Tool{
		Name:        "explore_graph",
		Description: "Explore uses/used_by edges of a node with confidence scores. Paginated, 20 edges per page.",
		InputSchema: objSchema(map[string]any{
			"node_id":   map[string]any{"type": "string", "description": "Node ID from search_hybrid."},
			"direction": map[string]any{"type": "string", "enum": []string{"uses", "used_by", "both"}, "default": "both"},
			"cursor":    map[string]any{"type": "string", "description": "Pagination cursor from a previous page."},
			"min_confidence": map[string]any{
				"type": "number", "default": 0.85, "description": "Exclude edges below this confidence. Default 0.85 drops the 0.80 heuristic tier (higher precision, ~17% fewer lines, <2%p recall cost per docs/eval); lower to 0.75 for max recall.",
			},
		}, []string{"node_id"}),
		Handler: exploreHandler(ws),
	})

	reg.Register(&mcp.Tool{
		Name:        "read_code",
		Description: "Read the exact source slice of one node.",
		InputSchema: objSchema(map[string]any{
			"node_id": map[string]any{"type": "string", "description": "Node ID to read."},
		}, []string{"node_id"}),
		Handler: readCodeHandler(ws),
	})

	reg.Register(&mcp.Tool{
		Name:        "run_local_benchmark",
		Description: "Compare grep-style context bytes vs graphin navigation bytes for a query, as a markdown report.",
		InputSchema: objSchema(map[string]any{
			"target_query":  map[string]any{"type": "string"},
			"expected_node": map[string]any{"type": "string"},
		}, []string{"target_query", "expected_node"}),
		Handler: benchmarkHandler(ws),
	})
}

func objSchema(props map[string]any, required []string) map[string]any {
	s := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

func bootstrapHandler(ws *workspace.Workspace) mcp.ToolHandler {
	type args struct {
		ModelType string `json:"model_type"`
		Offline   bool   `json:"offline"`
	}
	return func(ctx context.Context, raw json.RawMessage) (string, bool) {
		var a args
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &a); err != nil {
				st := ws.FSM.Status()
				return mcp.ErrorXML(mcp.ErrInternal, "invalid arguments: "+err.Error(), &st), true
			}
		}
		st, err := ws.Bootstrap(ctx, a.ModelType, a.Offline)
		if err != nil {
			if errors.Is(err, workspace.ErrLockHeld) {
				return mcp.ErrorXML(mcp.ErrLockHeld, "another process is indexing this workspace", &st), true
			}
			return mcp.ErrorXML(mcp.ErrInternal, err.Error(), &st), true
		}
		return st.XML(), false
	}
}

// notBootstrapped renders the shared guard response.
func notBootstrapped(ws *workspace.Workspace) string {
	st := ws.FSM.Status()
	return mcp.ErrorXML(mcp.ErrNotBootstrapped, "call bootstrap_workspace first", &st)
}

func searchHandler(ws *workspace.Workspace) mcp.ToolHandler {
	type args struct {
		Query string `json:"query"`
		TopK  int    `json:"top_k"`
	}
	return func(_ context.Context, raw json.RawMessage) (string, bool) {
		if !ws.Bootstrapped() {
			return notBootstrapped(ws), true
		}
		var a args
		if err := json.Unmarshal(raw, &a); err != nil || strings.TrimSpace(a.Query) == "" {
			st := ws.FSM.Status()
			return mcp.ErrorXML(mcp.ErrInternal, "query is required", &st), true
		}
		if a.TopK <= 0 {
			a.TopK = 5
		}
		if a.TopK > 20 {
			a.TopK = 20
		}

		results := ws.Router.Search(a.Query, a.TopK)
		semReady := ws.Router.SemanticReady()

		var sb strings.Builder
		st := ws.FSM.Status()
		if st.State != "ready" {
			sb.WriteString(st.XML())
			sb.WriteString("\n")
		}
		fmt.Fprintf(&sb, `<results semantic_ready="%t">`, semReady)
		if msg := ws.SemUnavailable(); msg != "" {
			fmt.Fprintf(&sb, "\n  <model_status code=%q hint=\"semantic search unavailable; lexical fallback active\" />",
				mcp.ErrModelUnavailable)
		}
		for _, r := range results {
			display := ws.DisplayName(r.NodeID)
			if display == "" {
				display = ws.Sym.SimpleName(r.NodeID)
			}
			sb.WriteString("\n  ")
			fmt.Fprintf(&sb, `<node id="%s" display_name="%s" rank="%d" match_type="%s" />`,
				mcp.EscapeAttr(r.NodeID), mcp.EscapeAttr(display), r.Rank, r.Match)
		}
		sb.WriteString("\n</results>")
		return sb.String(), false
	}
}

func exploreHandler(ws *workspace.Workspace) mcp.ToolHandler {
	type args struct {
		NodeID        string   `json:"node_id"`
		Direction     string   `json:"direction"`
		Cursor        string   `json:"cursor"`
		MinConfidence *float64 `json:"min_confidence"`
	}
	return func(_ context.Context, raw json.RawMessage) (string, bool) {
		if !ws.Bootstrapped() {
			return notBootstrapped(ws), true
		}
		var a args
		_ = json.Unmarshal(raw, &a)
		switch a.Direction {
		case "uses", "used_by", "both":
		default:
			a.Direction = "both"
		}
		minConf := float32(0.85) // 기본값: docs/eval H2(2026-07-25) — 0.80 티어 절단
		if a.MinConfidence != nil {
			minConf = float32(*a.MinConfidence)
		}

		page, err := ws.Explore(a.NodeID, a.Direction, a.Cursor, minConf)
		if err != nil {
			st := ws.FSM.Status()
			if errors.Is(err, workspace.ErrNodeNotFound) {
				return mcp.ErrorXML(mcp.ErrNodeNotFound, "unknown node: "+a.NodeID, &st), true
			}
			return mcp.ErrorXML(mcp.ErrInternal, err.Error(), &st), true
		}

		var sb strings.Builder
		if st := ws.FSM.Status(); st.State != "ready" {
			sb.WriteString(st.XML())
			sb.WriteString("\n")
		}
		fmt.Fprintf(&sb, `<graph_context target="%s">`, mcp.EscapeAttr(page.Target))
		writeEdges := func(tag string, edges []workspaceEdge) {
			if len(edges) == 0 {
				return
			}
			fmt.Fprintf(&sb, "\n  <%s>", tag)
			for _, e := range edges {
				fmt.Fprintf(&sb, "\n    <node id=\"%s\" type=\"%s\" confidence=\"%.2f\" />",
					mcp.EscapeAttr(e.NodeID), e.Type, e.Confidence)
			}
			fmt.Fprintf(&sb, "\n  </%s>", tag)
		}
		if a.Direction != "used_by" {
			writeEdges("uses", toEdges(page.Uses))
		}
		if a.Direction != "uses" {
			writeEdges("used_by", toEdges(page.UsedBy))
		}
		if page.HasMore {
			sb.WriteString("\n  <has_more>true</has_more>")
			fmt.Fprintf(&sb, "\n  <next_cursor>%s</next_cursor>", mcp.EscapeText(page.NextCursor))
		}
		sb.WriteString("\n</graph_context>")
		return sb.String(), false
	}
}

type workspaceEdge struct {
	NodeID     string
	Type       string
	Confidence float32
}

func toEdges(in []graph.EdgeOut) []workspaceEdge {
	out := make([]workspaceEdge, len(in))
	for i, e := range in {
		out[i] = workspaceEdge{NodeID: e.NodeID, Type: e.Type, Confidence: e.Confidence}
	}
	return out
}

func readCodeHandler(ws *workspace.Workspace) mcp.ToolHandler {
	type args struct {
		NodeID string `json:"node_id"`
	}
	return func(_ context.Context, raw json.RawMessage) (string, bool) {
		if !ws.Bootstrapped() {
			return notBootstrapped(ws), true
		}
		var a args
		_ = json.Unmarshal(raw, &a)

		cb, err := ws.ReadCode(a.NodeID)
		if err != nil {
			st := ws.FSM.Status()
			switch {
			case errors.Is(err, workspace.ErrNodeGone):
				return mcp.ErrorXML(mcp.ErrNodeGone, "node vanished after reparse: "+a.NodeID, &st), true
			case errors.Is(err, workspace.ErrNodeNotFound):
				return mcp.ErrorXML(mcp.ErrNodeNotFound, "unknown node: "+a.NodeID, &st), true
			default:
				return mcp.ErrorXML(mcp.ErrInternal, err.Error(), &st), true
			}
		}

		var sb strings.Builder
		if st := ws.FSM.Status(); st.State != "ready" {
			sb.WriteString(st.XML())
			sb.WriteString("\n")
		}
		fmt.Fprintf(&sb, `<code_block id="%s" file="%s" lines="%d-%d" reparsed="%t"`,
			mcp.EscapeAttr(cb.ID), mcp.EscapeAttr(cb.RelPath), cb.StartLine, cb.EndLine, cb.Reparsed)
		if cb.Partial {
			sb.WriteString(` partial="true"`)
		}
		sb.WriteString(">\n")
		mcp.WriteCDATA(&sb, cb.Code)
		sb.WriteString("\n</code_block>")
		return sb.String(), false
	}
}

// benchmarkHandler runs the §3.5 three-scenario comparison. Scenario C
// measures the true tool responses (post-truncation) by invoking the sibling
// handlers, so the report is falsifiable against real payloads.
func benchmarkHandler(ws *workspace.Workspace) mcp.ToolHandler {
	type args struct {
		TargetQuery  string `json:"target_query"`
		ExpectedNode string `json:"expected_node"`
	}
	return func(ctx context.Context, raw json.RawMessage) (string, bool) {
		if !ws.Bootstrapped() {
			return notBootstrapped(ws), true
		}
		var a args
		if err := json.Unmarshal(raw, &a); err != nil || strings.TrimSpace(a.TargetQuery) == "" {
			st := ws.FSM.Status()
			return mcp.ErrorXML(mcp.ErrInternal, "target_query is required", &st), true
		}

		// Scenario 1: grep full-file bytes.
		t0 := time.Now()
		fullBytes, files, err := bench.GrepFull(ws.Root, a.TargetQuery)
		if err != nil {
			st := ws.FSM.Status()
			return mcp.ErrorXML(mcp.ErrInternal, err.Error(), &st), true
		}
		fullMs := time.Since(t0).Milliseconds()

		// Scenario 2: grep -C 20 context bytes.
		t1 := time.Now()
		ctxBytes, err := bench.GrepContext(ws.Root, a.TargetQuery, 20)
		if err != nil {
			st := ws.FSM.Status()
			return mcp.ErrorXML(mcp.ErrInternal, err.Error(), &st), true
		}
		ctxMs := time.Since(t1).Milliseconds()

		// Scenario 3: graphin roundtrip — measure the actual tool payloads.
		t2 := time.Now()
		results := ws.Router.Search(a.TargetQuery, 5)
		searchXML, _ := searchHandler(ws)(ctx, mustJSON(map[string]any{
			"query": a.TargetQuery, "top_k": 5,
		}))
		hitRank := 0
		for _, r := range results {
			if r.NodeID == a.ExpectedNode {
				hitRank = r.Rank
			}
		}
		target := a.ExpectedNode
		if hitRank == 0 && len(results) > 0 {
			target = results[0].NodeID
		}
		exploreXML, _ := exploreHandler(ws)(ctx, mustJSON(map[string]any{
			"node_id": target, "direction": "both",
		}))
		readXML, _ := readCodeHandler(ws)(ctx, mustJSON(map[string]any{
			"node_id": target,
		}))
		navBytes := len(mcp.Truncate(searchXML)) + len(mcp.Truncate(exploreXML)) + len(mcp.Truncate(readXML))
		navMs := time.Since(t2).Milliseconds()

		md := buildBenchMarkdown(a, files, fullBytes, fullMs, ctxBytes, ctxMs, navBytes, navMs, hitRank, ws)

		var sb strings.Builder
		if st := ws.FSM.Status(); st.State != "ready" {
			sb.WriteString(st.XML())
			sb.WriteString("\n")
		}
		fmt.Fprintf(&sb,
			"<benchmark_report grep_full_bytes=\"%d\" grep_c20_bytes=\"%d\" graphin_bytes=\"%d\" hit=\"%t\" hit_rank=\"%d\">\n",
			fullBytes, ctxBytes, navBytes, hitRank > 0, hitRank)
		mcp.WriteCDATA(&sb, md)
		sb.WriteString("\n</benchmark_report>")
		return sb.String(), false
	}
}

func buildBenchMarkdown(a struct {
	TargetQuery  string `json:"target_query"`
	ExpectedNode string `json:"expected_node"`
}, files, fullBytes int, fullMs int64, ctxBytes int, ctxMs int64, navBytes int, navMs int64, hitRank int, ws *workspace.Workspace) string {
	saving := func(base int) string {
		if base <= 0 {
			return "n/a"
		}
		return fmt.Sprintf("%.1f%%", 100*(1-float64(navBytes)/float64(base)))
	}
	var md strings.Builder
	md.WriteString("## graphin local benchmark\n\n")
	fmt.Fprintf(&md, "- query: `%s`\n- expected node: `%s`\n", a.TargetQuery, a.ExpectedNode)
	if hitRank > 0 {
		fmt.Fprintf(&md, "- **expected node HIT at rank %d** (top_k=5)\n", hitRank)
	} else {
		md.WriteString("- **expected node MISS** in top_k=5 — 리포트 반증 가능성 확보를 위해 명기\n")
	}
	md.WriteString("\n시뮬레이션 가설: ① Grep Full = 매칭 파일 전체 바이트, ② Grep -C 20 = 매치 ±20라인, ③ graphin = search+explore+read 실제 응답 바이트.\n\n")
	md.WriteString("| scenario | bytes | est. tokens (÷4) | ms | savings vs scenario |\n")
	md.WriteString("|---|---:|---:|---:|---|\n")
	fmt.Fprintf(&md, "| Grep Full (%d files) | %d | %d | %d | baseline |\n", files, fullBytes, fullBytes/4, fullMs)
	fmt.Fprintf(&md, "| Grep -C 20 | %d | %d | %d | %s vs Full |\n", ctxBytes, ctxBytes/4, ctxMs, func() string {
		if fullBytes <= 0 {
			return "n/a"
		}
		return fmt.Sprintf("%.1f%%", 100*(1-float64(ctxBytes)/float64(fullBytes)))
	}())
	fmt.Fprintf(&md, "| graphin (search→explore→read) | %d | %d | %d | %s vs Full, %s vs -C20 |\n",
		navBytes, navBytes/4, navMs, saving(fullBytes), saving(ctxBytes))

	// §8: RRF k sweep, meaningful only once the vector engine is warm.
	if ws.Router.SemanticReady() && a.ExpectedNode != "" {
		md.WriteString("\n### RRF k sweep (expected-node rank)\n\n| k | rank |\n|---:|---:|\n")
		for _, k := range []int{20, 60, 100} {
			rank := 0
			for _, r := range ws.Router.SearchK(a.TargetQuery, 5, k) {
				if r.NodeID == a.ExpectedNode {
					rank = r.Rank
				}
			}
			if rank > 0 {
				fmt.Fprintf(&md, "| %d | %d |\n", k, rank)
			} else {
				fmt.Fprintf(&md, "| %d | miss |\n", k)
			}
		}
	}
	return md.String()
}

func mustJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}
