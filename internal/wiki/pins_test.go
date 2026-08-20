package wiki

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// tempWiki builds a workspace holding one target document and one set that
// cites a section of it.
func tempWiki(t *testing.T, doc string) (root string, sets []*Set) {
	t.Helper()
	root = t.TempDir()
	mustWrite(t, filepath.Join(root, "docs", "target.md"), doc)

	setSrc := "# S\n\n## G\n\n- [sec](../../target.md#section-one) — a summary\n"
	rel := filepath.Join(DirName, setsSubdir, "s.md")
	mustWrite(t, filepath.Join(root, rel), setSrc)

	s, err := ParseSet("docs/wiki/sets/s.md", []byte(setSrc))
	if err != nil {
		t.Fatal(err)
	}
	return root, []*Set{s}
}

func mustWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

const targetDoc = "# Target\n\n## Section one\n\nOriginal body.\n"

func TestCheckReportsUnpinnedThenClean(t *testing.T) {
	root, sets := tempWiki(t, targetDoc)

	// Before any admission every entry is unpinned: drift cannot be detected
	// for an entry whose original state was never recorded.
	probs := Check(root, sets, NewPins())
	if len(probs) != 1 || probs[0].Kind != ProblemUnpinned {
		t.Fatalf("problems = %v, want one unpinned", probs)
	}

	pins, bad := Repin(root, sets)
	if len(bad) != 0 {
		t.Fatalf("repin problems = %v", bad)
	}
	if probs := Check(root, sets, pins); len(probs) != 0 {
		t.Fatalf("problems after repin = %v, want none", probs)
	}
}

func TestCheckDetectsDrift(t *testing.T) {
	root, sets := tempWiki(t, targetDoc)
	pins, _ := Repin(root, sets)

	// The heading still resolves, so nothing is dangling — but the sentence
	// in the set may now describe text that is gone.
	mustWrite(t, filepath.Join(root, "docs", "target.md"),
		"# Target\n\n## Section one\n\nRewritten body.\n")

	probs := Check(root, sets, pins)
	if len(probs) != 1 || probs[0].Kind != ProblemDrift {
		t.Fatalf("problems = %v, want one drift", probs)
	}
}

func TestCheckDetectsDanglingAfterRename(t *testing.T) {
	root, sets := tempWiki(t, targetDoc)
	pins, _ := Repin(root, sets)

	// Renaming a heading breaks every entry pointing at it, and does so
	// without touching the set — the failure this guard exists to surface.
	mustWrite(t, filepath.Join(root, "docs", "target.md"),
		"# Target\n\n## Section one renamed\n\nOriginal body.\n")

	probs := Check(root, sets, pins)
	if len(probs) != 1 || probs[0].Kind != ProblemDangling {
		t.Fatalf("problems = %v, want one dangling", probs)
	}
}

func TestRepinSkipsDanglingRatherThanRecordingNothing(t *testing.T) {
	root, sets := tempWiki(t, "# Target\n\n## Elsewhere\n\nBody.\n")
	pins, probs := Repin(root, sets)
	if len(probs) != 1 || probs[0].Kind != ProblemDangling {
		t.Fatalf("problems = %v, want one dangling", probs)
	}
	// Pinning a broken ID would hide the break behind a green check.
	if len(pins.Pins["s"]) != 0 {
		t.Fatalf("pinned a dangling entry: %v", pins.Pins)
	}
}

func TestCheckFlagsMissingSummary(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "docs", "target.md"), targetDoc)
	src := "# S\n\n## G\n\n- [sec](../../target.md#section-one)\n"
	s, _ := ParseSet("docs/wiki/sets/s.md", []byte(src))

	pins, _ := Repin(root, []*Set{s})
	probs := Check(root, []*Set{s}, pins)
	if len(probs) != 1 || probs[0].Kind != ProblemNoSummary {
		t.Fatalf("problems = %v, want one no-summary", probs)
	}
}

func TestPinsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, PinsFile)

	// A workspace with sets and no lockfile is the normal pre-admission
	// state, not an error.
	p, err := LoadPins(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Pins) != 0 {
		t.Fatalf("fresh pins = %v", p.Pins)
	}

	p.Set("s", "docs/a.md#b", Pin{Hash: "b3:deadbeef", Rename: "b3:feedface"})
	if err := p.Save(path); err != nil {
		t.Fatal(err)
	}
	back, err := LoadPins(path)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := back.Get("s", "docs/a.md#b")
	if !ok || got.Hash != "b3:deadbeef" {
		t.Fatalf("Get = %+v ok=%v", got, ok)
	}
	// The rename key has to survive the round trip too, or a retitle after a
	// restart reads as a rewrite.
	if got.Rename != "b3:feedface" {
		t.Errorf("Rename = %q", got.Rename)
	}
}

func TestFormatHashPrefix(t *testing.T) {
	var h [32]byte
	h[0] = 0xab
	got := FormatHash(h)
	if !strings.HasPrefix(got, "b3:ab") || len(got) != 3+64 {
		t.Fatalf("FormatHash = %q", got)
	}
}
