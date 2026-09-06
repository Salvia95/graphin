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

// Context is additive (docs/keyword-plan.md P1): asking for ±N fills Window and
// Context on the kept lines and changes nothing else — not which lines are
// kept, not Matches, not the Regions the SWE-Explore baseline submits.
func TestContextIsAdditiveAndDoesNotMoveRegions(t *testing.T) {
	root := fixture(t)
	plain, err := Search(root, Options{Terms: []string{"cancelpayment"}, MaxLines: 3, ContextLines: 1})
	if err != nil {
		t.Fatal(err)
	}
	base, _ := Search(root, Options{Terms: []string{"cancelpayment"}, ContextLines: 1})
	if len(plain) != len(base) || plain[0].Matches != base[0].Matches ||
		len(plain[0].Regions) != len(base[0].Regions) || plain[0].Regions[0] != base[0].Regions[0] {
		t.Fatalf("keeping lines moved the baseline: %+v vs %+v", plain[0], base[0])
	}
	// pay.go: line 3 comment, line 4 func, line 5 caller — three consecutive
	// matches. The first window is 2-4; the second starts after it (5-5, and
	// then the file's real last line is 5); the third has nothing left.
	ls := plain[0].Lines
	if len(ls) != 3 {
		t.Fatalf("kept %d lines, want 3", len(ls))
	}
	if ls[0].Window != (Region{Start: 2, End: 4}) || len(ls[0].Context) != 3 {
		t.Fatalf("first window = %+v (%d lines), want 2-4", ls[0].Window, len(ls[0].Context))
	}
	if ls[1].Window != (Region{Start: 5, End: 5}) || len(ls[1].Context) != 1 {
		t.Fatalf("second window = %+v, want 5-5 (starts after the first)", ls[1].Window)
	}
	if ls[2].Window != (Region{}) || ls[2].Context != nil {
		t.Fatalf("third window = %+v, want none (already covered)", ls[2].Window)
	}
	if !strings.HasPrefix(ls[0].Context[2], "func cancelPayment") {
		t.Fatalf("context is not the raw lines: %q", ls[0].Context)
	}

	none, _ := Search(root, Options{Terms: []string{"cancelpayment"}, MaxLines: 3})
	for _, ln := range none[0].Lines {
		if ln.Context != nil || ln.Window != (Region{}) {
			t.Fatalf("context filled without being asked: %+v", ln)
		}
	}
}
