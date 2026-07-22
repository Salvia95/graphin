package parse

import (
	pathpkg "path"
	"strings"

	ts "github.com/tree-sitter/go-tree-sitter"

	"github.com/Salvia95/graphin/internal/nodeid"
)

// jsExtensions are stripped when deriving module paths, longest match first
// (".d.ts" before ".ts").
var jsExtensions = []string{
	".d.ts", ".ts", ".tsx", ".mts", ".cts", ".js", ".jsx", ".mjs", ".cjs",
}

// jsModulePath derives the dotted module path from a "/"-separated path:
// "src/order/service.ts" → "src.order.service"; "index" collapses to its
// directory like Python's __init__.py.
func jsModulePath(p string) string {
	for _, ext := range jsExtensions {
		if strings.HasSuffix(p, ext) {
			p = strings.TrimSuffix(p, ext)
			break
		}
	}
	p = strings.TrimSuffix(p, "/index")
	if p == "index" {
		p = ""
	}
	p = strings.TrimPrefix(p, "./")
	return strings.ReplaceAll(p, "/", ".")
}

type jsExtractor struct {
	src      []byte
	res      *FileResult
	dts      bool           // .d.ts: bodyless signatures are definitions
	ordinals map[string]int // container+"\x00"+name → definitions seen
	// pendingDecorators carries decorators that sit on an export_statement
	// (siblings of the exported class in the grammar) down to ex.class.
	pendingDecorators []*ts.Node
}

func extractJS(src []byte, root *ts.Node, res *FileResult) {
	res.Package = jsModulePath(res.RelPath)
	ex := &jsExtractor{
		src:      src,
		res:      res,
		dts:      strings.HasSuffix(res.RelPath, ".d.ts"),
		ordinals: map[string]int{},
	}
	eachNamed(root, func(c *ts.Node) { ex.visit(c, "") })
}

func (ex *jsExtractor) visit(n *ts.Node, container string) {
	switch n.Kind() {
	case "import_statement":
		ex.importStmt(n)
	case "export_statement":
		ex.export(n, container)
	case "lexical_declaration", "variable_declaration":
		ex.varDecl(n, container)
	case "function_declaration", "generator_function_declaration":
		ex.fnNode(n, n, text(n.ChildByFieldName("name"), ex.src), container, nodeid.KindFunction)
	case "function_signature":
		// TS overload/ambient signature: a definition only in .d.ts (§설계
		// 결정 4); elsewhere the implementation that follows is the node.
		if ex.dts {
			ex.fnNode(n, n, text(n.ChildByFieldName("name"), ex.src), container, nodeid.KindFunction)
		}
	case "class_declaration", "abstract_class_declaration":
		ex.class(n, "", container)
	case "interface_declaration":
		ex.iface(n, container)
	case "enum_declaration":
		ex.enum(n, container)
	case "ambient_declaration": // declare ... wrapper
		eachNamed(n, func(c *ts.Node) { ex.visit(c, container) })
	case "internal_module": // namespace X { ... } — members join the chain
		ex.namespace(n, container)
	case "expression_statement":
		eachNamed(n, func(c *ts.Node) {
			if c.Kind() == "internal_module" {
				ex.namespace(c, container)
			}
		})
	case "ERROR": // salvage declarations swallowed by a parse error (§2.4)
		eachNamed(n, func(c *ts.Node) { ex.visit(c, container) })
	}
}

func (ex *jsExtractor) ordinal(container, name string) int {
	key := container + "\x00" + name
	ex.ordinals[key]++
	return ex.ordinals[key]
}

// ---- imports (§설계 결정 2: 경로 import를 dotted 모듈 공간으로 정규화) ----

// resolveSpec maps an import specifier onto the dotted module space node IDs
// live in. Relative specifiers resolve against the importing file; bare
// package specifiers stay as written (dotted) — they point outside the index
// and never match, like JDK imports in Java.
func (ex *jsExtractor) resolveSpec(spec string) string {
	if strings.HasPrefix(spec, "./") || strings.HasPrefix(spec, "../") {
		joined := pathpkg.Join(pathpkg.Dir(ex.res.RelPath), spec)
		if !strings.HasPrefix(joined, "..") {
			return jsModulePath(joined)
		}
	}
	return strings.ReplaceAll(spec, "/", ".")
}

