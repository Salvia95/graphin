package parse

import (
	"testing"

	"github.com/Salvia95/graphin/internal/nodeid"
)

// Every grammar-parsed file gets one whole-file node, unranked, so a line
// outside every symbol still has an owner. Plain and markdown files already
// had their file node and must not gain a second one or become unranked.
func TestCodeFileNode(t *testing.T) {
	cases := map[string]string{
		"a/svc.go":      "package a\n\nimport \"fmt\"\n\nfunc F() { fmt.Println() }\n",
		"a/svc.py":      "import os\n\ndef f():\n    return os.sep\n",
		"a/svc.ts":      "import { x } from \"@/y\";\ndescribe(\"x\", () => {});\n",
		"a/svc.tsx":     "export const A = () => <div />;\n",
		"a/svc.js":      "module.exports = {};\n",
		"a/Svc.java":    "package a;\nimport java.util.List;\nclass Svc {}\n",
		"a/svc.kt":      "package a\nimport b.C\nfun f() = 1\n",
		"a/empty.ts":    "",
		"a/config.yaml": "k: v\n",
		"a/README.md":   "# T\n\nbody\n",
	}
	for rel, src := range cases {
		res, err := File(rel, []byte(src))
		if err != nil {
			t.Fatalf("%s: %v", rel, err)
		}
		var files []Node
		for _, n := range res.Nodes {
			if n.Kind == nodeid.KindFile {
				files = append(files, n)
			}
		}
		if len(files) != 1 {
			t.Fatalf("%s: %d file nodes, want 1", rel, len(files))
		}
		f := files[0]
		if f.ID != rel || f.StartByte != 0 || int(f.EndByte) != len(src) || f.StartLine != 1 {
			t.Errorf("%s: file node %q spans %d-%d line %d", rel, f.ID, f.StartByte, f.EndByte, f.StartLine)
		}
		grammar := grammarOf(res.Lang)
		if f.Unranked != grammar {
			t.Errorf("%s: Unranked = %v, want %v", rel, f.Unranked, grammar)
		}
		if grammar && (len(f.BodyTokens) != 0 || len(f.Calls) != 0 || f.Hash != res.FileHash) {
			t.Errorf("%s: code file node carries tokens %d, calls %d, or a hash other than the file's",
				rel, len(f.BodyTokens), len(f.Calls))
		}
	}
}

func grammarOf(l Language) bool {
	return l != LangPlain && l != LangMarkdown && l != LangDBSchema
}

// A root-level Python module that defines a function named after its own
// extension produces a symbol whose dotted id is the file's path. The symbol
// keeps the id; the file goes without a node rather than overwrite it.
func TestCodeFileNodeYieldsToACollidingSymbol(t *testing.T) {
	res, err := File("setup.py", []byte("def py():\n    pass\n"))
	if err != nil {
		t.Fatal(err)
	}
	var n int
	for _, nd := range res.Nodes {
		if nd.ID == "setup.py" {
			n++
			if nd.Kind != nodeid.KindFunction {
				t.Fatalf("setup.py is a %s node, want the function", nd.Kind)
			}
		}
	}
	if n != 1 {
		t.Fatalf("%d nodes named setup.py, want 1", n)
	}
}
