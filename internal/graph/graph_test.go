package graph

import (
	"testing"

	"github.com/llls2542/graphin/internal/merkle"
	"github.com/llls2542/graphin/internal/nodeid"
	"github.com/llls2542/graphin/internal/obs"
	"github.com/llls2542/graphin/internal/parse"
)

// ---- synthetic parse-result helpers (no tree-sitter needed) ----

func fileRes(lang parse.Language, rel, pkg string, imports []string, nodes ...parse.Node) *parse.FileResult {
	res := &parse.FileResult{RelPath: rel, Lang: lang, Package: pkg, Imports: imports, Nodes: nodes}
	res.FileHash = merkle.Sum([]byte(rel))
	return res
}

func methodNode(pkg, container, name string, params []string, min, max int, calls ...parse.Call) parse.Node {
	id := nodeid.Method(pkg, container, name, params)
	return parse.Node{
		ID: id, DisplayName: nodeid.Display(container, name), SimpleName: name,
		Kind: nodeid.KindMethod, Container: container,
		ArityMin: min, ArityMax: max, Calls: calls,
		Hash: merkle.Sum([]byte(id)),
	}
}

func classNode(pkg, name string, kind string, supers ...string) parse.Node {
	id := nodeid.Class(pkg, name)
	return parse.Node{
		ID: id, DisplayName: name, SimpleName: name, Kind: kind,
		Supers: supers, Hash: merkle.Sum([]byte(id)),
	}
}

func newEngine(t *testing.T) *Engine {
	t.Helper()
	e, err := Open(t.TempDir(), obs.Nop())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(e.Close)
	return e
}

func applyAll(e *Engine, results ...*parse.FileResult) {
	for _, res := range results {
		e.ApplyFile(res, merkle.Diff(merkle.NewTree(), res))
	}
	e.Flush()
}

func usesOf(t *testing.T, e *Engine, id string) []EdgeOut {
	t.Helper()
	p, err := e.Explore(id, "uses", "", 0.5)
	if err != nil {
		t.Fatalf("explore %s: %v", id, err)
	}
	return p.Uses
}

func usedByOf(t *testing.T, e *Engine, id string) []EdgeOut {
	t.Helper()
	p, err := e.Explore(id, "used_by", "", 0.5)
	if err != nil {
		t.Fatalf("explore %s: %v", id, err)
	}
	return p.UsedBy
}

func hasEdge(edges []EdgeOut, target, typ string, conf float32) bool {
	for _, e := range edges {
		if e.NodeID == target && e.Type == typ && e.Confidence == conf {
			return true
		}
	}
	return false
}

// ---- §7-P3-④: Kotlin default-arg arity range ----

func TestKotlinDefaultArgArityRangeMatch(t *testing.T) {
	pkg := "com.example.order"
	// fun cancel(orderId: Long, reason: String = "none", notify: Boolean = true) → arity 1..3
	callee := fileRes(parse.LangKotlin, "OrderService.kt", pkg, nil,
		methodNode(pkg, "OrderService", "cancel", []string{"Long", "String", "Boolean"}, 1, 3))
	caller := fileRes(parse.LangKotlin, "CancelFlow.kt", pkg, nil,
		methodNode(pkg, "CancelFlow", "runIt", []string{"Long"}, 1, 1,
			parse.Call{Name: "cancel", Args: 1, Recv: "service"}))

	e := newEngine(t)
	applyAll(e, callee, caller)

	cancelID := nodeid.Method(pkg, "OrderService", "cancel", []string{"Long", "String", "Boolean"})
	callerID := nodeid.Method(pkg, "CancelFlow", "runIt", []string{"Long"})

	if !hasEdge(usesOf(t, e, callerID), cancelID, "call", confSamePkg) {
		t.Fatalf("1-arg call must match arity range 1..3: %+v", usesOf(t, e, callerID))
	}
	ub := usedByOf(t, e, cancelID)
	if !hasEdge(ub, callerID, "call", confSamePkg) {
		t.Fatalf("used_by missing caller: %+v", ub)
	}

	// 0 args and 4 args fall outside the range.
	outside := fileRes(parse.LangKotlin, "Bad.kt", pkg, nil,
		methodNode(pkg, "Bad", "tooFew", nil, 0, 0, parse.Call{Name: "cancel", Args: 0}),
		methodNode(pkg, "Bad", "tooMany", nil, 0, 0, parse.Call{Name: "cancel", Args: 4}))
	applyAll(e, outside)
	for _, m := range []string{"tooFew", "tooMany"} {
		id := nodeid.Method(pkg, "Bad", m, nil)
		for _, u := range usesOf(t, e, id) {
			if u.NodeID == cancelID {
				t.Fatalf("%s must not match arity 1..3", m)
			}
		}
	}
}

