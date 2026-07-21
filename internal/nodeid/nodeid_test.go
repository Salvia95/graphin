package nodeid

import "testing"

func TestMethodIDFormats(t *testing.T) {
	got := Method("com.example", "OrderService", "process", []string{"ProcessRequest", "Boolean"})
	if got != "com.example.OrderService.process(ProcessRequest,Boolean)" {
		t.Fatal(got)
	}
	// §2.3: 파라미터 타입은 선언부 텍스트 그대로 — 제네릭 포함
	got = Method("com.example", "OrderService", "batch", []string{"List<ProcessRequest>"})
	if got != "com.example.OrderService.batch(List<ProcessRequest>)" {
		t.Fatal(got)
	}
	if Method("", "", "main", nil) != "main()" {
		t.Fatal("bare function ID")
	}
}

func TestPythonSuffixOnlyOnRedefinition(t *testing.T) {
	if got := Python("app.svc", "", "retry", 1); got != "app.svc.retry" {
		t.Fatal(got)
	}
	if got := Python("app.svc", "", "retry", 2); got != "app.svc.retry#2" {
		t.Fatal(got)
	}
	if got := Python("app.svc", "Order", "process", 1); got != "app.svc.Order.process" {
		t.Fatal(got)
	}
}

func TestFileClassKotlin(t *testing.T) {
	cases := map[string]string{
		"com/example/util/Money.kt": "MoneyKt",
		"money.kt":                  "MoneyKt",
		"build.gradle.kts":          "Build.gradleKt",
	}
	for in, want := range cases {
		if got := FileClassKotlin(in); got != want {
			t.Errorf("FileClassKotlin(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSimpleAndDisplay(t *testing.T) {
	if Simple("com.example.OrderService.process(ProcessRequest)") != "process" {
		t.Fatal("simple of method")
	}
	if Simple("app.svc.retry#2") != "retry" {
		t.Fatal("simple strips arity suffix")
	}
	if Display("OrderService", "process") != "OrderService.process" {
		t.Fatal("member display")
	}
	if Display("Outer.Inner", "run") != "Inner.run" {
		t.Fatal("display uses last container segment")
	}
	if Display("", "PaymentPort") != "PaymentPort" {
		t.Fatal("top-level display")
	}
}
