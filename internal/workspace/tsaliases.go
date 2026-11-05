package workspace

import (
	"encoding/json"
	"os"
	pathpkg "path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Salvia95/graphin/internal/parse"
)

// TypeScript path aliases (compilerOptions.paths / baseUrl).
//
// `import { X } from "@/features/order"` is a bare specifier, so the parser
// records it as the dotted module "@.features.order" — a module no file has,
// and the import never matched. Every call through it fell to the global
// tier (0.80), below the 0.85 default cut, which in one TS workspace was the
// single biggest share of missing caller edges. The parser stays a pure
// function of one file; the workspace knows the tree, so it reads the
// nearest tsconfig.json / jsconfig.json and rewrites the bare imports of each
// JS/TS file into the dotted module space its files live in.

// tsAliasRule maps one paths pattern. Prefixes and targets are dotted and
// root-relative: "@/*" → prefix "@.", target "web.src." for a tsconfig in
// web/ with "./src/*".
type tsAliasRule struct {
	prefix string
	exact  bool   // pattern without "*": the whole specifier must match
	target string // first listed target
}

type tsAliases struct {
	rules []tsAliasRule // longest prefix first, as TypeScript matches them
	base  string        // dotted baseUrl prefix ("web.src."), "" when unset
}

// rewrite maps the bare imports of one parse result through the aliases.
func (a *tsAliases) rewrite(res *parse.FileResult) {
	for i, imp := range res.Imports {
		if i >= len(res.ImportBare) || !res.ImportBare[i] {
			continue
		}
		if to, ok := a.mapImport(imp); ok {
			res.Imports[i] = to
		}
	}
}

func (a *tsAliases) mapImport(imp string) (string, bool) {
	for _, r := range a.rules {
		if r.exact {
			if imp == r.prefix {
				return strings.TrimSuffix(r.target, "."), true
			}
			if rest, ok := strings.CutPrefix(imp, r.prefix+"."); ok {
				return r.target + rest, true
			}
			continue
		}
		if rest, ok := strings.CutPrefix(imp, r.prefix); ok {
			return r.target + rest, true
		}
	}
	if a.base != "" {
		return a.base + imp, true
	}
	return "", false
}

// tsConfigNames are the files whose nearest copy governs a JS/TS file.
var tsConfigNames = []string{"tsconfig.json", "jsconfig.json"}

// isTSConfig reports whether rel can change some file's aliases: a config
// itself, or any tsconfig.*.json an "extends" may point at.
func isTSConfig(rel string) bool {
	b := pathpkg.Base(rel)
	return b == "jsconfig.json" || (strings.HasPrefix(b, "tsconfig") && strings.HasSuffix(b, ".json"))
}

// isJSFamily reports whether a language takes tsconfig aliases.
func isJSFamily(l parse.Language) bool {
	switch l {
	case parse.LangJavaScript, parse.LangTypeScript, parse.LangTSX:
		return true
	}
	return false
}

// applyTSAliases rewrites res's bare imports through the aliases of the
// nearest config above it. It caches per directory; resetTSAliases drops the
// cache when a config changes.
func (w *Workspace) applyTSAliases(res *parse.FileResult) {
	if res == nil || !isJSFamily(res.Lang) || len(res.ImportBare) == 0 {
		return
	}
	if a := w.tsAliasesFor(pathpkg.Dir(res.RelPath)); a != nil {
		a.rewrite(res)
	}
}

func (w *Workspace) resetTSAliases() {
	w.tsAliasMu.Lock()
	w.tsAliasCache = map[string]*tsAliases{}
	w.tsAliasMu.Unlock()
}

