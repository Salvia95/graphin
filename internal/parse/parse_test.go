package parse

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Salvia95/graphin/internal/nodeid"
)

const fixtures = "../../testdata/fixtures"

func parseFixture(t *testing.T, lang, rel string) *FileResult {
	t.Helper()
	abs := filepath.Join(fixtures, lang, rel)
	src, err := os.ReadFile(abs)
	if err != nil {
		t.Fatal(err)
	}
	res, err := File(rel, src)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func nodeByID(res *FileResult, id string) *Node {
	for i := range res.Nodes {
		if res.Nodes[i].ID == id {
			return &res.Nodes[i]
		}
	}
	return nil
}

func ids(res *FileResult) []string {
	out := make([]string, len(res.Nodes))
	for i, n := range res.Nodes {
		out[i] = n.ID
	}
	return out
}

// --- Java ---

func TestJavaOverloadIDsDistinct(t *testing.T) {
	res := parseFixture(t, "java", "src/main/java/com/example/order/domain/OrderService.java")
	if res.Partial {
		t.Fatal("fixture should parse cleanly")
	}
	if res.Package != "com.example.order.domain" {
		t.Fatalf("package = %q", res.Package)
	}
	for _, want := range []string{
		"com.example.order.domain.OrderService",
		"com.example.order.domain.OrderService.process(ProcessRequest)",
		"com.example.order.domain.OrderService.process(ProcessRequest,boolean)",
		"com.example.order.domain.OrderService.cancelPayment(long,String)",
		"com.example.order.domain.OrderService.batch(List<ProcessRequest>)", // §2.3: 타입 텍스트 그대로
		"com.example.order.domain.OrderService.Receipt",
		"com.example.order.domain.OrderService.Receipt.orderId()",
	} {
		if nodeByID(res, want) == nil {
			t.Errorf("missing node %s\nhave: %v", want, ids(res))
		}
	}
}

func TestJavaImplementsCaptured(t *testing.T) {
	res := parseFixture(t, "java", "src/main/java/com/example/payment/adapter/PgPaymentAdapter.java")
	cls := nodeByID(res, "com.example.payment.adapter.PgPaymentAdapter")
	if cls == nil {
		t.Fatal("class node missing")
	}
	if len(cls.Supers) != 1 || cls.Supers[0] != "PaymentPort" {
		t.Fatalf("Supers = %v, want [PaymentPort]", cls.Supers)
	}
	port := parseFixture(t, "java", "src/main/java/com/example/payment/port/PaymentPort.java")
	if n := nodeByID(port, "com.example.payment.port.PaymentPort"); n == nil || n.Kind != nodeid.KindInterface {
		t.Fatalf("PaymentPort should be an interface node, got %+v", n)
	}
}

func TestJavaCallsExtracted(t *testing.T) {
	res := parseFixture(t, "java", "src/main/java/com/example/order/adapter/in/web/OrderController.java")
	m := nodeByID(res, "com.example.order.adapter.in.web.OrderController.handle(ProcessRequest)")
	if m == nil {
		t.Fatalf("handle method missing: %v", ids(res))
	}
	found := false
	for _, c := range m.Calls {
		if c.Name == "process" && c.Args == 1 && c.Recv == "orderService" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected call process/1 on orderService, got %+v", m.Calls)
	}
}

// --- Kotlin ---

func TestKotlinDefaultArgArityRange(t *testing.T) {
	res := parseFixture(t, "kotlin", "src/main/kotlin/com/example/order/OrderService.kt")
	if res.Partial {
		t.Fatal("fixture should parse cleanly")
	}
	cancel := nodeByID(res, "com.example.order.OrderService.cancel(Long,String,Boolean)")
	if cancel == nil {
		t.Fatalf("cancel missing: %v", ids(res))
	}
	if cancel.ArityMin != 1 || cancel.ArityMax != 3 {
		t.Fatalf("cancel arity = %d..%d, want 1..3 (§2.1.3 기본 인자 호환 범위)", cancel.ArityMin, cancel.ArityMax)
	}
}

func TestKotlinTopLevelFileKtAndReceiver(t *testing.T) {
	res := parseFixture(t, "kotlin", "src/main/kotlin/com/example/util/Money.kt")
	// §2.3: 확장 함수 리시버가 첫 파라미터로 — com.example.util.MoneyKt.toWon(Long)
	if nodeByID(res, "com.example.util.MoneyKt.toWon(Long)") == nil {
		t.Fatalf("MoneyKt.toWon(Long) missing: %v", ids(res))
	}
	if nodeByID(res, "com.example.util.MoneyKt.sum(List<Money>)") == nil {
		t.Fatalf("MoneyKt.sum(List<Money>) missing: %v", ids(res))
	}
}

func TestKotlinInterfaceAndDelegation(t *testing.T) {
	res := parseFixture(t, "kotlin", "src/main/kotlin/com/example/payment/PaymentGateway.kt")
	gw := nodeByID(res, "com.example.payment.PaymentGateway")
	if gw == nil || gw.Kind != nodeid.KindInterface {
		t.Fatalf("PaymentGateway should be interface, got %+v", gw)
	}
	pg := nodeByID(res, "com.example.payment.PgGateway")
	if pg == nil || len(pg.Supers) != 1 || pg.Supers[0] != "PaymentGateway" {
		t.Fatalf("PgGateway Supers = %+v", pg)
	}
}

func TestKotlinCallWithOneArgAgainstDefaults(t *testing.T) {
	res := parseFixture(t, "kotlin", "src/main/kotlin/com/example/order/CancelFlow.kt")
	run := nodeByID(res, "com.example.order.CancelFlow.run(Long)")
	if run == nil {
		t.Fatalf("run missing: %v", ids(res))
	}
	var cancelCall *Call
	for i, c := range run.Calls {
		if c.Name == "cancel" {
			cancelCall = &run.Calls[i]
		}
	}
	if cancelCall == nil || cancelCall.Args != 1 {
		t.Fatalf("expected cancel call with 1 arg, got %+v", run.Calls)
	}
}

// --- Python ---

func TestPythonAritySuffixOnlyOnRedefinition(t *testing.T) {
	res := parseFixture(t, "python", "app/services/payment.py")
	if nodeByID(res, "app.services.payment.retry") == nil {
		t.Fatalf("first retry should have no suffix: %v", ids(res))
	}
	if nodeByID(res, "app.services.payment.retry#2") == nil {
		t.Fatalf("second retry should be #2: %v", ids(res))
	}
	if nodeByID(res, "app.services.payment.PaymentClient.charge") == nil {
		t.Fatalf("single-definition method must have no suffix: %v", ids(res))
	}
}

func TestPythonStarArgsOpenArity(t *testing.T) {
	res := parseFixture(t, "python", "app/services/order.py")
	cancel := nodeByID(res, "app.services.order.OrderService.cancel")
	if cancel == nil {
		t.Fatalf("cancel missing: %v", ids(res))
	}
	if cancel.ArityMin != 1 || cancel.ArityMax != nodeid.UnboundedArity {
		t.Fatalf("cancel arity = %d..%d, want 1..unbounded", cancel.ArityMin, cancel.ArityMax)
	}
	proc := nodeByID(res, "app.services.order.OrderService.process")
	if proc == nil || proc.ArityMin != 1 || proc.ArityMax != 1 {
		t.Fatalf("process arity should exclude self: %+v", proc)
	}
}

// --- partial (§2.4 / §7-P2-③) ---

func TestSyntaxErrorFilesAreParsedPartial(t *testing.T) {
	cases := []struct{ lang, rel string }{
		{"java", "src/main/java/com/example/broken/Broken.java"},
		{"kotlin", "src/main/kotlin/com/example/broken/Broken.kt"},
		{"python", "app/broken.py"},
	}
	for _, c := range cases {
		res := parseFixture(t, c.lang, c.rel)
		if !res.Partial {
			t.Errorf("%s: expected partial=true", c.rel)
		}
		if len(res.Nodes) == 0 {
			t.Errorf("%s: partial tree should still yield salvageable nodes", c.rel)
		}
	}
}

// TestSubtreeHashStableUnderOffsetShift is the precondition for the 2-Track
// pipeline: identical method text hashes identically regardless of position.
func TestSubtreeHashStableUnderOffsetShift(t *testing.T) {
	rel := "src/main/java/com/example/common/Logger.java"
	src, err := os.ReadFile(filepath.Join(fixtures, "java", rel))
	if err != nil {
		t.Fatal(err)
	}
	before, err := File(rel, src)
	if err != nil {
		t.Fatal(err)
	}
	shifted := append([]byte("import java.util.ArrayList;\n"), src...)
	after, err := File(rel, shifted)
	if err != nil {
		t.Fatal(err)
	}

	id := "com.example.common.Logger.info(String)"
	b, a := nodeByID(before, id), nodeByID(after, id)
	if b == nil || a == nil {
		t.Fatal("info(String) missing")
	}
	if b.Hash != a.Hash {
		t.Fatal("subtree hash changed although method text is identical")
	}
	if b.StartByte == a.StartByte {
		t.Fatal("offsets should have shifted")
	}
}
