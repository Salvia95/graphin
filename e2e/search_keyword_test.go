package e2e

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
)

var keywordNodeRe = regexp.MustCompile(`<node id="([^"]+)" line="(\d+)"`)

const keywordFixture = `package billing

// RETRY_BUDGET is the kind of constant no parser reports as a call: it is
// spelled once and read from a config string at runtime.
const RETRY_BUDGET = 3

func chargeOnce() error { return nil }

func chargeWithRetry() error {
	for i := 0; i < RETRY_BUDGET; i++ {
		if err := chargeOnce(); err == nil {
			return nil
		}
	}
	return nil
}
`

// The point of the keyword retriever is that a text hit comes back as a node
// id, so the string flows into explore_graph and read_code without a second
// locating call.
func TestSearchKeywordResolvesHitsToNodes(t *testing.T) {
	root := t.TempDir()
	copyTree(t, javaFixtures, root)
	writeFile(t, root, "billing/retry.go", keywordFixture)
	c := newClient(t, root)
	c.bootstrapAndWait(root)

	text, isErr := c.tool("search_keyword", map[string]any{"pattern": "RETRY_BUDGET"})
	if isErr {
		t.Fatalf("search_keyword: %s", text)
	}
	if !strings.Contains(text, `node_ids="true"`) {
		t.Fatalf("expected ids to be resolvable after bootstrap:\n%s", text)
	}
	if !strings.Contains(text, "billing/retry.go") {
		t.Fatalf("the file holding the constant is missing:\n%s", text)
	}
	ms := keywordNodeRe.FindAllStringSubmatch(text, -1)
	if len(ms) == 0 {
		t.Fatalf("no matched line carried a node id:\n%s", text)
	}
	// The hit inside chargeWithRetry must resolve to that function, not to the
	// whole file — the smallest span wins, which is what makes the id useful.
	var sawFunc bool
	for _, m := range ms {
		if strings.Contains(m[1], "chargeWithRetry") {
			sawFunc = true
		}
	}
	if !sawFunc {
		t.Fatalf("the use inside chargeWithRetry did not resolve to the function:\n%s", text)
	}
}

// Keyword search reads the tree, not the index, so it is the one retriever that
// still answers during warmup — the window where an agent would otherwise leave
// for its host's grep.
func TestSearchKeywordAnswersBeforeBootstrap(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "billing/retry.go", keywordFixture)
	c := newClient(t, root)

	text, isErr := c.tool("search_keyword", map[string]any{"pattern": "RETRY_BUDGET"})
	if isErr {
		t.Fatalf("search_keyword before bootstrap: %s", text)
	}
	if !strings.Contains(text, "billing/retry.go") {
		t.Fatalf("no hit before bootstrap:\n%s", text)
	}
	// Honest about why there are no ids: the index is not up, the hits are not
	// outside the graph. Those two read the same without the flag.
	if !strings.Contains(text, `node_ids="false"`) {
		t.Fatalf("expected node_ids=\"false\" before bootstrap:\n%s", text)
	}
}

func TestSearchKeywordRejectsBadRegex(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "billing/retry.go", keywordFixture)
	c := newClient(t, root)

	text, isErr := c.tool("search_keyword", map[string]any{"pattern": "charge(", "regex": true})
	if !isErr {
		t.Fatalf("an invalid regex should be an error, got:\n%s", text)
	}
	if !strings.Contains(text, "regular expression") {
		t.Fatalf("the error should say what was wrong:\n%s", text)
	}
}

// The loop signals: what the ranking chose from, what this response costs, and
// — when the query names something the index does not hold — where to go next.
func TestSearchResponseCarriesLoopSignals(t *testing.T) {
	root := t.TempDir()
	copyTree(t, javaFixtures, root)
	writeFile(t, root, "billing/retry.go", keywordFixture)
	c := newClient(t, root)
	c.bootstrapAndWait(root)

	text, isErr := c.tool("search_hybrid", map[string]any{"query": "cancelPayment", "top_k": 3})
	if isErr {
		t.Fatalf("search_hybrid: %s", text)
	}
	if !strings.Contains(text, "candidates=") {
		t.Fatalf("response does not say how large the candidate pool was:\n%s", text)
	}
	m := regexp.MustCompile(`<cost bytes="(\d+)" />`).FindStringSubmatch(text)
	if m == nil {
		t.Fatalf("response does not report its own cost:\n%s", text)
	}
	var n int
	if _, err := fmt.Sscanf(m[1], "%d", &n); err != nil || n != len(text) {
		t.Fatalf("cost says %s, response is %d bytes", m[1], len(text))
	}

	// RETRY_BUDGET is a package-level constant. It is spelled inside the
	// function that reads it, so the ranking answers with that user — which is
	// not wrong, and is also not the declaration. The response has to name the
	// difference and point at the retriever that can close it.
	text, isErr = c.tool("search_hybrid", map[string]any{"query": "RETRY_BUDGET"})
	if isErr {
		t.Fatalf("search_hybrid: %s", text)
	}
	if !strings.Contains(text, "no indexed symbol is named it") || !strings.Contains(text, "search_keyword") {
		t.Fatalf("expected a hint saying RETRY_BUDGET names no symbol, and naming search_keyword:\n%s", text)
	}
	kw, isErr := c.tool("search_keyword", map[string]any{"pattern": "RETRY_BUDGET"})
	if isErr || !strings.Contains(kw, "billing/retry.go") {
		t.Fatalf("the retriever the hint pointed at did not find it:\n%s", kw)
	}
}
