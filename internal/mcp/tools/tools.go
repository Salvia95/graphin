// Package tools binds the §3 MCP tools to the workspace engines.
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
	"github.com/Salvia95/graphin/internal/keyword"
	"github.com/Salvia95/graphin/internal/mcp"
	"github.com/Salvia95/graphin/internal/nodeid"
	"github.com/Salvia95/graphin/internal/search"
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
		Description: "Hybrid (exact/lexical/semantic) symbol search. Returns entry-point node IDs with the file and line each one starts at — enough to answer \"where is X\" without a second call — but no code bodies.",
		InputSchema: objSchema(map[string]any{
			"query": map[string]any{"type": "string", "description": "Natural language or symbol query."},
			"top_k": map[string]any{"type": "integer", "default": 5, "maximum": 20, "description": "Max results."},
			"target": map[string]any{
				"type": "string", "enum": []string{"code", "docs", "db"},
				"description": "Restrict results to one population. Omit to search everything. " +
					"Use \"code\" for a sentence-shaped question about implementation (\"how does X invalidate Y\") — " +
					"prose queries otherwise rank long markdown files above the functions they describe, because a whole " +
					"document is one node and matches prose better than code does. \"docs\" is markdown files and sections, " +
					"\"db\" is schema snapshot nodes. Symbol-shaped queries rarely need this.",
			},
		}, []string{"query"}),
		Handler: searchHandler(ws),
	})

	reg.Register(&mcp.Tool{
		Name: "search_keyword",
		Description: "Literal or regex file search over the same tree the index walks, ranked by match count. " +
			"Returns the matching lines with the node id that owns each one, so a string hit flows straight into " +
			"explore_graph and read_code. Reach for it when you know the exact text — an error message, a config " +
			"key, a magic constant, a string built at runtime that no parser can see as a call — and when a symbol " +
			"you are certain exists did not come back from search_hybrid. It also works before the index is warm, " +
			"which is when the ranked retrievers are least useful.",
		InputSchema: objSchema(map[string]any{
			"pattern": map[string]any{"type": "string", "description": "Text to find. Case-insensitive."},
			"regex": map[string]any{
				"type": "boolean", "default": false,
				"description": "Treat pattern as a regular expression (RE2). Off by default so that dots and " +
					"brackets in a name are not silently reinterpreted.",
			},
			"path": map[string]any{
				"type": "string", "description": "Only search files whose path contains this substring.",
			},
			"top_k": map[string]any{"type": "integer", "default": 5, "maximum": 20, "description": "Max files."},
		}, []string{"pattern"}),
		Handler: keywordHandler(ws),
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
		Name: "read_code",
		Description: "Read the exact source slice of one node, or of several at once with node_ids. " +
			"A multi-node read never cuts inside a node: whole nodes come back in the order requested " +
			"until the response budget runs out, and every node left out is listed with its reason.",
		InputSchema: objSchema(map[string]any{
			"node_id": map[string]any{"type": "string", "description": "Node ID to read."},
			"node_ids": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"maxItems":    maxReadNodes,
				"description": "Read several nodes in one call, in this order. Use instead of node_id, not with it.",
			},
		}, nil),
		Handler: readCodeHandler(ws),
	})

	reg.Register(&mcp.Tool{
		Name: "diagnose_index",
		Description: "Report the index's own health: node/edge/shard counts, edges whose target is missing, " +
			"files parsed with syntax errors, reverse-index stats, semantic search state (including a " +
			"vectors.bin written by a different embedding model than the one configured), the effective " +
			"startup flags, and .graphin disk usage. Any problem worth acting on comes back as a <hint>. " +
			"Use it when search returns nothing for a symbol you expect to exist, when explore_graph is " +
			"missing an edge you know is there, or before concluding the code is at fault — it separates " +
			"\"the index is wrong\" from \"the code is not what you thought\". Counting partial nodes scans " +
			"every node, so this is heavier than the other tools; it is a diagnostic, not a per-question call.",
		InputSchema: objSchema(nil, nil),
		Handler:     diagnoseHandler(ws),
	})

	registerWiki(reg, ws)

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

