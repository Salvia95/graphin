package graph

import (
	"testing"

	"github.com/Salvia95/graphin/internal/merkle"
	"github.com/Salvia95/graphin/internal/nodeid"
	"github.com/Salvia95/graphin/internal/parse"
)

func jsFnNode(module, name string, calls ...parse.Call) parse.Node {
	id := nodeid.Python(module, "", name, 1)
	return parse.Node{
		ID: id, DisplayName: name, SimpleName: name, Kind: nodeid.KindFunction,
		ArityMin: 1, ArityMax: 1, Calls: calls, Hash: merkle.Sum([]byte(id)),
	}
}

// JS/TS import는 파싱 시점에 dotted 모듈 공간으로 정규화된다(§설계 결정 2).
// named import와 namespace import(*) 모두 imported 티어(0.90)로 매칭되고,
// 다른 디렉토리의 모듈은 same-package 티어를 받지 않아야 한다.
func TestJSImportScopeRanking(t *testing.T) {
	callee := fileRes(parse.LangJavaScript, "src/util/check.js", "src.util.check", nil,
		jsFnNode("src.util.check", "validate"))
	named := fileRes(parse.LangJavaScript, "src/api/handler.js", "src.api.handler",
		[]string{"src.util.check.validate"}, // import { validate } from "../util/check"
		jsFnNode("src.api.handler", "handleRequest", parse.Call{Name: "validate", Args: 1}))
	ns := fileRes(parse.LangJavaScript, "src/api/batch.js", "src.api.batch",
		[]string{"src.util.check.*"}, // import * as check from "../util/check"
		jsFnNode("src.api.batch", "runBatch", parse.Call{Name: "validate", Args: 1, Recv: "check"}))

	e := newEngine(t)
	applyAll(e, callee, named, ns)

	valID := "src.util.check.validate"
	if u := usesOf(t, e, "src.api.handler.handleRequest"); !hasEdge(u, valID, "call", confImported) {
		t.Fatalf("named import should give imported-tier call edge: %+v", u)
	}
	if u := usesOf(t, e, "src.api.batch.runBatch"); !hasEdge(u, valID, "call", confImported) {
		t.Fatalf("namespace import should give imported-tier call edge: %+v", u)
	}
	ub := usedByOf(t, e, valID)
	if !hasEdge(ub, "src.api.handler.handleRequest", "call", confImported) ||
		!hasEdge(ub, "src.api.batch.runBatch", "call", confImported) {
		t.Fatalf("used_by missing callers: %+v", ub)
	}
}

// 같은 디렉토리(부모 패키지 샤드)의 JS 모듈 간 호출은 same-package 티어를 받는다.
func TestJSSameDirShardTier(t *testing.T) {
	callee := fileRes(parse.LangJavaScript, "src/order/repo.js", "src.order.repo", nil,
		jsFnNode("src.order.repo", "save"))
	caller := fileRes(parse.LangJavaScript, "src/order/service.js", "src.order.service", nil,
		jsFnNode("src.order.service", "place", parse.Call{Name: "save", Args: 1, Recv: "repo"}))

	e := newEngine(t)
	applyAll(e, callee, caller)

	if u := usesOf(t, e, "src.order.service.place"); !hasEdge(u, "src.order.repo.save", "call", confSamePkg) {
		t.Fatalf("same-dir modules should rank same-package: %+v", u)
	}
}