func (ex *jsExtractor) addImport(imp string) {
	if imp != "" && imp != "." && imp != ".*" {
		ex.res.Imports = append(ex.res.Imports, imp)
	}
}

func (ex *jsExtractor) importStmt(n *ts.Node) {
	base := ex.resolveSpec(stringLiteral(n.ChildByFieldName("source"), ex.src))
	if base == "" {
		return
	}
	clause := (*ts.Node)(nil)
	eachNamed(n, func(c *ts.Node) {
		if c.Kind() == "import_clause" {
			clause = c
		}
	})
	if clause == nil { // side-effect import
		ex.addImport(base)
		return
	}
	eachNamed(clause, func(c *ts.Node) {
		switch c.Kind() {
		case "identifier": // default import: local alias as name (heuristic)
			ex.addImport(base + "." + text(c, ex.src))
		case "namespace_import": // * as x → wildcard visibility
			ex.addImport(base + ".*")
		case "named_imports":
			eachNamed(c, func(s *ts.Node) {
				if s.Kind() != "import_specifier" {
					return
				}
				if name := text(s.ChildByFieldName("name"), ex.src); name != "" {
					ex.addImport(base + "." + name)
				}
			})
		}
	})
}

// maybeRequire records `const x = require("./m")` as an import: identifier
// bindings act like namespace imports, destructurings like named imports.
func (ex *jsExtractor) maybeRequire(decl, call *ts.Node) {
	fn := call.ChildByFieldName("function")
	if fn == nil || fn.Kind() != "identifier" || text(fn, ex.src) != "require" {
		return
	}
	args := call.ChildByFieldName("arguments")
	if args == nil || args.NamedChildCount() == 0 {
		return
	}
	base := ex.resolveSpec(stringLiteral(args.NamedChild(0), ex.src))
	if base == "" {
		return
	}
	name := decl.ChildByFieldName("name")
	if name == nil {
		ex.addImport(base)
		return
	}
	switch name.Kind() {
	case "identifier":
		ex.addImport(base + ".*")
	case "object_pattern":
		eachNamed(name, func(p *ts.Node) {
			switch p.Kind() {
			case "shorthand_property_identifier_pattern":
				ex.addImport(base + "." + text(p, ex.src))
			case "pair_pattern":
				if k := p.ChildByFieldName("key"); k != nil {
					ex.addImport(base + "." + text(k, ex.src))
				}
			}
		})
	default:
		ex.addImport(base)
	}
}

// ---- exports ----

func (ex *jsExtractor) export(n *ts.Node, container string) {
	src := ex.src
	// Re-export (`export ... from "m"`) pulls symbols through this module:
	// record as imports so scope ranking sees them.
	if source := n.ChildByFieldName("source"); source != nil {
		base := ex.resolveSpec(stringLiteral(source, src))
		named := false
		eachNamed(n, func(c *ts.Node) {
			if c.Kind() != "export_clause" {
				return
			}
			eachNamed(c, func(s *ts.Node) {
				if s.Kind() != "export_specifier" {
					return
				}
				if name := text(s.ChildByFieldName("name"), src); name != "" {
					ex.addImport(base + "." + name)
					named = true
				}
			})
		})
		if !named && base != "" { // export * from "m"
			ex.addImport(base + ".*")
		}
		return
	}
	if d := n.ChildByFieldName("declaration"); d != nil {
		eachNamed(n, func(c *ts.Node) {
			if c.Kind() == "decorator" {
				ex.pendingDecorators = append(ex.pendingDecorators, c)
			}
		})
		ex.visit(d, container)
		ex.pendingDecorators = nil
		return
	}
	// `export default <expr>`: anonymous functions/classes become "default"
	// nodes (§설계 결정 3); identifier re-exports add nothing.
	if v := n.ChildByFieldName("value"); v != nil {
		switch v.Kind() {
		case "function_expression", "arrow_function", "generator_function_expression":
			name := text(v.ChildByFieldName("name"), src)
			if name == "" {
				name = "default"
			}
			ex.fnNode(v, v, name, container, nodeid.KindFunction)
		case "class":
			ex.class(v, "default", container)
		}
		return
	}
	eachNamed(n, func(c *ts.Node) {
		if c.Kind() != "export_clause" && c.Kind() != "decorator" {
			ex.visit(c, container)
		}
	})
}

