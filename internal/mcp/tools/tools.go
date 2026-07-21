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

	"github.com/llls2542/graphin/internal/mcp"
	"github.com/llls2542/graphin/internal/workspace"
)

// Register wires all tools into the registry.
func Register(reg *mcp.Registry, ws *workspace.Workspace) {
	reg.Register(&mcp.Tool{
		Name:        "bootstrap_workspace",
		Description: "Index the workspace and start the file watcher. Returns immediately; progress is embedded in every subsequent tool response.",
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
				"type": "number", "default": 0.5, "description": "Exclude edges below this confidence.",
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
		NodeID string `json:"node_id"`
	}
	return func(_ context.Context, raw json.RawMessage) (string, bool) {
		if !ws.Bootstrapped() {
			return notBootstrapped(ws), true
		}
		var a args
		_ = json.Unmarshal(raw, &a)
		// The graph engine arrives in Phase 3; until then every node is unknown.
		st := ws.FSM.Status()
		return mcp.ErrorXML(mcp.ErrNodeNotFound, "unknown node: "+a.NodeID, &st), true
	}
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

func benchmarkHandler(ws *workspace.Workspace) mcp.ToolHandler {
	return func(_ context.Context, _ json.RawMessage) (string, bool) {
		if !ws.Bootstrapped() {
			return notBootstrapped(ws), true
		}
		st := ws.FSM.Status()
		return mcp.ErrorXML(mcp.ErrInternal, "benchmark is implemented in Phase 5", &st), true
	}
}
