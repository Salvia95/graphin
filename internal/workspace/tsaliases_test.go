package workspace

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Salvia95/graphin/internal/parse"
)

func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, body := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLoadTSAliases(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		// JSONC: comments and trailing commas, an extends chain, paths in
		// the base config and baseUrl in neither.
		"web/tsconfig.base.json": `{
  // shared
  "compilerOptions": {
    "paths": {
      "@/*": ["./src/*",],
      "@ui/*": ["./src/components/ui/*"],
      "env": ["./src/env.ts"], /* exact */
    },
  },
}`,
		"web/tsconfig.json": `{ "extends": "./tsconfig.base", "compilerOptions": { "strict": true } }`,
	})
	a := loadTSAliases(root, "web/tsconfig.json")
	if a == nil {
		t.Fatal("no aliases loaded")
	}
	for in, want := range map[string]string{
		"@.features.order.useOrder": "web.src.features.order.useOrder",
		"@ui.button.Button":         "web.src.components.ui.button.Button", // longest prefix wins
		"env.API_URL":               "web.src.env.API_URL",
		"env":                       "web.src.env",
	} {
		if got, ok := a.mapImport(in); !ok || got != want {
			t.Errorf("mapImport(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}
	if got, ok := a.mapImport("react.useState"); ok {
		t.Errorf("a package import was rewritten to %q", got)
	}

	// baseUrl: paths resolve against it, and any other bare specifier is
	// tried under it.
	writeTree(t, root, map[string]string{
		"app/tsconfig.json": `{"compilerOptions": {"baseUrl": "src", "paths": {"~/*": ["*"]}}}`,
	})
	b := loadTSAliases(root, "app/tsconfig.json")
	if got, _ := b.mapImport("~.lib.x"); got != "app.src.lib.x" {
		t.Errorf("baseUrl-relative alias: %q", got)
	}
	if got, _ := b.mapImport("components.Button"); got != "app.src.components.Button" {
		t.Errorf("baseUrl bare import: %q", got)
	}
}

// End to end through the workspace: an aliased import makes a cross-module
// call resolve at the imported tier instead of the global one, the nearest
// config governs, and editing the config remaps without touching the
// importing file.
func TestAliasedImportResolvesTheCall(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"web/tsconfig.json":               `{"compilerOptions": {"paths": {"@/*": ["./src/*"]}}}`,
		"web/src/features/order/total.ts": "export function total(items: number[]) { return items.length; }\n",
		"web/src/legacy/total.ts":         "export function total(items: number[]) { return 0; }\n",
		"web/src/pages/cart.ts":           "import { total } from \"@/features/order/total\";\nexport function cart(xs: number[]) { return total(xs); }\n",
	})
	w := bootstrapDocs(t, root)
	defer w.Close()

	callee := "web.src.features.order.total.total"
	conf := func() float32 {
		p, err := w.graph.Explore("web.src.pages.cart.cart", "uses", "", 0.5)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range p.Uses {
			if e.NodeID == callee {
				return e.Confidence
			}
		}
		return 0
	}
	if c := conf(); c != 0.90 {
		t.Fatalf("cart → total via @/ alias: confidence %v, want 0.90 (imported tier)", c)
	}

	// Point the alias somewhere else: the import no longer reaches the
	// feature module, so the edge falls back to the global tier.
	writeTree(t, root, map[string]string{
		"web/tsconfig.json": `{"compilerOptions": {"paths": {"@/*": ["./lib/*"]}}}`,
	})
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) && conf() == 0.90 {
		time.Sleep(50 * time.Millisecond)
	}
	if c := conf(); c == 0.90 {
		t.Fatalf("a tsconfig edit did not remap the import (confidence still %v)", c)
	}
}

// Bare imports of other languages and relative JS imports are never touched.
func TestApplyTSAliasesLeavesOthersAlone(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{"tsconfig.json": `{"compilerOptions": {"baseUrl": "src"}}`})
	w := New(Config{Root: root})
	res := &parse.FileResult{RelPath: "a/m.py", Lang: parse.LangPython, Imports: []string{"os.path"}}
	w.applyTSAliases(res)
	if res.Imports[0] != "os.path" {
		t.Fatalf("python import rewritten: %v", res.Imports)
	}
	js := &parse.FileResult{RelPath: "src/m.ts", Lang: parse.LangTypeScript,
		Imports: []string{"src.b.x", "lib.y"}, ImportBare: []bool{false, true}}
	w.applyTSAliases(js)
	if js.Imports[0] != "src.b.x" || js.Imports[1] != "src.lib.y" {
		t.Fatalf("got %v", js.Imports)
	}
}

// A component rendered from another module through an alias is a used_by
// edge on the component — the caller list a refactor needs.
func TestJSXRenderAcrossModules(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"tsconfig.json":                `{"compilerOptions": {"paths": {"@/*": ["./src/*"]}}}`,
		"src/components/OrderList.tsx": "export function OrderList({ items }: { items: string[] }) { return <ul>{items}</ul>; }\n",
		"src/pages/Cart.tsx":           "import { OrderList } from \"@/components/OrderList\";\nexport default function Cart() { return <main><OrderList items={[]} /></main>; }\n",
	})
	w := bootstrapDocs(t, root)
	defer w.Close()
	p, err := w.graph.Explore("src.components.OrderList.OrderList", "used_by", "", 0.85)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range p.UsedBy {
		if e.NodeID == "src.pages.Cart.Cart" && e.Type == "call" && e.Confidence == 0.90 {
			return
		}
	}
	t.Fatalf("Cart does not render OrderList at the imported tier: %+v", p.UsedBy)
}