// varDecl indexes function-valued declarators (`const f = () => {}` — the
// dominant modern style) and require() bindings.
func (ex *jsExtractor) varDecl(n *ts.Node, container string) {
	eachNamed(n, func(d *ts.Node) {
		if d.Kind() != "variable_declarator" {
			return
		}
		val := d.ChildByFieldName("value")
		if val == nil {
			return
		}
		switch val.Kind() {
		case "arrow_function", "function_expression", "generator_function_expression":
			name := d.ChildByFieldName("name")
			if name != nil && name.Kind() == "identifier" {
				ex.fnNode(d, val, text(name, ex.src), container, nodeid.KindFunction)
			}
		case "call_expression":
			ex.maybeRequire(d, val)
		}
	})
}

// ---- node builders ----

// fnNode emits one function/method node. spanN carries the source span
// (declarator for `const f = ...`), fn carries parameters/body. Bodyless
// signatures (interface/.d.ts) simply have no calls.
func (ex *jsExtractor) fnNode(spanN, fn *ts.Node, name, container, kind string) {
	name = strings.TrimPrefix(name, "#") // private members: # breaks ID syntax
	if name == "" {
		return
	}
	ord := ex.ordinal(container, name)
	params, min, max := jsParamsOf(ex.src, fn)

	node := Node{
		ID:          nodeid.Python(ex.res.Package, container, name, ord),
		DisplayName: nodeid.Display(container, name),
		SimpleName:  name,
		Kind:        kind,
		Container:   container,
		Hash:        subtreeHash(spanN, ex.src),
		ArityMin:    min,
		ArityMax:    max,
		Params:      params,
	}
	node.StartByte, node.EndByte = span(spanN)
	if body := fn.ChildByFieldName("body"); body != nil {
		node.Calls = jsCalls(ex.src, body)
		node.DBRefs = jsClientRefs(node.Calls)
		if body.Kind() == "statement_block" {
			chain := joinContainer(container, name)
			eachNamed(body, func(m *ts.Node) {
				switch m.Kind() {
				case "function_declaration", "generator_function_declaration",
					"class_declaration", "abstract_class_declaration",
					"lexical_declaration", "variable_declaration", "ERROR":
					ex.visit(m, chain)
				}
			})
		}
	}
	ex.res.Nodes = append(ex.res.Nodes, node)
}

func (ex *jsExtractor) class(n *ts.Node, nameOverride, outer string) {
	src := ex.src
	name := nameOverride
	if name == "" {
		name = text(n.ChildByFieldName("name"), src)
	}
	if name == "" {
		return
	}
	chain := joinContainer(outer, name)
	ord := ex.ordinal(outer, name)
	decorators := ex.pendingDecorators
	ex.pendingDecorators = nil
	eachNamed(n, func(c *ts.Node) {
		if c.Kind() == "decorator" {
			decorators = append(decorators, c)
		}
	})

	node := Node{
		ID:          nodeid.Python(ex.res.Package, outer, name, ord),
		DisplayName: nodeid.Display(outer, name),
		SimpleName:  name,
		Kind:        nodeid.KindClass,
		Container:   outer,
		Hash:        subtreeHash(n, src),
		DBRefs:      jsEntityRefs(src, decorators, name),
	}
	node.StartByte, node.EndByte = span(n)
	eachNamed(n, func(c *ts.Node) {
		if c.Kind() != "class_heritage" {
			return
		}
		eachNamed(c, func(h *ts.Node) {
			switch h.Kind() {
			case "extends_clause": // TS: extends_clause value: expr
				if v := h.ChildByFieldName("value"); v != nil {
					node.Supers = append(node.Supers, text(v, src))
				}
			case "implements_clause":
				eachNamed(h, func(t *ts.Node) {
					node.Supers = append(node.Supers, text(t, src))
				})
			default: // JS: bare identifier / member_expression
				node.Supers = append(node.Supers, text(h, src))
			}
		})
	})
	ex.res.Nodes = append(ex.res.Nodes, node)

	body := n.ChildByFieldName("body")
	if body == nil {
		return
	}
	eachNamed(body, func(m *ts.Node) {
		switch m.Kind() {
		case "method_definition", "abstract_method_signature":
			ex.member(m, m, chain)
		case "method_signature":
			// In a class body these are overload signatures of the
			// method_definition that follows — definitions only in .d.ts.
			if ex.dts {
				ex.member(m, m, chain)
			}
		case "field_definition", "public_field_definition":
			val := m.ChildByFieldName("value")
			if val == nil {
				return
			}
			switch val.Kind() {
			case "arrow_function", "function_expression", "generator_function_expression":
				ex.member(m, val, chain)
			}
		}
	})
}