// objSchema builds a tool's input schema.
//
// A nil props may not reach the wire unchanged: a nil Go map marshals to JSON
// null, and a client that validates properties as an object rejects the whole
// tools/list response over it — every tool vanishes from the session, not just
// the one with the bad schema, and the server has no idea because by its own
// lights it answered correctly. That is what an argument-less tool did to this
// server for two releases; see TestInputSchemasMarshalValid.
func objSchema(props map[string]any, required []string) map[string]any {
	if props == nil {
		props = map[string]any{}
	}
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
		Query  string `json:"query"`
		TopK   int    `json:"top_k"`
		Target string `json:"target"`
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
		// An unrecognised target is refused rather than ignored: silently
		// searching everything would read as "there is no code here" when the
		// caller believes it asked for code only.
		target := strings.TrimSpace(a.Target)
		if target != "" && !nodeid.IsTarget(target) {
			st := ws.FSM.Status()
			return mcp.ErrorXML(mcp.ErrInternal,
				fmt.Sprintf("unknown target %q: use code, docs or db, or omit it to search everything", target), &st), true
		}
		var filter search.Filter
		if target != "" {
			filter = func(id string) bool { return nodeid.Target(ws.NodeKind(id), id) == target }
		}

		results, stats := ws.Router.SearchFilteredStats(a.Query, a.TopK, filter)
		semReady := ws.Router.SemanticReady()

		var sb strings.Builder
		st := ws.FSM.Status()
		if st.State != "ready" {
			sb.WriteString(st.XML())
			sb.WriteString("\n")
		}
		// Echo the filter. Without it a short list reads as "the workspace has
		// little of this", when it may mean the filter excluded the rest.
		// candidates is the pool the ranking chose from, not the pool it
		// returned. Five results out of four candidates and five out of three
		// thousand are the same response otherwise, and only one of them means
		// "this query named something".
		if target != "" {
			fmt.Fprintf(&sb, `<results semantic_ready="%t" target="%s" candidates="%d">`,
				semReady, mcp.EscapeAttr(target), stats.LexicalMatched)
		} else {
			fmt.Fprintf(&sb, `<results semantic_ready="%t" candidates="%d">`, semReady, stats.LexicalMatched)
		}
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
			fmt.Fprintf(&sb, `<node id="%s" display_name="%s" rank="%d" match_type="%s"`,
				mcp.EscapeAttr(r.NodeID), mcp.EscapeAttr(display), r.Rank, r.Match)
			// Where it is, not just what it is. Without this the cheapest
			// question an agent asks — "where is X" — still needs a second
			// call, and grep answers it in one
			// (docs/eval/2026-08-07-adoption-diagnosis §권고 1).
			if rel, line, ok := ws.NodeLocation(r.NodeID); ok {
				fmt.Fprintf(&sb, ` file="%s" line="%d"`, mcp.EscapeAttr(rel), line)
			}
			sb.WriteString(" />")
		}
		if h := searchHint(stats, len(results), ws.Lex.Len(), target); h != "" {
			fmt.Fprintf(&sb, "\n  <hint>%s</hint>", mcp.EscapeText(h))
		}
		body := sb.String() + "\n</results>"
		return body + costLine(len(body)), false
	}
}