// tsAliasesFor returns the aliases governing files in dir ("." is the root),
// or nil when no config above it defines any. Two parse workers may compute
// the same directory at once; both get the same answer.
func (w *Workspace) tsAliasesFor(dir string) *tsAliases {
	w.tsAliasMu.Lock()
	a, ok := w.tsAliasCache[dir]
	w.tsAliasMu.Unlock()
	if ok {
		return a
	}
	found := false
	for _, name := range tsConfigNames {
		rel := pathpkg.Join(dir, name)
		if _, err := os.Stat(filepath.Join(w.Root, filepath.FromSlash(rel))); err == nil {
			a, found = loadTSAliases(w.Root, rel), true
			break
		}
	}
	if !found && dir != "." && dir != "" {
		a = w.tsAliasesFor(pathpkg.Dir(dir))
	}
	w.tsAliasMu.Lock()
	w.tsAliasCache[dir] = a
	w.tsAliasMu.Unlock()
	return a
}

type tsCompilerOptions struct {
	BaseURL *string             `json:"baseUrl"`
	Paths   map[string][]string `json:"paths"`
}

type tsConfigFile struct {
	Extends         json.RawMessage   `json:"extends"`
	CompilerOptions tsCompilerOptions `json:"compilerOptions"`
}

// loadTSAliases reads one config and the relative configs it extends. Paths
// and baseUrl each come from the nearest config in the chain that sets them,
// resolved against that config's own directory, as TypeScript does.
// Package extends ("@tsconfig/next") live in node_modules and are skipped.
func loadTSAliases(root, rel string) *tsAliases {
	var baseDir string // root-relative directory baseUrl points at
	haveBase := false
	var paths map[string][]string
	pathsDir := ""
	seen := map[string]bool{}
	for cur := rel; cur != "" && !seen[cur] && len(seen) < 8; {
		seen[cur] = true
		cfg, ok := readTSConfig(filepath.Join(root, filepath.FromSlash(cur)))
		if !ok {
			break
		}
		dir := pathpkg.Dir(cur)
		if !haveBase && cfg.CompilerOptions.BaseURL != nil {
			baseDir = pathpkg.Join(dir, *cfg.CompilerOptions.BaseURL)
			haveBase = true
		}
		if paths == nil && cfg.CompilerOptions.Paths != nil {
			paths = cfg.CompilerOptions.Paths
			pathsDir = dir
		}
		cur = relativeExtends(cfg.Extends, dir)
	}
	if paths == nil && !haveBase {
		return nil
	}
	if haveBase {
		pathsDir = baseDir // paths resolve against baseUrl when there is one
	}

	a := &tsAliases{}
	for pat, targets := range paths {
		if len(targets) == 0 {
			continue
		}
		wild := strings.HasSuffix(pat, "*")
		tgt := pathpkg.Join(pathsDir, strings.TrimSuffix(targets[0], "*"))
		if pathpkg.IsAbs(tgt) || strings.HasPrefix(tgt, "..") {
			continue // outside the tree: nothing indexed there
		}
		r := tsAliasRule{exact: !wild}
		if wild {
			r.prefix = dotted(strings.TrimSuffix(pat, "*"))
			r.target = dottedDir(tgt)
		} else {
			r.prefix = dotted(pat)
			r.target = dottedDir(moduleOf(tgt))
		}
		if r.prefix == "" && !wild {
			continue
		}
		a.rules = append(a.rules, r)
	}
	sort.Slice(a.rules, func(i, j int) bool {
		if len(a.rules[i].prefix) != len(a.rules[j].prefix) {
			return len(a.rules[i].prefix) > len(a.rules[j].prefix)
		}
		return a.rules[i].prefix < a.rules[j].prefix
	})
	if haveBase && !strings.HasPrefix(baseDir, "..") {
		a.base = dottedDir(baseDir)
	}
	if len(a.rules) == 0 && a.base == "" {
		return nil
	}
	return a
}

// dotted turns a specifier fragment into the parser's dotted form.
func dotted(s string) string { return strings.ReplaceAll(s, "/", ".") }

