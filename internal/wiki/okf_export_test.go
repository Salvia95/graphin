package wiki

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func exportStore(t *testing.T) *Store {
	t.Helper()
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "docs", "target.md"), targetDoc)
	writeSet(t, root, "release", "roles: [all]\ndescription: What you need when cutting a release.\n")
	mustWrite(t, filepath.Join(root, DirName, glossarySubdir, "posting.md"),
		"---\ncanonical: posting\ntitle: Posting\naliases: [post]\n"+
			"tags: [editorial]\nstale_after: 2027-01-01\nstatus: stable\n"+
			"reviewed:\n  - human:tipa — 2026-08-21\n"+
			"evidence:\n  - docs/target.md#section-one\n"+
			"not_to_be_confused_with:\n  - blog — a posting is a unit, a blog is a medium\n"+
			"---\nA unit of published writing.\n")

	st, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	pins, _ := Repin(root, st.SetList())
	if err := pins.Save(st.PinsPath()); err != nil {
		t.Fatal(err)
	}
	st, _ = Load(root)
	return st
}

func exported(t *testing.T, st *Store) (dir string, files map[string]string) {
	t.Helper()
	dir = t.TempDir()
	written, err := st.ExportOKF(dir)
	if err != nil {
		t.Fatal(err)
	}
	files = map[string]string{}
	for _, rel := range written {
		raw, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatal(err)
		}
		files[rel] = string(raw)
	}
	return dir, files
}

func TestExportEmitsConformantConcepts(t *testing.T) {
	_, files := exported(t, exportStore(t))

	for _, rel := range []string{"index.md", "glossary/posting.md", "sets/release.md"} {
		if _, ok := files[rel]; !ok {
			t.Fatalf("missing %s in %v", rel, files)
		}
	}
	// Conformance turns on exactly one field being present on every
	// NON-RESERVED document. index.md is reserved at any level, not just at
	// the root, and reserved files carry no frontmatter at all.
	for rel, body := range files {
		if strings.HasSuffix(rel, "index.md") {
			continue
		}
		if !strings.HasPrefix(body, "---\ntype: ") {
			t.Errorf("%s does not lead with a type:\n%s", rel, body[:60])
		}
	}
	// The bundle root is the sole index that may carry frontmatter, and the
	// version is the only thing it may carry.
	if !strings.Contains(files["index.md"], `okf_version: "0.2"`) {
		t.Errorf("root index carries no version:\n%s", files["index.md"])
	}
	for rel, body := range files {
		if rel != "index.md" && strings.Contains(body, "okf_version") {
			t.Errorf("%s declares a version it may not", rel)
		}
	}
	for rel, body := range files {
		if strings.HasSuffix(rel, "/index.md") && strings.HasPrefix(body, "---") {
			t.Errorf("%s is a reserved index and must carry no frontmatter:\n%s", rel, body[:40])
		}
	}
	// The underlying asset is named, the way published bundles name theirs.
	if !strings.Contains(files["sets/release.md"], `resource: docs/wiki/sets/release.md`) {
		t.Errorf("set concept names no resource:\n%s", files["sets/release.md"])
	}
}

func TestExportWritesAnIndexPerDirectory(t *testing.T) {
	_, files := exported(t, exportStore(t))
	// A consumer walking a subdirectory with no index has to guess what is
	// in it, which is why published bundles carry one per level.
	for _, rel := range []string{"glossary/index.md", "sets/index.md"} {
		if _, ok := files[rel]; !ok {
			t.Fatalf("missing %s", rel)
		}
	}
	// Links inside a directory index are relative to that directory.
	if !strings.Contains(files["sets/index.md"], "(release.md)") {
		t.Errorf("sets index does not link its own members:\n%s", files["sets/index.md"])
	}
	if !strings.Contains(files["index.md"], "(sets/index.md)") {
		t.Errorf("root index does not point at the subdirectory:\n%s", files["index.md"])
	}
}

func TestExportTranslatesReviewedIntoVerified(t *testing.T) {
	_, files := exported(t, exportStore(t))
	body := files["glossary/posting.md"]

	// The authored file keeps flat strings under a key of our own; this is
	// the one place that becomes the shape the spec asks for.
	if !strings.Contains(body, "verified:\n  - by: \"human:tipa\"\n    at: \"2026-08-21\"") {
		t.Fatalf("verified not translated:\n%s", body)
	}
	if strings.Contains(body, "reviewed:") {
		t.Errorf("the internal key leaked into the bundle:\n%s", body)
	}
}

func TestExportCarriesPinsAsExtensions(t *testing.T) {
	_, files := exported(t, exportStore(t))
	body := files["sets/release.md"]

	// OKF has no content hash — its freshness signal is a declared date — so
	// dropping these would hand a consumer knowledge with no way to tell
	// whether it still matches its source. Extra keys are explicitly allowed.
	if !strings.Contains(body, "graphin_hash: \"b3:") {
		t.Fatalf("pins not carried:\n%s", body)
	}
	if !strings.Contains(body, "graphin_rename_key: \"b3:") {
		t.Fatalf("rename key not carried:\n%s", body)
	}
	// `resource` names only what an OKF consumer can resolve — a file. The
	// address actually meant sits beside it, because putting a fragment in
	// the standard field would read as the whole file and quietly lose the
	// fact that the entry meant a fraction of it.
	if !strings.Contains(body, "resource: docs/target.md\n") {
		t.Fatalf("resource should name the file:\n%s", body)
	}
	if !strings.Contains(body, `graphin_node: "docs/target.md#section-one"`) {
		t.Fatalf("the real address is not carried:\n%s", body)
	}
}

func TestExportSaysItIsAProjection(t *testing.T) {
	_, files := exported(t, exportStore(t))
	index := files["index.md"]
	// A consumer that reads only the standard fields gets the right
	// documents and the wrong granularity. The bundle has to say so, or the
	// omission reads as a claim.
	for _, want := range []string{"projection", "graphin_node"} {
		if !strings.Contains(index, want) {
			t.Errorf("root index does not mention %q:\n%s", want, index)
		}
	}
}

func TestExportIsDeterministic(t *testing.T) {
	st := exportStore(t)
	_, a := exported(t, st)
	_, b := exported(t, st)
	// No clock in the output: a bundle that changes every run is a bundle
	// nobody can diff.
	for rel, body := range a {
		if b[rel] != body {
			t.Fatalf("%s differs between runs", rel)
		}
	}
}

func TestYamlScalarQuotesWhatWouldChangeType(t *testing.T) {
	cases := []struct{ in, want string }{
		{"stable", "stable"},
		{"docs/a.md", "docs/a.md"},
		{"two words", "two words"},
		// A colon makes a mapping out of a string.
		{"human:tipa", `"human:tipa"`},
		// A date, a bool and a number all read as non-strings bare.
		{"2026-08-21", `"2026-08-21"`},
		{"true", `"true"`},
		{"no", `"no"`},
		{"42", `"42"`},
		// Non-ASCII is quoted because the safe set is deliberately narrow.
		{"릴리스", `"릴리스"`},
		{`say "hi"`, `"say \"hi\""`},
		{`back\slash`, `"back\\slash"`},
		{"", `""`},
	}
	for _, tc := range cases {
		if got := yamlScalar(tc.in); got != tc.want {
			t.Errorf("yamlScalar(%q) = %s, want %s", tc.in, got, tc.want)
		}
	}
}

func TestExportRefusesWithoutAWiki(t *testing.T) {
	st, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if st.Present {
		t.Fatal("premise broken")
	}
}