func TestPythonStarArgsOpenMaxArity(t *testing.T) {
	pkg := "app.services"
	callee := fileRes(parse.LangPython, "app/services/order.py", "app.services.order", nil,
		parse.Node{
			ID: "app.services.order.OrderService.cancel", SimpleName: "cancel",
			DisplayName: "OrderService.cancel", Kind: nodeid.KindMethod,
			Container: "OrderService", ArityMin: 1, ArityMax: nodeid.UnboundedArity,
			Hash: merkle.Sum([]byte("cancel")),
		})
	caller := fileRes(parse.LangPython, "app/services/flow.py", "app.services.flow", nil,
		parse.Node{
			ID: "app.services.flow.run", SimpleName: "run", DisplayName: "run",
			Kind: nodeid.KindFunction, ArityMin: 0, ArityMax: 0,
			Calls: []parse.Call{{Name: "cancel", Args: 5, Recv: "svc"}},
			Hash:  merkle.Sum([]byte("run")),
		})
	e := newEngine(t)
	applyAll(e, callee, caller)
	_ = pkg

	if !hasEdge(usesOf(t, e, "app.services.flow.run"),
		"app.services.order.OrderService.cancel", "call", confSamePkg) {
		t.Fatalf("*args must accept 5 args: %+v", usesOf(t, e, "app.services.flow.run"))
	}
}

// ---- scope ranking (§2.1.3) ----

func TestScopeRankOrdering(t *testing.T) {
	e := newEngine(t)
	// Same-named 1-arg method in three packages.
	mk := func(pkg string) *parse.FileResult {
		return fileRes(parse.LangJava, pkg+"/Svc.java", pkg, nil,
			methodNode(pkg, "Svc", "charge", []string{"long"}, 1, 1))
	}
	applyAll(e, mk("same.pkg"), mk("imported.pkg"), mk("global.pkg"))

	// Caller in same.pkg importing imported.pkg.Svc.
	caller := fileRes(parse.LangJava, "same.pkg/Caller.java", "same.pkg",
		[]string{"imported.pkg.Svc"},
		methodNode("same.pkg", "Caller", "go", nil, 0, 0, parse.Call{Name: "charge", Args: 1}))
	applyAll(e, caller)

	callerID := nodeid.Method("same.pkg", "Caller", "go", nil)
	uses := usesOf(t, e, callerID)
	if !hasEdge(uses, "same.pkg.Svc.charge(long)", "call", confSamePkg) {
		t.Fatalf("same-package candidate must win at 0.95: %+v", uses)
	}
	for _, u := range uses {
		if u.NodeID == "imported.pkg.Svc.charge(long)" || u.NodeID == "global.pkg.Svc.charge(long)" {
			t.Fatalf("lower tiers must be suppressed when same-pkg exists: %+v", uses)
		}
	}

	// Caller elsewhere with only the import → 0.90 tier.
	caller2 := fileRes(parse.LangJava, "other.pkg/C2.java", "other.pkg",
		[]string{"imported.pkg.Svc"},
		methodNode("other.pkg", "C2", "go2", nil, 0, 0, parse.Call{Name: "charge", Args: 1}))
	applyAll(e, caller2)
	uses2 := usesOf(t, e, nodeid.Method("other.pkg", "C2", "go2", nil))
	if !hasEdge(uses2, "imported.pkg.Svc.charge(long)", "call", confImported) {
		t.Fatalf("imported candidate must score 0.90: %+v", uses2)
	}
}

