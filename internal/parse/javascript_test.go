package parse

import (
	"testing"

	"github.com/Salvia95/graphin/internal/nodeid"
)

// --- JavaScript ---

func TestJSModuleAndNodeIDs(t *testing.T) {
	res := parseFixture(t, "javascript", "src/order/service.js")
	if res.Partial {
		t.Fatal("fixture should parse cleanly")
	}
	if res.Package != "src.order.service" {
		t.Fatalf("package = %q, want src.order.service", res.Package)
	}
	for _, want := range []string{
		"src.order.service.OrderService",
		"src.order.service.OrderService.handle",
		"src.order.service.OrderService.process",
		"src.order.service.OrderService.of",
		"src.order.service.OrderService.onDone", // 클래스 필드 화살표 함수
		"src.order.service.OrderService.process.normalize", // 중첩 함수
		"src.order.service.placeOrder",
		"src.order.service.default", // 익명 default export (§설계 결정 3)
	} {
		if nodeByID(res, want) == nil {
			t.Errorf("missing node %s\nhave: %v", want, ids(res))
		}
	}
	cls := nodeByID(res, "src.order.service.OrderService")
	if cls.Kind != nodeid.KindClass || len(cls.Supers) != 1 || cls.Supers[0] != "BaseService" {
		t.Fatalf("OrderService = %+v", cls)
	}
	if m := nodeByID(res, "src.order.service.OrderService.onDone"); m.Kind != nodeid.KindMethod {
		t.Fatalf("field arrow should be a method, got %q", m.Kind)
	}
}

func TestJSArityRanges(t *testing.T) {
	res := parseFixture(t, "javascript", "src/order/service.js")
	// placeOrder(req, opts = {}, ...tags): 기본값은 optional, rest는 open
	po := nodeByID(res, "src.order.service.placeOrder")
	if po == nil || po.ArityMin != 1 || po.ArityMax != nodeid.UnboundedArity {
		t.Fatalf("placeOrder arity = %+v, want 1..unbounded", po)
	}
	pr := nodeByID(res, "src.order.service.OrderService.process")
	if pr == nil || pr.ArityMin != 1 || pr.ArityMax != 2 {
		t.Fatalf("process arity = %d..%d, want 1..2", pr.ArityMin, pr.ArityMax)
	}
}

func TestJSImportsNormalized(t *testing.T) {
	res := parseFixture(t, "javascript", "src/api/handler.js")
	want := map[string]bool{
		"src.order.service.svc":        false, // default import → 별칭 휴리스틱
		"src.order.service.placeOrder": false, // named import
		"src.util.check.*":             false, // namespace import
	}
	for _, imp := range res.Imports {
		if _, ok := want[imp]; ok {
			want[imp] = true
		}
	}
	for imp, seen := range want {
		if !seen {
			t.Errorf("missing import %s, have %v", imp, res.Imports)
		}
	}

	svc := parseFixture(t, "javascript", "src/order/service.js")
	found := map[string]bool{}
	for _, imp := range svc.Imports {
		found[imp] = true
	}
	if !found["src.util.check.validate"] {
		t.Errorf("relative named import not normalized: %v", svc.Imports)
	}
	if !found["src.order.repo.*"] { // const repo = require("./repo")
		t.Errorf("require binding not recorded: %v", svc.Imports)
	}
	if !found["big.js.Big"] { // bare specifier stays inert but recorded
		t.Errorf("bare specifier import missing: %v", svc.Imports)
	}
}

func TestJSCallsExtracted(t *testing.T) {
	res := parseFixture(t, "javascript", "src/order/service.js")
	h := nodeByID(res, "src.order.service.OrderService.handle")
	if h == nil {
		t.Fatalf("handle missing: %v", ids(res))
	}
	byName := map[string]Call{}
	for _, c := range h.Calls {
		byName[c.Name] = c
	}
	if c, ok := byName["validate"]; !ok || c.Args != 1 || c.Recv != "" {
		t.Errorf("validate call = %+v", byName)
	}
	if c, ok := byName["process"]; !ok || c.Args != 2 || c.Recv != "this" {
		t.Errorf("this.process call = %+v", byName)
	}
	if c, ok := byName["Receipt"]; !ok || c.Args != 1 { // new Receipt(...)
		t.Errorf("constructor call = %+v", byName)
	}
}

func TestJSIndexModuleCollapse(t *testing.T) {
	res := parseFixture(t, "javascript", "src/order/index.js")
	if res.Package != "src.order" {
		t.Fatalf("index.js should collapse to its directory, got %q", res.Package)
	}
	if nodeByID(res, "src.order.orderRoot") == nil {
		t.Fatalf("orderRoot missing: %v", ids(res))
	}
	// re-export pulls the symbol through as an import
	found := false
	for _, imp := range res.Imports {
		if imp == "src.order.service.placeOrder" {
			found = true
		}
	}
	if !found {
		t.Fatalf("re-export not recorded as import: %v", res.Imports)
	}
}

// --- TypeScript ---

