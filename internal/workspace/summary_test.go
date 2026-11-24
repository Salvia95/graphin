package workspace

import (
	"strings"
	"testing"
)

func summaryWS(t *testing.T) *Workspace {
	t.Helper()
	ws, _ := scopeWS(t, map[string]string{
		"go.mod": "module example.com/shop\n\ngo 1.22\n",
		"order/order.go": `package order

// OrderService cancels orders.
type OrderService struct{}

// Cancel cancels an order.
func (s *OrderService) Cancel(
	id string,
	reason string,
) error {
	refund(id)
	refundAll()
	return nil
}

func refund(id string) {}

func refundAll() {}
`,
		"app/svc.py": `import os


@decorator
def handle(event, ctx):
    """Handle one event."""
    return dispatch(event)


def dispatch(event):
    return event
`,
		"docs/guide.md": "# Guide\n\n## Retry policy\n\nRetries back off.\n",
	})
	return ws
}

func idOf(t *testing.T, ws *Workspace, name string) string {
	t.Helper()
	ids := ws.Sym.Lookup(name)
	if len(ids) != 1 {
		t.Fatalf("%s: want one id, got %v", name, ids)
	}
	return ids[0]
}

func TestSignatures(t *testing.T) {
	ws := summaryWS(t)
	cases := map[string]string{
		"Cancel": "func (s *OrderService) Cancel( id string, reason string, ) error",
		// The Go parser spans a type from its name, not the type keyword.
		"OrderService": "OrderService struct",
		"refund":       "func refund(id string)",
		"handle":       "def handle(event, ctx):",
	}
	for name, want := range cases {
		if got := ws.Signature(idOf(t, ws, name)); got != want {
			t.Errorf("%s: signature %q, want %q", name, got, want)
		}
	}
	if got := ws.Signature("docs/guide.md#retry-policy"); got != "## Retry policy" {
		t.Errorf("section signature %q", got)
	}
	if got := ws.Signature("order/order.go"); got != "" {
		t.Errorf("a file has no signature, got %q", got)
	}
	if got := ws.Signature("no.such.node"); got != "" {
		t.Errorf("unknown node: %q", got)
	}
}

// The reference line is where the body mentions the name as a whole word:
// refund must not match refundAll, and the declaration line is not a call.
func TestRefLine(t *testing.T) {
	ws := summaryWS(t)
	cancel := idOf(t, ws, "Cancel")
	line, text, ok := ws.RefLine(cancel, "refund")
	if !ok || text != "refund(id)" || line != 11 {
		t.Fatalf("refund: line %d %q %v", line, text, ok)
	}
	if line, text, ok := ws.RefLine(cancel, "refundAll"); !ok || line != 12 || text != "refundAll()" {
		t.Fatalf("refundAll: line %d %q %v", line, text, ok)
	}
	if _, _, ok := ws.RefLine(cancel, "dispatch"); ok {
		t.Fatal("a name the unit never mentions must not be found")
	}
}

func TestSnippetCentresAndClips(t *testing.T) {
	ws := summaryWS(t)
	cancel := idOf(t, ws, "Cancel")
	from, lines, ok := ws.Snippet(cancel, 11, 1)
	if !ok || from != 10 || len(lines) != 3 || strings.TrimSpace(lines[1]) != "refund(id)" {
		t.Fatalf("centred: from %d %q", from, lines)
	}
	// Near the unit's end the window slides back instead of shrinking.
	from, lines, _ = ws.Snippet(cancel, 14, 2)
	if len(lines) != 5 || lines[len(lines)-1] != "}" || from != 10 {
		t.Fatalf("clipped at end: from %d %q", from, lines)
	}
	// No centre: the unit's first lines.
	from, lines, _ = ws.Snippet(cancel, 0, 1)
	if from != 7 || !strings.HasPrefix(lines[0], "func (s *OrderService) Cancel(") {
		t.Fatalf("head: from %d %q", from, lines)
	}
}
