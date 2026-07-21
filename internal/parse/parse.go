// Package parse turns source files into language-neutral node lists via
// tree-sitter (§2.1.2). CGO objects never escape this package: trees are
// closed before returning (§2.2 메모리 통제) and results are plain Go values.
package parse

import (
	"fmt"
	pathpkg "path"
	"strings"
	"sync"

	tskotlin "github.com/tree-sitter-grammars/tree-sitter-kotlin/bindings/go"
	ts "github.com/tree-sitter/go-tree-sitter"
	tsjava "github.com/tree-sitter/tree-sitter-java/bindings/go"
	tspython "github.com/tree-sitter/tree-sitter-python/bindings/go"
	"github.com/zeebo/blake3"

	"github.com/Salvia95/graphin/internal/lexical"
)

// Language identifies a supported grammar.
type Language int

const (
	LangUnknown Language = iota
	LangJava
	LangKotlin
	LangPython
	// LangPlain marks indexable text files without a grammar (§보완 A):
	// configs, SQL, docs. They become single file-kind nodes.
	LangPlain
)

// DetectLanguage maps a file path to its grammar (or the plaintext
// fallback).
func DetectLanguage(path string) Language {
	switch {
	case strings.HasSuffix(path, ".java"):
		return LangJava
	case strings.HasSuffix(path, ".kt"), strings.HasSuffix(path, ".kts"):
		return LangKotlin
	case strings.HasSuffix(path, ".py"):
		return LangPython
	}
	base := pathpkg.Base(path)
	if plainBasenames[base] {
		return LangPlain
	}
	if i := strings.LastIndexByte(base, '.'); i >= 0 && plainExtensions[base[i:]] {
		return LangPlain
	}
	return LangUnknown
}

// plainExtensions/plainBasenames define the anchor-less text files indexed
// as whole-file nodes so agents can still discover and read them.
var plainExtensions = map[string]bool{
	".yml": true, ".yaml": true, ".properties": true, ".json": true,
	".xml": true, ".sql": true, ".md": true, ".markdown": true,
	".toml": true, ".txt": true, ".conf": true, ".ini": true, ".cfg": true,
	".gradle": true, ".sh": true,
}

var plainBasenames = map[string]bool{
	"Dockerfile": true, "Makefile": true, "makefile": true, "Jenkinsfile": true,
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
	BodyTokens  []string // capped lexical tokens of the node's source slice
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

// maxBodyTokens caps the per-node body token contribution (§보완 B) so the
// lexical index stays within the memory budget.
const maxBodyTokens = 256

// File parses one source file and extracts its nodes. The tree-sitter tree is
// released before returning; plaintext files skip tree-sitter entirely.
func File(relPath string, src []byte) (*FileResult, error) {
	lang := DetectLanguage(relPath)
	if lang == LangUnknown {
		return nil, fmt.Errorf("unsupported language: %s", relPath)
	}
	res := &FileResult{
		RelPath:  relPath,
		Lang:     lang,
		FileHash: blake3.Sum256(src),
	}

	if lang == LangPlain {
		extractPlain(src, res)
	} else {
		pool := parserPools[lang]
		parser := pool.Get().(*ts.Parser)
		defer pool.Put(parser)

		tree := parser.Parse(src, nil)
		if tree == nil {
			return nil, fmt.Errorf("parse failed: %s", relPath)
		}
		defer tree.Close()

		root := tree.RootNode()
		res.Partial = root.HasError()
		switch lang {
		case LangJava:
			extractJava(src, root, res)
		case LangKotlin:
			extractKotlin(src, root, res)
		case LangPython:
			extractPython(src, root, res)
		}
	}

	// §보완 B: node bodies (literals, comments, fields) become lexical
	// tokens so search_hybrid reaches sub-symbol content.
	for i := range res.Nodes {
		n := &res.Nodes[i]
		if n.StartByte < n.EndByte && int(n.EndByte) <= len(src) {
			n.BodyTokens = lexical.TokenizeCapped(string(src[n.StartByte:n.EndByte]), maxBodyTokens)
		}
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
