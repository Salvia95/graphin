package graph

import (
	"testing"

	"github.com/Salvia95/graphin/internal/merkle"
	"github.com/Salvia95/graphin/internal/nodeid"
	"github.com/Salvia95/graphin/internal/obs"
	"github.com/Salvia95/graphin/internal/parse"
)

// sectionNode builds a markdown section the way internal/parse/markdown.go
// does: the hash covers the node's own span only, so a section that gains a
// sibling further down the document keeps its hash.
func sectionNode(id, display string, children ...string) parse.Node {
	return parse.Node{
		ID: id, DisplayName: display, SimpleName: display,
		Kind: nodeid.KindSection, Contains: children,
		Hash: merkle.Sum([]byte(id + "|" + display)),
	}
}

func mdFile(rel string, nodes ...parse.Node) *parse.FileResult {
	res := &parse.FileResult{RelPath: rel, Lang: parse.LangMarkdown, Nodes: nodes}
	res.FileHash = merkle.Sum([]byte(rel))
	return res
}

func containsTargets(edges []EdgeOut) []string {
	var out []string
	for _, e := range edges {
		if e.Type == "contains" {
			out = append(out, e.NodeID)
		}
	}
	return out
}

// TestContainsRefreshesWhenChildAdded pins the defect a knowledge-set build
// surfaced: appending a section to an existing document left the parent's
// child list frozen at whatever it held when the parent was last rewritten.
//
// The parent is deliberately byte-identical across both passes — that is the
// whole point. A markdown section hashes its own span (heading to first child
// heading), so adding a heading at the end of the document cannot change it,
// and the 2-Track diff correctly classifies the parent as OffsetOnly. The
// child list therefore has to refresh on its own, not on the hash.
func TestContainsRefreshesWhenChildAdded(t *testing.T) {
	e := newEngine(t)
	tree := merkle.NewTree()

	const doc = "docs/spec.md"
	const root = doc + "#spec"
	apply := func(res *parse.FileResult) {
		e.ApplyFile(res, merkle.Diff(tree, res))
		tree.Update(res)
		e.Flush()
	}

	first := mdFile(doc,
		sectionNode(root, "Spec", doc+"#one"),
		sectionNode(doc+"#one", "One"),
	)
	apply(first)
	if got := containsTargets(usesOf(t, e, root)); len(got) != 1 || got[0] != doc+"#one" {
		t.Fatalf("first pass: contains = %v", got)
	}

	// Same parent node, byte for byte — only the child list grew.
	second := mdFile(doc,
		sectionNode(root, "Spec", doc+"#one", doc+"#two"),
		sectionNode(doc+"#one", "One"),
		sectionNode(doc+"#two", "Two"),
	)
	d := merkle.Diff(tree, second)
	for _, n := range d.Changed {
		if n.ID == root {
			t.Fatal("premise broken: the parent must be OffsetOnly, or this test proves nothing")
		}
	}
	apply(second)

	got := containsTargets(usesOf(t, e, root))
	if len(got) != 2 || got[0] != doc+"#one" || got[1] != doc+"#two" {
		t.Fatalf("second pass: contains = %v, want both sections", got)
	}
	// The reverse index has to learn it too — /browse and used_by read that side.
	if ub := usedByOf(t, e, doc+"#two"); !hasEdge(ub, root, "contains", 1.0) {
		t.Fatalf("used_by of the new section = %+v, want contains from %s", ub, root)
	}
}

// The mirror case: deleting a heading must not leave the parent pointing at a
// node that no longer exists. Without the refresh the stale target survives as
// a dangling edge, which is exactly what diagnose_index counts.
func TestContainsDropsRemovedChild(t *testing.T) {
	e := newEngine(t)
	tree := merkle.NewTree()

	const doc = "docs/spec.md"
	const root = doc + "#spec"
	apply := func(res *parse.FileResult) {
		e.ApplyFile(res, merkle.Diff(tree, res))
		tree.Update(res)
		e.Flush()
	}

	apply(mdFile(doc,
		sectionNode(root, "Spec", doc+"#one", doc+"#two"),
		sectionNode(doc+"#one", "One"),
		sectionNode(doc+"#two", "Two"),
	))
	apply(mdFile(doc,
		sectionNode(root, "Spec", doc+"#one"),
		sectionNode(doc+"#one", "One"),
	))

	got := containsTargets(usesOf(t, e, root))
	if len(got) != 1 || got[0] != doc+"#one" {
		t.Fatalf("contains = %v, want the deleted section gone", got)
	}
}

// Code nodes never carry Contains, so the refresh must not drag them into a
// resolve. That matters because shards persist Uses but not the raw call
// lists: a needless resolve after a reload would rebuild a method's edges from
// an empty RawCalls and silently erase them.
func TestContainsRefreshLeavesCodeEdgesAlone(t *testing.T) {
	dir := t.TempDir()
	e, err := Open(dir, obs.Nop())
	if err != nil {
		t.Fatal(err)
	}
	const pkg = "com.example"
	caller := methodNode(pkg, "A", "run", nil, 0, 0, parse.Call{Name: "target", Args: 0})
	callee := methodNode(pkg, "B", "target", nil, 0, 0)
	res := fileRes(parse.LangJava, "A.java", pkg, nil, caller, callee)
	applyAll(e, res)
	before := usesOf(t, e, caller.ID)
	if len(before) == 0 {
		t.Fatal("premise broken: the caller should have a resolved edge")
	}
	e.Close()

	// Reload from disk: raw calls are gone (only Uses is persisted), then
	// reindex the unchanged file the way a bootstrap does.
	e2, err := Open(dir, obs.Nop())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(e2.Close)
	tree := merkle.NewTree()
	tree.Update(res)
	e2.ApplyFile(res, merkle.Diff(tree, res))
	e2.Flush()

	if after := usesOf(t, e2, caller.ID); len(after) != len(before) {
		t.Fatalf("code edges changed across reload: %d → %d (%+v)", len(before), len(after), after)
	}
}
