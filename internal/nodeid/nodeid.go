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
	// symbols (configs, SQL, docs): whole file = one node, ID = rel path.
	KindFile = "file"
)

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