// searchHint turns the two dead ends of a search loop into an instruction.
// Both are invisible in the result list itself: an empty list says nothing
// about which retriever to try next, and a full list drawn from a third of the
// repository looks exactly like a full list drawn from four nodes.
func searchHint(stats search.Stats, returned, indexed int, target string) string {
	// Checked before the empty case, because it is the one that actually
	// fires. A query naming a symbol that is not here still comes back full:
	// the tokenizer keeps the common words and the ranking obliges.
	if len(stats.UnnamedIdents) > 0 {
		return fmt.Sprintf("%s appears in the code but no indexed symbol is named it — a package-level "+
			"constant or a name the parser does not lift into a node. The ranking can only answer with its "+
			"users; search_keyword points at the declaration",
			strings.Join(quoteAll(stats.UnnamedIdents), ", "))
	}
	if len(stats.AbsentIdents) > 0 {
		return fmt.Sprintf("no indexed symbol spells %s. search_keyword still finds it as text "+
			"(a string built at runtime, a config key, a file the parser skipped); if that is empty too, "+
			"it is not in this workspace", strings.Join(quoteAll(stats.AbsentIdents), ", "))
	}
	if returned == 0 {
		h := "no ranked result. search_keyword finds an exact string the ranking cannot; " +
			"otherwise name the symbol rather than describing it"
		if target != "" {
			h += `, or drop target="` + target + `" — it may have excluded the answer`
		}
		return h
	}
	if m := stats.LexicalMatched; indexed > 0 && m >= broadMatchFloor && m*100/indexed >= broadMatchPercent {
		return fmt.Sprintf("this query touched %d of %d indexed nodes, so the ranking did most of the "+
			"deciding. Narrow it, or use search_keyword if you know the exact text", m, indexed)
	}
	return ""
}

func quoteAll(ss []string) []string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = `"` + s + `"`
	}
	return out
}

// A query is "broad" when it matched both a large share of the index and a
// large number outright: the share alone would fire on every query in a
// ten-node workspace.
const (
	broadMatchFloor   = 100
	broadMatchPercent = 25
)

// costLine reports what this response costs the caller's context. MCP is
// stateless, so graphin cannot keep a running budget — the agent sums these,
// and a number that excluded its own element would drift as it did.
//
// The value includes the element carrying it, which needs a fixed point:
// widening the number widens the response. It settles in one or two passes.
func costLine(bodyLen int) string {
	n := bodyLen
	for range 4 {
		line := fmt.Sprintf("\n<cost bytes=\"%d\" />", n)
		total := bodyLen + len(line)
		if total == n {
			return line
		}
		n = total
	}
	return fmt.Sprintf("\n<cost bytes=\"%d\" />", n)
}

// keywordMaxLines caps matched lines per file, and keywordMaxText caps each
// one. search_hybrid returns no bodies because a node is large; a matched line
// is not a body, it is the evidence of the match. Without it the agent has to
// read_code every candidate to tell a real hit from an incidental one, which is
// the exact cost this retriever exists to avoid.
const (
	keywordMaxLines = 3
	keywordMaxText  = 160
)

