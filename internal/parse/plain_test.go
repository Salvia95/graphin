package parse

import (
	"slices"
	"testing"

	"github.com/Salvia95/graphin/internal/nodeid"
)

func TestPlainFileBecomesSingleNode(t *testing.T) {
	src := []byte(`spring:
  datasource:
    url: jdbc:postgresql://localhost/kinder
    username: app
`)
	res, err := File("config/application.yml", src)
	if err != nil {
		t.Fatal(err)
	}
	if res.Lang != LangPlain || res.Partial {
		t.Fatalf("lang=%v partial=%v", res.Lang, res.Partial)
	}
	if len(res.Nodes) != 1 {
		t.Fatalf("plain file must yield exactly one node, got %d", len(res.Nodes))
	}
	n := res.Nodes[0]
	if n.ID != "config/application.yml" || n.Kind != nodeid.KindFile {
		t.Fatalf("node = %+v", n)
	}
	if n.SimpleName != "application.yml" { // Tier-0 filename match
		t.Fatalf("simple name = %q", n.SimpleName)
	}
	if n.StartByte != 0 || int(n.EndByte) != len(src) {
		t.Fatalf("span = %d..%d, want whole file", n.StartByte, n.EndByte)
	}
	for _, want := range []string{"datasource", "postgresql", "kinder"} {
		if !slices.Contains(n.BodyTokens, want) {
			t.Fatalf("BodyTokens missing %q: %v", want, n.BodyTokens)
		}
	}
	if res.Package != "config" {
		t.Fatalf("package = %q", res.Package)
	}
}

func TestPlainBasenamesDetected(t *testing.T) {
	for _, p := range []string{"Dockerfile", "deploy/Makefile", "db/init.sql"} {
		if DetectLanguage(p) != LangPlain {
			t.Errorf("%s should be LangPlain", p)
		}
	}
	// Markdown left the plain fallback: it has headings, so it gets section
	// nodes instead (docs/markdown-spec.md §3.1).
	for _, p := range []string{"docs/guide.md", "NOTES.markdown"} {
		if DetectLanguage(p) != LangMarkdown {
			t.Errorf("%s should be LangMarkdown", p)
		}
	}
	for _, p := range []string{"img/logo.png", "bin/tool", "a.class"} {
		if DetectLanguage(p) != LangUnknown {
			t.Errorf("%s should stay unknown", p)
		}
	}
}

// TestBodyTokensOnParsedNodes proves §보완 B: string literals inside method
// bodies are searchable tokens now.
func TestBodyTokensOnParsedNodes(t *testing.T) {
	res := parseFixture(t, "java", "src/main/java/com/example/order/domain/OrderService.java")
	m := nodeByID(res, "com.example.order.domain.OrderService.process(ProcessRequest,boolean)")
	if m == nil {
		t.Fatal("method missing")
	}
	// "processing" only occurs in the log string literal, never in a name.
	if !slices.Contains(m.BodyTokens, "processing") {
		t.Fatalf("literal token missing from BodyTokens: %v", m.BodyTokens)
	}
}
