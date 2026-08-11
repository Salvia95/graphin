package parse

import (
	"slices"
	"testing"

	"github.com/Salvia95/graphin/internal/nodeid"
)

func goParse(t *testing.T, rel, src string) *FileResult {
	t.Helper()
	res, err := File(rel, []byte(src))
	if err != nil {
		t.Fatalf("parse %s: %v", rel, err)
	}
	if res.Lang != LangGo {
		t.Fatalf("lang = %v, want LangGo", res.Lang)
	}
	return res
}

const goSample = `package graph

import (
	"fmt"
	ts "github.com/tree-sitter/go-tree-sitter"
	"github.com/Salvia95/graphin/internal/merkle"
)

type Engine struct {
	dir string
}

type Sink interface {
	Reader
	Write(p []byte) (int, error)
}

func Open(dir string, log Logger) (*Engine, error) {
	return &Engine{dir: dir}, nil
}

func (e *Engine) ApplyFile(res *FileResult, diff merkle.FileDiff) {
	e.resolve(res)
	fmt.Println(diff)
}

func variadic(a, b int, rest ...string) {}

func unnamed(int, string) {}
`

// The package is the directory, not the file — every file under
// internal/graph must land in one package ID, because that is what makes the
// same-package confidence tier mean anything.
func TestGoPackageIsTheDirectory(t *testing.T) {
	res := goParse(t, "internal/graph/engine.go", goSample)
	if res.Package != "internal.graph" {
		t.Fatalf("package = %q, want internal.graph", res.Package)
	}
	// A root-level file has no directory to name and falls back to the clause.
	if got := goParse(t, "main.go", "package main\n\nfunc main() {}\n"); got.Package != "main" {
		t.Fatalf("root package = %q, want main", got.Package)
	}
}

func TestGoNodesAndKinds(t *testing.T) {
	res := goParse(t, "internal/graph/engine.go", goSample)
	for _, tc := range []struct{ id, kind string }{
		{"internal.graph.Engine", nodeid.KindClass},
		{"internal.graph.Sink", nodeid.KindInterface},
		{"internal.graph.Open", nodeid.KindFunction},
		{"internal.graph.Engine.ApplyFile", nodeid.KindMethod},
	} {
		n := nodeByID(res, tc.id)
		if n == nil {
			t.Fatalf("missing node %s (got %v)", tc.id, ids(res))
		}
		if n.Kind != tc.kind {
			t.Errorf("%s kind = %s, want %s", tc.id, n.Kind, tc.kind)
		}
	}
}

// A method belongs to its receiver type. Without this the graph would hold a
// package-level function named ApplyFile and lose the only relation that makes
// "what does Engine do" answerable.
func TestGoMethodAttachesToReceiver(t *testing.T) {
	res := goParse(t, "internal/graph/engine.go", goSample)
	m := nodeByID(res, "internal.graph.Engine.ApplyFile")
	if m == nil {
		t.Fatalf("method not found: %v", ids(res))
	}
	if m.Container != "Engine" {
		t.Fatalf("container = %q, want Engine", m.Container)
	}
	// The pointer and the binder name are not part of the type.
	if m.SimpleName != "ApplyFile" {
		t.Fatalf("simple name = %q", m.SimpleName)
	}
}

// Only interface embedding is a supertype. Struct embedding is composition and
// must not claim to be one.
func TestGoInterfaceEmbeddingIsSuper(t *testing.T) {
	res := goParse(t, "internal/graph/engine.go", goSample)
	sink := nodeByID(res, "internal.graph.Sink")
	if sink == nil || !slices.Contains(sink.Supers, "Reader") {
		t.Fatalf("Sink supers = %v, want Reader", sink.Supers)
	}
	if eng := nodeByID(res, "internal.graph.Engine"); len(eng.Supers) != 0 {
		t.Fatalf("struct must not carry supers: %v", eng.Supers)
	}
}

func TestGoArity(t *testing.T) {
	res := goParse(t, "internal/graph/engine.go", goSample)
	for _, tc := range []struct {
		id       string
		min, max int
	}{
		{"internal.graph.Open", 2, 2},
		{"internal.graph.Engine.ApplyFile", 2, 2},
		{"internal.graph.variadic", 2, nodeid.UnboundedArity}, // `a, b int` is two
		{"internal.graph.unnamed", 2, 2},                      // `func f(int, string)`
	} {
		n := nodeByID(res, tc.id)
		if n == nil {
			t.Fatalf("missing %s", tc.id)
		}
		if n.ArityMin != tc.min || n.ArityMax != tc.max {
			t.Errorf("%s arity = %d..%d, want %d..%d", tc.id, n.ArityMin, n.ArityMax, tc.min, tc.max)
		}
	}
}

func TestGoCalls(t *testing.T) {
	res := goParse(t, "internal/graph/engine.go", goSample)
	m := nodeByID(res, "internal.graph.Engine.ApplyFile")
	var got []string
	for _, c := range m.Calls {
		got = append(got, c.Recv+"|"+c.Name)
	}
	for _, want := range []string{"e|resolve", "fmt|Println"} {
		if !slices.Contains(got, want) {
			t.Fatalf("calls = %v, want %s", got, want)
		}
	}
}

// Imports carry both the path as written and its two-segment tail. The tail is
// the one that matches an intra-repository package ID, because parsing cannot
// read go.mod to learn where the module prefix ends.
func TestGoImportsCarryMatchableTail(t *testing.T) {
	res := goParse(t, "internal/graph/engine.go", goSample)
	for _, want := range []string{
		"github.com.Salvia95.graphin.internal.merkle.*",
		"internal.merkle.*",
		"fmt.*",
	} {
		if !slices.Contains(res.Imports, want) {
			t.Fatalf("imports = %v, want %s", res.Imports, want)
		}
	}
}