func keywordHandler(ws *workspace.Workspace) mcp.ToolHandler {
	type args struct {
		Pattern string `json:"pattern"`
		Regex   bool   `json:"regex"`
		Path    string `json:"path"`
		TopK    int    `json:"top_k"`
	}
	return func(_ context.Context, raw json.RawMessage) (string, bool) {
		var a args
		if err := json.Unmarshal(raw, &a); err != nil || strings.TrimSpace(a.Pattern) == "" {
			st := ws.FSM.Status()
			return mcp.ErrorXML(mcp.ErrInternal, "pattern is required", &st), true
		}
		if a.TopK <= 0 {
			a.TopK = 5
		}
		if a.TopK > 20 {
			a.TopK = 20
		}
		opts, err := keyword.Compile(a.Pattern, a.Regex)
		if err != nil {
			st := ws.FSM.Status()
			return mcp.ErrorXML(mcp.ErrInternal, "pattern is not a valid regular expression: "+err.Error(), &st), true
		}
		opts.PathContains = strings.TrimSpace(a.Path)
		opts.MaxLines, opts.MaxFiles = keywordMaxLines, a.TopK

		// Deliberately not gated on bootstrap. This retriever reads the tree,
		// not the index, so it is the one search that still answers while the
		// others are warming up — and that window is exactly when an agent
		// would otherwise leave for its host's grep.
		hits, err := keyword.Search(ws.Root, opts)
		if err != nil {
			st := ws.FSM.Status()
			return mcp.ErrorXML(mcp.ErrInternal, "keyword search failed: "+err.Error(), &st), true
		}

		paths := make([]string, 0, len(hits))
		for _, h := range hits {
			paths = append(paths, h.RelPath)
		}
		byFile := map[string][]workspace.FileNode{}
		if ws.Bootstrapped() {
			byFile = ws.NodesInFiles(paths)
		}

		var sb strings.Builder
		st := ws.FSM.Status()
		if st.State != "ready" {
			sb.WriteString(st.XML())
			sb.WriteString("\n")
		}
		mode := "literal"
		if a.Regex {
			mode = "regex"
		}
		// Echo the mode and whether ids could be resolved at all. A response
		// with no ids means the index is not up yet, not that the hits are
		// outside the graph — those read the same otherwise.
		fmt.Fprintf(&sb, `<results retriever="keyword" mode="%s" files="%d" node_ids="%t">`,
			mode, len(hits), ws.Bootstrapped())
		for i, h := range hits {
			fmt.Fprintf(&sb, "\n  <file path=\"%s\" matches=\"%d\" rank=\"%d\">",
				mcp.EscapeAttr(h.RelPath), h.Matches, i+1)
			for _, ln := range h.Lines {
				id, _ := workspace.NodeAtOffset(byFile[h.RelPath], uint32(ln.Byte))
				sb.WriteString("\n    ")
				if id != "" {
					fmt.Fprintf(&sb, `<node id="%s" line="%d" match_type="keyword">`, mcp.EscapeAttr(id), ln.No)
				} else {
					fmt.Fprintf(&sb, `<node line="%d" match_type="keyword">`, ln.No)
				}
				sb.WriteString(mcp.EscapeText(truncRunes(ln.Text, keywordMaxText)))
				sb.WriteString("</node>")
			}
			sb.WriteString("\n  </file>")
		}
		body := sb.String() + "\n</results>"
		return body + costLine(len(body)), false
	}
}

// truncRunes cuts on a rune boundary so a multibyte identifier never ends in
// half a character.
func truncRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
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

// maxReadNodes bounds a multi-node read. It matches explore_graph's page size,
// and it also bounds how much source the handler holds at once while deciding
// what fits.
const maxReadNodes = 20

func readCodeHandler(ws *workspace.Workspace) mcp.ToolHandler {
	type args struct {
		NodeID  string   `json:"node_id"`
		NodeIDs []string `json:"node_ids"`
	}
	return func(_ context.Context, raw json.RawMessage) (string, bool) {
		if !ws.Bootstrapped() {
			return notBootstrapped(ws), true
		}
		var a args
		_ = json.Unmarshal(raw, &a)

		st := ws.FSM.Status()
		switch {
		case a.NodeID != "" && len(a.NodeIDs) > 0:
			// Guessing which one the caller meant is exactly the kind of
			// silent choice this tool exists to avoid.
			return mcp.ErrorXML(mcp.ErrInternal, "pass node_id or node_ids, not both", &st), true
		case a.NodeID == "" && len(a.NodeIDs) == 0:
			return mcp.ErrorXML(mcp.ErrInternal, "node_id or node_ids is required", &st), true
		case len(a.NodeIDs) > maxReadNodes:
			return mcp.ErrorXML(mcp.ErrInternal,
				fmt.Sprintf("node_ids holds %d ids, at most %d per call", len(a.NodeIDs), maxReadNodes), &st), true
		}

		if a.NodeID != "" {
			return readOne(ws, a.NodeID)
		}
		return readMany(ws, a.NodeIDs)
	}
}

// readOne keeps the single-node contract byte for byte: one <code_block>, and
// a hard error when the node cannot be read.
func readOne(ws *workspace.Workspace, id string) (string, bool) {
	cb, err := ws.ReadCode(id)
	if err != nil {
		st := ws.FSM.Status()
		code, msg := readErr(err, id)
		return mcp.ErrorXML(code, msg, &st), true
	}
	var sb strings.Builder
	writeStatusPrefix(&sb, ws)
	writeCodeBlock(&sb, cb)
	return sb.String(), false
}

