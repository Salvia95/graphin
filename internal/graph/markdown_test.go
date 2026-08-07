package graph

import (
	"testing"

	"github.com/Salvia95/graphin/internal/parse"
)

// mdDoc mirrors what extractMarkdown produces for
//
//	docs/a.md   preamble
//	## 부모
//	### 자식
func mdDoc() *parse.FileResult {
	file := parse.Node{
		ID: "docs/a.md", DisplayName: "a.md", SimpleName: "a.md", Kind: "file",
		Contains: []string{"docs/a.md#부모"},
	}
	parent := parse.Node{
		ID: "docs/a.md#부모", DisplayName: "부모", SimpleName: "부모", Kind: "section",
		Contains: []string{"docs/a.md#자식"},
	}
	child := parse.Node{
		ID: "docs/a.md#자식", DisplayName: "자식", SimpleName: "자식", Kind: "section",
	}
	return fileRes(parse.LangMarkdown, "docs/a.md", "docs", nil, file, parent, child)
}

// Containment is structural, not inferred: the parser hands the engine exact
// target IDs, so the edge is certain and survives any min_confidence.
func TestMarkdownContainsEdgesAreCertain(t *testing.T) {
	e := newEngine(t)
	applyAll(e, mdDoc())

	out := usesOf(t, e, "docs/a.md")
	if len(out) != 1 || out[0].NodeID != "docs/a.md#부모" {
		t.Fatalf("file uses = %+v", out)
	}
	if out[0].Type != "contains" {
		t.Fatalf("edge type = %q, want contains", out[0].Type)
	}
	if out[0].Confidence != 1.0 {
		t.Fatalf("confidence = %v, want 1.0", out[0].Confidence)
	}

	// The hierarchy is one level per edge, not a flattened list: the file
	// does not directly contain the grandchild.
	kids := usesOf(t, e, "docs/a.md#부모")
	if len(kids) != 1 || kids[0].NodeID != "docs/a.md#자식" {
		t.Fatalf("parent uses = %+v", kids)
	}
}

// The default min_confidence is 0.85; contains must never be filtered out.
func TestMarkdownContainsSurvivesDefaultConfidence(t *testing.T) {
	e := newEngine(t)
	applyAll(e, mdDoc())
	p, err := e.Explore("docs/a.md", "uses", "", 0.85)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Uses) != 1 {
		t.Fatalf("contains dropped at the default threshold: %+v", p.Uses)
	}
}

// used_by is what makes a section navigable back to its document.
func TestMarkdownSectionKnowsItsParent(t *testing.T) {
	e := newEngine(t)
	applyAll(e, mdDoc())
	p, err := e.Explore("docs/a.md#자식", "used_by", "", 0.5)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.UsedBy) != 1 || p.UsedBy[0].NodeID != "docs/a.md#부모" {
		t.Fatalf("used_by = %+v", p.UsedBy)
	}
}