func TestStoplistSuppressesGlobalOnly(t *testing.T) {
	e := newEngine(t)
	applyAll(e, fileRes(parse.LangJava, "far.away/S.java", "far.away", nil,
		methodNode("far.away", "S", "process", []string{"int"}, 1, 1)))

	// Global-tier call to stoplisted "process": suppressed.
	applyAll(e, fileRes(parse.LangJava, "caller.pkg/C.java", "caller.pkg", nil,
		methodNode("caller.pkg", "C", "go", nil, 0, 0, parse.Call{Name: "process", Args: 1})))
	if uses := usesOf(t, e, nodeid.Method("caller.pkg", "C", "go", nil)); len(uses) != 0 {
		t.Fatalf("stoplisted global match must be suppressed: %+v", uses)
	}

	// Same-package call to "process": §3.3 example requires this edge.
	applyAll(e, fileRes(parse.LangJava, "far.away/C2.java", "far.away", nil,
		methodNode("far.away", "C2", "go2", nil, 0, 0, parse.Call{Name: "process", Args: 1})))
	uses := usesOf(t, e, nodeid.Method("far.away", "C2", "go2", nil))
	if !hasEdge(uses, "far.away.S.process(int)", "call", confSamePkg) {
		t.Fatalf("scoped stoplist-name call must still edge: %+v", uses)
	}
}

func TestImplementsEdgeCertain(t *testing.T) {
	e := newEngine(t)
	applyAll(e,
		fileRes(parse.LangJava, "com.p/Port.java", "com.p", nil,
			classNode("com.p", "Port", nodeid.KindInterface)),
		fileRes(parse.LangJava, "com.p/Adapter.java", "com.p", nil,
			classNode("com.p", "Adapter", nodeid.KindClass, "Port")))

	uses := usesOf(t, e, "com.p.Adapter")
	if !hasEdge(uses, "com.p.Port", "implements", confCertain) {
		t.Fatalf("implements must be 1.0 certain: %+v", uses)
	}
}

// ---- §7-P3-①: delete → used_by immediately gone (tombstone, no compaction) ----

func TestMethodDeleteRemovesUsedByImmediately(t *testing.T) {
	pkg := "com.del"
	callee := fileRes(parse.LangJava, "com.del/Svc.java", pkg, nil,
		methodNode(pkg, "Svc", "pay", []string{"long"}, 1, 1))
	caller := fileRes(parse.LangJava, "com.del/Caller.java", pkg, nil,
		methodNode(pkg, "Caller", "go", nil, 0, 0, parse.Call{Name: "pay", Args: 1}))
	e := newEngine(t)
	applyAll(e, callee, caller)

	payID := nodeid.Method(pkg, "Svc", "pay", []string{"long"})
	callerID := nodeid.Method(pkg, "Caller", "go", nil)
	if !hasEdge(usedByOf(t, e, payID), callerID, "call", confSamePkg) {
		t.Fatal("precondition: used_by should list the caller")
	}

	// Delete the caller; its outgoing edges must vanish from used_by NOW.
	e.RemoveNodes([]string{callerID})
	if ub := e.rev.Query(payID, 0); len(ub) != 0 {
		t.Fatalf("used_by must be empty immediately after delete: %+v", ub)
	}
}

// ---- §7-P3-⑤: deterministic explore sort + cursor pagination ----

func TestDeterministicSortAndPagination(t *testing.T) {
	pkg := "com.page"
	e := newEngine(t)
	target := fileRes(parse.LangJava, "com.page/T.java", pkg, nil,
		methodNode(pkg, "T", "hot", []string{"int"}, 1, 1))
	applyAll(e, target)
	hotID := nodeid.Method(pkg, "T", "hot", []string{"int"})

	// 25 same-package callers + 3 imported-tier callers → 28 used_by edges.
	var callers []*parse.FileResult
	for i := 0; i < 25; i++ {
		name := "c" + string(rune('a'+i%26)) + string(rune('a'+i/26))
		callers = append(callers, fileRes(parse.LangJava, pkg+"/"+name+".java", pkg, nil,
			methodNode(pkg, "K"+name, "go", nil, 0, 0, parse.Call{Name: "hot", Args: 1})))
	}
	for i := 0; i < 3; i++ {
		p := "ext.pkg" + string(rune('0'+i))
		callers = append(callers, fileRes(parse.LangJava, p+"/E.java", p,
			[]string{"com.page.T"},
			methodNode(p, "E", "go", nil, 0, 0, parse.Call{Name: "hot", Args: 1})))
	}
	applyAll(e, callers...)

	page1, err := e.Explore(hotID, "used_by", "", 0.5)
	if err != nil {
		t.Fatal(err)
	}
	if len(page1.UsedBy) != PageSize || !page1.HasMore || page1.NextCursor == "" {
		t.Fatalf("page1: %d edges hasMore=%v", len(page1.UsedBy), page1.HasMore)
	}
	// Determinism: same call → identical page.
	again, _ := e.Explore(hotID, "used_by", "", 0.5)
	for i := range page1.UsedBy {
		if page1.UsedBy[i] != again.UsedBy[i] {
			t.Fatalf("non-deterministic ordering at %d", i)
		}
	}
	// Sort law: confidence desc, then same-pkg first, then id asc.
	for i := 1; i < len(page1.UsedBy); i++ {
		a, b := page1.UsedBy[i-1], page1.UsedBy[i]
		if a.Confidence < b.Confidence {
			t.Fatalf("confidence not descending at %d", i)
		}
	}

	page2, err := e.Explore(hotID, "used_by", page1.NextCursor, 0.5)
	if err != nil {
		t.Fatal(err)
	}
	if page2.HasMore || len(page2.UsedBy) != 8 {
		t.Fatalf("page2: %d edges hasMore=%v", len(page2.UsedBy), page2.HasMore)
	}
	seen := map[string]bool{}
	for _, e := range append(append([]EdgeOut{}, page1.UsedBy...), page2.UsedBy...) {
		if seen[e.NodeID] {
			t.Fatalf("duplicate across pages: %s", e.NodeID)
		}
		seen[e.NodeID] = true
	}
	if len(seen) != 28 {
		t.Fatalf("union = %d edges, want 28 (no loss)", len(seen))
	}
}

