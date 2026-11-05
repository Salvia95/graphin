package graph

import (
	"testing"

	"github.com/Salvia95/graphin/internal/merkle"
	"github.com/Salvia95/graphin/internal/nodeid"
	"github.com/Salvia95/graphin/internal/obs"
	"github.com/Salvia95/graphin/internal/parse"
)

// A caller's edges depend on the definitions it looked up, not only on its
// own bytes. Each test changes the definition side, or the imports outside
// the caller's span, and leaves the caller node byte-identical — the case the
// 2-Track diff classifies OffsetOnly and that used to keep the old edge.

func dangling(e *Engine) int {
	_, totals := e.DanglingEdges(0)
	return totals.Code
}

func TestRemovedCalleeDropsTheCallersEdge(t *testing.T) {
	e := newEngine(t)
	pkg := "com.a"
	applyAll(e,
		fileRes(parse.LangJava, "a/Svc.java", pkg, nil,
			methodNode(pkg, "Svc", "charge", []string{"long"}, 1, 1)),
		fileRes(parse.LangJava, "a/Flow.java", pkg, nil,
			methodNode(pkg, "Flow", "run", nil, 0, 0, parse.Call{Name: "charge", Args: 1})))
	chargeID := nodeid.Method(pkg, "Svc", "charge", []string{"long"})
	runID := nodeid.Method(pkg, "Flow", "run", nil)

	e.RemoveNodes([]string{chargeID})
	e.Flush()
	if hasEdge(usesOf(t, e, runID), chargeID, "call", confSamePkg) || dangling(e) != 0 {
		t.Fatalf("the caller still points at the removed callee: %+v, dangling %d",
			usesOf(t, e, runID), dangling(e))
	}
}

// A rename moves the caller to the new definition.
func TestRenamedCalleeRetargetsTheCaller(t *testing.T) {
	e := newEngine(t)
	tree := merkle.NewTree()
	apply := func(res *parse.FileResult) {
		e.ApplyFile(res, merkle.Diff(tree, res))
		tree.Update(res)
	}
	pkg := "com.a"
	svc := func(name string) *parse.FileResult {
		return fileRes(parse.LangJava, "a/Svc.java", pkg, nil,
			methodNode(pkg, "Svc", name, []string{"long"}, 1, 1))
	}
	flow := fileRes(parse.LangJava, "a/Flow.java", pkg, nil,
		methodNode(pkg, "Flow", "run", nil, 0, 0,
			parse.Call{Name: "charge", Args: 1}, parse.Call{Name: "bill", Args: 1}))
	apply(svc("charge"))
	apply(flow)
	e.Flush()
	runID := nodeid.Method(pkg, "Flow", "run", nil)
	oldID := nodeid.Method(pkg, "Svc", "charge", []string{"long"})
	newID := nodeid.Method(pkg, "Svc", "bill", []string{"long"})
	if !hasEdge(usesOf(t, e, runID), oldID, "call", confSamePkg) {
		t.Fatal("premise: run calls charge")
	}

	apply(svc("bill"))
	e.Flush()
	uses := usesOf(t, e, runID)
	if hasEdge(uses, oldID, "call", confSamePkg) || !hasEdge(uses, newID, "call", confSamePkg) {
		t.Fatalf("after the rename run should call bill only: %+v", uses)
	}
}

// Imports sit outside every node's span, so changing only them leaves the
// class node unchanged. Its import edge must follow anyway.
func TestImportChangeRetargetsAnUnchangedNode(t *testing.T) {
	e := newEngine(t)
	tree := merkle.NewTree()
	apply := func(res *parse.FileResult) {
		e.ApplyFile(res, merkle.Diff(tree, res))
		tree.Update(res)
	}
	apply(fileRes(parse.LangJava, "x/Svc.java", "com.x", nil, classNode("com.x", "Svc", nodeid.KindClass)))
	apply(fileRes(parse.LangJava, "y/Svc.java", "com.y", nil, classNode("com.y", "Svc", nodeid.KindClass)))
	user := func(imp string) *parse.FileResult {
		return fileRes(parse.LangJava, "c/User.java", "com.c", []string{imp},
			classNode("com.c", "User", nodeid.KindClass))
	}
	apply(user("com.x.Svc"))
	e.Flush()
	userID := nodeid.Class("com.c", "User")
	if !hasEdge(usesOf(t, e, userID), "com.x.Svc", "import", confCertain) {
		t.Fatalf("premise: User imports com.x.Svc: %+v", usesOf(t, e, userID))
	}

	next := user("com.y.Svc")
	for _, n := range merkle.Diff(tree, next).Changed {
		if n.ID == userID {
			t.Fatal("premise broken: User must be OffsetOnly, or this test proves nothing")
		}
	}
	apply(next)
	e.Flush()
	uses := usesOf(t, e, userID)
	if hasEdge(uses, "com.x.Svc", "import", confCertain) || !hasEdge(uses, "com.y.Svc", "import", confCertain) {
		t.Fatalf("import edge did not follow the import: %+v", uses)
	}
}

