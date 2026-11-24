package tools

import (
	"fmt"
	"path"
	"strings"

	"github.com/Salvia95/graphin/internal/graph"
	"github.com/Salvia95/graphin/internal/mcp"
	"github.com/Salvia95/graphin/internal/nodeid"
	"github.com/Salvia95/graphin/internal/workspace"
)

// Detail levels. ids is the response every tool gave before chained answers
// existed, kept byte for byte; full adds what lets the agent judge a unit
// without a second call (docs/tool-contract-spec.md §3.3-3.4). Until the
// eval says otherwise, ids stays the default.
const (
	detailIDs  = "ids"
	detailFull = "full"
	detailCode = "code"
)

func parseDetail(s string, allowed ...string) (string, string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return allowed[0], ""
	}
	for _, a := range allowed {
		if s == a {
			return s, ""
		}
	}
	return "", fmt.Sprintf("unknown detail %q: use %s", s, strings.Join(allowed, " or "))
}

// graphFold is how much of a full response survives the budget: everything,
// then without reference lines' text, then without signatures. Edges are
// never dropped to fit — a missing caller reads as no caller.
type graphFold int

const (
	foldNone graphFold = iota
	foldRefText
	foldSignature
)

// edgeDetail is what full adds to one neighbour.
type edgeDetail struct {
	name, kind, signature string
	refLine               int
	refText               string
}

func renderGraph(ws *workspace.Workspace, head string, page *graph.Page, direction, detail string) string {
	if detail != detailFull {
		return renderGraphAt(ws, head, page, direction, nil, foldNone)
	}
	details := map[string]edgeDetail{}
	add := func(e graph.EdgeOut, holder, mentioned string) {
		d := edgeDetail{name: displayName(ws, e.NodeID), kind: ws.NodeKind(e.NodeID), signature: ws.Signature(e.NodeID)}
		if refLineApplies(ws, e, holder) {
			d.refLine, d.refText, _ = ws.RefLine(holder, refName(ws, mentioned))
		}
		details[e.NodeID+"\x00"+e.Type] = d
	}
	// A reference is written in the unit that makes it: in the neighbour for
	// a caller, in the target for a dependency.
	for _, e := range page.Uses {
		add(e, page.Target, e.NodeID)
	}
	for _, e := range page.UsedBy {
		add(e, e.NodeID, page.Target)
	}
	for f := foldNone; ; f++ {
		body := renderGraphAt(ws, head, page, direction, details, f)
		if len(body) <= keywordBudget || f == foldSignature {
			if f > foldNone {
				body = strings.Replace(body, "\n</graph_context>",
					fmt.Sprintf("\n  <folded what=\"%s\" />\n</graph_context>", foldedWhat(f)), 1)
			}
			return body
		}
	}
}

func foldedWhat(f graphFold) string {
	if f == foldRefText {
		return "ref_text"
	}
	return "ref_text signature"
}

// refLineApplies reports whether a neighbour's reference can be located by
// name: containment is structure, not a mention, and a DB object's source is
// a schema snapshot, not code that names it.
func refLineApplies(ws *workspace.Workspace, e graph.EdgeOut, holder string) bool {
	if e.Type == "contains" || e.Type == "foreign_key" {
		return false
	}
	return !nodeid.IsDBKind(ws.NodeKind(holder))
}

// refName is the word a unit is mentioned by: its simple name, or for a file
// its base name without the extension (an import of "billing/charge.py"
// reads "charge").
func refName(ws *workspace.Workspace, id string) string {
	if strings.Contains(id, "/") {
		base := path.Base(id)
		if i := strings.IndexByte(base, '#'); i >= 0 {
			return ""
		}
		return strings.TrimSuffix(base, path.Ext(base))
	}
	if ws.Sym != nil {
		if s := ws.Sym.SimpleName(id); s != "" {
			return s
		}
	}
	return nodeid.Simple(id)
}

func displayName(ws *workspace.Workspace, id string) string {
	if n := ws.DisplayName(id); n != "" {
		return n
	}
	if ws.Sym != nil {
		return ws.Sym.SimpleName(id)
	}
	return ""
}

