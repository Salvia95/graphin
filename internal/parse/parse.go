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
	tsjs "github.com/tree-sitter/tree-sitter-javascript/bindings/go"
	tspython "github.com/tree-sitter/tree-sitter-python/bindings/go"
	tsts "github.com/tree-sitter/tree-sitter-typescript/bindings/go"
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
	LangJavaScript // .js/.jsx/.mjs/.cjs — JSX is built into the JS grammar
	LangTypeScript // .ts/.mts/.cts (including .d.ts)
	LangTSX        // .tsx — separate tree-sitter grammar
	// LangPlain marks indexable text files without a grammar (§보완 A):
	// configs, SQL, docs. They become single file-kind nodes.
	LangPlain
	// LangDBSchema marks graphindb RDB snapshot files (*.graphindb.json —
	// schema/graphindb.md). Tables/views/functions/RLS/triggers become
	// individual nodes; columns and constraints fold into the table node.
	LangDBSchema
)

// FileScoped reports whether the language derives its package from the file
// path (Python modules, JS/TS modules) rather than a declared package.
func (l Language) FileScoped() bool {
	switch l {
	case LangPython, LangJavaScript, LangTypeScript, LangTSX:
		return true
	}
	return false
}

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
	case strings.HasSuffix(path, ".min.js"), strings.HasSuffix(path, ".min.mjs"),
		strings.HasSuffix(path, ".min.cjs"):
		return LangUnknown // minified = generated
	case strings.HasSuffix(path, ".js"), strings.HasSuffix(path, ".jsx"),
		strings.HasSuffix(path, ".mjs"), strings.HasSuffix(path, ".cjs"):
		return LangJavaScript
	case strings.HasSuffix(path, ".tsx"):
		return LangTSX
	case strings.HasSuffix(path, ".ts"), strings.HasSuffix(path, ".mts"),
		strings.HasSuffix(path, ".cts"):
		return LangTypeScript
	case strings.HasSuffix(path, graphindbSuffix):
		return LangDBSchema
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
	// .prisma: 매니페스트로 라우팅되면 DB 노드, 아니면 plain 파일 노드.
	".prisma": true,
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

// DBRefSource classifies how a code node referenced a DB table (Phase 7a,
// docs/phase7-spec.md §1).
type DBRefSource uint8

const (
	// DBRefExplicit is a written physical name: JPA @Table(name=...),
	// SQLAlchemy __tablename__, Django Meta.db_table, TypeORM @Entity("x").
	DBRefExplicit DBRefSource = iota
	// DBRefClient is an ORM client member access (prisma.<model>.<op>);
	// resolves through DB-node aliases, registry-bound.
	DBRefClient
	// DBRefConvention is an entity class name without a physical mapping;
	// resolution also tries the snake_case form, registry-bound.
	DBRefConvention
)

// DBRef is one code→DB table reference emitted by an extractor. The name is
// recorded as written — datasource/schema are unknowable in code and are
// judged at edge resolution.
type DBRef struct {
	Name   string
	Source DBRefSource
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
	Params      []string // parameter type texts as written; DB 노드는 컬럼 "name type"
	Supers      []string // extends/implements names as written (class kinds); DB 노드는 참조 FQN
	LogicalRefs []string // graphindb 전용: enforced:false 논리/다형성 참조 대상 FQN
	Calls       []Call
	BodyTokens  []string // capped lexical tokens of the node's source slice
	DBRefs      []DBRef  // 코드 노드 전용: 코드→DB 테이블 참조 (Phase 7a)
	Aliases     []string // DB 노드 전용: 추가 조회명 (prisma 모델명 등)
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
	jsLang     = ts.NewLanguage(tsjs.Language())
	tsLang     = ts.NewLanguage(tsts.LanguageTypescript())
	tsxLang    = ts.NewLanguage(tsts.LanguageTSX())
)

func grammar(lang Language) *ts.Language {
	switch lang {
	case LangJava:
		return javaLang
	case LangKotlin:
		return kotlinLang
	case LangPython:
		return pythonLang
	case LangJavaScript:
		return jsLang
	case LangTypeScript:
		return tsLang
	case LangTSX:
		return tsxLang
	}
	return nil
}

// Parsers are not goroutine-safe; pool one per language per worker.
var parserPools = map[Language]*sync.Pool{
	LangJava:       newPool(LangJava),
	LangKotlin:     newPool(LangKotlin),
	LangPython:     newPool(LangPython),
	LangJavaScript: newPool(LangJavaScript),
	LangTypeScript: newPool(LangTypeScript),
	LangTSX:        newPool(LangTSX),
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
	return FileWithRoute(relPath, src, nil)
}

// FileWithRoute parses one file, optionally under a graphindb manifest route:
// a routed SSOT file (schema.sql, schema.prisma, project JSON) extracts DB
// nodes per the route's format instead of its surface syntax. route == nil is
// exactly File().
func FileWithRoute(relPath string, src []byte, route *DBRoute) (*FileResult, error) {
	if route != nil {
		res := &FileResult{RelPath: relPath, Lang: LangDBSchema, FileHash: blake3.Sum256(src)}
		extractDBRouted(src, route, res)
		attachBodyTokens(res, src)
		return res, nil
	}
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
	} else if lang == LangDBSchema {
		extractDBSchema(src, res)
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
		case LangJavaScript, LangTypeScript, LangTSX:
			extractJS(src, root, res)
		}
	}

	attachBodyTokens(res, src)
	return res, nil
}

// attachBodyTokens (§보완 B): node bodies (literals, comments, fields) become
// lexical tokens so search_hybrid reaches sub-symbol content.
func attachBodyTokens(res *FileResult, src []byte) {
	for i := range res.Nodes {
		n := &res.Nodes[i]
		if n.StartByte < n.EndByte && int(n.EndByte) <= len(src) {
			n.BodyTokens = lexical.TokenizeCapped(string(src[n.StartByte:n.EndByte]), maxBodyTokens)
		}
	}
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
