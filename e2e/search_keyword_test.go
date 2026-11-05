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

// context=N brings the neighbourhood of each match in the same response,
// numbered and marked, still under the owning node id (docs/keyword-plan.md P1).
func TestSearchKeywordContextWindow(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "billing/retry.go", keywordFixture)
	c := newClient(t, root)
	c.bootstrapAndWait(root)

	text, isErr := c.tool("search_keyword", map[string]any{"pattern": "chargeOnce()", "context": 1})
	if isErr {
		t.Fatalf("search_keyword: %s", text)
	}
	if !strings.Contains(text, `context="1"`) {
		t.Fatalf("response must echo the context it applied:\n%s", text)
	}
	// The call inside chargeWithRetry: line 11, window 10-12, match marked.
	if !strings.Contains(text, `window="10-12"`) || !strings.Contains(text, "11> ") ||
		!strings.Contains(text, "10  ") || !strings.Contains(text, "12  ") {
		t.Fatalf("expected a numbered 10-12 window with line 11 marked:\n%s", text)
	}
	if !strings.Contains(text, `id="`) {
		t.Fatalf("a windowed hit must still carry its node id:\n%s", text)
	}

	// The default response is byte-for-byte the pre-context shape: no context
	// attribute, no windows.
	plain, _ := c.tool("search_keyword", map[string]any{"pattern": "chargeOnce()"})
	if strings.Contains(plain, "context=") || strings.Contains(plain, "window=") {
		t.Fatalf("default response grew a context shape:\n%s", plain)
	}

	if text, isErr := c.tool("search_keyword", map[string]any{"pattern": "chargeOnce", "context": 6}); !isErr ||
		!strings.Contains(text, "between 0 and 5") {
		t.Fatalf("context above the cap must be refused, not clamped:\n%s", text)
	}
}

// Nothing matched: the response says which retriever to try instead of
// leaving an empty list to interpret.
func TestSearchKeywordEmptyCarriesHint(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "billing/retry.go", keywordFixture)
	c := newClient(t, root)

	text, isErr := c.tool("search_keyword", map[string]any{"pattern": "no such text anywhere"})
	if isErr {
		t.Fatalf("search_keyword: %s", text)
	}
	if !strings.Contains(text, `files="0"`) || !strings.Contains(text, "<hint>") ||
		!strings.Contains(text, "search_hybrid") {
		t.Fatalf("an empty keyword result must carry a redirecting hint:\n%s", text)
	}
}

// Files that would push a windowed response past its budget are folded to one
// line each, never cut mid-window, and the whole thing stays under the cap.
func TestSearchKeywordFoldsFilesOverBudget(t *testing.T) {
	root := t.TempDir()
	long := strings.Repeat("x", 140)
	for i := 0; i < 8; i++ {
		var b strings.Builder
		b.WriteString("package p\n\n")
		for j := 0; j < 16; j++ {
			fmt.Fprintf(&b, "// filler %s %d\n", long, j)
			if j%5 == 0 {
				fmt.Fprintf(&b, "const NEEDLE_%d_%d = %q\n", i, j, long)
			}
		}
		writeFile(t, root, fmt.Sprintf("pkg/f%d.go", i), b.String())
	}
	c := newClient(t, root)
	c.bootstrapAndWait(root)

	text, isErr := c.tool("search_keyword", map[string]any{"pattern": "NEEDLE_", "context": 5, "top_k": 8})
	if isErr {
		t.Fatalf("search_keyword: %s", text)
	}
	if len(text) > 12*1024 {
		t.Fatalf("response is %d bytes, over the cap", len(text))
	}
	if !strings.Contains(text, `omitted="budget"`) {
		t.Fatalf("expected at least one folded file:\n%s", text)
	}
	if !strings.Contains(text, `window="`) {
		t.Fatalf("expected the first files to keep their windows:\n%s", text)
	}
	if strings.Contains(text, "...(truncated") || strings.Contains(text, "truncated") {
		t.Fatalf("the server-level truncation must never fire on this path:\n%s", text)
	}
}

