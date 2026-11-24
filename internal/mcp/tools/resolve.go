package tools

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/Salvia95/graphin/internal/mcp"
	"github.com/Salvia95/graphin/internal/nodeid"
	"github.com/Salvia95/graphin/internal/workspace"
)

// A ref is how an agent names a unit of code: an exact node id, a bare name
// ("cancelOrder"), a qualified one ("OrderService.cancel"), a file path, or a
// file and line ("src/order.go:120"). Node ids are an internal format an agent
// cannot derive — guessing one was the commonest NODE_NOT_FOUND — so every
// tool that reads or explores a unit takes a ref and resolves it here
// (docs/tool-contract-spec.md §3.1).

// maxCandidates bounds the list an ambiguous or unknown ref comes back with.
const maxCandidates = 10

// pathLineRef is "path:line" or "path:start-end". The path must hold a slash
// or a dot so a Kotlin-style "Type:3" never reads as a file. A range resolves
// to the unit enclosing its first line; read_code reads the range itself
// (splitPathRange).
var pathLineRef = regexp.MustCompile(`^(.*[/.].*?):(\d+)(?:-(\d+))?$`)

// resolution is the outcome of one ref. Exactly one of id and candidates is
// meaningful: a single id when the ref named one unit, candidates when it
// named several (ambiguous) or none (suggestions, unresolved=true).
type resolution struct {
	id         string
	candidates []string
	unresolved bool
	// narrowed is set when several units shared the name and one was
	// taken anyway, for the reason in chose: "non-test" when all others were
	// tests, "table" when all others were objects attached to that table.
	narrowed int
	chose    string
}

func resolveRef(ws *workspace.Workspace, ref string) resolution {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return resolution{unresolved: true}
	}
	// 1. An exact id (a file path is one), or one a rename redirected.
	if ws.HasNode(ref) {
		return resolution{id: ref}
	}
	if r := ws.ResolveNodeID(ref); r != ref && ws.HasNode(r) {
		return resolution{id: r}
	}
	// 2. path:line — the unit enclosing that line.
	if m := pathLineRef.FindStringSubmatch(ref); m != nil {
		path := strings.TrimPrefix(m[1], "./")
		if line, err := strconv.Atoi(m[2]); err == nil {
			if id, ok := ws.NodeAtFileLine(path, line); ok {
				return resolution{id: id}
			}
		}
	}
	// 3. A name: bare names through the symbol table, qualified names by
	// suffix. Separators agents carry over from other languages and docs
	// (Type::method, Type#method) mean the same dotted member.
	var ids []string
	if q := normalizeQualified(ref); strings.Contains(q, ".") {
		ids = ws.NodesNamed(q)
	} else if ws.Sym != nil {
		ids = ws.Sym.Lookup(q)
	}
	switch len(ids) {
	case 0:
		return resolution{unresolved: true, candidates: suggestFor(ws, ref)}
	case 1:
		return resolution{id: ids[0]}
	}
	ids = orderCandidates(ws, ids)
	if nonTest := nonTestOnly(ws, ids); nonTest != "" {
		return resolution{id: nonTest, narrowed: len(ids), chose: "non-test"}
	}
	if table := tableOverAttachments(ws, ids); table != "" {
		return resolution{id: table, narrowed: len(ids), chose: "table"}
	}
	return resolution{candidates: ids}
}

// tableOverAttachments returns the one table or view among ids when every
// other id is an object attached to it — an RLS policy bundle or a trigger,
// whose id extends the table's. They share the table's name so a search for
// the table finds them too; asking which one was meant by "resume" only costs
// a call, since the table is what the name names.
func tableOverAttachments(ws *workspace.Workspace, ids []string) string {
	table := ""
	for _, id := range ids {
		if k := ws.NodeKind(id); k == nodeid.KindTable || k == nodeid.KindView {
			if table != "" {
				return ""
			}
			table = id
		}
	}
	if table == "" {
		return ""
	}
	for _, id := range ids {
		if id != table && !strings.HasPrefix(id, table+".") {
			return ""
		}
	}
	return table
}

// splitPathRange recognises "path:start-end" for an indexed file and returns
// the file's id and the range. read_code then reads exactly those lines —
// what an agent quoting a location from a search result or a stack trace
// wants — rather than the whole unit enclosing the first one.
func splitPathRange(ws *workspace.Workspace, ref string) (path, lines string, ok bool) {
	m := pathLineRef.FindStringSubmatch(strings.TrimSpace(ref))
	if m == nil || m[3] == "" {
		return "", "", false
	}
	path = strings.TrimPrefix(m[1], "./")
	if !ws.HasNode(path) {
		return "", "", false
	}
	return path, m[2] + "-" + m[3], true
}

func normalizeQualified(ref string) string {
	if strings.Contains(ref, "/") {
		return ref
	}
	r := strings.ReplaceAll(ref, "::", ".")
	return strings.ReplaceAll(r, "#", ".")
}

// suggestFor ranks what the ref might have meant, so an unknown name is a
// list to choose from rather than a dead end — the agent may have no other
// tool to search with.
func suggestFor(ws *workspace.Workspace, ref string) []string {
	if ws.Router == nil {
		return nil
	}
	results, _ := ws.Router.SearchFilteredStats(ref, 5, nil)
	out := make([]string, 0, len(results))
	for _, r := range results {
		out = append(out, r.NodeID)
	}
	return out
}