// readMany returns whole nodes in the order asked for, stopping at the first
// one that would not fit, and accounts for every id that did not come back.
//
// The response is therefore a PREFIX of the request. Fitting smaller later
// nodes into the gap would silently reorder the caller's priorities, and
// cutting inside a node would hand back half of a section that the caller had
// already decided to read — the failure this tool exists to remove
// (docs/markdown-spec.md §4).
func readMany(ws *workspace.Workspace, ids []string) (string, bool) {
	type item struct {
		id     string
		block  string // rendered <code_block>, empty when it could not be read
		reason string // omission reason when block is empty
	}
	items := make([]item, 0, len(ids))
	for _, id := range ids {
		cb, err := ws.ReadCode(id)
		if err != nil {
			items = append(items, item{id: id, reason: omitReason(err)})
			continue
		}
		var b strings.Builder
		writeCodeBlock(&b, cb)
		items = append(items, item{id: id, block: b.String()})
	}

	var head strings.Builder
	writeStatusPrefix(&head, ws)

	// tail is what the footer costs if the read stops right here: every id
	// from this point on still has to be named.
	tail := func(rest []item) int {
		n := 0
		for _, it := range rest {
			r := it.reason
			if r == "" {
				r = "budget"
			}
			n += omitLineLen(it.id, r)
		}
		return n
	}

	// The cut is decided against the COMPLETE response, not against the blocks
	// alone: stopping later costs more body but fewer omission lines.
	budget := mcp.MaxResponseBytes
	used := head.Len() + len(fmt.Sprintf("<code_blocks requested=\"%d\" returned=\"%d\">\n", len(ids), len(ids))) + len("</code_blocks>")
	cut := len(items)
	for i, it := range items {
		if it.block == "" {
			used += omitLineLen(it.id, it.reason)
			continue
		}
		if used+len(it.block)+1+tail(items[i+1:]) > budget {
			cut = i
			break
		}
		used += len(it.block) + 1
	}

	var sb strings.Builder
	sb.WriteString(head.String())
	returned := 0
	for _, it := range items[:cut] {
		if it.block != "" {
			returned++
		}
	}
	fmt.Fprintf(&sb, "<code_blocks requested=\"%d\" returned=\"%d\">\n", len(ids), returned)
	for _, it := range items[:cut] {
		if it.block == "" {
			writeOmitted(&sb, it.id, it.reason)
			continue
		}
		sb.WriteString(it.block)
		sb.WriteString("\n")
	}
	for _, it := range items[cut:] {
		reason := it.reason
		if reason == "" {
			reason = "budget"
		}
		writeOmitted(&sb, it.id, reason)
	}
	sb.WriteString("</code_blocks>")
	return sb.String(), false
}

func writeStatusPrefix(sb *strings.Builder, ws *workspace.Workspace) {
	if st := ws.FSM.Status(); st.State != "ready" {
		sb.WriteString(st.XML())
		sb.WriteString("\n")
	}
}

func writeCodeBlock(sb *strings.Builder, cb *workspace.CodeBlock) {
	fmt.Fprintf(sb, `<code_block id="%s" file="%s" lines="%d-%d" reparsed="%t"`,
		mcp.EscapeAttr(cb.ID), mcp.EscapeAttr(cb.RelPath), cb.StartLine, cb.EndLine, cb.Reparsed)
	if cb.Partial {
		sb.WriteString(` partial="true"`)
	}
	sb.WriteString(">\n")
	mcp.WriteCDATA(sb, cb.Code)
	sb.WriteString("\n</code_block>")
}

func writeOmitted(sb *strings.Builder, id, reason string) {
	fmt.Fprintf(sb, "  <omitted id=\"%s\" reason=\"%s\" />\n", mcp.EscapeAttr(id), reason)
}

func omitLineLen(id, reason string) int {
	var b strings.Builder
	writeOmitted(&b, id, reason)
	return b.Len()
}