// dottedDir turns a root-relative directory into a dotted prefix ending in
// "." ("" for the root itself).
func dottedDir(dir string) string {
	dir = strings.Trim(pathpkg.Clean(dir), "/")
	if dir == "." || dir == "" {
		return ""
	}
	return dotted(dir) + "."
}

// moduleOf strips a file target to its module path, the way the parser
// derives a module from a file ("src/env.ts", "src/lib/index.ts").
func moduleOf(p string) string {
	for _, ext := range []string{".d.ts", ".ts", ".tsx", ".mts", ".cts", ".js", ".jsx", ".mjs", ".cjs"} {
		if strings.HasSuffix(p, ext) {
			p = strings.TrimSuffix(p, ext)
			break
		}
	}
	return strings.TrimSuffix(p, "/index")
}

func relativeExtends(raw json.RawMessage, dir string) string {
	var one string
	if json.Unmarshal(raw, &one) != nil {
		var many []string
		if json.Unmarshal(raw, &many) != nil || len(many) == 0 {
			return ""
		}
		one = many[len(many)-1] // later entries override earlier ones
	}
	if !strings.HasPrefix(one, "./") && !strings.HasPrefix(one, "../") {
		return ""
	}
	p := pathpkg.Join(dir, one)
	if !strings.HasSuffix(p, ".json") {
		p += ".json"
	}
	if strings.HasPrefix(p, "..") {
		return ""
	}
	return p
}

func readTSConfig(abs string) (tsConfigFile, bool) {
	var cfg tsConfigFile
	b, err := os.ReadFile(abs)
	if err != nil {
		return cfg, false
	}
	return cfg, json.Unmarshal(stripJSONC(b), &cfg) == nil
}

// stripJSONC removes the comments and trailing commas tsconfig allows.
func stripJSONC(b []byte) []byte {
	out := make([]byte, 0, len(b))
	inStr, esc := false, false
	for i := 0; i < len(b); i++ {
		c := b[i]
		if inStr {
			out = append(out, c)
			switch {
			case esc:
				esc = false
			case c == '\\':
				esc = true
			case c == '"':
				inStr = false
			}
			continue
		}
		switch {
		case c == '"':
			inStr = true
			out = append(out, c)
		case c == '/' && i+1 < len(b) && b[i+1] == '/':
			for i < len(b) && b[i] != '\n' {
				i++
			}
			if i < len(b) {
				out = append(out, '\n')
			}
		case c == '/' && i+1 < len(b) && b[i+1] == '*':
			i += 2
			for i+1 < len(b) && !(b[i] == '*' && b[i+1] == '/') {
				i++
			}
			i++
		case c == ']' || c == '}':
			// drop a trailing comma before the closer
			j := len(out) - 1
			for j >= 0 && (out[j] == ' ' || out[j] == '\t' || out[j] == '\n' || out[j] == '\r') {
				j--
			}
			if j >= 0 && out[j] == ',' {
				out = append(out[:j], out[j+1:]...)
			}
			out = append(out, c)
		default:
			out = append(out, c)
		}
	}
	return out
}

// reapplyJSImportsLocked drops the alias cache and re-parses every indexed
// JS/TS file into the graph. Only imports can have changed — the bytes did
// not — so lexical and semantic indexes are left alone, and the graph
// re-resolves the nodes whose imports moved. Callers hold w.indexMu.
func (w *Workspace) reapplyJSImportsLocked() int {
	w.resetTSAliases()
	if w.graph == nil {
		return 0
	}
	var rels []string
	for rel := range w.merkle.Files {
		if isJSFamily(parse.DetectLanguage(rel)) {
			rels = append(rels, rel)
		}
	}
	sort.Strings(rels)
	n := 0
	for _, rel := range rels {
		if w.reresolveFileLocked(rel) {
			n++
		}
	}
	if n > 0 {
		w.Log.Event("tsconfig_reapply", map[string]any{"files": n})
	}
	return n
}
