// Package parse turns source files into language-neutral node lists via
// tree-sitter (§2.1.2). CGO objects never escape this package: trees are
// closed before returning (§2.2 메모리 통제) and results are plain Go values.
package parse

import (
	"fmt"
	"strings"
	"sync"

	tskotlin "github.com/tree-sitter-grammars/tree-sitter-kotlin/bindings/go"
	ts "github.com/tree-sitter/go-tree-sitter"
	tsjava "github.com/tree-sitter/tree-sitter-java/bindings/go"
	tspython "github.com/tree-sitter/tree-sitter-python/bindings/go"
	"github.com/zeebo/blake3"
)

// Language identifies a supported grammar.
type Language int

const (
	LangUnknown Language = iota
	LangJava
	LangKotlin
	LangPython
)

// DetectLanguage maps a file path to its grammar.
func DetectLanguage(path string) Language {
	switch {
	case strings.HasSuffix(path, ".java"):
		return LangJava
	case strings.HasSuffix(path, ".kt"), strings.HasSuffix(path, ".kts"):
		return LangKotlin
	case strings.HasSuffix(path, ".py"):
		return LangPython
	default:
		return LangUnknown
	}
}

// Call is one call site found inside a node body.
type Call struct {
	Name string // callee simple name as written
	Args int    // argument count at the call site
	Recv string // receiver expression text ("" for bare calls)
}

// Node is one indexable semantic subtree (class/interface/method/function).
type Node struct {
	ID          string
	DisplayName string
	SimpleName  string
	Kind        string // nodeid.Kind*
	Container   string // enclosing container chain within the file ("" if top-level)
	StartByte   uint32
	EndByte     uint32
	Hash        [32]byte // BLAKE3 of the subtree source slice
	ArityMin    int
	ArityMax    int      // nodeid.UnboundedArity when open
	Params      []string // parameter type texts as written
	Supers      []string // extends/implements names as written (class kinds)
	Calls       []Call
}

// FileResult is the full extraction of one file.
type FileResult struct {
	RelPath  string
	Lang     Language
	Package  string // declared package (Java/Kotlin) or module path (Python)
	Imports  []string
	Nodes    []Node
	Partial  bool // tree contained ERROR nodes (§2.4)
	FileHash [32]byte
}

var (
	javaLang   = ts.NewLanguage(tsjava.Language())
	kotlinLang = ts.NewLanguage(tskotlin.Language())
	pythonLang = ts.NewLanguage(tspython.Language())
)

func grammar(lang Language) *ts.Language {
	switch lang {
	case LangJava:
		return javaLang
	case LangKotlin:
		return kotlinLang
	case LangPython:
		return pythonLang
	}
	return nil
}

// Parsers are not goroutine-safe; pool one per language per worker.
var parserPools = map[Language]*sync.Pool{
	LangJava:   newPool(LangJava),
	LangKotlin: newPool(LangKotlin),
	LangPython: newPool(LangPython),
}

func newPool(lang Language) *sync.Pool {
	return &sync.Pool{New: func() any {
		p := ts.NewParser()
		if err := p.SetLanguage(grammar(lang)); err != nil {
			panic(fmt.Sprintf("tree-sitter language init (%d): %v", lang, err))
		}
		return p
	}}
}

// File parses one source file and extracts its nodes. The tree-sitter tree is
// released before returning.
func File(relPath string, src []byte) (*FileResult, error) {
	lang := DetectLanguage(relPath)
	if lang == LangUnknown {
		return nil, fmt.Errorf("unsupported language: %s", relPath)
	}
	pool := parserPools[lang]
	parser := pool.Get().(*ts.Parser)
	defer pool.Put(parser)

	tree := parser.Parse(src, nil)
	if tree == nil {
		return nil, fmt.Errorf("parse failed: %s", relPath)
	}
	defer tree.Close()

	root := tree.RootNode()
	res := &FileResult{
		RelPath:  relPath,
		Lang:     lang,
		Partial:  root.HasError(),
		FileHash: blake3.Sum256(src),
	}
	switch lang {
	case LangJava:
		extractJava(src, root, res)
	case LangKotlin:
		extractKotlin(src, root, res)
	case LangPython:
		extractPython(src, root, res)
	}
	return res, nil
}

// ---- shared tree helpers ----

func text(n *ts.Node, src []byte) string {
	if n == nil {
		return ""
	}
	return n.Utf8Text(src)
}

func eachNamed(n *ts.Node, f func(c *ts.Node)) {
	for i := uint(0); i < n.NamedChildCount(); i++ {
		if c := n.NamedChild(i); c != nil {
			f(c)
		}
	}
}

// hasToken reports whether n has an anonymous child token of the given kind
// (e.g. the "interface" keyword).
func hasToken(n *ts.Node, kind string) bool {
	for i := uint(0); i < n.ChildCount(); i++ {
		c := n.Child(i)
		if c != nil && !c.IsNamed() && c.Kind() == kind {
			return true
		}
	}
	return false
}

func span(n *ts.Node) (uint32, uint32) {
	return uint32(n.StartByte()), uint32(n.EndByte())
}

func subtreeHash(n *ts.Node, src []byte) [32]byte {
	return blake3.Sum256(src[n.StartByte():n.EndByte()])
}

func joinContainer(outer, name string) string {
	if outer == "" {
		return name
	}
	return outer + "." + name
}
