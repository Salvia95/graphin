package e2e

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
)

// A long section is indexed in chunks, and search_hybrid reports a hit on any
// of them as the section itself: the id read_code takes, the line the
// section starts at, and hit_line for where in it the match is. No chunk id
// ever reaches the caller.
func TestSearchHybridFindsTheBackOfALongSection(t *testing.T) {
	var b strings.Builder
	b.WriteString("# 운영 메모\n\n")
	for p := 0; p < 12; p++ {
		for w := 0; w < 60; w++ {
			b.WriteString("q")
			n := p*100 + w
			for {
				b.WriteByte(byte('a' + n%26))
				n /= 26
				if n == 0 {
					break
				}
			}
			b.WriteByte(' ')
		}
		b.WriteString("\n\n")
	}
	doc := b.String()
	tail := strings.Fields(doc[strings.LastIndex(doc, "\n\nq")+2:])[30]

	root := t.TempDir()
	copyTree(t, javaFixtures, root)
	writeFile(t, root, "docs/ops.md", doc)
	c := newClient(t, root)
	c.bootstrapAndWait(root)

	text, isErr := c.tool("search_hybrid", map[string]any{"query": tail, "target": "docs"})
	if isErr {
		t.Fatalf("search_hybrid: %s", text)
	}
	m := regexp.MustCompile(`<node id="([^"]+)"[^>]*line="(\d+)" hit_line="(\d+)"`).FindStringSubmatch(text)
	if m == nil || m[1] != "docs/ops.md#운영-메모" || m[2] != "1" {
		t.Fatalf("expected the section with its start line and a hit_line:\n%s", text)
	}
	wantLine := 1 + strings.Count(doc[:strings.Index(doc, tail+" ")], "\n")
	// A chunk starts on a paragraph boundary at or before the matched word,
	// and holds at most maxBodyTokens of 60-word paragraphs (two lines each).
	var hit int
	for _, ch := range m[3] {
		hit = hit*10 + int(ch-'0')
	}
	if hit > wantLine || hit < wantLine-10 {
		t.Fatalf("hit_line %d is not the chunk holding line %d:\n%s", hit, wantLine, text)
	}
	if strings.Contains(text, "~") {
		t.Fatalf("a chunk id leaked into the response:\n%s", text)
	}
	rc, isErr := c.tool("read_code", map[string]any{"node_id": m[1]})
	if isErr || !strings.Contains(rc, tail) {
		t.Fatalf("read_code on the section does not hold the matched word:\n%.400s", rc)
	}

	// The next move hit_line exists for: read around it, not the section.
	part, isErr := c.tool("read_code", map[string]any{
		"node_id": m[1], "lines": fmt.Sprintf("%d-%d", hit, hit+20),
	})
	if isErr || !strings.Contains(part, tail) || !strings.Contains(part, `kind="range"`) {
		t.Fatalf("read_code lines around hit_line does not hold the matched word:\n%.600s", part)
	}
	if len(part) >= len(rc)/2 {
		t.Fatalf("a 21-line range (%dB) is not much smaller than the section (%dB)", len(part), len(rc))
	}
}

// lines reads part of one node — the file node too, whose id is its path —
// and every misuse is refused out loud rather than guessed at.
func TestReadCodeLines(t *testing.T) {
	root := t.TempDir()
	copyTree(t, javaFixtures, root)
	writeFile(t, root, "docs/a.md", "# A\n\none\ntwo\nthree\n\n# B\n\nfour\n")
	c := newClient(t, root)
	c.bootstrapAndWait(root)

	text, isErr := c.tool("read_code", map[string]any{"node_id": "docs/a.md", "lines": "4-5"})
	if isErr || !strings.Contains(text, `lines="4-5"`) || !strings.Contains(text, "two\nthree") ||
		strings.Contains(text, "one") {
		t.Fatalf("file-node range read:\n%s", text)
	}
	text, isErr = c.tool("read_code", map[string]any{"node_id": "docs/a.md#a", "lines": "5-50"})
	if isErr || !strings.Contains(text, `lines="5-6"`) || strings.Contains(text, "# B") {
		t.Fatalf("a range is clipped to its node:\n%s", text)
	}

	for _, bad := range []map[string]any{
		{"node_id": "docs/a.md", "lines": "9-3"},
		{"node_id": "docs/a.md", "lines": "x"},
		{"node_id": "docs/a.md", "lines": "1-201"},
		{"node_ids": []string{"docs/a.md"}, "lines": "1-2"},
		{"node_id": "docs/a.md#b", "lines": "1-3"},
	} {
		if text, isErr := c.tool("read_code", bad); !isErr {
			t.Fatalf("%v should be refused:\n%s", bad, text)
		}
	}
}
