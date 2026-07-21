// Package ignore implements a dependency-free gitignore-syntax matcher for
// §2.4: .gitignore files (nested) plus .graphin/ignore share one rule list.
package ignore

import (
	pathpkg "path"
	"strings"
)

type rule struct {
	base     string // "/"-separated dir the pattern file lives in ("" = root)
	negate   bool
	dirOnly  bool
	anchored bool // pattern contained a non-trailing slash → relative to base
	segs     []string
}

// Matcher evaluates rules in order; the last matching rule wins, per git.
type Matcher struct {
	rules []rule
}

func NewMatcher() *Matcher { return &Matcher{} }

// AddFile parses one ignore file whose directory is baseRel ("" for root).
func (m *Matcher) AddFile(baseRel string, content []byte) {
	for _, line := range strings.Split(string(content), "\n") {
		m.addLine(baseRel, line)
	}
}

// AddPatterns adds pattern lines directly (used for .graphin/ignore).
func (m *Matcher) AddPatterns(baseRel string, lines []string) {
	for _, line := range lines {
		m.addLine(baseRel, line)
	}
}

func (m *Matcher) addLine(baseRel, line string) {
	line = strings.TrimRight(line, " \t\r")
	if line == "" || strings.HasPrefix(line, "#") {
		return
	}
	r := rule{base: strings.Trim(baseRel, "/")}
	if strings.HasPrefix(line, "!") {
		r.negate = true
		line = line[1:]
	}
	if strings.HasSuffix(line, "/") {
		r.dirOnly = true
		line = strings.TrimSuffix(line, "/")
	}
	if strings.HasPrefix(line, "/") {
		r.anchored = true
		line = strings.TrimPrefix(line, "/")
	}
	if strings.Contains(line, "/") {
		r.anchored = true
	}
	if line == "" {
		return
	}
	r.segs = strings.Split(line, "/")
	m.rules = append(m.rules, r)
}

// Ignored reports whether relPath ("/"-separated, relative to root) is
// excluded. Git semantics: once a directory is excluded, nothing inside it
// can be re-included, so every ancestor is checked first.
func (m *Matcher) Ignored(relPath string, isDir bool) bool {
	relPath = strings.Trim(relPath, "/")
	if relPath == "" {
		return false
	}
	segs := strings.Split(relPath, "/")
	for i := 1; i < len(segs); i++ {
		if m.match(strings.Join(segs[:i], "/"), true) {
			return true
		}
	}
	return m.match(relPath, isDir)
}

func (m *Matcher) match(rel string, isDir bool) bool {
	ignored := false
	for _, r := range m.rules {
		if r.matches(rel, isDir) {
			ignored = !r.negate
		}
	}
	return ignored
}

func (r *rule) matches(rel string, isDir bool) bool {
	if r.dirOnly && !isDir {
		return false
	}
	sub := rel
	if r.base != "" {
		if rel == r.base || !strings.HasPrefix(rel, r.base+"/") {
			return false
		}
		sub = rel[len(r.base)+1:]
	}
	segs := strings.Split(sub, "/")
	if r.anchored {
		return matchSegs(r.segs, segs)
	}
	// Unanchored pattern: shell glob against the basename at any depth.
	return matchSegs(r.segs, segs[len(segs)-1:])
}

// matchSegs matches glob segments against path segments; "**" spans any
// number of segments (including zero).
func matchSegs(pat, path []string) bool {
	if len(pat) == 0 {
		return len(path) == 0
	}
	if pat[0] == "**" {
		for i := 0; i <= len(path); i++ {
			if matchSegs(pat[1:], path[i:]) {
				return true
			}
		}
		return false
	}
	if len(path) == 0 {
		return false
	}
	ok, err := pathpkg.Match(pat[0], path[0])
	if err != nil || !ok {
		return false
	}
	return matchSegs(pat[1:], path[1:])
}
