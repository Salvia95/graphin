package parse

import (
	"strings"

	ts "github.com/tree-sitter/go-tree-sitter"

	"github.com/Salvia95/graphin/internal/nodeid"
)

// pyModule derives the module path from the workspace-relative file path:
// "app/services/order.py" → "app.services.order", "__init__.py" collapses to
// its package.
func pyModule(relPath string) string {
	p := strings.TrimSuffix(relPath, ".py")
	p = strings.TrimSuffix(p, "/__init__")
	p = strings.TrimPrefix(p, "./")
	return strings.ReplaceAll(p, "/", ".")
}

type pyExtractor struct {
	src      []byte
	res      *FileResult
	ordinals map[string]int // container+"\x00"+name → definitions seen
}

func extractPython(src []byte, root *ts.Node, res *FileResult) {
	res.Package = pyModule(res.RelPath)
	ex := &pyExtractor{src: src, res: res, ordinals: map[string]int{}}
	eachNamed(root, func(c *ts.Node) {
		switch c.Kind() {
		case "import_statement": // import a.b, c
			eachNamed(c, func(d *ts.Node) {
				res.Imports = append(res.Imports, text(d, src))
			})
		case "import_from_statement": // from a.b import x, y
			mod := text(c.ChildByFieldName("module_name"), src)
			names := false
			eachNamed(c, func(d *ts.Node) {
				if d.Kind() == "dotted_name" && text(d, src) != mod {
					res.Imports = append(res.Imports, mod+"."+text(d, src))
					names = true
				}
			})
			if !names && mod != "" {
				res.Imports = append(res.Imports, mod)
			}
		default:
			ex.visit(c, "")
		}
	})
}

func (ex *pyExtractor) visit(n *ts.Node, container string) {
	switch n.Kind() {
	case "decorated_definition":
		if def := n.ChildByFieldName("definition"); def != nil {
			ex.visit(def, container)
		}
	case "class_definition":
		ex.class(n, container)
	case "function_definition":
		ex.function(n, container)
	case "ERROR": // salvage definitions swallowed by a parse error (§2.4)
		eachNamed(n, func(c *ts.Node) { ex.visit(c, container) })
	}
}

func (ex *pyExtractor) ordinal(container, name string) int {
	key := container + "\x00" + name
	ex.ordinals[key]++
	return ex.ordinals[key]
}

func (ex *pyExtractor) class(n *ts.Node, outer string) {
	src := ex.src
	name := text(n.ChildByFieldName("name"), src)
	if name == "" {
		return
	}
	chain := joinContainer(outer, name)
	ord := ex.ordinal(outer, name)

	node := Node{
		ID:          nodeid.Python(ex.res.Package, outer, name, ord),
		DisplayName: nodeid.Display(outer, name),
		SimpleName:  name,
		Kind:        nodeid.KindClass,
		Container:   outer,
		Hash:        subtreeHash(n, src),
	}
	node.StartByte, node.EndByte = span(n)
	if supers := n.ChildByFieldName("superclasses"); supers != nil {
		eachNamed(supers, func(s *ts.Node) {
			switch s.Kind() {
			case "identifier", "attribute":
				node.Supers = append(node.Supers, text(s, src))
			}
		})
	}
	ex.res.Nodes = append(ex.res.Nodes, node)

	if body := n.ChildByFieldName("body"); body != nil {
		eachNamed(body, func(m *ts.Node) { ex.visit(m, chain) })
	}
}

func (ex *pyExtractor) function(n *ts.Node, container string) {
	src := ex.src
	name := text(n.ChildByFieldName("name"), src)
	if name == "" {
		return
	}
	ord := ex.ordinal(container, name)
	min, max := pyArity(src, n.ChildByFieldName("parameters"), container != "")

	kind := nodeid.KindFunction
	if container != "" {
		kind = nodeid.KindMethod
	}
	node := Node{
		ID:          nodeid.Python(ex.res.Package, container, name, ord),
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
		node.Calls = pyCalls(src, body)
		// nested defs/classes become children of this scope
		chain := joinContainer(container, name)
		eachNamed(body, func(m *ts.Node) {
			switch m.Kind() {
			case "function_definition", "class_definition", "decorated_definition":
				ex.visit(m, chain)
			}
		})
	}
	ex.res.Nodes = append(ex.res.Nodes, node)
}

// pyArity computes the §2.1.3 compatibility range: defaults widen the
// minimum, *args/**kwargs open the maximum, and a leading self/cls on
// methods is excluded.
func pyArity(src []byte, params *ts.Node, isMethod bool) (int, int) {
	if params == nil {
		return 0, 0
	}
	var (
		required, optional int
		open               bool
		first              = true
	)
	eachNamed(params, func(p *ts.Node) {
		kind := p.Kind()
		selfLike := false
		if first && isMethod && kind == "identifier" {
			t := text(p, src)
			selfLike = t == "self" || t == "cls"
		}
		first = false
		if selfLike {
			return
		}
		switch kind {
		case "identifier", "typed_parameter":
			required++
		case "default_parameter", "typed_default_parameter":
			optional++
		case "list_splat_pattern", "dictionary_splat_pattern":
			open = true
		}
	})
	min := required
	max := required + optional
	if open {
		max = nodeid.UnboundedArity
	}
	return min, max
}

func pyCalls(src []byte, body *ts.Node) []Call {
	var calls []Call
	var walk func(n *ts.Node)
	walk = func(n *ts.Node) {
		if n.Kind() == "call" {
			c := Call{}
			if fn := n.ChildByFieldName("function"); fn != nil {
				switch fn.Kind() {
				case "identifier":
					c.Name = text(fn, src)
				case "attribute":
					c.Name = text(fn.ChildByFieldName("attribute"), src)
					c.Recv = text(fn.ChildByFieldName("object"), src)
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
