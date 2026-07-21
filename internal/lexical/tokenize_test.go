package lexical

import (
	"reflect"
	"slices"
	"testing"
)

// TestCamelSnakeSplitTokens covers the §2.1.1 token decomposition rules.
func TestCamelSnakeSplitTokens(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"cancelPayment", []string{"cancel", "payment"}},
		{"HTTPServer2", []string{"http", "server", "2"}},
		{"snake_case_name", []string{"snake", "case", "name"}},
		{"OrderService", []string{"order", "service"}},
		{"kebab-case", []string{"kebab", "case"}},
		{"plain", []string{"plain"}},
	}
	for _, c := range cases {
		if got := SplitIdentifier(c.in); !reflect.DeepEqual(got, c.want) {
			t.Errorf("SplitIdentifier(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestTokenizeJoinsCompositeForms(t *testing.T) {
	got := Tokenize("OrderService.process(ProcessRequest)")
	for _, want := range []string{"order", "service", "orderservice", "process", "processrequest"} {
		if !slices.Contains(got, want) {
			t.Errorf("Tokenize missing %q in %v", want, got)
		}
	}
	// snake and camel meet on the joined form
	if !slices.Contains(Tokenize("cancel_payment"), "cancelpayment") {
		t.Error("snake_case did not produce joined form")
	}
}

func TestBuildDocTokensCoversNameFQNAndSignature(t *testing.T) {
	got := BuildDocTokens(
		"cancelPayment",
		"com.example.order.domain.OrderService.cancelPayment(long,String)",
		"long orderId, String reason",
	)
	for _, want := range []string{"cancelpayment", "cancel", "payment", "orderservice", "domain", "reason"} {
		if !slices.Contains(got, want) {
			t.Errorf("BuildDocTokens missing %q", want)
		}
	}
}
