package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mkRefWorkspace is a small Go module with the shapes a name can take:
// a unique function, a method, a name shared by two types, and a name
// shared only with a test double.
func mkRefWorkspace(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"go.mod": "module example.com/shop\n\ngo 1.22\n",
		"order/order.go": `package order

type OrderService struct{}

// Cancel cancels an order.
func (s *OrderService) Cancel(id string) error {
	refund(id)
	return nil
}

func refund(id string) {}

func (s *OrderService) Close() {}
`,
		"order/store.go": `package order

type Store struct{}

func (s *Store) Close() {}
`,
		"fakes/fakes_test.go": `package fakes

func refund(id string) {}
`,
	}
	for rel, body := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// An agent names a unit the way it reads in code; it cannot derive graphin's
// id format. Every form must land on the same unit as the exact id.
func TestRefsResolveByNameQualifiedNameAndPathLine(t *testing.T) {
	root := mkRefWorkspace(t)
	c := newClient(t, root)
	c.bootstrapAndWait(root)

	byID := func(ref string) string {
		t.Helper()
		text, isErr := c.tool("read_code", map[string]any{"symbol": ref})
		if isErr {
			t.Fatalf("read_code(%q): %s", ref, text)
		}
		return text
	}
	cancel := byID("OrderService.Cancel")
	if !strings.Contains(cancel, "func (s *OrderService) Cancel") {
		t.Fatalf("qualified name did not read the method:\n%s", cancel)
	}
	id := attr(t, cancel, "resolved", "id")
	for _, ref := range []string{"Cancel", "OrderService::Cancel", "order/order.go:7", "./order/order.go:8"} {
		got := byID(ref)
		if attr(t, got, "resolved", "id") != id {
			t.Fatalf("%q resolved elsewhere than %q:\n%s", ref, id, got)
		}
	}

	// An exact id answers exactly as before: no <resolved> note.
	exact := byID(id)
	if strings.Contains(exact, "<resolved") {
		t.Fatalf("an exact id must not carry a resolution note:\n%s", exact)
	}

	// explore_graph takes the same refs.
	g, isErr := c.tool("explore_graph", map[string]any{"symbol": "OrderService.Cancel", "direction": "uses"})
	if isErr || !strings.Contains(g, "refund") {
		t.Fatalf("explore_graph by name: %s", g)
	}
}

// A name shared by two units is not guessed: the candidates come back.
func TestAmbiguousRefListsCandidates(t *testing.T) {
	root := mkRefWorkspace(t)
	c := newClient(t, root)
	c.bootstrapAndWait(root)

	for _, tool := range []string{"read_code", "explore_graph"} {
		text, isErr := c.tool(tool, map[string]any{"symbol": "Close"})
		if !isErr || !strings.Contains(text, `code="AMBIGUOUS_REF"`) {
			t.Fatalf("%s: want AMBIGUOUS_REF, got:\n%s", tool, text)
		}
		if !strings.Contains(text, "order/order.go") || !strings.Contains(text, "order/store.go") {
			t.Fatalf("%s: both units must be listed with their files:\n%s", tool, text)
		}
	}
	// Qualifying the name settles it.
	if text, isErr := c.tool("read_code", map[string]any{"symbol": "Store.Close"}); isErr ||
		!strings.Contains(text, "func (s *Store) Close") {
		t.Fatalf("Store.Close: %s", text)
	}
	// In a batch, an ambiguous ref is accounted for, not dropped.
	text, _ := c.tool("read_code", map[string]any{"symbols": []string{"Close", "Store.Close"}})
	if !strings.Contains(text, `reason="ambiguous"`) || !strings.Contains(text, "func (s *Store) Close") {
		t.Fatalf("batch with an ambiguous ref:\n%s", text)
	}
}

// A name shared only with a test double resolves to the implementation, and
// says that it chose.
func TestRefSkipsTestDoubles(t *testing.T) {
	root := mkRefWorkspace(t)
	c := newClient(t, root)
	c.bootstrapAndWait(root)

	text, isErr := c.tool("read_code", map[string]any{"symbol": "refund"})
	if isErr {
		t.Fatalf("read_code(refund): %s", text)
	}
	if !strings.Contains(text, `chose="non-test"`) || !strings.Contains(text, `file="order/order.go"`) {
		t.Fatalf("want the non-test refund, marked as chosen:\n%s", text)
	}
}

// An unknown name keeps the NODE_NOT_FOUND line eval-rag counts, and adds
// what it might have meant so the agent can continue without a search tool.
func TestUnknownRefSuggests(t *testing.T) {
	root := mkRefWorkspace(t)
	c := newClient(t, root)
	c.bootstrapAndWait(root)

	text, isErr := c.tool("explore_graph", map[string]any{"symbol": "cancelOrder"})
	if !isErr || !strings.Contains(text, `<error code="NODE_NOT_FOUND">unknown node: cancelOrder</error>`) {
		t.Fatalf("want the unchanged NODE_NOT_FOUND line:\n%s", text)
	}
	if !strings.Contains(text, "<suggestions") || !strings.Contains(text, "Cancel") {
		t.Fatalf("want suggestions naming Cancel:\n%s", text)
	}
}

// symbol and node_id are one parameter under two names.
func TestRefAliasesConflict(t *testing.T) {
	root := mkRefWorkspace(t)
	c := newClient(t, root)
	c.bootstrapAndWait(root)

	text, isErr := c.tool("read_code", map[string]any{"symbol": "Cancel", "node_id": "refund"})
	if !isErr || !strings.Contains(text, "not both") {
		t.Fatalf("conflicting aliases must be refused:\n%s", text)
	}
	if text, isErr := c.tool("explore_graph", map[string]any{}); !isErr || !strings.Contains(text, "symbol is required") {
		t.Fatalf("explore_graph without a ref:\n%s", text)
	}
}

// attr reads one attribute of the first <tag ...> element in text.
func attr(t *testing.T, text, tag, name string) string {
	t.Helper()
	i := strings.Index(text, "<"+tag+" ")
	if i < 0 {
		t.Fatalf("no <%s> in:\n%s", tag, text)
	}
	el := text[i:]
	el = el[:strings.IndexByte(el, '>')]
	key := " " + name + `="`
	j := strings.Index(el, key)
	if j < 0 {
		t.Fatalf("<%s> has no %s: %s", tag, name, el)
	}
	v := el[j+len(key):]
	return v[:strings.IndexByte(v, '"')]
}
