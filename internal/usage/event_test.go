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

func TestOverlapsQueryVsPattern(t *testing.T) {
	q := Tokens("where is the order cancellation handled")
	if !Overlaps(q, Tokens("cancelOrder|OrderCancellation")) {
		t.Fatal("expected same-intent overlap")
	}
	if Overlaps(q, Tokens("parseTokenizer")) {
		t.Fatal("expected no overlap for unrelated pattern")
	}
}
