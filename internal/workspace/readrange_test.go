package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// rangeDoc: line 1 is a preamble, the section "# A" is lines 2-7, and "# B"
// starts on line 8 — so a range read on A that runs past line 7 must stop
// there rather than hand back B's heading.
const rangeDoc = "preamble\n# A\n\na1\na2\na3\n\n# B\n\nb1\n"

func TestReadRangeClipsToTheNode(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "doc.md"), []byte(rangeDoc), 0o644); err != nil {
		t.Fatal(err)
	}
	w := bootstrapDocs(t, root)
	defer w.Close()

	cases := []struct {
		id       string
		from, to int
		want     string
		lo, hi   int
	}{
		{"doc.md#a", 4, 5, "a1\na2", 4, 5},
		{"doc.md#a", 5, 5, "a2", 5, 5},
		// Past the node on both sides: clipped to the section's own lines.
		{"doc.md#a", 1, 100, "# A\n\na1\na2\na3\n", 2, 7},
		// The file node spans the whole file, so its range is the file's.
		{"doc.md", 8, 10, "# B\n\nb1", 8, 10},
		{"doc.md", 9, 500, "\nb1", 9, 10},
	}
	for _, c := range cases {
		cb, err := w.ReadRange(c.id, c.from, c.to)
		if err != nil {
			t.Fatalf("ReadRange(%s, %d, %d): %v", c.id, c.from, c.to, err)
		}
		if cb.Code != c.want || cb.StartLine != c.lo || cb.EndLine != c.hi || !cb.Range {
			t.Fatalf("ReadRange(%s, %d, %d) = lines %d-%d %q range=%t, want lines %d-%d %q",
				c.id, c.from, c.to, cb.StartLine, cb.EndLine, cb.Code, cb.Range, c.lo, c.hi, c.want)
		}
	}

	if _, err := w.ReadRange("doc.md#a", 8, 10); !errors.Is(err, ErrRangeOutside) {
		t.Fatalf("a range past the node must be refused, got %v", err)
	}
	if _, err := w.ReadRange("doc.md#nope", 1, 2); !errors.Is(err, ErrNodeNotFound) {
		t.Fatalf("unknown node: got %v", err)
	}
}

// A range is asked for in lines, so a node that starts mid-line still yields
// that whole first line — unlike ReadCode, which starts at the node's byte.
func TestReadRangeIsLineAligned(t *testing.T) {
	root := t.TempDir()
	src := "package p\n\nvar x = 1; func F() int {\n\treturn x\n}\n"
	if err := os.WriteFile(filepath.Join(root, "p.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	w := bootstrapDocs(t, root)
	defer w.Close()

	var id string
	w.nodesMu.RLock()
	for k := range w.nodes {
		if strings.HasSuffix(k, "F") {
			id = k
		}
	}
	w.nodesMu.RUnlock()
	if id == "" {
		t.Fatal("no node for F")
	}
	whole, err := w.ReadCode(id)
	if err != nil || !strings.HasPrefix(whole.Code, "func F") {
		t.Fatalf("ReadCode(%s) = %+v, %v", id, whole, err)
	}
	cb, err := w.ReadRange(id, 1, 3)
	if err != nil {
		t.Fatal(err)
	}
	if cb.StartLine != 3 || cb.EndLine != 3 || cb.Code != "var x = 1; func F() int {" {
		t.Fatalf("got lines %d-%d %q", cb.StartLine, cb.EndLine, cb.Code)
	}
}

// An edit since indexing is picked up the same way ReadCode picks it up:
// reparse inline, then read the lines from the fresh bytes.
func TestReadRangeReparsesAnEditedFile(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "doc.md")
	if err := os.WriteFile(p, []byte(rangeDoc), 0o644); err != nil {
		t.Fatal(err)
	}
	w := bootstrapDocs(t, root)
	defer w.Close()

	if err := os.WriteFile(p, []byte(strings.Replace(rangeDoc, "a2", "a2 edited", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	cb, err := w.ReadRange("doc.md#a", 5, 5)
	if err != nil {
		t.Fatal(err)
	}
	if cb.Code != "a2 edited" || !cb.Reparsed {
		t.Fatalf("got %q reparsed=%t", cb.Code, cb.Reparsed)
	}
}
