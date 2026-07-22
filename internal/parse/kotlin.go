package parse

import (
	"strings"

	ts "github.com/tree-sitter/go-tree-sitter"

	"github.com/Salvia95/graphin/internal/nodeid"
)

func extractKotlin(src []byte, root *ts.Node, res *FileResult) {
	fileClass := nodeid.FileClassKotlin(res.RelPath)
	var visit func(c *ts.Node)
	visit = func(c *ts.Node) {
		switch c.Kind() {
		case "package_header":
			if id := c.NamedChild(0); id != nil {
				res.Package = text(id, src)
			}
		case "import":
			imp := strings.TrimSpace(strings.TrimPrefix(text(c, src), "import"))
			res.Imports = append(res.Imports, strings.TrimSpace(strings.TrimSuffix(imp, ";")))
		case "class_declaration", "object_declaration":
			kotlinType(src, c, "", res)
		case "function_declaration":
			kotlinFunction(src, c, fileClass, nodeid.KindFunction, res)
		case "ERROR": // salvage declarations swallowed by a parse error (§2.4)
			eachNamed(c, visit)
		}
	}
	eachNamed(root, visit)
}

func kotlinType(src []byte, n *ts.Node, outer string, res *FileResult) {
	name := text(n.ChildByFieldName("name"), src)
	if name == "" {
		return
	}
	chain := joinContainer(outer, name)
	kind := nodeid.KindClass
	if hasToken(n, "interface") {
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
	node.DBRefs = kotlinJPARefs(src, n, name)
	eachNamed(n, func(c *ts.Node) {
		if c.Kind() == "delegation_specifiers" {
			eachNamed(c, func(d *ts.Node) { // (delegation_specifier (user_type|constructor_invocation))
				node.Supers = append(node.Supers, kotlinSuperName(src, d))
			})
		}
	})
	res.Nodes = append(res.Nodes, node)

	eachNamed(n, func(c *ts.Node) {
		if c.Kind() != "class_body" {
			return
		}
		eachNamed(c, func(m *ts.Node) {
			switch m.Kind() {
			case "function_declaration":
				kotlinFunction(src, m, chain, nodeid.KindMethod, res)
			case "class_declaration", "object_declaration":
				kotlinType(src, m, chain, res)
			}
		})
	})
}

// kotlinJPARefs reads JPA table mappings off a class's annotations (Phase
// 7a), mirroring javaJPARefs. Grammar shapes: marker `annotation > user_type`,
// with-args `annotation > constructor_invocation(user_type, value_arguments)`.
func kotlinJPARefs(src []byte, n *ts.Node, className string) []DBRef {
	entity, table := false, ""
	eachNamed(n, func(c *ts.Node) {
		if c.Kind() != "modifiers" {
			return
		}
		eachNamed(c, func(a *ts.Node) {
			if a.Kind() != "annotation" {
				return
			}
			inner := a.NamedChild(0)
			if inner == nil {
				return
			}
			switch inner.Kind() {
			case "user_type":
				if annSimpleName(text(inner, src)) == "Entity" {
					entity = true
				}
			case "constructor_invocation":
				typ := (*ts.Node)(nil)
				args := (*ts.Node)(nil)
				eachNamed(inner, func(p *ts.Node) {
					switch p.Kind() {
					case "user_type":
						typ = p
					case "value_arguments":
						args = p
					}
				})
				switch annSimpleName(text(typ, src)) {
				case "Entity":
					entity = true
				case "Table":
					if args != nil {
						eachNamed(args, func(v *ts.Node) {
							if v.Kind() != "value_argument" {
								return
							}
							if v.NamedChildCount() >= 2 &&
								text(v.NamedChild(0), src) == "name" {
								table = kotlinStringContent(v.NamedChild(1), src)
							}
						})
					}
				}
			}
		})
	})
	if table != "" {
		return []DBRef{{Name: table, Source: DBRefExplicit}}
	}
	if entity {
		return []DBRef{{Name: className, Source: DBRefConvention}}
	}
	return nil
}

// kotlinStringContent reads the content of a string_literal node.
func kotlinStringContent(n *ts.Node, src []byte) string {
	if n == nil || n.Kind() != "string_literal" {
		return ""
	}
	var sb strings.Builder
	eachNamed(n, func(c *ts.Node) {
		if c.Kind() == "string_content" {
			sb.WriteString(text(c, src))
		}
	})
	return sb.String()
}

// kotlinSuperName extracts the supertype name from a delegation_specifier,
// unwrapping constructor invocations ("Base(1)" → "Base").
func kotlinSuperName(src []byte, d *ts.Node) string {
	n := d
	for {
		child := n.NamedChild(0)
		if child == nil {
			break
		}
		switch child.Kind() {
		case "constructor_invocation", "user_type":
			n = child
			continue
		}
		break
	}
	s := text(n, src)
	if i := strings.IndexByte(s, '('); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

func kotlinFunction(src []byte, fn *ts.Node, container, kind string, res *FileResult) {
	nameNode := fn.ChildByFieldName("name")
	name := text(nameNode, src)
	if name == "" {
		return
	}

	// Extension receiver: a type node positioned before the function name
	// becomes the first ID parameter (§2.3: MoneyKt.toWon(Long)).
	var receiver string
	eachNamed(fn, func(c *ts.Node) {
		if c.EndByte() <= nameNode.StartByte() {
			switch c.Kind() {
			case "user_type", "nullable_type":
				receiver = text(c, src)
			}
		}
	})

	paramTypes, min, max := kotlinParams(src, fn)
	idParams := paramTypes
	if receiver != "" {
		idParams = append([]string{receiver}, paramTypes...)
	}

	node := Node{
		ID:          nodeid.Method(res.Package, container, name, idParams),
		DisplayName: nodeid.Display(container, name),
		SimpleName:  name,
		Kind:        kind,
		Container:   container,
		Hash:        subtreeHash(fn, src),
		ArityMin:    min,
		ArityMax:    max,
		Params:      idParams,
	}
	node.StartByte, node.EndByte = span(fn)
	eachNamed(fn, func(c *ts.Node) {
		if c.Kind() == "function_body" {
			node.Calls = kotlinCalls(src, c)
		}
	})
	node.DBRefs = appendSQLRefs(node.DBRefs, LangKotlin, src, fn)
	res.Nodes = append(res.Nodes, node)
}

// kotlinParams reads function_value_parameters. In this grammar a default
// value appears as a *sibling* expression following its parameter, so the
// arity range is total minus the number of parameters trailed by a
// non-parameter named node (§2.1.3 Kotlin 기본 인자).
func kotlinParams(src []byte, fn *ts.Node) ([]string, int, int) {
	var params *ts.Node
	eachNamed(fn, func(c *ts.Node) {
		if c.Kind() == "function_value_parameters" && params == nil {
			params = c
		}
	})
	if params == nil {
		return nil, 0, 0
	}
	var (
		types    []string
		defaults int
		vararg   bool
		lastPar  = -1 // index into types of the most recent parameter
		counted  = map[int]bool{}
	)
	eachNamed(params, func(c *ts.Node) {
		if c.Kind() == "parameter" {
			t := ""
			if cnt := c.NamedChildCount(); cnt > 0 {
				t = text(c.NamedChild(cnt-1), src)
			}
			if strings.HasPrefix(strings.TrimSpace(text(c, src)), "vararg") {
				vararg = true
			}
			types = append(types, t)
			lastPar = len(types) - 1
			return
		}
		// any other named node right after a parameter is its default value
		if lastPar >= 0 && !counted[lastPar] {
			counted[lastPar] = true
			defaults++
		}
	})
	min, max := len(types)-defaults, len(types)
	if vararg {
		max = nodeid.UnboundedArity
	}
	return types, min, max
}

func kotlinCalls(src []byte, body *ts.Node) []Call {
	var calls []Call
	var walk func(n *ts.Node)
	walk = func(n *ts.Node) {
		if n.Kind() == "call_expression" {
			callee := n.NamedChild(0)
			c := Call{}
			if callee != nil {
				switch callee.Kind() {
				case "identifier":
					c.Name = text(callee, src)
				case "navigation_expression":
					if cnt := callee.NamedChildCount(); cnt >= 2 {
						c.Name = text(callee.NamedChild(cnt-1), src)
						c.Recv = text(callee.NamedChild(0), src)
					}
				}
			}
			eachNamed(n, func(part *ts.Node) {
				switch part.Kind() {
				case "value_arguments":
					eachNamed(part, func(a *ts.Node) {
						if a.Kind() == "value_argument" {
							c.Args++
						}
					})
				case "annotated_lambda", "lambda_literal": // trailing lambda argument
					c.Args++
				}
			})
			if c.Name != "" {
				calls = append(calls, c)
			}
		}
		eachNamed(n, walk)
	}
	walk(body)
	return calls
}