// TestCursorStableAcrossShardRegeneration (§4.4-b): resume after the shard
// is rebuilt — seek-keys must not duplicate or lose edges.
func TestCursorStableAcrossShardRegeneration(t *testing.T) {
	pkg := "com.regen"
	e := newEngine(t)
	applyAll(e, fileRes(parse.LangJava, pkg+"/T.java", pkg, nil,
		methodNode(pkg, "T", "hot", []string{"int"}, 1, 1)))
	hotID := nodeid.Method(pkg, "T", "hot", []string{"int"})

	var callers []*parse.FileResult
	for i := 0; i < 25; i++ {
		name := "k" + string(rune('a'+i%26)) + string(rune('a'+i/26))
		callers = append(callers, fileRes(parse.LangJava, pkg+"/"+name+".java", pkg, nil,
			methodNode(pkg, "K"+name, "go", nil, 0, 0, parse.Call{Name: "hot", Args: 1})))
	}
	applyAll(e, callers...)

	page1, err := e.Explore(hotID, "used_by", "", 0.5)
	if err != nil {
		t.Fatal(err)
	}

	// Force a shard regeneration between pages.
	applyAll(e, fileRes(parse.LangJava, pkg+"/New.java", pkg, nil,
		methodNode(pkg, "Nw", "unrelated", nil, 0, 0)))

	page2, err := e.Explore(hotID, "used_by", page1.NextCursor, 0.5)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, ed := range append(append([]EdgeOut{}, page1.UsedBy...), page2.UsedBy...) {
		if seen[ed.NodeID] {
			t.Fatalf("duplicate after regeneration: %s", ed.NodeID)
		}
		seen[ed.NodeID] = true
	}
	if len(seen) != 25 {
		t.Fatalf("union = %d, want 25", len(seen))
	}
}

// ---- persistence roundtrip: engine reload from shards + reverse files ----

func TestEngineReloadFromDisk(t *testing.T) {
	dir := t.TempDir()
	e, err := Open(dir, obs.Nop())
	if err != nil {
		t.Fatal(err)
	}
	pkg := "com.disk"
	applyAll(e,
		fileRes(parse.LangJava, pkg+"/S.java", pkg, nil,
			methodNode(pkg, "S", "pay", []string{"long"}, 1, 1)),
		fileRes(parse.LangJava, pkg+"/C.java", pkg, nil,
			methodNode(pkg, "C", "go", nil, 0, 0, parse.Call{Name: "pay", Args: 1})))
	e.Close()

	e2, err := Open(dir, obs.Nop())
	if err != nil {
		t.Fatal(err)
	}
	defer e2.Close()
	payID := nodeid.Method(pkg, "S", "pay", []string{"long"})
	callerID := nodeid.Method(pkg, "C", "go", nil)
	if !hasEdge(usesOf(t, e2, callerID), payID, "call", confSamePkg) {
		t.Fatal("uses lost across restart")
	}
	if !hasEdge(usedByOf(t, e2, payID), callerID, "call", confSamePkg) {
		t.Fatal("used_by lost across restart")
	}
}