func readErr(err error, id string) (string, string) {
	switch {
	case errors.Is(err, workspace.ErrNodeGone):
		return mcp.ErrNodeGone, "node vanished after reparse: " + id
	case errors.Is(err, workspace.ErrNodeNotFound):
		return mcp.ErrNodeNotFound, "unknown node: " + id
	default:
		return mcp.ErrInternal, err.Error()
	}
}

func omitReason(err error) string {
	switch {
	case errors.Is(err, workspace.ErrNodeGone):
		return "gone"
	case errors.Is(err, workspace.ErrNodeNotFound):
		return "not_found"
	default:
		return "error"
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
		// Two graphin shapes, because grep -C20 is only a substitute for one of
		// them. "Where is X" ends at search — results carry file/line — and that
		// is what -C20 answers too. The call graph is a capability grep does not
		// have, so search→explore→read is reported but never the comparison axis
		// (docs/eval/2026-08-07-adoption-diagnosis).
		locateBytes := len(mcp.Truncate(searchXML))
		navBytes := locateBytes + len(mcp.Truncate(exploreXML)) + len(mcp.Truncate(readXML))
		navMs := time.Since(t2).Milliseconds()

		md := buildBenchMarkdown(a, files, fullBytes, fullMs, ctxBytes, ctxMs, locateBytes, navBytes, navMs, hitRank, ws)

		var sb strings.Builder
		if st := ws.FSM.Status(); st.State != "ready" {
			sb.WriteString(st.XML())
			sb.WriteString("\n")
		}
		fmt.Fprintf(&sb,
			"<benchmark_report grep_full_bytes=\"%d\" grep_c20_bytes=\"%d\" graphin_locate_bytes=\"%d\" graphin_bytes=\"%d\" hit=\"%t\" hit_rank=\"%d\">\n",
			fullBytes, ctxBytes, locateBytes, navBytes, hitRank > 0, hitRank)
		mcp.WriteCDATA(&sb, md)
		sb.WriteString("\n</benchmark_report>")
		return sb.String(), false
	}
}

func buildBenchMarkdown(a struct {
	TargetQuery  string `json:"target_query"`
	ExpectedNode string `json:"expected_node"`
}, files, fullBytes int, fullMs int64, ctxBytes int, ctxMs int64, locateBytes, navBytes int, navMs int64, hitRank int, ws *workspace.Workspace) string {
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
	md.WriteString("\n시뮬레이션 가설: ① Grep Full = 매칭 파일 전체 바이트, ② Grep -C 20 = 매치 ±20라인, ③ graphin locate = search 응답만(결과가 file·line을 포함하므로 \"어디 있나\"는 여기서 끝난다), ④ graphin 전체 = search+explore+read.\n\n**비교 축은 ②↔③이다.** grep은 호출 그래프를 주지 않으므로 ④는 참고치로만 싣는다.\n\n")
	md.WriteString("| scenario | bytes | est. tokens (÷4) | ms | savings vs scenario |\n")
	md.WriteString("|---|---:|---:|---:|---|\n")
	fmt.Fprintf(&md, "| Grep Full (%d files) | %d | %d | %d | baseline |\n", files, fullBytes, fullBytes/4, fullMs)
	fmt.Fprintf(&md, "| Grep -C 20 | %d | %d | %d | %s vs Full |\n", ctxBytes, ctxBytes/4, ctxMs, func() string {
		if fullBytes <= 0 {
			return "n/a"
		}
		return fmt.Sprintf("%.1f%%", 100*(1-float64(ctxBytes)/float64(fullBytes)))
	}())
	locSaving := func(base int) string {
		if base <= 0 {
			return "n/a"
		}
		return fmt.Sprintf("%.1f%%", 100*(1-float64(locateBytes)/float64(base)))
	}
	fmt.Fprintf(&md, "| **graphin locate (search)** | %d | %d | — | **%s vs -C20** |\n",
		locateBytes, locateBytes/4, locSaving(ctxBytes))
	fmt.Fprintf(&md, "| graphin 전체 (search→explore→read) | %d | %d | %d | %s vs Full (참고) |\n",
		navBytes, navBytes/4, navMs, saving(fullBytes))

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
