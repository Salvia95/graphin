package wiki

import "testing"

func TestSplitFrontmatter(t *testing.T) {
	tests := []struct {
		name     string
		src      string
		wantFM   string
		wantBody string
	}{
		{"none", "# Title\n", "", "# Title\n"},
		{"simple", "---\na: b\n---\n# Title\n", "a: b\n", "# Title\n"},
		{"empty block", "---\n---\nbody", "", "body"},
		// An unterminated block is body, not a truncated header: guessing
		// where it ended would drop fields the author believed were saved.
		{"unterminated", "---\na: b\nstill going", "", "---\na: b\nstill going"},
		// `---` inside the body is a horizontal rule, not a second delimiter.
		{"rule in body", "---\na: b\n---\ntext\n---\nmore\n", "a: b\n", "text\n---\nmore\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fm, body := splitFrontmatter([]byte(tc.src))
			if string(fm) != tc.wantFM {
				t.Errorf("frontmatter = %q, want %q", fm, tc.wantFM)
			}
			if string(body) != tc.wantBody {
				t.Errorf("body = %q, want %q", body, tc.wantBody)
			}
		})
	}
}

func TestParseFrontmatter(t *testing.T) {
	fm := parseFrontmatter([]byte(`
mode: pinned
quoted: "keeps spaces"
roles: [backend, frontend]
empty_inline: []
prerequisites:
  - basics
  - versioning
declared_empty:
# a comment
`))
	if got := fm.Get("mode"); got != "pinned" {
		t.Errorf("mode = %q", got)
	}
	if got := fm.Get("quoted"); got != "keeps spaces" {
		t.Errorf("quoted = %q", got)
	}
	if got := fm.List("roles"); len(got) != 2 || got[0] != "backend" || got[1] != "frontend" {
		t.Errorf("roles = %v", got)
	}
	if got := fm.List("prerequisites"); len(got) != 2 || got[0] != "basics" {
		t.Errorf("prerequisites = %v", got)
	}
	// An explicitly empty list must stay distinguishable from an absent key:
	// "no roles" is a statement, "not applicable" is not.
	if got, ok := fm.Lists["empty_inline"]; !ok || len(got) != 0 {
		t.Errorf("empty_inline = %v, ok=%v", got, ok)
	}
	if got, ok := fm.Lists["declared_empty"]; !ok || len(got) != 0 {
		t.Errorf("declared_empty = %v, ok=%v", got, ok)
	}
	if got := fm.List("absent"); got != nil {
		t.Errorf("absent = %v, want nil", got)
	}
}

func TestFrontmatterScalarReadsAsList(t *testing.T) {
	// `roles: backend` and `roles: [backend]` mean the same thing to an
	// author, so they must mean the same thing to the reader.
	fm := parseFrontmatter([]byte("roles: backend\n"))
	got := fm.List("roles")
	if len(got) != 1 || got[0] != "backend" {
		t.Fatalf("List = %v, want [backend]", got)
	}
}
