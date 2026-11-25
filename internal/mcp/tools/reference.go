package tools

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Salvia95/graphin/internal/mcp"
)

// The tool reference (docs/tools.md) is rendered from the registry, never
// written by hand: the hand-written tables in the README drifted from what
// the server actually exposes (search_keyword and the wiki tools went missing
// for months). TestToolReferenceUpToDate fails when the file and the
// definitions disagree; `go test ./internal/mcp/tools -run
// TestToolReferenceUpToDate -update` rewrites it.

// referenceGroups orders the reference the way a reader looks for a tool:
// what an agent navigates with first, then what keeps the index healthy, then
// the knowledge layer. A tool missing from every group lands in "Other", so a
// new tool can never silently drop out of the document.
var referenceGroups = []struct {
	title, blurb string
	tools        []string
}{
	{"Navigation", "What an agent uses to find, trace and read code.",
		[]string{"search_hybrid", "search_keyword", "explore_graph", "read_code"}},
	{"Index", "Starting, inspecting and scoping the index.",
		[]string{"bootstrap_workspace", "diagnose_index", "index_scope", "run_local_benchmark"}},
	{"Knowledge layer", "Knowledge sets over `docs/wiki` (see the graphin-guide `knowledge` skill).",
		[]string{"wiki_preflight", "wiki_resolve", "wiki_propose", "wiki_write_set", "wiki_edit_set"}},
}

// RenderReference renders the Markdown tool reference for every tool in reg.
func RenderReference(reg *mcp.Registry) string {
	byName := map[string]*mcp.Tool{}
	for _, t := range reg.List() {
		byName[t.Name] = t
	}
	var sb strings.Builder
	sb.WriteString("# Tool reference\n\n")
	sb.WriteString("<!-- Generated from the server's tool definitions — do not edit by hand.\n")
	sb.WriteString("     Regenerate: go test ./internal/mcp/tools -run TestToolReferenceUpToDate -update -->\n\n")
	sb.WriteString("Every MCP tool the graphin server exposes, with its full input schema. ")
	sb.WriteString("The descriptions are exactly what an agent sees. Tools marked *on demand* are ")
	sb.WriteString("kept behind the client's tool search to save prompt tokens; the rest are loaded up front.\n\n")
	sb.WriteString("Errors come back as `<error code=\"…\">` in the result: `NODE_NOT_FOUND` (with suggestions), ")
	sb.WriteString("`AMBIGUOUS_REF` (with candidates), `NODE_GONE`, `NOT_BOOTSTRAPPED`, `LOCK_HELD`, ")
	sb.WriteString("`LEADER_CHANGED`, `MODEL_UNAVAILABLE`, `INTERNAL`.\n\n")

	seen := map[string]bool{}
	type group struct {
		title, blurb string
		tools        []*mcp.Tool
	}
	var groups []group
	for _, g := range referenceGroups {
		gr := group{title: g.title, blurb: g.blurb}
		for _, n := range g.tools {
			if t, ok := byName[n]; ok {
				gr.tools = append(gr.tools, t)
				seen[n] = true
			}
		}
		groups = append(groups, gr)
	}
	var other []*mcp.Tool
	for _, t := range reg.List() {
		if !seen[t.Name] {
			other = append(other, t)
		}
	}
	if len(other) > 0 {
		groups = append(groups, group{title: "Other", tools: other})
	}

	sb.WriteString("| Group | Tools |\n|---|---|\n")
	for _, g := range groups {
		names := make([]string, 0, len(g.tools))
		for _, t := range g.tools {
			names = append(names, fmt.Sprintf("[`%s`](#%s)", t.Name, t.Name))
		}
		fmt.Fprintf(&sb, "| %s | %s |\n", g.title, strings.Join(names, " · "))
	}
	for _, g := range groups {
		fmt.Fprintf(&sb, "\n## %s\n\n", g.title)
		if g.blurb != "" {
			sb.WriteString(g.blurb + "\n")
		}
		for _, t := range g.tools {
			writeToolReference(&sb, t)
		}
	}
	return sb.String()
}

func writeToolReference(sb *strings.Builder, t *mcp.Tool) {
	fmt.Fprintf(sb, "\n### %s\n\n", t.Name)
	if t.Deferred {
		sb.WriteString("*Loaded on demand.*\n\n")
	}
	// The descriptions separate their parts with single newlines, which
	// Markdown would run together; each line becomes its own paragraph.
	sb.WriteString(strings.ReplaceAll(strings.TrimSpace(t.Description), "\n", "\n\n") + "\n")

	props, _ := t.InputSchema["properties"].(map[string]any)
	if len(props) == 0 {
		sb.WriteString("\nNo parameters.\n")
		return
	}
	required := map[string]bool{}
	switch r := t.InputSchema["required"].(type) {
	case []string:
		for _, n := range r {
			required[n] = true
		}
	case []any:
		for _, n := range r {
			if s, ok := n.(string); ok {
				required[s] = true
			}
		}
	}
	names := make([]string, 0, len(props))
	for n := range props {
		names = append(names, n)
	}
	// Required first, then alphabetical: the order is stable across runs,
	// unlike the map the schema is built from.
	sort.Slice(names, func(i, j int) bool {
		if required[names[i]] != required[names[j]] {
			return required[names[i]]
		}
		return names[i] < names[j]
	})
	sb.WriteString("\n| Parameter | Type | Required | Default | Description |\n|---|---|---|---|---|\n")
	for _, n := range names {
		p, _ := props[n].(map[string]any)
		req := ""
		if required[n] {
			req = "yes"
		}
		fmt.Fprintf(sb, "| `%s` | %s | %s | %s | %s |\n", n, schemaType(p), req, schemaDefault(p), schemaNotes(p))
	}
}

func schemaType(p map[string]any) string {
	t, _ := p["type"].(string)
	if t == "array" {
		if it, ok := p["items"].(map[string]any); ok {
			if s, ok := it["type"].(string); ok {
				return s + "[]"
			}
		}
	}
	return t
}

func schemaDefault(p map[string]any) string {
	d, ok := p["default"]
	if !ok {
		return ""
	}
	return fmt.Sprintf("`%v`", d)
}

func schemaNotes(p map[string]any) string {
	var parts []string
	if d, ok := p["description"].(string); ok && d != "" {
		parts = append(parts, d)
	}
	if e, ok := p["enum"].([]string); ok {
		parts = append(parts, "One of: `"+strings.Join(e, "`, `")+"`.")
	}
	var bounds []string
	for _, k := range []string{"minimum", "maximum", "maxItems"} {
		if v, ok := p[k]; ok {
			bounds = append(bounds, fmt.Sprintf("%s %v", k, v))
		}
	}
	if len(bounds) > 0 {
		parts = append(parts, "("+strings.Join(bounds, ", ")+")")
	}
	return strings.ReplaceAll(strings.Join(parts, " "), "|", "\\|")
}
