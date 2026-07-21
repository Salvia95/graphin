package parse

import (
	"strings"

	ts "github.com/tree-sitter/go-tree-sitter"

	"github.com/llls2542/graphin/internal/nodeid"
)

func extractJava(src []byte, root *ts.Node, res *FileResult) {
	var visit func(c *ts.Node)
	visit = func(c *ts.Node) {
		switch c.Kind() {
		case "package_declaration":
			if id := c.NamedChild(0); id != nil {
				res.Package = text(id, src)
			}
		case "import_declaration":
			imp := strings.TrimSuffix(strings.TrimSpace(strings.TrimPrefix(text(c, src), "import")), ";")
			imp = strings.TrimSpace(strings.TrimPrefix(imp, "static"))
			res.Imports = append(res.Imports, strings.TrimSpace(imp))
		case "class_declaration", "interface_declaration", "enum_declaration", "record_declaration":
			javaType(src, c, "", res)
		case "ERROR": // salvage declarations swallowed by a parse error (§2.4)
			eachNamed(c, visit)
		}
	}
	eachNamed(root, visit)
}

func javaType(src []byte, n *ts.Node, outer string, res *FileResult) {
	name := text(n.ChildByFieldName("name"), src)
	if name == "" {
		return
	}
	chain := joinContainer(outer, name)
	kind := nodeid.KindClass
	if n.Kind() == "interface_declaration" {
		kind = nodeid.KindInterface
	}

	node := Node{
		ID:          nodeid.Class(res.Package, chain),
		DisplayName: nodeid.Display(outer, name),
		SimpleName:  name,
		Kind:        kind,
		Container:   outer,
		Hash:        subtreeHash(n, src),
	}
	node.StartByte, node.EndByte = span(n)
	if sc := n.ChildByFieldName("superclass"); sc != nil { // (superclass (type))
		if t := sc.NamedChild(0); t != nil {
			node.Supers = append(node.Supers, text(t, src))
		}
	}
	if ifs := n.ChildByFieldName("interfaces"); ifs != nil { // (super_interfaces (type_list ...))
		if list := ifs.NamedChild(0); list != nil {
			eachNamed(list, func(t *ts.Node) {
				node.Supers = append(node.Supers, text(t, src))
			})
		}
	}
	res.Nodes = append(res.Nodes, node)

	body := n.ChildByFieldName("body")
	if body == nil {
		return
	}
	eachNamed(body, func(m *ts.Node) {
		switch m.Kind() {
		case "method_declaration", "constructor_declaration":
			javaMethod(src, m, chain, res)
		case "class_declaration", "interface_declaration", "enum_declaration", "record_declaration":
			javaType(src, m, chain, res)
		}
	})
}

func javaMethod(src []byte, m *ts.Node, container string, res *FileResult) {
	name := text(m.ChildByFieldName("name"), src)
	if name == "" {
		return
	}
	params, min, max := javaParams(src, m.ChildByFieldName("parameters"))

	node := Node{
		ID:          nodeid.Method(res.Package, container, name, params),
		DisplayName: nodeid.Display(container, name),
		SimpleName:  name,
		Kind:        nodeid.KindMethod,
		Container:   container,
		Hash:        subtreeHash(m, src),
		ArityMin:    min,
		ArityMax:    max,
		Params:      params,
	}
	node.StartByte, node.EndByte = span(m)
	if body := m.ChildByFieldName("body"); body != nil {
		node.Calls = javaCalls(src, body)
	}
	res.Nodes = append(res.Nodes, node)
}

// javaParams returns parameter type texts as written plus the arity range
// (varargs open the maximum).
func javaParams(src []byte, params *ts.Node) ([]string, int, int) {
	if params == nil {
		return nil, 0, 0
	}
	var types []string
	varargs := false
	eachNamed(params, func(p *ts.Node) {
		switch p.Kind() {
		case "formal_parameter":
			types = append(types, text(p.ChildByFieldName("type"), src))
		case "spread_parameter": // "String... args"
			if t := p.NamedChild(0); t != nil {
				types = append(types, text(t, src)+"...")
			}
			varargs = true
		}
	})
	min, max := len(types), len(types)
	if varargs {
		min--
		max = nodeid.UnboundedArity
	}
	return types, min, max
}

func javaCalls(src []byte, body *ts.Node) []Call {
	var calls []Call
	var walk func(n *ts.Node)
	walk = func(n *ts.Node) {
		switch n.Kind() {
		case "method_invocation":
			c := Call{
				Name: text(n.ChildByFieldName("name"), src),
				Recv: text(n.ChildByFieldName("object"), src),
			}
			if args := n.ChildByFieldName("arguments"); args != nil {
				c.Args = int(args.NamedChildCount())
			}
			if c.Name != "" {
				calls = append(calls, c)
			}
		case "object_creation_expression": // new Receipt(...) → constructor call
			typeName := text(n.ChildByFieldName("type"), src)
			if i := strings.LastIndexByte(typeName, '.'); i >= 0 {
				typeName = typeName[i+1:]
			}
			if j := strings.IndexByte(typeName, '<'); j >= 0 {
				typeName = typeName[:j]
			}
			c := Call{Name: typeName}
			if args := n.ChildByFieldName("arguments"); args != nil {
				c.Args = int(args.NamedChildCount())
			}
			if c.Name != "" {
				calls = append(calls, c)
			}
		case "method_reference": // this::process — arg count unknowable (-1)
			if cnt := n.NamedChildCount(); cnt > 0 {
				if last := n.NamedChild(cnt - 1); last != nil {
					calls = append(calls, Call{Name: text(last, src), Args: -1})
				}
			}
		}
		eachNamed(n, walk)
	}
	walk(body)
	return calls
}