func TestTSInterfaceEnumAlias(t *testing.T) {
	res := parseFixture(t, "typescript", "src/payment/port.ts")
	if res.Partial {
		t.Fatal("fixture should parse cleanly")
	}
	pp := nodeByID(res, "src.payment.port.PaymentPort")
	if pp == nil || pp.Kind != nodeid.KindInterface {
		t.Fatalf("PaymentPort should be interface, got %+v", pp)
	}
	if len(pp.Supers) != 1 || pp.Supers[0] != "Auditable" {
		t.Fatalf("PaymentPort Supers = %v", pp.Supers)
	}
	// 인터페이스 메서드 시그니처: TS `?`는 optional (§2.1.3 호환 범위)
	ch := nodeByID(res, "src.payment.port.PaymentPort.charge")
	if ch == nil || ch.Kind != nodeid.KindMethod || ch.ArityMin != 1 || ch.ArityMax != 2 {
		t.Fatalf("charge = %+v, want method 1..2", ch)
	}
	if st := nodeByID(res, "src.payment.port.Status"); st == nil || st.Kind != nodeid.KindClass {
		t.Fatalf("enum should be a class-kind node, got %+v", st)
	}
	if nodeByID(res, "src.payment.port.PortAlias") != nil {
		t.Fatal("type alias must not be indexed")
	}
}

func TestTSOverloadSignaturesCollapse(t *testing.T) {
	res := parseFixture(t, "typescript", "src/payment/adapter.ts")
	fee := nodeByID(res, "src.payment.adapter.fee")
	if fee == nil {
		t.Fatalf("fee missing: %v", ids(res))
	}
	if nodeByID(res, "src.payment.adapter.fee#2") != nil {
		t.Fatalf("overload signatures must collapse to the implementation: %v", ids(res))
	}
	// fee(amount, rate = 0.1): 기본값 있는 required_parameter → optional
	if fee.ArityMin != 1 || fee.ArityMax != 2 {
		t.Fatalf("fee arity = %d..%d, want 1..2", fee.ArityMin, fee.ArityMax)
	}
}

func TestTSClassImplementsAndImports(t *testing.T) {
	res := parseFixture(t, "typescript", "src/payment/adapter.ts")
	pg := nodeByID(res, "src.payment.adapter.PgAdapter")
	if pg == nil || len(pg.Supers) != 1 || pg.Supers[0] != "PaymentPort" {
		t.Fatalf("PgAdapter = %+v", pg)
	}
	if nodeByID(res, "src.payment.adapter.PgAdapter.charge") == nil ||
		nodeByID(res, "src.payment.adapter.PgAdapter.refund") == nil {
		t.Fatalf("methods missing: %v", ids(res))
	}
	found := 0
	for _, imp := range res.Imports {
		if imp == "src.payment.port.PaymentPort" || imp == "src.payment.port.Status" {
			found++
		}
	}
	if found != 2 {
		t.Fatalf("imports not normalized: %v", res.Imports)
	}
}

func TestTSXComponents(t *testing.T) {
	res := parseFixture(t, "typescript", "src/components/widget.tsx")
	if res.Partial {
		t.Fatal("fixture should parse cleanly")
	}
	w := nodeByID(res, "src.components.widget.Widget")
	if w == nil || w.Kind != nodeid.KindFunction {
		t.Fatalf("Widget = %+v", w)
	}
	// JSX 콜백 내부 호출도 본문 워크로 잡힌다
	found := false
	for _, c := range w.Calls {
		if c.Name == "report" && c.Args == 1 {
			found = true
		}
	}
	if !found {
		t.Fatalf("report call missing: %+v", w.Calls)
	}
	if nodeByID(res, "src.components.widget.Page") == nil {
		t.Fatalf("Page missing: %v", ids(res))
	}
}

func TestDTSSignaturesIndexed(t *testing.T) {
	res := parseFixture(t, "typescript", "src/types.d.ts")
	if res.Package != "src.types" {
		t.Fatalf("package = %q, want src.types (.d.ts stripped)", res.Package)
	}
	tr := nodeByID(res, "src.types.track")
	if tr == nil || tr.ArityMin != 1 || tr.ArityMax != 2 {
		t.Fatalf("track = %+v, want 1..2", tr)
	}
	if nodeByID(res, "src.types.legacyTrack") == nil {
		t.Fatalf("declare function in .d.ts should be indexed: %v", ids(res))
	}
}

// --- detection & partial ---

func TestDetectLanguageJS(t *testing.T) {
	cases := []struct {
		path string
		want Language
	}{
		{"src/a.js", LangJavaScript},
		{"src/a.jsx", LangJavaScript},
		{"src/a.mjs", LangJavaScript},
		{"src/a.cjs", LangJavaScript},
		{"src/a.ts", LangTypeScript},
		{"src/a.mts", LangTypeScript},
		{"src/a.cts", LangTypeScript},
		{"src/a.d.ts", LangTypeScript},
		{"src/a.tsx", LangTSX},
		{"vendor/lib.min.js", LangUnknown}, // minified = generated
	}
	for _, c := range cases {
		if got := DetectLanguage(c.path); got != c.want {
			t.Errorf("DetectLanguage(%q) = %d, want %d", c.path, got, c.want)
		}
	}
}

func TestJSBrokenFileIsPartial(t *testing.T) {
	res := parseFixture(t, "javascript", "src/broken.js")
	if !res.Partial {
		t.Fatal("expected partial=true")
	}
	if nodeByID(res, "src.broken.ok") == nil {
		t.Fatalf("intact declarations should be salvaged: %v", ids(res))
	}
}
