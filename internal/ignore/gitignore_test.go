package ignore

import "testing"

func TestBasicAndDirOnly(t *testing.T) {
	m := NewMatcher()
	m.AddPatterns("", []string{"*.log", "build/"})

	if !m.Ignored("a/b/debug.log", false) {
		t.Error("*.log should match at any depth")
	}
	if !m.Ignored("build", true) {
		t.Error("build/ should match the directory")
	}
	if m.Ignored("build", false) {
		t.Error("build/ must not match a plain file named build")
	}
	if !m.Ignored("sub/build/gen/Foo.java", false) {
		t.Error("contents under an ignored dir are ignored")
	}
}

func TestNegation(t *testing.T) {
	m := NewMatcher()
	m.AddPatterns("", []string{"*.log", "!important.log"})

	if m.Ignored("important.log", false) {
		t.Error("negation should re-include important.log")
	}
	if !m.Ignored("other.log", false) {
		t.Error("other.log stays ignored")
	}
}

func TestNoReincludeInsideExcludedDir(t *testing.T) {
	m := NewMatcher()
	m.AddPatterns("", []string{"secret/", "!secret/ok.txt"})
	if !m.Ignored("secret/ok.txt", false) {
		t.Error("git semantics: cannot re-include inside an excluded directory")
	}
}

func TestDoubleStar(t *testing.T) {
	m := NewMatcher()
	m.AddPatterns("", []string{"**/generated", "docs/**", "a/**/b"})

	if !m.Ignored("x/y/generated", true) {
		t.Error("**/generated should match at any depth")
	}
	if !m.Ignored("docs/api/index.md", false) {
		t.Error("docs/** should match contents")
	}
	if !m.Ignored("a/b", false) {
		t.Error("a/**/b matches zero middle segments")
	}
	if !m.Ignored("a/x/y/b", false) {
		t.Error("a/**/b matches deep middles")
	}
}

func TestAnchoredVsUnanchored(t *testing.T) {
	m := NewMatcher()
	m.AddPatterns("", []string{"/rooted.txt", "src/main.py"})

	if !m.Ignored("rooted.txt", false) {
		t.Error("/rooted.txt should match at root")
	}
	if m.Ignored("deep/rooted.txt", false) {
		t.Error("/rooted.txt must not match deeper")
	}
	if !m.Ignored("src/main.py", false) {
		t.Error("src/main.py anchored to root should match")
	}
	if m.Ignored("other/src/main.py", false) {
		t.Error("anchored pattern must not float")
	}
}

// TestNestedGitignorePrecedence: a nested .gitignore's rules are relative to
// its own directory and later rules win.
func TestNestedGitignorePrecedence(t *testing.T) {
	m := NewMatcher()
	m.AddFile("", []byte("*.tmp\n"))
	m.AddFile("sub", []byte("!keep.tmp\n"))

	if m.Ignored("sub/keep.tmp", false) {
		t.Error("nested negation should win for its subtree")
	}
	if !m.Ignored("keep.tmp", false) {
		t.Error("root files unaffected by nested negation")
	}
	if !m.Ignored("sub/other.tmp", false) {
		t.Error("non-negated files stay ignored")
	}
}
