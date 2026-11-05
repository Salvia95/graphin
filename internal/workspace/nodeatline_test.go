package workspace

import (
	"strings"
	"testing"

	"github.com/Salvia95/graphin/internal/nodeid"
	"github.com/Salvia95/graphin/internal/parse"
)

// lineRange returns the [start, end) bytes of 1-based line no, newline
// excluded — what keyword.Line carries.
func lineRange(src string, no int) (uint32, uint32) {
	start := 0
	for i := 1; i < no; i++ {
		start += strings.IndexByte(src[start:], '\n') + 1
	}
	end := start + strings.IndexByte(src[start:], '\n')
	return uint32(start), uint32(end)
}

func ownerOf(t *testing.T, rel, src string, no int) string {
	t.Helper()
	res, err := parse.File(rel, []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	var nodes []FileNode
	for _, n := range res.Nodes {
		nodes = append(nodes, FileNode{ID: n.ID, StartByte: n.StartByte, EndByte: n.EndByte})
	}
	s, e := lineRange(src, no)
	id, _ := NodeAtLine(nodes, s, e)
	return id
}

// A declaration's span starts at the declaration, not at the line: after the
// indentation of a method, after `export `. The line must still resolve to
// the symbol it declares, not to the class around it or to nothing.
func TestNodeAtLineResolvesDeclarationLines(t *testing.T) {
	py := "class MCP:\n    def call(self, method):\n        return method\n"
	if got := ownerOf(t, "scripts/mcp.py", py, 2); !strings.HasSuffix(got, "MCP.call") {
		t.Errorf("indented def resolved to %q, want the method", got)
	}
	ts := "import { a } from \"./a\";\n\nexport function run(): void {\n  a();\n}\n"
	if got := ownerOf(t, "src/run.ts", ts, 3); !strings.HasSuffix(got, ".run") {
		t.Errorf("export function line resolved to %q, want the function", got)
	}
	if got := ownerOf(t, "src/run.ts", ts, 4); !strings.HasSuffix(got, ".run") {
		t.Errorf("body line resolved to %q, want the function", got)
	}
	if got := ownerOf(t, "src/run.ts", ts, 1); got != "src/run.ts" {
		t.Errorf("import line resolved to %q, want the file node", got)
	}
	// An empty line still has an owner.
	if got := ownerOf(t, "src/run.ts", ts, 2); got != "src/run.ts" {
		t.Errorf("blank line resolved to %q, want the file node", got)
	}
}

// The narrowest overlapping span wins; the line after a node does not belong
// to it just because the node's end touches the line start.
func TestNodeAtLineNarrowestOverlap(t *testing.T) {
	nodes := []FileNode{
		{ID: "file", StartByte: 0, EndByte: 100},
		{ID: "class", StartByte: 10, EndByte: 90},
		{ID: "method", StartByte: 24, EndByte: 40},
	}
	for _, tc := range []struct {
		start, end uint32
		want       string
	}{
		{0, 9, "file"},     // above the class
		{20, 30, "method"}, // line start in class, declaration mid-line
		{40, 50, "class"},  // method ended exactly at the line start
		{95, 100, "file"},  // below the class
		{60, 60, "class"},  // empty line
	} {
		if got, _ := NodeAtLine(nodes, tc.start, tc.end); got != tc.want {
			t.Errorf("[%d,%d) -> %q, want %q", tc.start, tc.end, got, tc.want)
		}
	}
	if _, ok := NodeAtLine(nil, 0, 5); ok {
		t.Error("no nodes must resolve to nothing")
	}
}

// The code file node is indexed for ownership and reading, never for ranking:
// it is in the node map, absent from the symbol table and BM25.
func TestCodeFileNodeIsAddressableButUnranked(t *testing.T) {
	w := tempWorkspaceWithOrderService(t) // exactly one indexed file
	fileID := orderServiceRel
	if w.NodeKind(fileID) != nodeid.KindFile {
		t.Fatalf("%s has no file node", fileID)
	}
	if cb, err := w.ReadCode(fileID); err != nil || cb.StartLine != 1 {
		t.Fatalf("read the file node: %v %+v", err, cb)
	}
	base := fileID[strings.LastIndexByte(fileID, '/')+1:]
	for _, q := range []string{base, "OrderService", "processing"} {
		for _, r := range w.Router.Search(q, 50) {
			if r.NodeID == fileID {
				t.Fatalf("search %q ranked the file node", q)
			}
		}
	}
	w.nodesMu.RLock()
	total := len(w.nodes)
	w.nodesMu.RUnlock()
	if w.Sym.Len() != total-1 {
		t.Fatalf("symbol table holds %d of %d nodes, want all but the file node", w.Sym.Len(), total)
	}
}
