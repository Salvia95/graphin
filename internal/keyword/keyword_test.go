package keyword

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("src/pay.go", "package src\n\n// cancelPayment refunds\nfunc cancelPayment() {}\nfunc other() { cancelPayment() }\n")
	write("src/order.go", "package src\n\nfunc cancel() {}\n")
	write("docs/notes.md", "# Notes\n\ncancelPayment is described here.\n")
	return root
}

func TestSearchRanksByMatchCount(t *testing.T) {
	root := fixture(t)
	hits, err := Search(root, Options{Terms: []string{"cancelpayment"}, MaxLines: 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 2 {
		t.Fatalf("files = %d, want 2 (pay.go, notes.md)", len(hits))
	}
	if hits[0].RelPath != "src/pay.go" || hits[0].Matches != 3 {
		t.Fatalf("top = %s with %d matches, want src/pay.go with 3", hits[0].RelPath, hits[0].Matches)
	}
}

// The byte offset is what turns a text hit into a node id, so it has to point
// at the line the match is on — not at the file, and not one line off.
func TestMatchedLineCarriesItsOwnOffset(t *testing.T) {
	root := fixture(t)
	hits, err := Search(root, Options{Terms: []string{"func cancelpayment"}, MaxLines: 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) == 0 || len(hits[0].Lines) == 0 {
		t.Fatal("no matched lines")
	}
	src, err := os.ReadFile(filepath.Join(root, hits[0].RelPath))
	if err != nil {
		t.Fatal(err)
	}
	ln := hits[0].Lines[0]
	if got := string(src[ln.Byte:]); !strings.HasPrefix(got, "func cancelPayment") {
		t.Fatalf("offset %d points at %q", ln.Byte, got[:min(30, len(got))])
	}
}

func TestRegexIsOptInAndCaseInsensitive(t *testing.T) {
	root := fixture(t)
	// As a literal the dot is a dot, so it matches nothing; as a regex it
	// stands for the capital P and the name is found.
	lit, err := Compile("cancel.ayment", false)
	if err != nil {
		t.Fatal(err)
	}
	lit.MaxLines = 1
	if hits, _ := Search(root, lit); len(hits) != 0 {
		t.Fatalf("literal matched %d files, want 0", len(hits))
	}
	re, err := Compile("cancel.ayment", true)
	if err != nil {
		t.Fatal(err)
	}
	re.MaxLines = 1
	if hits, _ := Search(root, re); len(hits) == 0 {
		t.Fatal("regex matched nothing")
	}
}

func TestPathFilterAndFileCap(t *testing.T) {
	root := fixture(t)
	hits, err := Search(root, Options{Terms: []string{"cancel"}, PathContains: "docs/", MaxLines: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].RelPath != "docs/notes.md" {
		t.Fatalf("hits = %v, want only docs/notes.md", hits)
	}
	capped, _ := Search(root, Options{Terms: []string{"cancel"}, MaxFiles: 1, MaxLines: 1})
	if len(capped) != 1 {
		t.Fatalf("MaxFiles=1 returned %d", len(capped))
	}
}

// The merged ±context window is what the SWE-Explore grep baseline submits;
// it has to stay in 1-based inclusive lines.
func TestContextWindowsMerge(t *testing.T) {
	root := fixture(t)
	hits, err := Search(root, Options{Terms: []string{"cancelpayment"}, ContextLines: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) == 0 || len(hits[0].Regions) != 1 {
		t.Fatalf("regions = %v, want one merged window", hits[0].Regions)
	}
	if r := hits[0].Regions[0]; r.Start != 2 || r.End < 5 {
		t.Fatalf("window = %v, want it to start at line 2 and cover the matches", r)
	}
}
