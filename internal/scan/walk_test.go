package scan

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/llls2542/graphin/internal/obs"
)

func write(t *testing.T, root, rel, content string) {
	t.Helper()
	abs := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func rels(r *Result) []string {
	out := make([]string, len(r.Files))
	for i, f := range r.Files {
		out[i] = f.RelPath
	}
	return out
}

func TestWalkFilters(t *testing.T) {
	root := t.TempDir()
	write(t, root, "src/A.java", "class A {}")
	write(t, root, "src/b.py", "x = 1")
	write(t, root, "src/readme.md", "not source")
	write(t, root, "build/Gen.java", "class Gen {}")           // default exclude
	write(t, root, "node_modules/x/y.py", "x")                 // default exclude
	write(t, root, ".graphin/tmp.py", "x")                     // hard exclude
	write(t, root, "ignored/C.java", "class C {}")             // via .gitignore
	write(t, root, ".gitignore", "ignored/\n")
	write(t, root, "big/Big.java", "class B { "+strings.Repeat("int x;", 300000)+" }") // >1MB

	if err := os.Symlink(filepath.Join(root, "src"), filepath.Join(root, "link")); err != nil {
		t.Skip("symlinks unavailable:", err)
	}

	res, err := Walk(root, obs.Nop())
	if err != nil {
		t.Fatal(err)
	}
	got := rels(res)
	want := []string{"src/A.java", "src/b.py"}
	if len(got) != len(want) {
		t.Fatalf("files = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("files = %v, want %v", got, want)
		}
	}
}

func TestGraphinIgnoreFile(t *testing.T) {
	root := t.TempDir()
	write(t, root, "gen/G.java", "class G {}")
	write(t, root, "src/A.java", "class A {}")
	write(t, root, ".graphin/ignore", "gen/\n")

	res, err := Walk(root, obs.Nop())
	if err != nil {
		t.Fatal(err)
	}
	got := rels(res)
	if len(got) != 1 || got[0] != "src/A.java" {
		t.Fatalf("files = %v, want only src/A.java", got)
	}
}

func TestIndexableForWatcherEvents(t *testing.T) {
	root := t.TempDir()
	if Indexable(root, "build/Gen.java", nil) {
		t.Error("build/ must be rejected")
	}
	if Indexable(root, "src/readme.md", nil) {
		t.Error("non-source must be rejected")
	}
	if !Indexable(root, "src/A.java", nil) {
		t.Error("plain source must pass")
	}
}
