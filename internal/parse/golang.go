package parse

import (
	"strings"

	ts "github.com/tree-sitter/go-tree-sitter"

	"github.com/Salvia95/graphin/internal/nodeid"
)

// goPackage derives the package ID from the workspace-relative directory:
// "internal/graph/engine.go" → "internal.graph". Go's package is a directory,
// not a file, so this is a declared-package language (FileScoped is false) and
// every file in a directory shares one ID — which is what makes the same-package
// confidence tier mean what it says.
//
// A file at the repository root has no directory to name, so it falls back to
// the declared package clause.
func goPackage(relPath, declared string) string {
	dir := relPath
	if i := strings.LastIndexByte(dir, '/'); i >= 0 {
		dir = dir[:i]
	} else {
		dir = ""
	}
	dir = strings.TrimPrefix(dir, "./")
	if dir == "" || dir == "." {
		return declared
	}
	return strings.ReplaceAll(dir, "/", ".")
}

type goExtractor struct {
	src      []byte
	res      *FileResult
	ordinals map[string]int // container+"\x00"+name → definitions seen
}

func extractGo(src []byte, root *ts.Node, res *FileResult) {
	ex := &goExtractor{src: src, res: res, ordinals: map[string]int{}}

	// The package clause comes first in every legal Go file, and the ID of
	// every node below depends on it, so resolve it before anything else.
	declared := ""
	eachNamed(root, func(c *ts.Node) {
		if c.Kind() == "package_clause" && declared == "" {
			declared = text(c.NamedChild(0), src)
		}
	})
	res.Package = goPackage(res.RelPath, declared)

	eachNamed(root, func(c *ts.Node) {
		switch c.Kind() {
		case "package_clause":
		case "import_declaration":
			ex.imports(c)
		default:
			ex.visit(c)
		}
	})
}

// imports records each import path twice, in the dotted form the resolver
// compares against (§2.1.3 scope ranking):
//
//	github.com/Salvia95/graphin/internal/graph
//	  → github.com.Salvia95.graphin.internal.graph.*   (the path as written)
//	  → internal.graph.*                               (the two-segment tail)
//
// The tail is what actually matches: a package's ID here is its directory, and
// an intra-module import path is the module path followed by that directory.
// Parsing is per-file and pure, so it cannot read go.mod to learn where the
// module prefix ends — the tail is the deterministic approximation, bounded to
// one extra string per import. Without it every cross-package call inside the
// repository would fall to the global tier and read as 0.80 when it is known.
func (ex *goExtractor) imports(n *ts.Node) {
	var walk func(c *ts.Node)
	walk = func(c *ts.Node) {
		if c.Kind() == "import_spec" {
			path := ""
			eachNamed(c, func(d *ts.Node) {
				if strings.Contains(d.Kind(), "string") {
					path = strings.Trim(text(d, ex.src), `"`+"`")
				}
			})
			if path == "" {
				return
			}
			dotted := strings.ReplaceAll(path, "/", ".")
			ex.res.Imports = append(ex.res.Imports, dotted+".*")
			if segs := strings.Split(path, "/"); len(segs) >= 2 {
				tail := segs[len(segs)-2] + "." + segs[len(segs)-1]
				if tail != dotted {
					ex.res.Imports = append(ex.res.Imports, tail+".*")
				}
			}
			return
		}
		eachNamed(c, walk)
	}
	walk(n)
}

func (ex *goExtractor) visit(n *ts.Node) {
	switch n.Kind() {
	case "type_declaration":
		eachNamed(n, func(c *ts.Node) {
			if c.Kind() == "type_spec" || c.Kind() == "type_alias" {
				ex.typeSpec(c)
			}
		})
	case "function_declaration":
		ex.function(n, "")
	case "method_declaration":
		ex.method(n)
	case "ERROR": // salvage definitions swallowed by a parse error (§2.4)
		eachNamed(n, ex.visit)
	}
}

func (ex *goExtractor) ordinal(container, name string) int {
	key := container + "\x00" + name
	ex.ordinals[key]++
	return ex.ordinals[key]
}

