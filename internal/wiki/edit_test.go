package wiki

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const setWithFront = `---
type: knowledge_set
description: 원래 카탈로그 줄
tags: [ops, release]
roles:
  - backend
  - ops
prerequisites: []
mode: pinned
---

# 운영

본문은 손대지 않는다.

## G

- [one](../../target.md#section-one) — 요약
`

// The property the console rests on is that the review is an ordinary diff. A
// writer that re-rendered the file would reformat every line the author chose
// and turn a one-line change into an unreviewable rewrite.
func TestEditSetFrontTouchesOnlyTheKeysAsked(t *testing.T) {
	root := t.TempDir()
	rel := filepath.Join(DirName, setsSubdir, "ops.md")
	mustWrite(t, filepath.Join(root, rel), setWithFront)

	desc := "짧게 다시 쓴 줄"
	if _, err := EditSetFront(root, "ops", SetFrontEdits{Description: &desc}); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(root, rel))

	if !strings.Contains(got, "description: 짧게 다시 쓴 줄") {
		t.Errorf("description not rewritten:\n%s", got)
	}
	for _, keep := range []string{
		"tags: [ops, release]", "  - backend", "  - ops", "mode: pinned",
		"# 운영", "본문은 손대지 않는다.", "- [one](../../target.md#section-one) — 요약",
	} {
		if !strings.Contains(got, keep) {
			t.Errorf("lost %q:\n%s", keep, got)
		}
	}
	if strings.Contains(got, "원래 카탈로그 줄") {
		t.Errorf("old description survived:\n%s", got)
	}
}

// Demoting is emptying the push list: the set drops from every delegation of
// that role down to task matching, and stays reachable.
func TestEditSetFrontCanEmptyTheRoles(t *testing.T) {
	root := t.TempDir()
	rel := filepath.Join(DirName, setsSubdir, "ops.md")
	mustWrite(t, filepath.Join(root, rel), setWithFront)

	none := []string{}
	if _, err := EditSetFront(root, "ops", SetFrontEdits{Roles: &none}); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(root, rel))
	if !strings.Contains(got, "roles: []") || strings.Contains(got, "  - backend") {
		t.Errorf("roles not emptied:\n%s", got)
	}
	// A nil field is not the same as an empty one, and the description was nil.
	if !strings.Contains(got, "description: 원래 카탈로그 줄") {
		t.Errorf("an unasked field changed:\n%s", got)
	}

	store, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if got := store.SetList()[0]; len(got.Roles) != 0 {
		t.Errorf("reparsed roles = %v, want none", got.Roles)
	}
}

func TestEditSetFrontRefusesAnUnknownSet(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, DirName, setsSubdir, "ops.md"), setWithFront)
	d := "x"
	if _, err := EditSetFront(root, "nope", SetFrontEdits{Description: &d}); err == nil {
		t.Fatal("want an error for a set that does not exist")
	}
}

// Deprecated is still served on purpose, so no status frees a slot under the
// cap. Retiring is a deletion, and the diff is what makes it reversible.
func TestRetireTermFreesASlot(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, DirName, setsSubdir, "s.md"), "# S\n")
	mustWrite(t, GlossaryPath(root, "drift"),
		"---\ntype: glossary\ncanonical: drift\nstatus: stable\n---\n\n본문.\n")

	store, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(store.Terms) != 1 {
		t.Fatalf("terms = %d, want 1", len(store.Terms))
	}

	rel, err := RetireTerm(root, "drift")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(rel, "drift.md") {
		t.Errorf("path = %q", rel)
	}
	if store, err = Load(root); err != nil {
		t.Fatal(err)
	}
	if len(store.Terms) != 0 {
		t.Errorf("terms = %d after retiring, want 0", len(store.Terms))
	}
}

func TestRetireTermIsNotFoundWhenThereIsNone(t *testing.T) {
	if _, err := RetireTerm(t.TempDir(), "nothing"); err != ErrNoTerm {
		t.Fatalf("err = %v, want ErrNoTerm", err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// A one-key change should read as a one-line diff. Rewriting `tags: [a, b]`
// into block form makes it look like the header was rewritten.
func TestEditSetFrontKeepsTheListShapeTheAuthorUsed(t *testing.T) {
	root := t.TempDir()
	rel := filepath.Join(DirName, setsSubdir, "ops.md")
	mustWrite(t, filepath.Join(root, rel),
		"---\ntype: knowledge_set\ndescription: x\nroles: [backend, ops]\nmode: live\n---\n\n# S\n")

	one := []string{"backend"}
	if _, err := EditSetFront(root, "ops", SetFrontEdits{Roles: &one}); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(root, rel))
	if !strings.Contains(got, "roles: [backend]") {
		t.Errorf("inline list not preserved:\n%s", got)
	}

	// The block form stays block.
	mustWrite(t, filepath.Join(root, rel),
		"---\ntype: knowledge_set\ndescription: x\nroles:\n  - backend\n  - ops\nmode: live\n---\n\n# S\n")
	if _, err := EditSetFront(root, "ops", SetFrontEdits{Roles: &one}); err != nil {
		t.Fatal(err)
	}
	got = readFile(t, filepath.Join(root, rel))
	if !strings.Contains(got, "roles:\n  - backend\nmode: live") {
		t.Errorf("block list not preserved:\n%s", got)
	}
}