// A data file that repeats the pattern would top a match-count ranking; the
// caller wanted code. Data files go after source and prose and come back
// folded, unless the caller pointed at them with path=.
func TestSearchKeywordFoldsDataFilesAfterCode(t *testing.T) {
	root := t.TempDir()
	copyTree(t, javaFixtures, root)
	writeFile(t, root, "billing/retry.go", keywordFixture)
	var sb strings.Builder
	sb.WriteString("[\n")
	for i := 0; i < 40; i++ {
		fmt.Fprintf(&sb, "  {\"task\": \"RETRY_BUDGET-%d\"},\n", i)
	}
	sb.WriteString("  {}\n]\n")
	writeFile(t, root, "docs/eval/scores.json", sb.String())
	c := newClient(t, root)
	c.bootstrapAndWait(root)

	text, isErr := c.tool("search_keyword", map[string]any{"pattern": "RETRY_BUDGET"})
	if isErr {
		t.Fatalf("search_keyword: %s", text)
	}
	goAt, jsonAt := strings.Index(text, `<file path="billing/retry.go"`), strings.Index(text, `<file path="docs/eval/scores.json"`)
	if goAt < 0 || jsonAt < 0 {
		t.Fatalf("both files must be listed:\n%s", text)
	}
	if jsonAt < goAt {
		t.Fatalf("the data file must rank after the source file:\n%s", text)
	}
	if !strings.Contains(text, `<file path="docs/eval/scores.json" matches="40" rank="2" folded="data" />`) {
		t.Fatalf("the data file must come back folded with its count:\n%s", text)
	}
	if !strings.Contains(text, `<file path="billing/retry.go" matches="3" rank="1">`) {
		t.Fatalf("the source file keeps rank 1 and its lines:\n%s", text)
	}

	// path= is the caller asking for it: opened, with lines.
	text, isErr = c.tool("search_keyword", map[string]any{"pattern": "RETRY_BUDGET", "path": "scores.json"})
	if isErr {
		t.Fatalf("search_keyword with path: %s", text)
	}
	if strings.Contains(text, `folded="data"`) || !strings.Contains(text, ` line="2" match_type="keyword"`) {
		t.Fatalf("a data file the caller pointed at must open:\n%s", text)
	}
}

const keywordTSFixture = `import { headRef } from "@/runtime/session";

export function logHeadRef(ref: string): void {
  console.log("headRef", headRef(ref));
}

export class Ledger {
  settleHeadRef(ref: string): string {
    return headRef(ref);
  }
}

describe("headRef", () => {
  it("settles headRef", () => {
    expect(new Ledger().settleHeadRef("x")).toBe("x");
  });
});
`

// Every matched line in an indexed source file has an owner: the narrowest
// node overlapping the line, and the file node when no symbol does. The
// shapes below are the ones that used to come back id-less or mis-owned in
// TypeScript (nklcbdt, 2026-11-05): an import, a describe/it block, a
// declaration after `export `, and an indented method whose line start sat in
// the class.
func TestSearchKeywordOwnsEveryLineOfASourceFile(t *testing.T) {
	root := t.TempDir()
	copyTree(t, javaFixtures, root)
	writeFile(t, root, "web/ledger.test.ts", keywordTSFixture)
	c := newClient(t, root)
	c.bootstrapAndWait(root)

	const file = "web/ledger.test.ts"
	// One pattern per shape: a response keeps only the first few matched
	// lines of a file, so a single shared pattern would not reach them all.
	cases := []struct {
		pattern, line string
		ok            func(string) bool
	}{
		{`from "@/runtime/session"`, "1", func(id string) bool { return id == file }},
		{"export function logHeadRef", "3", func(id string) bool { return strings.HasSuffix(id, "logHeadRef") }},
		{"settleHeadRef(ref: string)", "8", func(id string) bool { return strings.Contains(id, "settleHeadRef") }},
		{`describe("headRef"`, "13", func(id string) bool { return id == file }},
		{`it("settles headRef"`, "14", func(id string) bool { return id == file }},
	}
	for _, tc := range cases {
		text, isErr := c.tool("search_keyword", map[string]any{"pattern": tc.pattern, "path": "web/"})
		if isErr {
			t.Fatalf("search_keyword %q: %s", tc.pattern, text)
		}
		if strings.Contains(text, `<node line=`) {
			t.Errorf("%q: a matched line in an indexed source file came back without an id:\n%s", tc.pattern, text)
			continue
		}
		var got string
		for _, m := range keywordNodeRe.FindAllStringSubmatch(text, -1) {
			if m[2] == tc.line {
				got = m[1]
			}
		}
		if !tc.ok(got) {
			t.Errorf("%q: line %s resolved to %q:\n%s", tc.pattern, tc.line, got, text)
		}
	}
	if t.Failed() {
		t.FailNow()
	}

	// The file node is addressable: read_code returns the whole file, and
	// explore_graph answers rather than refusing an unknown id.
	rc, isErr := c.tool("read_code", map[string]any{"node_id": file})
	if isErr || !strings.Contains(rc, `lines="1-`) || !strings.Contains(rc, "describe(") {
		t.Fatalf("read_code on the file node:\n%s", rc)
	}
	if eg, isErr := c.tool("explore_graph", map[string]any{"node_id": file}); isErr {
		t.Fatalf("explore_graph on the file node: %s", eg)
	}

	// ...and never ranked: search_hybrid answers what it answered before the
	// file node existed, so a query for the file's name or its text does not
	// surface the whole file over the symbols in it.
	for _, q := range []string{"ledger.test.ts", "settles headRef", "headRef"} {
		sh, isErr := c.tool("search_hybrid", map[string]any{"query": q, "top_k": 20})
		if isErr {
			t.Fatalf("search_hybrid %q: %s", q, sh)
		}
		if strings.Contains(sh, `id="`+file+`"`) {
			t.Fatalf("search_hybrid %q ranked the file node:\n%s", q, sh)
		}
	}
}
