package tools

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Salvia95/graphin/internal/mcp"
)

var updateReference = flag.Bool("update", false, "rewrite docs/tools.md from the tool definitions")

// docs/tools.md is generated from the registry. This test is how CI notices
// a description or schema changed without the reference following — the
// drift that left the README's hand-written tool table months out of date.
func TestToolReferenceUpToDate(t *testing.T) {
	reg := mcp.NewRegistry()
	Register(reg, nil)
	got := RenderReference(reg)
	path := filepath.Join("..", "..", "..", "docs", "tools.md")
	if *updateReference {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v — generate it with: go test ./internal/mcp/tools -run TestToolReferenceUpToDate -update", path, err)
	}
	if string(want) != got {
		t.Fatalf("docs/tools.md is out of date with the tool definitions; regenerate it with:\n" +
			"  go test ./internal/mcp/tools -run TestToolReferenceUpToDate -update")
	}
}

// Every registered tool is in the reference, grouped or under "Other".
func TestToolReferenceCoversEveryTool(t *testing.T) {
	reg := mcp.NewRegistry()
	Register(reg, nil)
	doc := RenderReference(reg)
	for _, tool := range reg.List() {
		if !strings.Contains(doc, "\n### "+tool.Name+"\n") {
			t.Errorf("%s is missing from the reference", tool.Name)
		}
	}
}
