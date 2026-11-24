package tools

import (
	"fmt"
	"strings"

	"github.com/Salvia95/graphin/internal/mcp"
	"github.com/Salvia95/graphin/internal/workspace"
)

// A full search answer (docs/tool-contract-spec.md §3.3): every result says
// what it is (kind, signature) and shows the lines around the match; the top
// result also brings its body and its callers. Most "where" and "how"
// questions then end in one call instead of a search followed by reads the
// agent may never make.

// searchHit is one ranked result as the id-only response renders it: open is
// the <node ...> element without its close, so ids mode is byte for byte the
// response from before full existed.
type searchHit struct {
	id      string
	hitLine int
	open    string
}

const (
	// A top result's body comes back whole up to these sizes; past them, a
	// window around the hit.
	searchBodyMaxLines = 80
	searchBodyMaxBytes = 4096
	searchBodyRadius   = 30
	// snippetRadius is the half-window for results below the top one.
	snippetRadius = 3
	maxCallers    = 3
)

// searchFold is how much of a full answer survives the budget, each level
// giving up one more thing, least useful first.
type searchFold int

const (
	searchFoldNone     searchFold = iota
	searchFoldSnippets            // results below the top lose their snippets
	searchFoldBody                // the top body narrows to half the window
	searchFoldCallers             // the top result loses its callers
	searchFoldTopBody             // the top result keeps only its signature
)

var searchFoldNames = [...]string{"", "snippets", "snippets body", "snippets body callers", "snippets body callers"}

func renderSearch(ws *workspace.Workspace, head string, hits []searchHit, tail string, full bool) string {
	if !full {
		var sb strings.Builder
		sb.WriteString(head)
		for _, h := range hits {
			sb.WriteString(h.open)
			sb.WriteString(" />")
		}
		sb.WriteString(tail)
		sb.WriteString("\n</results>")
		return sb.String()
	}
	for f := searchFoldNone; ; f++ {
		body := renderSearchAt(ws, head, hits, tail, f)
		if len(body) <= keywordBudget || f == searchFoldTopBody {
			if f > searchFoldNone {
				body = strings.Replace(body, "\n</results>",
					fmt.Sprintf("\n  <folded what=\"%s\" />\n</results>", searchFoldNames[f]), 1)
			}
			return body
		}
	}
}

func renderSearchAt(ws *workspace.Workspace, head string, hits []searchHit, tail string, f searchFold) string {
	var sb strings.Builder
	sb.WriteString(head)
	for i, h := range hits {
		sb.WriteString(h.open)
		if k := ws.NodeKind(h.id); k != "" {
			fmt.Fprintf(&sb, ` kind="%s"`, mcp.EscapeAttr(k))
		}
		if sig := ws.Signature(h.id); sig != "" {
			fmt.Fprintf(&sb, ` signature="%s"`, mcp.EscapeAttr(sig))
		}
		var inner strings.Builder
		if i == 0 {
			if f < searchFoldTopBody {
				radius := searchBodyRadius
				if f >= searchFoldBody {
					radius /= 2
				}
				writeTopBody(&inner, ws, h, radius)
			}
			if f < searchFoldCallers {
				writeCallers(&inner, ws, h.id)
			}
		} else if f < searchFoldSnippets {
			if from, lines, ok := ws.Snippet(h.id, h.hitLine, snippetRadius); ok && len(lines) > 0 {
				fmt.Fprintf(&inner, "\n    <snippet lines=\"%d-%d\">", from, from+len(lines)-1)
				mcp.WriteCDATA(&inner, strings.Join(lines, "\n"))
				inner.WriteString("</snippet>")
			}
		}
		if inner.Len() == 0 {
			sb.WriteString(" />")
			continue
		}
		sb.WriteString(">")
		sb.WriteString(inner.String())
		sb.WriteString("\n  </node>")
	}
	sb.WriteString(tail)
	sb.WriteString("\n</results>")
	return sb.String()
}

// writeTopBody writes the top result's source: whole when it is short,
// otherwise the window around the hit (or its opening lines), marked
// partial so it is never taken for the whole unit.
func writeTopBody(sb *strings.Builder, ws *workspace.Workspace, h searchHit, radius int) {
	cb, err := ws.ReadCode(h.id)
	if err != nil {
		return
	}
	if n := cb.EndLine - cb.StartLine + 1; n <= searchBodyMaxLines && len(cb.Code) <= searchBodyMaxBytes {
		fmt.Fprintf(sb, "\n    <body lines=\"%d-%d\">", cb.StartLine, cb.EndLine)
		mcp.WriteCDATA(sb, cb.Code)
		sb.WriteString("</body>")
		return
	}
	from, lines, ok := ws.Snippet(h.id, h.hitLine, radius)
	if !ok || len(lines) == 0 {
		return
	}
	fmt.Fprintf(sb, "\n    <body lines=\"%d-%d\" partial=\"true\" unit_lines=\"%d-%d\">",
		from, from+len(lines)-1, cb.StartLine, cb.EndLine)
	mcp.WriteCDATA(sb, strings.Join(lines, "\n"))
	sb.WriteString("</body>")
}

// writeCallers names who uses the top result, with the line of each call.
func writeCallers(sb *strings.Builder, ws *workspace.Workspace, id string) {
	page, err := ws.Explore(id, "used_by", "", 0.85)
	if err != nil || len(page.UsedBy) == 0 {
		return
	}
	edges := page.UsedBy
	more := page.HasMore || len(edges) > maxCallers
	if len(edges) > maxCallers {
		edges = edges[:maxCallers]
	}
	fmt.Fprintf(sb, "\n    <callers shown=\"%d\" more=\"%t\">", len(edges), more)
	for _, e := range edges {
		fmt.Fprintf(sb, "\n      <node id=\"%s\" name=\"%s\" type=\"%s\"",
			mcp.EscapeAttr(e.NodeID), mcp.EscapeAttr(displayName(ws, e.NodeID)), e.Type)
		if rel, line, ok := ws.NodeLocation(e.NodeID); ok {
			fmt.Fprintf(sb, ` file="%s" line="%d"`, mcp.EscapeAttr(rel), line)
		}
		if refLineApplies(ws, e, e.NodeID) {
			if line, _, ok := ws.RefLine(e.NodeID, refName(ws, id)); ok {
				fmt.Fprintf(sb, ` ref_line="%d"`, line)
			}
		}
		sb.WriteString(" />")
	}
	sb.WriteString("\n    </callers>")
}