// member emits one class/interface member; spanN is the member node, fn the
// node carrying parameters/body (the value for field-bound arrows).
func (ex *jsExtractor) member(spanN, fn *ts.Node, chain string) {
	name := spanN.ChildByFieldName("name")
	if name == nil {
		name = spanN.ChildByFieldName("property") // JS field_definition
	}
	if name == nil || name.Kind() == "computed_property_name" {
		return
	}
	ex.fnNode(spanN, fn, text(name, ex.src), chain, nodeid.KindMethod)
}

func (ex *jsExtractor) iface(n *ts.Node, outer string) {
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
		Kind:        nodeid.KindInterface,
		Container:   outer,
		Hash:        subtreeHash(n, src),
	}
	node.StartByte, node.EndByte = span(n)
	eachNamed(n, func(c *ts.Node) {
		if c.Kind() != "extends_type_clause" {
			return
		}
		eachNamed(c, func(t *ts.Node) {
			node.Supers = append(node.Supers, text(t, src))
		})
	})
	ex.res.Nodes = append(ex.res.Nodes, node)

	if body := n.ChildByFieldName("body"); body != nil {
		eachNamed(body, func(m *ts.Node) {
			if m.Kind() == "method_signature" {
				ex.member(m, m, chain)
			}
		})
	}
}

// enum becomes a single class-kind node: referenced like a value container,
// members carry no navigation value of their own.
func (ex *jsExtractor) enum(n *ts.Node, outer string) {
	name := text(n.ChildByFieldName("name"), ex.src)
	if name == "" {
		return
	}
	ord := ex.ordinal(outer, name)
	node := Node{
		ID:          nodeid.Python(ex.res.Package, outer, name, ord),
		DisplayName: nodeid.Display(outer, name),
		SimpleName:  name,
		Kind:        nodeid.KindClass,
		Container:   outer,
		Hash:        subtreeHash(n, ex.src),
	}
	node.StartByte, node.EndByte = span(n)
	ex.res.Nodes = append(ex.res.Nodes, node)
}

func (ex *jsExtractor) namespace(n *ts.Node, outer string) {
	name := n.ChildByFieldName("name")
	if name == nil || name.Kind() == "string" {
		return // `declare module "pkg"` augments externals — not a def here
	}
	chain := joinContainer(outer, text(name, ex.src))
	if body := n.ChildByFieldName("body"); body != nil {
		eachNamed(body, func(m *ts.Node) { ex.visit(m, chain) })
	}
}

// ---- signatures & calls ----

// jsParamsOf returns parameter texts as written plus the §2.1.3 arity range:
// defaults and TS `?` widen the maximum downward-compatibly, rest parameters
// open it, a TS `this` pseudo-parameter is excluded.
func jsParamsOf(src []byte, fn *ts.Node) ([]string, int, int) {
	params := fn.ChildByFieldName("parameters")
	if params == nil {
		// arrow shorthand: `x => ...`
		if single := fn.ChildByFieldName("parameter"); single != nil {
			return []string{text(single, src)}, 1, 1
		}
		return nil, 0, 0
	}
	var (
		texts              []string
		required, optional int
		open               bool
	)
	eachNamed(params, func(p *ts.Node) {
		kind := p.Kind()
		switch kind {
		case "required_parameter", "optional_parameter": // TS wrappers
			pat := p.ChildByFieldName("pattern")
			if pat != nil && pat.Kind() == "this" {
				return
			}
			texts = append(texts, text(p, src))
			if pat != nil && pat.Kind() == "rest_pattern" {
				open = true
				return
			}
			if kind == "optional_parameter" || p.ChildByFieldName("value") != nil {
				optional++
			} else {
				required++
			}
		case "identifier", "object_pattern", "array_pattern":
			texts = append(texts, text(p, src))
			required++
		case "assignment_pattern": // default value
			texts = append(texts, text(p, src))
			optional++
		case "rest_pattern":
			texts = append(texts, text(p, src))
			open = true
		}
	})
	min := required
	max := required + optional
	if open {
		max = nodeid.UnboundedArity
	}
	return texts, min, max
}

