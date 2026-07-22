package parse

// SQL-literal table references (Phase 7b, docs/phase7-spec.md §2): string
// literals inside code nodes yield DBRefs only when a real SQL keyword
// context is present (FROM/JOIN with SELECT|DELETE, INSERT INTO,
// UPDATE … SET). Resolution is registry-bound — free-text identifiers that
// match no indexed table never create edges, and SQL refs never dangle.

import (
	"regexp"
	"strings"

	ts "github.com/tree-sitter/go-tree-sitter"
)

var (
	sqlFromRe   = regexp.MustCompile(`(?is)\b(?:from|join)\s+([a-zA-Z_][\w$.]*)`)
	sqlInsertRe = regexp.MustCompile(`(?is)\binsert\s+into\s+([a-zA-Z_][\w$.]*)`)
	sqlUpdateRe = regexp.MustCompile(`(?is)\bupdate\s+([a-zA-Z_][\w$.]*)`)
)

// sqlIdentNoise: identifiers legally following the anchor keywords that are
// never table names (subquery/clause openers).
var sqlIdentNoise = map[string]bool{
	"select": true, "where": true, "values": true, "set": true,
	"lateral": true, "only": true, "unnest": true, "generate_series": true,
}

// hasSQLWord reports a word-boundary match of w in the lowercased text.
func hasSQLWord(low, w string) bool {
	for i := 0; ; {
		j := strings.Index(low[i:], w)
		if j < 0 {
			return false
		}
		j += i
		before := j == 0 || !isIdentByte(low[j-1])
		afterIdx := j + len(w)
		after := afterIdx >= len(low) || !isIdentByte(low[afterIdx])
		if before && after {
			return true
		}
		i = j + 1
	}
}

func isIdentByte(c byte) bool {
	return c == '_' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

// sqlTableNames extracts referenced table identifiers from one literal.
// Schema-qualified names collapse to the last segment (matching resolves by
// simple name; ambiguity guards handle collisions).
func sqlTableNames(literal string) []string {
	low := strings.ToLower(literal)
	var out []string
	seen := map[string]bool{}
	add := func(matches [][]string) {
		for _, g := range matches {
			name := g[1]
			if i := strings.LastIndexByte(name, '.'); i >= 0 {
				name = name[i+1:]
			}
			key := strings.ToLower(name)
			if name == "" || seen[key] || sqlIdentNoise[key] {
				continue
			}
			seen[key] = true
			out = append(out, name)
		}
	}
	if (hasSQLWord(low, "select") || hasSQLWord(low, "delete")) && hasSQLWord(low, "from") {
		add(sqlFromRe.FindAllStringSubmatch(literal, -1))
	}
	if hasSQLWord(low, "insert") {
		add(sqlInsertRe.FindAllStringSubmatch(literal, -1))
	}
	if hasSQLWord(low, "update") && hasSQLWord(low, "set") {
		add(sqlUpdateRe.FindAllStringSubmatch(literal, -1))
	}
	return out
}

// sqlStringKinds names each grammar's string-literal node kinds. A matched
// node is scanned whole and not descended into (python concatenated_string
// gates its parts as one literal).
var sqlStringKinds = map[Language]map[string]bool{
	LangJava:       {"string_literal": true},
	LangKotlin:     {"string_literal": true},
	LangPython:     {"string": true, "concatenated_string": true},
	LangJavaScript: {"string": true, "template_string": true},
	LangTypeScript: {"string": true, "template_string": true},
	LangTSX:        {"string": true, "template_string": true},
}

// appendSQLRefs walks n's subtree for SQL-bearing string literals and
// appends deduplicated DBRefSQL refs.
func appendSQLRefs(refs []DBRef, lang Language, src []byte, n *ts.Node) []DBRef {
	kinds := sqlStringKinds[lang]
	if kinds == nil || n == nil {
		return refs
	}
	seen := map[string]bool{}
	for _, r := range refs {
		if r.Source == DBRefSQL {
			seen[r.Name] = true
		}
	}
	var walk func(c *ts.Node)
	walk = func(c *ts.Node) {
		if kinds[c.Kind()] {
			for _, name := range sqlTableNames(text(c, src)) {
				if !seen[name] {
					seen[name] = true
					refs = append(refs, DBRef{Name: name, Source: DBRefSQL})
				}
			}
			return
		}
		eachNamed(c, walk)
	}
	walk(n)
	return refs
}
