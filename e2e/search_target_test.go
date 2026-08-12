package e2e

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// A sentence-shaped question about code used to come back as documentation.
// A whole markdown file is one node, so its prose matches a prose query better
// than the function it describes ever can, and the implementation falls off
// the list entirely (docs/eval/2026-08-11-adoption-remeasure §D). The target
// parameter is the way to ask the question without the prose in the way.
const prosePayments = `# Cancelling a payment

When an order is cancelled the service must cancel the payment and refund it.
Cancelling a payment refunds the charge; the refund is issued to the payment
method used for the original charge. A cancelled payment cannot be refunded
twice, so the cancel path records that the refund was issued before it returns.
Refund and cancel are the same operation seen from the payment side and the
order side.
`

var nodeIDRe = regexp.MustCompile(`<node id="([^"]+)"`)

func nodeIDs(text string) []string {
	var out []string
	for _, m := range nodeIDRe.FindAllStringSubmatch(text, -1) {
		out = append(out, m[1])
	}
	return out
}

func isDocID(id string) bool {
	path, _, _ := strings.Cut(id, "#")
	return strings.HasSuffix(path, ".md") || strings.HasSuffix(path, ".markdown")
}

func writeFile(t *testing.T, root, rel, body string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func proseWorkspace(t *testing.T) *client {
	t.Helper()
	root := t.TempDir()
	copyTree(t, javaFixtures, root)
	writeFile(t, root, "docs/payments.md", prosePayments)
	// Structured data indexed as one anchor-less node: it is neither prose nor
	// implementation, and it has no business answering a prose question.
	writeFile(t, root, "docs/scores.json",
		`{"query":"cancel the payment and refund it","cancel":1,"payment":2,"refund":3}`)
	c := newClient(t, root)
	c.bootstrapAndWait(root)
	return c
}

const proseQuestion = "how does the service cancel a payment and refund it"

func TestSearchTargetSeparatesCodeFromProse(t *testing.T) {
	c := proseWorkspace(t)

	// Precondition: unfiltered, prose wins. If this stops holding the fixture
	// no longer reproduces the defect and the rest proves nothing.
	plain, isErr := c.tool("search_hybrid", map[string]any{"query": proseQuestion})
	if isErr {
		t.Fatalf("search: %s", plain)
	}
	ids := nodeIDs(plain)
	if len(ids) == 0 {
		t.Fatalf("unfiltered search returned nothing:\n%s", plain)
	}
	var prose int
	for _, id := range ids {
		if isDocID(id) || strings.HasSuffix(id, ".json") {
			prose++
		}
	}
	if prose == 0 {
		t.Fatalf("precondition broken: prose took no slot, fixture no longer reproduces the defect:\n%s", plain)
	}

	t.Run("code excludes prose and data", func(t *testing.T) {
		text, isErr := c.tool("search_hybrid", map[string]any{"query": proseQuestion, "target": "code"})
		if isErr {
			t.Fatalf("search: %s", text)
		}
		got := nodeIDs(text)
		if len(got) == 0 {
			t.Fatalf("target=code returned nothing; the implementation is indexed:\n%s", text)
		}
		for _, id := range got {
			if isDocID(id) || strings.HasSuffix(id, ".json") {
				t.Errorf("target=code returned %q, which is not code", id)
			}
		}
		// The filter must be visible, or a short list reads as an empty repo.
		if !strings.Contains(text, `target="code"`) {
			t.Errorf("response does not echo the filter:\n%s", text)
		}
	})

	t.Run("docs returns only markdown", func(t *testing.T) {
		text, isErr := c.tool("search_hybrid", map[string]any{"query": proseQuestion, "target": "docs"})
		if isErr {
			t.Fatalf("search: %s", text)
		}
		got := nodeIDs(text)
		if len(got) == 0 {
			t.Fatalf("target=docs returned nothing:\n%s", text)
		}
		for _, id := range got {
			if !isDocID(id) {
				t.Errorf("target=docs returned %q, which is not documentation", id)
			}
		}
	})

	t.Run("unknown target is refused, not ignored", func(t *testing.T) {
		text, isErr := c.tool("search_hybrid", map[string]any{"query": proseQuestion, "target": "source"})
		if !isErr {
			t.Fatalf("unknown target accepted; results would silently include everything:\n%s", text)
		}
	})

	t.Run("omitting target searches everything", func(t *testing.T) {
		text, isErr := c.tool("search_hybrid", map[string]any{"query": proseQuestion, "target": ""})
		if isErr {
			t.Fatalf("empty target must mean unfiltered: %s", text)
		}
		if strings.Contains(text, `target="`) {
			t.Errorf("unfiltered response must not claim a filter:\n%s", text)
		}
	})
}