func jsCalls(src []byte, body *ts.Node) []Call {
	var calls []Call
	add := func(fn, args *ts.Node) {
		c := Call{}
		if fn != nil {
			switch fn.Kind() {
			case "identifier":
				c.Name = text(fn, src)
			case "member_expression":
				c.Name = strings.TrimPrefix(text(fn.ChildByFieldName("property"), src), "#")
				c.Recv = text(fn.ChildByFieldName("object"), src)
			}
		}
		if args != nil && args.Kind() == "arguments" {
			c.Args = int(args.NamedChildCount())
		} else if args != nil { // tagged template: tag`...`
			c.Args = 1
		}
		if c.Name != "" {
			calls = append(calls, c)
		}
	}
	var walk func(n *ts.Node)
	walk = func(n *ts.Node) {
		switch n.Kind() {
		case "call_expression":
			add(n.ChildByFieldName("function"), n.ChildByFieldName("arguments"))
		case "new_expression": // new Receipt(...) → constructor-ish call
			add(n.ChildByFieldName("constructor"), n.ChildByFieldName("arguments"))
		}
		eachNamed(n, walk)
	}
	walk(body)
	return calls
}

// jsEntityRefs reads TypeORM-style @Entity decorators (Phase 7a): a string
// argument (or an options object with a name property) is an explicit
// physical table name; a bare @Entity falls back to the class-name
// convention.
func jsEntityRefs(src []byte, decorators []*ts.Node, className string) []DBRef {
	entity, table := false, ""
	for _, d := range decorators {
		inner := d.NamedChild(0)
		if inner == nil {
			continue
		}
		var nameNode, args *ts.Node
		switch inner.Kind() {
		case "call_expression":
			nameNode = inner.ChildByFieldName("function")
			args = inner.ChildByFieldName("arguments")
		case "identifier":
			nameNode = inner
		default:
			continue
		}
		if nameNode == nil || annSimpleName(text(nameNode, src)) != "Entity" {
			continue
		}
		entity = true
		if args == nil {
			continue
		}
		eachNamed(args, func(a *ts.Node) {
			if table != "" {
				return
			}
			switch a.Kind() {
			case "string":
				table = stringLiteral(a, src)
			case "object": // @Entity({ name: "x" })
				eachNamed(a, func(p *ts.Node) {
					if p.Kind() != "pair" {
						return
					}
					if text(p.ChildByFieldName("key"), src) == "name" {
						if v := p.ChildByFieldName("value"); v != nil && v.Kind() == "string" {
							table = stringLiteral(v, src)
						}
					}
				})
			}
		})
	}
	if table != "" {
		return []DBRef{{Name: table, Source: DBRefExplicit}}
	}
	if entity {
		return []DBRef{{Name: className, Source: DBRefConvention}}
	}
	return nil
}

// jsClientRefs derives prisma-client table refs from call receivers:
// `prisma.<model>.<op>()` / `this.prisma.<model>.<op>()` (Phase 7a). The
// model member resolves through the alias names registered by the prisma
// SSOT parser; $-prefixed members are client API, not models.
func jsClientRefs(calls []Call) []DBRef {
	var refs []DBRef
	seen := map[string]bool{}
	for _, c := range calls {
		recv := strings.TrimPrefix(c.Recv, "this.")
		dot := strings.IndexByte(recv, '.')
		if dot < 0 || recv[:dot] != "prisma" {
			continue
		}
		member := recv[dot+1:]
		if member == "" || strings.ContainsAny(member, ".([") ||
			strings.HasPrefix(member, "$") || seen[member] {
			continue
		}
		seen[member] = true
		refs = append(refs, DBRef{Name: member, Source: DBRefClient})
	}
	return refs
}

// stringLiteral concatenates the fragments of a string node ("./m" → ./m).
func stringLiteral(n *ts.Node, src []byte) string {
	if n == nil {
		return ""
	}
	var sb strings.Builder
	eachNamed(n, func(c *ts.Node) {
		if c.Kind() == "string_fragment" {
			sb.WriteString(c.Utf8Text(src))
		}
	})
	return sb.String()
}