// typeSpec turns `type X struct{…}` / `type X interface{…}` / `type X = Y` into
// one node. Interfaces record their embedded interfaces as supertypes, which is
// the only Go relation that means what `extends` means. Struct embedding is
// composition, not subtyping, so it is deliberately not a supertype here.
func (ex *goExtractor) typeSpec(n *ts.Node) {
	src := ex.src
	name := text(n.ChildByFieldName("name"), src)
	if name == "" {
		return
	}
	kind := nodeid.KindClass
	var supers []string
	if body := n.ChildByFieldName("type"); body != nil && body.Kind() == "interface_type" {
		kind = nodeid.KindInterface
		eachNamed(body, func(el *ts.Node) {
			if el.Kind() == "type_elem" {
				supers = append(supers, text(el, src))
			}
		})
	}
	node := Node{
		ID:          nodeid.Python(ex.res.Package, "", name, ex.ordinal("", name)),
		DisplayName: name,
		SimpleName:  name,
		Kind:        kind,
		Hash:        subtreeHash(n, src),
		Supers:      supers,
	}
	node.StartByte, node.EndByte = span(n)
	ex.res.Nodes = append(ex.res.Nodes, node)
}

func (ex *goExtractor) function(n *ts.Node, container string) {
	src := ex.src
	name := text(n.ChildByFieldName("name"), src)
	if name == "" {
		return
	}
	kind := nodeid.KindFunction
	if container != "" {
		kind = nodeid.KindMethod
	}
	min, max := goArity(src, n.ChildByFieldName("parameters"))
	node := Node{
		ID:          nodeid.Python(ex.res.Package, container, name, ex.ordinal(container, name)),
		DisplayName: nodeid.Display(container, name),
		SimpleName:  name,
		Kind:        kind,
		Container:   container,
		Hash:        subtreeHash(n, src),
		ArityMin:    min,
		ArityMax:    max,
	}
	node.StartByte, node.EndByte = span(n)
	if body := n.ChildByFieldName("body"); body != nil {
		node.Calls = goCalls(src, body)
		node.DBRefs = appendSQLRefs(node.DBRefs, LangGo, src, body)
	}
	ex.res.Nodes = append(ex.res.Nodes, node)
}

// method attaches the function to its receiver type, which is the container in
// every other language's terms: `func (e *Engine) ApplyFile(…)` becomes
// `internal.graph.Engine.ApplyFile`.
func (ex *goExtractor) method(n *ts.Node) {
	recv := goReceiverType(ex.src, n.ChildByFieldName("receiver"))
	ex.function(n, recv)
}

// goReceiverType reads the type name out of `(e *Engine)` or `(e Engine[T])`,
// dropping the pointer, the binder name and any type parameters — the node it
// must attach to is declared under the bare name.
func goReceiverType(src []byte, recv *ts.Node) string {
	if recv == nil {
		return ""
	}
	name := ""
	var walk func(n *ts.Node)
	walk = func(n *ts.Node) {
		if name != "" {
			return
		}
		if n.Kind() == "type_identifier" {
			name = text(n, src)
			return
		}
		eachNamed(n, walk)
	}
	eachNamed(recv, func(d *ts.Node) {
		if d.Kind() == "parameter_declaration" {
			if t := d.ChildByFieldName("type"); t != nil {
				walk(t)
			}
		}
	})
	return name
}

// goArity counts declared parameters. Go has no default arguments, so the range
// is a point unless the signature is variadic — one declaration can still name
// several parameters (`a, b int`), and an unnamed one (`func f(int)`) is still
// a parameter.
func goArity(src []byte, params *ts.Node) (int, int) {
	if params == nil {
		return 0, 0
	}
	count, variadic := 0, false
	eachNamed(params, func(p *ts.Node) {
		switch p.Kind() {
		case "parameter_declaration":
			names := 0
			eachNamed(p, func(c *ts.Node) {
				if c.Kind() == "identifier" {
					names++
				}
			})
			if names == 0 {
				names = 1 // unnamed parameter: `func f(int)`
			}
			count += names
		case "variadic_parameter_declaration":
			variadic = true
		}
	})
	if variadic {
		return count, nodeid.UnboundedArity
	}
	return count, count
}

func goCalls(src []byte, body *ts.Node) []Call {
	var calls []Call
	var walk func(n *ts.Node)
	walk = func(n *ts.Node) {
		if n.Kind() == "call_expression" {
			c := Call{}
			if fn := n.ChildByFieldName("function"); fn != nil {
				switch fn.Kind() {
				case "identifier":
					c.Name = text(fn, src)
				case "selector_expression":
					c.Name = text(fn.ChildByFieldName("field"), src)
					c.Recv = text(fn.ChildByFieldName("operand"), src)
				}
			}
			if args := n.ChildByFieldName("arguments"); args != nil {
				c.Args = int(args.NamedChildCount())
			}
			if c.Name != "" {
				calls = append(calls, c)
			}
		}
		eachNamed(n, walk)
	}
	walk(body)
	return calls
}
