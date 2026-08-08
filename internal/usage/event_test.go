package usage

import "testing"

func TestClassifyMCPSuffixIgnoresServerSegment(t *testing.T) {
	// The middle segment is the user's config key — any key must match.
	for _, tool := range []string{
		"mcp__graphin__search_hybrid",
		"mcp__mykey__search_hybrid",
		"mcp__some-other.name__search_hybrid",
	} {
		if c := Classify(tool, nil); c != ClassGSearch {
			t.Fatalf("Classify(%q) = %s, want g_search", tool, c)
		}
	}
	if c := Classify("mcp__k__explore_graph", nil); c != ClassGExplore {
		t.Fatalf("explore_graph: got %s", c)
	}
	if c := Classify("mcp__k__read_code", nil); c != ClassGRead {
		t.Fatalf("read_code: got %s", c)
	}
	if c := Classify("mcp__k__bootstrap_workspace", nil); c != ClassGBoot {
		t.Fatalf("bootstrap_workspace: got %s", c)
	}
}

func TestClassifyBuiltins(t *testing.T) {
	cases := map[string]Class{
		"Grep": ClassSearch, "Glob": ClassSearch,
		"Read": ClassRead, "Edit": ClassAction, "Write": ClassAction,
		"WebFetch": ClassOther, "Task": ClassOther,
	}
	for tool, want := range cases {
		if c := Classify(tool, nil); c != want {
			t.Fatalf("Classify(%q) = %s, want %s", tool, c, want)
		}
	}
}

func TestClassifyBashBySearchFlag(t *testing.T) {
	if c := Classify("Bash", map[string]any{"search": true}); c != ClassSearch {
		t.Fatalf("search Bash: got %s", c)
	}
	if c := Classify("Bash", map[string]any{"search": false}); c != ClassOther {
		t.Fatalf("non-search Bash: got %s", c)
	}
	if c := Classify("Bash", nil); c != ClassOther {
		t.Fatalf("payload-less Bash: got %s", c)
	}
}

func TestClassifyUnknownMCPToolIsOther(t *testing.T) {
	if c := Classify("mcp__other__some_tool", nil); c != ClassOther {
		t.Fatalf("foreign MCP tool: got %s", c)
	}
}

func TestIsGraphinNav(t *testing.T) {
	for _, c := range []Class{ClassGSearch, ClassGExplore, ClassGRead} {
		if !c.IsGraphinNav() {
			t.Fatalf("%s should be nav", c)
		}
	}
	for _, c := range []Class{ClassGBoot, ClassGBench, ClassSearch, ClassRead} {
		if c.IsGraphinNav() {
			t.Fatalf("%s should not be nav", c)
		}
	}
}

func TestTokensSplitsCamelAndSnake(t *testing.T) {
	got := Tokens("persistIndexesLocked merkle_json")
	for _, want := range []string{"persist", "indexes", "locked", "merkle", "json"} {
		if !got[want] {
			t.Fatalf("Tokens missing %q: %v", want, got)
		}
	}
	if got["the"] || got["fo"] {
		t.Fatalf("stopword/short token leaked: %v", got)
	}
}

// The cases are the real patterns from the 2026-08-07 adoption log, bucketed
// the way the diagnosis bucketed them by hand (findings §검색의 형태).
func TestPatternShape(t *testing.T) {
	for _, tc := range []struct {
		p    string
		want Shape
	}{
		// symbol — snake, screaming, camel, pascal, and one leading keyword
		{"CONTEXT_TYPES", ShapeSymbol},
		{"validate_and_seal", ShapeSymbol},
		{"periodField", ShapeSymbol},
		{"ContextObject", ShapeSymbol},
		{"class PublishBody", ShapeSymbol},
		{"def _context_type_findings", ShapeSymbol},
		{`codebase_path\`, ShapeSymbol}, // trailing shell escape residue
		{`"headRef"`, ShapeSymbol},      // quoted by the shell
		{"DATE_RE", ShapeSymbol},        // screaming caps with underscore
		{"HTTPServer", ShapeSymbol},     // adjacent capitals still mixed case

		// regex/glob — a metacharacter anywhere decides it
		{"^ *$", ShapeRegex},
		{"*.go", ShapeRegex},
		{"^###", ShapeRegex},
		{`^#\+`, ShapeRegex},
		{"<title>[^<]*</title>", ShapeRegex},
		{"cancelOrder|OrderCancellation", ShapeRegex},

		// literal/prose — including the runtime-debugging greps that made the
		// old denominator lie, and bare lowercase words we refuse to guess on
		{"database is locked", ShapeLiteral},
		{"필수 컨텍스트 미충족", ShapeLiteral},
		{"sqlalchemy.exc", ShapeLiteral}, // '.' is not a metacharacter, but this is not an identifier either
		{"engineering/61.md", ShapeLiteral},
		{"admin", ShapeLiteral},
		{"E", ShapeLiteral}, // single capital: too short to be screaming caps

		{"", ShapeNone},
		{`\`, ShapeNone}, // nothing but escape residue
	} {
		if got := PatternShape(tc.p); got != tc.want {
			t.Errorf("PatternShape(%q) = %s, want %s", tc.p, got, tc.want)
		}
	}
}

func TestOverlapsQueryVsPattern(t *testing.T) {
	q := Tokens("where is the order cancellation handled")
	if !Overlaps(q, Tokens("cancelOrder|OrderCancellation")) {
		t.Fatal("expected same-intent overlap")
	}
	if Overlaps(q, Tokens("parseTokenizer")) {
		t.Fatal("expected no overlap for unrelated pattern")
	}
}