// orderCandidates puts implementation before tests, then sorts by id, so the
// list reads the same on every call.
func orderCandidates(ws *workspace.Workspace, ids []string) []string {
	out := append([]string(nil), ids...)
	sort.SliceStable(out, func(i, j int) bool {
		ti, tj := isTestNode(ws, out[i]), isTestNode(ws, out[j])
		if ti != tj {
			return !ti
		}
		return out[i] < out[j]
	})
	return out
}

// nonTestOnly returns the one candidate outside test code when every other
// candidate is a test, else "". A test double that shares the name of the
// thing it tests is the common shape, and asking the agent to choose there
// only costs a call.
func nonTestOnly(ws *workspace.Workspace, ids []string) string {
	found := ""
	for _, id := range ids {
		if isTestNode(ws, id) {
			continue
		}
		if found != "" {
			return ""
		}
		found = id
	}
	return found
}

func isTestNode(ws *workspace.Workspace, id string) bool {
	rel, _, ok := ws.NodeLocation(id)
	return ok && isTestPath(rel)
}

// isTestPath recognizes the test-file conventions of the languages graphin
// parses. A false negative only means the agent is asked to choose.
func isTestPath(rel string) bool {
	low := strings.ToLower(rel)
	base := low[strings.LastIndexByte(low, '/')+1:]
	switch {
	case strings.HasSuffix(base, "_test.go"),
		strings.HasPrefix(base, "test_") && strings.HasSuffix(base, ".py"),
		strings.HasSuffix(base, "_test.py"),
		strings.Contains(base, ".test."), strings.Contains(base, ".spec."),
		strings.HasSuffix(base, "test.java"), strings.HasSuffix(base, "test.kt"),
		strings.HasSuffix(base, "tests.java"), strings.HasSuffix(base, "tests.kt"):
		return true
	}
	for _, dir := range []string{"/__tests__/", "/src/test/"} {
		if strings.Contains("/"+low, dir) {
			return true
		}
	}
	return false
}

// refFailure renders a ref that did not resolve to one unit. Unknown refs
// keep the NODE_NOT_FOUND line exactly as before — eval-rag counts invented
// ids by it — and add what the ref might have meant. Ambiguous refs list the
// units sharing the name instead of picking one: a silent choice is the kind
// of guess this server refuses to make.
func refFailure(ws *workspace.Workspace, ref string, r resolution) string {
	var sb strings.Builder
	if r.unresolved {
		sb.WriteString(mcp.ErrorXML(mcp.ErrNodeNotFound, "unknown node: "+ref, nil))
		if len(r.candidates) > 0 {
			writeCandidates(&sb, ws, "suggestions", r.candidates)
			sb.WriteString("\n<hint>no unit is named that; these matched it as a search. " +
				"Pass one of their ids, a qualified name such as Type.method, or path:line</hint>")
		}
	} else {
		sb.WriteString(mcp.ErrorXML(mcp.ErrAmbiguousRef,
			fmt.Sprintf("%d units are named %s", len(r.candidates), ref), nil))
		writeCandidates(&sb, ws, "candidates", r.candidates)
		sb.WriteString("\n<hint>pass one of these ids, a qualified name such as Type.method, or path:line</hint>")
	}
	st := ws.FSM.Status()
	sb.WriteString("\n")
	sb.WriteString(st.XML())
	return sb.String()
}

func writeCandidates(sb *strings.Builder, ws *workspace.Workspace, tag string, ids []string) {
	fmt.Fprintf(sb, "\n<%s count=\"%d\">", tag, len(ids))
	for i, id := range ids {
		if i == maxCandidates {
			fmt.Fprintf(sb, "\n  <more count=\"%d\" />", len(ids)-maxCandidates)
			break
		}
		writeRefNode(sb, ws, "  ", id)
	}
	fmt.Fprintf(sb, "\n</%s>", tag)
}

// writeRefNode writes one <node> line with what an agent needs to pick it:
// its id, name, kind and location.
func writeRefNode(sb *strings.Builder, ws *workspace.Workspace, indent, id string) {
	name := ws.DisplayName(id)
	if name == "" && ws.Sym != nil {
		name = ws.Sym.SimpleName(id)
	}
	fmt.Fprintf(sb, "\n%s<node id=\"%s\" display_name=\"%s\"", indent, mcp.EscapeAttr(id), mcp.EscapeAttr(name))
	if k := ws.NodeKind(id); k != "" {
		fmt.Fprintf(sb, ` kind="%s"`, mcp.EscapeAttr(k))
	}
	if rel, line, ok := ws.NodeLocation(id); ok {
		fmt.Fprintf(sb, ` file="%s" line="%d"`, mcp.EscapeAttr(rel), line)
	}
	sb.WriteString(" />")
}

// resolvedNote says which unit a ref resolved to when the agent did not pass
// an id, so the answer can cite it — and when a name was narrowed past its
// test doubles, that it was.
func resolvedNote(ref string, r resolution) string {
	if ref == r.id {
		return ""
	}
	s := fmt.Sprintf(`<resolved ref="%s" id="%s"`, mcp.EscapeAttr(ref), mcp.EscapeAttr(r.id))
	if r.narrowed > 0 {
		s += fmt.Sprintf(` candidates="%d" chose="%s"`, r.narrowed, r.chose)
	}
	return s + " />\n"
}
