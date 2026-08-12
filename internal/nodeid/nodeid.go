// Package nodeid implements the §2.3 node ID rules. It is pure Go (no CGO)
// because the parser, graph engine and MCP layer all depend on it.
//
//	Java/Kotlin method: pkg.Class.method(ParamTypeText,...)
//	Kotlin top-level:   pkg.FileKt.fn(...)
//	Python:             module.Class.method   (+ "#n" only on redefinition)
//	Class/type:         plain FQN
//
// Parameter types are recorded exactly as written in the declaration — no
// type resolution.
package nodeid

import "strings"

// Kind classifies a node.
const (
	KindClass     = "class"
	KindInterface = "interface"
	KindMethod    = "method"
	KindFunction  = "function"
	// KindFile is the fallback node for text files without parseable
	// symbols (configs, SQL): whole file = one node, ID = rel path.
	// Markdown keeps a file node too, as the root of its section tree.
	KindFile = "file"
	// KindSection is one markdown heading and the prose under it, up to the
	// next heading of any level (docs/markdown-spec.md §3.4).
	// ID = "<rel path>#<github-style slug>".
	KindSection = "section"
)

// DB-domain kinds for graphindb snapshot nodes (schema/graphindb.md).
const (
	KindTable      = "table"
	KindView       = "view"
	KindDBFunction = "db_function"
	KindProcedure  = "procedure"
	KindRLSPolicy  = "rls_policy"
	KindTrigger    = "trigger"
)

// IsDBKind reports whether kind belongs to the graphindb RDB domain: those
// nodes resolve edges by FQN, not by simple-name scope ranking.
func IsDBKind(kind string) bool {
	switch kind {
	case KindTable, KindView, KindDBFunction, KindProcedure, KindRLSPolicy, KindTrigger:
		return true
	}
	return false
}

// Target partitions indexed nodes into the populations an agent asks about.
// The names match the usage report's target split (docs/usage-spec.md) so a
// filter and the adoption metric mean the same thing by "code".
const (
	TargetCode = "code" // grammar-derived symbols: the implementation
	TargetDocs = "docs" // markdown file roots and their sections
	TargetDB   = "db"   // graphindb snapshot nodes
	// TargetText is the anchor-less remainder: .json/.yml/.toml and the rest
	// of the plain extension list. Not prose and not implementation, so it
	// belongs to neither of the two populations a caller asks for by name.
	TargetText = "text"
)

// docExtensions are the file suffixes whose file node roots a section tree
// (docs/markdown-spec.md).
var docExtensions = [...]string{".md", ".markdown"}

// Target classifies one indexed node. It needs the kind *and* the ID: KindFile
// is the shared fallback for a markdown root and for anchor-less config text,
// so the kind alone cannot tell a README from a scores.json — and telling them
// apart is the whole point of the split.
//
// Unclassifiable input returns "". Callers filtering by target must treat that
// as "does not match": admitting a node we cannot name would break the promise
// the filter makes.
//
// usage.targetOf answers the same question from a node ID alone, because it
// reads event logs and has no index to ask for a kind. That approximation puts
// .json/.yml in "code"; this one, which can see the kind, calls them text.
func Target(kind, id string) string {
	switch {
	case IsDBKind(kind):
		return TargetDB
	case kind == KindSection:
		return TargetDocs
	case kind == KindFile:
		if hasDocExtension(id) {
			return TargetDocs
		}
		return TargetText
	case kind == KindClass, kind == KindInterface, kind == KindMethod, kind == KindFunction:
		return TargetCode
	}
	return ""
}

// IsTarget reports whether name is a target a caller may filter by. TargetText
// is deliberately absent: it is a residue, not something anyone asks for.
func IsTarget(name string) bool {
	switch name {
	case TargetCode, TargetDocs, TargetDB:
		return true
	}
	return false
}

func hasDocExtension(id string) bool {
	// A file node ID is the rel path, so the extension is the whole tail. The
	// comparison is case-insensitive because README.MD is the same document.
	for _, ext := range docExtensions {
		if len(id) >= len(ext) && strings.EqualFold(id[len(id)-len(ext):], ext) {
			return true
		}
	}
	return false
}

// UnboundedArity marks an open maximum (Python *args/**kwargs, Java varargs).
const UnboundedArity = -1

// Method builds a Java/Kotlin-style method or function ID. container may be
// a nested chain ("Outer.Inner") or a synthetic file class ("MoneyKt").
func Method(pkg, container, name string, paramTypes []string) string {
	var sb strings.Builder
	if pkg != "" {
		sb.WriteString(pkg)
		sb.WriteString(".")
	}
	if container != "" {
		sb.WriteString(container)
		sb.WriteString(".")
	}
	sb.WriteString(name)
	sb.WriteString("(")
	sb.WriteString(strings.Join(paramTypes, ","))
	sb.WriteString(")")
	return sb.String()
}

// Class builds a plain-FQN type ID.
func Class(pkg, name string) string {
	if pkg == "" {
		return name
	}
	return pkg + "." + name
}

// Python builds a module-scoped node ID ("module.Container.name") shared by
// Python and JS/TS. ordinal is the 1-based definition index
// within the same scope; the "#n" suffix appears only from the second
// definition on (§2.3).
func Python(module, container, name string, ordinal int) string {
	id := module
	if container != "" {
		id += "." + container
	}
	id += "." + name
	if ordinal >= 2 {
		id += "#" + itoa(ordinal)
	}
	return id
}

// DBNode builds a graphindb node ID: "db.<datasource>.<schema>.<name>". The
// "db." prefix namespaces DB nodes away from code packages. Sub-object IDs
// (RLS bundle, trigger) append their own segment at the call site.
func DBNode(datasource, schema, name string) string {
	return "db." + datasource + "." + schema + "." + name
}

// DBDisplay builds the DB-node display_name "<datasource>.<schema>.<name>":
// unlike Display, it keeps the datasource so same-named tables in different
// databases stay distinguishable in search results.
func DBDisplay(datasource, schema, name string) string {
	return datasource + "." + schema + "." + name
}

// FileClassKotlin derives the synthetic container for top-level Kotlin
// functions: "Money.kt" → "MoneyKt".
func FileClassKotlin(fileName string) string {
	base := fileName
	if i := strings.LastIndexByte(base, '/'); i >= 0 {
		base = base[i+1:]
	}
	base = strings.TrimSuffix(base, ".kts")
	base = strings.TrimSuffix(base, ".kt")
	base = strings.ReplaceAll(base, "-", "_")
	base = strings.ReplaceAll(base, " ", "_")
	if base == "" {
		return "Kt"
	}
	return strings.ToUpper(base[:1]) + base[1:] + "Kt"
}

// Simple returns the simple (member) name of an ID: the last dot-separated
// segment before any parameter list or arity suffix.
func Simple(id string) string {
	s := id
	if i := strings.IndexByte(s, '('); i >= 0 {
		s = s[:i]
	}
	if i := strings.IndexByte(s, '#'); i >= 0 {
		s = s[:i]
	}
	if i := strings.LastIndexByte(s, '.'); i >= 0 {
		s = s[i+1:]
	}
	return s
}

// Display builds the §3.2 display_name: "Container.member" for members,
// the simple name for top-level nodes.
func Display(container, name string) string {
	if container == "" {
		return name
	}
	if i := strings.LastIndexByte(container, '.'); i >= 0 {
		container = container[i+1:]
	}
	return container + "." + name
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