// A closer definition appearing later wins over the global one the caller
// was first resolved to.
func TestNewCloserDefinitionWinsTheCall(t *testing.T) {
	e := newEngine(t)
	tree := merkle.NewTree()
	apply := func(res *parse.FileResult) {
		e.ApplyFile(res, merkle.Diff(tree, res))
		tree.Update(res)
	}
	apply(fileRes(parse.LangJava, "far/Far.java", "com.far", nil,
		methodNode("com.far", "Far", "settle", nil, 0, 0)))
	apply(fileRes(parse.LangJava, "a/Flow.java", "com.a", nil,
		methodNode("com.a", "Flow", "run", nil, 0, 0, parse.Call{Name: "settle", Args: 0})))
	e.Flush()
	runID := nodeid.Method("com.a", "Flow", "run", nil)
	farID := nodeid.Method("com.far", "Far", "settle", nil)
	if !hasEdge(usesOf(t, e, runID), farID, "call", confGlobal) {
		t.Fatalf("premise: run → far settle at the global tier: %+v", usesOf(t, e, runID))
	}

	apply(fileRes(parse.LangJava, "a/Near.java", "com.a", nil,
		methodNode("com.a", "Near", "settle", nil, 0, 0)))
	e.Flush()
	nearID := nodeid.Method("com.a", "Near", "settle", nil)
	uses := usesOf(t, e, runID)
	if !hasEdge(uses, nearID, "call", confSamePkg) || hasEdge(uses, farID, "call", confGlobal) {
		t.Fatalf("the same-package definition should take the call: %+v", uses)
	}
}

// A callee deleted while the server was down: on restart the caller comes
// back from its shard with the old edge, and the initial scan re-applies the
// caller's file but not the callee's (it is gone). The first flush must
// re-resolve the caller from its fresh raw references.
func TestRestartReResolvesAgainstTheCurrentTree(t *testing.T) {
	dir := t.TempDir()
	e, err := Open(dir, obs.Nop())
	if err != nil {
		t.Fatal(err)
	}
	pkg := "com.a"
	svc := fileRes(parse.LangJava, "a/Svc.java", pkg, nil,
		methodNode(pkg, "Svc", "charge", []string{"long"}, 1, 1))
	flow := fileRes(parse.LangJava, "a/Flow.java", pkg, nil,
		methodNode(pkg, "Flow", "run", nil, 0, 0, parse.Call{Name: "charge", Args: 1}))
	applyAll(e, svc, flow)
	e.Close()

	e2, err := Open(dir, obs.Nop())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(e2.Close)
	chargeID := nodeid.Method(pkg, "Svc", "charge", []string{"long"})
	runID := nodeid.Method(pkg, "Flow", "run", nil)

	// What the initial scan does: drop the vanished file's nodes, re-apply
	// the surviving file. The caller is byte-identical, so OffsetOnly.
	e2.RemoveNodes([]string{chargeID})
	tree := merkle.NewTree()
	tree.Update(flow)
	e2.ApplyFile(flow, merkle.Diff(tree, flow))
	e2.Flush()

	if hasEdge(usesOf(t, e2, runID), chargeID, "call", confSamePkg) || dangling(e2) != 0 {
		t.Fatalf("restart kept the stale edge: %+v, dangling %d", usesOf(t, e2, runID), dangling(e2))
	}
}

// Re-registering an unchanged file must not send its callers back through
// resolution: a save that changes nothing a lookup reads costs nothing.
func TestUnchangedDefinitionsInvalidateNothing(t *testing.T) {
	e := newEngine(t)
	tree := merkle.NewTree()
	pkg := "com.a"
	svc := fileRes(parse.LangJava, "a/Svc.java", pkg, nil,
		methodNode(pkg, "Svc", "charge", []string{"long"}, 1, 1))
	flow := fileRes(parse.LangJava, "a/Flow.java", pkg, nil,
		methodNode(pkg, "Flow", "run", nil, 0, 0, parse.Call{Name: "charge", Args: 1}))
	for _, r := range []*parse.FileResult{svc, flow} {
		e.ApplyFile(r, merkle.Diff(tree, r))
		tree.Update(r)
	}
	e.Flush()

	e.ApplyFile(svc, merkle.Diff(tree, svc))
	e.stateMu.Lock()
	changed := e.res.takeChanged()
	e.stateMu.Unlock()
	if len(changed) != 0 {
		t.Fatalf("an identical re-apply reported changed lookups: %v", changed)
	}
}
