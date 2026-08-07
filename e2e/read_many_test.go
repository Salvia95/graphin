package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Salvia95/graphin/internal/mcp"
)

// mkMarkdownWorkspace writes a document whose sections are large enough that
// only some of them fit one response.
func mkMarkdownWorkspace(t *testing.T, sectionKB int, n int) string {
	t.Helper()
	root := t.TempDir()
	var sb strings.Builder
	sb.WriteString("서문\n\n")
	filler := strings.Repeat("가나다라마바사아자차 ", 100) // ~3KB per repeat block
	for i := 1; i <= n; i++ {
		sb.WriteString("## 섹션 ")
		sb.WriteByte(byte('0' + i))
		sb.WriteString("\n\n")
		for k := 0; k < sectionKB; k++ {
			sb.WriteString(filler)
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}
	if err := os.WriteFile(filepath.Join(root, "doc.md"), []byte(sb.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

// A multi-node read must return whole nodes and account for every id it did
// not return (docs/markdown-spec.md §4). Cutting inside a section would hand
// back half of something the caller had already decided to read.
func TestReadCodeManyNeverCutsInsideANode(t *testing.T) {
	root := mkMarkdownWorkspace(t, 2, 5)
	c := newClient(t, root)
	c.bootstrapAndWait(root)

	ids := []string{
		"doc.md#섹션-1", "doc.md#섹션-2", "doc.md#섹션-3",
		"doc.md#섹션-4", "doc.md#섹션-5",
	}
	text, isErr := c.tool("read_code", map[string]any{"node_ids": ids})
	if isErr {
		t.Fatalf("read_code: %s", text)
	}

	// The response fits the budget on its own — Truncate must never fire and
	// therefore never cut inside a CDATA body.
	if len(text) > mcp.MaxResponseBytes {
		t.Fatalf("response is %d bytes, over the %d budget", len(text), mcp.MaxResponseBytes)
	}
	if strings.Contains(text, "<truncated") {
		t.Fatal("multi-node read fell back to byte truncation")
	}

	// Every requested id is either returned or named as omitted.
	for _, id := range ids {
		if !strings.Contains(text, id) {
			t.Fatalf("id %s vanished from the response:\n%s", id, text)
		}
	}
	if !strings.Contains(text, `reason="budget"`) {
		t.Fatalf("nothing was reported as dropped, so this case proves nothing:\n%s", text)
	}

	// Prefix semantics: the returned blocks are the first ones asked for.
	first := strings.Index(text, `<code_block id="doc.md#섹션-1"`)
	if first < 0 {
		t.Fatalf("the first requested node must always come back:\n%s", text)
	}
	// …and each block that is present carries a complete CDATA body.
	if strings.Count(text, "<![CDATA[") != strings.Count(text, "]]>") {
		t.Fatal("unbalanced CDATA: a body was cut")
	}
}

func TestReadCodeManyReportsUnknownIDsPerNode(t *testing.T) {
	root := mkMarkdownWorkspace(t, 1, 2)
	c := newClient(t, root)
	c.bootstrapAndWait(root)

	text, isErr := c.tool("read_code", map[string]any{
		"node_ids": []string{"doc.md#섹션-1", "doc.md#없는-섹션"},
	})
	// One bad id must not fail the whole call: the good one still comes back.
	if isErr {
		t.Fatalf("a single unknown id must not fail the call: %s", text)
	}
	if !strings.Contains(text, `<code_block id="doc.md#섹션-1"`) {
		t.Fatalf("good node missing:\n%s", text)
	}
	if !strings.Contains(text, `id="doc.md#없는-섹션" reason="not_found"`) {
		t.Fatalf("unknown node not accounted for:\n%s", text)
	}
}

func TestReadCodeSingleNodeContractUnchanged(t *testing.T) {
	root := mkMarkdownWorkspace(t, 1, 1)
	c := newClient(t, root)
	c.bootstrapAndWait(root)

	text, isErr := c.tool("read_code", map[string]any{"node_id": "doc.md#섹션-1"})
	if isErr {
		t.Fatalf("read_code: %s", text)
	}
	// Still a bare <code_block>, not wrapped in <code_blocks>. (A
	// <system_status> prefix may precede it while indexing finishes; that is
	// the pre-existing shape and not part of what changed here.)
	if strings.Contains(text, "<code_blocks") {
		t.Fatalf("single-node response gained the multi-node wrapper:\n%s", text)
	}
	if n := strings.Count(text, "<code_block "); n != 1 {
		t.Fatalf("want exactly one code_block, got %d:\n%s", n, text)
	}
	if strings.Contains(text, "<omitted") {
		t.Fatalf("single-node response gained an omission list:\n%s", text)
	}
}

func TestReadCodeRejectsAmbiguousArgs(t *testing.T) {
	root := mkMarkdownWorkspace(t, 1, 1)
	c := newClient(t, root)
	c.bootstrapAndWait(root)

	text, isErr := c.tool("read_code", map[string]any{
		"node_id": "doc.md#섹션-1", "node_ids": []string{"doc.md#섹션-1"},
	})
	if !isErr || !strings.Contains(text, "not both") {
		t.Fatalf("passing both must be rejected: %s", text)
	}
	text, isErr = c.tool("read_code", map[string]any{})
	if !isErr || !strings.Contains(text, "required") {
		t.Fatalf("passing neither must be rejected: %s", text)
	}
}