func renderGraphAt(ws *workspace.Workspace, head string, page *graph.Page, direction string,
	details map[string]edgeDetail, fold graphFold) string {
	var sb strings.Builder
	sb.WriteString(head)
	fmt.Fprintf(&sb, `<graph_context target="%s"`, mcp.EscapeAttr(page.Target))
	if details != nil {
		fmt.Fprintf(&sb, ` detail="full" name="%s"`, mcp.EscapeAttr(displayName(ws, page.Target)))
		if k := ws.NodeKind(page.Target); k != "" {
			fmt.Fprintf(&sb, ` kind="%s"`, mcp.EscapeAttr(k))
		}
		if rel, line, ok := ws.NodeLocation(page.Target); ok {
			fmt.Fprintf(&sb, ` file="%s" line="%d"`, mcp.EscapeAttr(rel), line)
		}
		if sig := ws.Signature(page.Target); sig != "" {
			fmt.Fprintf(&sb, ` signature="%s"`, mcp.EscapeAttr(sig))
		}
	}
	sb.WriteString(">")
	writeEdges := func(tag string, edges []graph.EdgeOut) {
		if len(edges) == 0 {
			return
		}
		fmt.Fprintf(&sb, "\n  <%s>", tag)
		for _, e := range edges {
			fmt.Fprintf(&sb, "\n    <node id=\"%s\" type=\"%s\" confidence=\"%.2f\"",
				mcp.EscapeAttr(e.NodeID), e.Type, e.Confidence)
			// Where each neighbour is, as a search result says it: a hop
			// can then be cited (or read with lines) without a search to
			// find its file. Multi-hop answers failed on exactly that — the
			// second file found, named, and never cited. A target not in
			// the index has no location; its absence says so.
			if rel, line, ok := ws.NodeLocation(e.NodeID); ok {
				fmt.Fprintf(&sb, ` file="%s" line="%d"`, mcp.EscapeAttr(rel), line)
			}
			if d, ok := details[e.NodeID+"\x00"+e.Type]; ok {
				fmt.Fprintf(&sb, ` name="%s"`, mcp.EscapeAttr(d.name))
				if d.kind != "" {
					fmt.Fprintf(&sb, ` kind="%s"`, mcp.EscapeAttr(d.kind))
				}
				if d.signature != "" && fold < foldSignature {
					fmt.Fprintf(&sb, ` signature="%s"`, mcp.EscapeAttr(d.signature))
				}
				if d.refLine > 0 {
					fmt.Fprintf(&sb, ` ref_line="%d"`, d.refLine)
					if fold < foldRefText {
						fmt.Fprintf(&sb, ` ref_text="%s"`, mcp.EscapeAttr(d.refText))
					}
				}
			}
			sb.WriteString(" />")
		}
		fmt.Fprintf(&sb, "\n  </%s>", tag)
	}
	if direction != "used_by" {
		writeEdges("uses", page.Uses)
	}
	if direction != "uses" {
		writeEdges("used_by", page.UsedBy)
	}
	if page.HasMore {
		sb.WriteString("\n  <has_more>true</has_more>")
		fmt.Fprintf(&sb, "\n  <next_cursor>%s</next_cursor>", mcp.EscapeText(page.NextCursor))
	}
	sb.WriteString("\n</graph_context>")
	return sb.String()
}

// maxNeighbors is how many callers and how many dependencies a full read
// names. Enough to keep going from the unit; a full list is explore's job.
const maxNeighbors = 5

// writeNeighbors appends who calls a unit and what it calls, so a read is a
// place to continue from rather than a dead end. Skipped, and said to be,
// when the code already fills the response: the code is what was asked for.
func writeNeighbors(sb *strings.Builder, ws *workspace.Workspace, id string) {
	var nb strings.Builder
	nb.WriteString("\n<neighbors>")
	section := func(dir string) {
		page, err := ws.Explore(id, dir, "", 0.85)
		if err != nil {
			return
		}
		edges, holderIsNeighbour := page.Uses, false
		if dir == "used_by" {
			edges, holderIsNeighbour = page.UsedBy, true
		}
		if len(edges) == 0 {
			return
		}
		more := page.HasMore || len(edges) > maxNeighbors
		if len(edges) > maxNeighbors {
			edges = edges[:maxNeighbors]
		}
		fmt.Fprintf(&nb, "\n  <%s shown=\"%d\" more=\"%t\">", dir, len(edges), more)
		for _, e := range edges {
			fmt.Fprintf(&nb, "\n    <node id=\"%s\" name=\"%s\" type=\"%s\" confidence=\"%.2f\"",
				mcp.EscapeAttr(e.NodeID), mcp.EscapeAttr(displayName(ws, e.NodeID)), e.Type, e.Confidence)
			if rel, line, ok := ws.NodeLocation(e.NodeID); ok {
				fmt.Fprintf(&nb, ` file="%s" line="%d"`, mcp.EscapeAttr(rel), line)
			}
			holder, mentioned := id, e.NodeID
			if holderIsNeighbour {
				holder, mentioned = e.NodeID, id
			}
			if refLineApplies(ws, e, holder) {
				if line, _, ok := ws.RefLine(holder, refName(ws, mentioned)); ok {
					fmt.Fprintf(&nb, ` ref_line="%d"`, line)
				}
			}
			nb.WriteString(" />")
		}
		fmt.Fprintf(&nb, "\n  </%s>", dir)
	}
	section("used_by")
	section("uses")
	nb.WriteString("\n</neighbors>")
	if nb.Len() == len("\n<neighbors>\n</neighbors>") {
		return
	}
	if sb.Len()+nb.Len() > keywordBudget {
		sb.WriteString("\n<folded what=\"neighbors\" />")
		return
	}
	sb.WriteString(nb.String())
}
