// Package usage implements the adoption/fallback instrumentation pipeline
// (docs/usage-spec.md): the `graphin usage ingest` hook sink that appends
// PostToolUse events to <ws>/.graphin/usage/events.jsonl, and the
// `graphin usage report` reader that turns adjacent tool-call sequences into
// the four headline metrics (§0).
package usage

import (
	"regexp"
	"strings"
)

// Event is one logged tool call (spec §3, schema v1).
type Event struct {
	V         int            `json:"v"`
	TS        string         `json:"ts"`
	SessionID string         `json:"session_id"`
	PromptID  string         `json:"prompt_id,omitempty"`
	ToolUseID string         `json:"tool_use_id,omitempty"`
	Parallel  bool           `json:"parallel,omitempty"`
	AgentID   string         `json:"agent_id,omitempty"`
	AgentType string         `json:"agent_type,omitempty"`
	CWD       string         `json:"cwd,omitempty"`
	Tool      string         `json:"tool"`
	P         map[string]any `json:"p,omitempty"`
}

// Class is the report-time normalization of a tool name (spec §4.1).
// Classification never happens at log time so definitions can evolve without
// invalidating collected data.
type Class string

const (
	ClassGSearch  Class = "g_search"
	ClassGExplore Class = "g_explore"
	ClassGRead    Class = "g_read"
	ClassGBoot    Class = "g_boot"
	ClassGBench   Class = "g_bench"
	ClassSearch   Class = "search"
	ClassRead     Class = "read"
	ClassAction   Class = "action"
	ClassOther    Class = "other"
)

// IsGraphinNav reports whether c is a graphin navigation call
// (search_hybrid/search_keyword/explore_graph/read_code) — the funnel the
// headline metrics track. bootstrap/benchmark are graphin tools but not
// navigation.
func (c Class) IsGraphinNav() bool {
	return c == ClassGSearch || c == ClassGExplore || c == ClassGRead
}

// Retriever names for the g_search split (spec §4.3). The two engines answer
// different question shapes — a symbol or a sentence goes to hybrid, an exact
// string to keyword — so "is graphin adopted" has a different answer for each,
// and a merged rate hides the one that is failing.
const (
	RetrieverHybrid  = "hybrid"
	RetrieverKeyword = "keyword"
)

// Retriever reports which search engine a g_search event used, or "" for any
// other event.
func (e Event) Retriever() string {
	name := e.Tool
	if m := mcpSuffix.FindStringSubmatch(e.Tool); m != nil {
		name = m[1]
	}
	switch name {
	case "search_hybrid":
		return RetrieverHybrid
	case "search_keyword":
		return RetrieverKeyword
	}
	return ""
}

// SearchQuery returns what a g_search event asked for. search_hybrid logs a
// `query`, search_keyword a `pattern`; same-intent overlap and the fallback
// pair table need one accessor for both, or a keyword run that ends in grep
// would be judged with no query to compare against.
func (e Event) SearchQuery() string {
	if q, _ := e.P["query"].(string); q != "" {
		return q
	}
	s, _ := e.P["pattern"].(string)
	return s
}

// mcpSuffix extracts the tool segment of an mcp__<server>__<tool> name. The
// server segment is the user's `claude mcp add <key>` config key, not the
// server's own name, so it must never participate in matching (spec §3).
var mcpSuffix = regexp.MustCompile(`^mcp__.+__([a-zA-Z0-9_]+)$`)

// Classify maps a raw tool_name (+ extracted payload) to its Class.
func Classify(tool string, p map[string]any) Class {
	name := tool
	if m := mcpSuffix.FindStringSubmatch(tool); m != nil {
		name = m[1]
	}
	switch name {
	// Both retrievers are the same class: a keyword search that ends in grep
	// is a fallback like any other. Before 2026-09-05 search_keyword fell to
	// `other`, so a window that only ever used it was invisible to every
	// headline metric — neither adoption nor fallback, and not even a
	// graphin call for the discovery-failure denominator.
	case "search_hybrid", "search_keyword":
		return ClassGSearch
	case "explore_graph":
		return ClassGExplore
	case "read_code":
		return ClassGRead
	case "bootstrap_workspace":
		return ClassGBoot
	case "run_local_benchmark":
		return ClassGBench
	case "Grep", "Glob":
		return ClassSearch
	case "Read":
		return ClassRead
	case "Edit", "Write", "NotebookEdit":
		return ClassAction
	case "Bash":
		if b, ok := p["search"].(bool); ok && b {
			return ClassSearch
		}
		return ClassOther
	default:
		return ClassOther
	}
}

// stopwords excluded from same-intent token overlap (spec §4.2): generic
// query filler that would fabricate overlap between unrelated searches.
var stopwords = map[string]bool{
	"the": true, "and": true, "for": true, "where": true, "what": true,
	"how": true, "code": true, "file": true, "function": true, "class": true,
	"def": true, "func": true, "implementation": true, "find": true,
}

// Tokens normalizes a query or grep pattern for same-intent overlap:
// split on non-alphanumerics, snake_case and camelCase boundaries, then
// lowercase and drop tokens shorter than 3 chars plus stopwords. Camel
// splitting matters because fallback grep patterns are usually identifiers
// (`persistIndexesLocked`) while queries are prose ("persist indexes").
func Tokens(s string) map[string]bool {
	out := map[string]bool{}
	var w []rune
	flush := func() {
		if len(w) >= 3 {
			t := strings.ToLower(string(w))
			if !stopwords[t] {
				out[t] = true
			}
		}
		w = w[:0]
	}
	for _, r := range s {
		switch {
		case r >= 'A' && r <= 'Z':
			flush() // camel boundary
			w = append(w, r)
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			w = append(w, r)
		default: // separator (incl. '_')
			flush()
		}
	}
	flush()
	return out
}

// Overlaps reports whether the two token sets share at least one token.
func Overlaps(a, b map[string]bool) bool {
	for t := range a {
		if b[t] {
			return true
		}
	}
	return false
}

// Shape is the coarse form of a search pattern (spec §4.2). It exists for one
// reason: `discovery_failure` must not count searches graphin could never have
// answered. A window spent grepping `database is locked` or `^E` through a
// traceback is runtime debugging, not a missed navigation — counting it made
// the 2026-08-07 diagnosis report 57% where the honest number was half that.
type Shape string

const (
	ShapeSymbol  Shape = "symbol"  // CONTEXT_TYPES · validate_and_seal · class PublishBody
	ShapeRegex   Shape = "regex"   // ^ *$ · *.go · ^###
	ShapeLiteral Shape = "literal" // database is locked · 필수 컨텍스트 미충족
	ShapeNone    Shape = "none"    // no pattern recorded (search-Bash we could not parse)
)

// regexMeta are the characters that make a pattern a pattern. '.' is
// deliberately absent: it is the single most common character inside real
// identifiers (`sqlalchemy.exc`, `foo.go`) and treating it as a metacharacter
// would classify half of everything as regex.
const regexMeta = `^$*+?[]{}()|\`

// SearchPattern returns the grep/glob pattern an event searched for, or "".
func (e Event) SearchPattern() string {
	s, _ := e.P["pattern"].(string)
	return s
}

// PatternShape classifies a search pattern by form, using the rules the
// adoption diagnosis used on the raw log (findings §검색의 형태): normalize
// shell residue, then metacharacters, then identifier shape.
//
// It is deliberately conservative — every ambiguous case lands outside
// `symbol`. This metric's failure mode is over-reporting failure, so a rule
// that shrinks the denominator when unsure is the right bias.
func PatternShape(p string) Shape {
	p = normalizePattern(p)
	if p == "" {
		return ShapeNone
	}
	if strings.ContainsAny(p, regexMeta) {
		return ShapeRegex
	}
	// Prose is several words; a symbol search is the symbol, sometimes behind
	// one keyword (`class PublishBody`, `def _context_type_findings`). Past two
	// tokens it is a sentence, and a capitalized first word would otherwise
	// make every English error message look like a symbol.
	toks := strings.Fields(p)
	if len(toks) > 2 {
		return ShapeLiteral
	}
	for _, t := range toks {
		if strongIdent(t) {
			return ShapeSymbol
		}
	}
	return ShapeLiteral
}

// normalizePattern strips what the shell leaves behind: trailing backslashes
// from line continuations and escaping, and one layer of matched quotes.
func normalizePattern(p string) string {
	p = strings.TrimSpace(p)
	p = strings.TrimRight(p, `\`)
	p = strings.TrimSpace(p)
	if len(p) >= 2 {
		if q := p[0]; (q == '\'' || q == '"') && p[len(p)-1] == q {
			p = strings.TrimSpace(p[1 : len(p)-1])
		}
	}
	return p
}

// strongIdent reports whether t is an identifier that could not be ordinary
// prose. A plain lowercase word ("database") is indistinguishable from English,
// so it does not qualify — only the shapes code writes and prose does not:
// snake_case, camelCase/PascalCase, or SCREAMING_CAPS.
func strongIdent(t string) bool {
	if t == "" {
		return false
	}
	hasUpper, hasLower, hasUnderscore := false, false, false
	for i, r := range t {
		switch {
		case r == '_':
			hasUnderscore = true
		case r >= 'a' && r <= 'z':
			hasLower = true
		case r >= 'A' && r <= 'Z':
			hasUpper = true
		case r >= '0' && r <= '9':
			if i == 0 {
				return false // identifiers do not start with a digit
			}
		default:
			return false // anything else (incl. non-ASCII prose) is not an identifier
		}
	}
	switch {
	case hasUnderscore:
		return true
	case hasUpper && hasLower:
		return true
	case hasUpper && len(t) >= 2: // SCREAMING_CAPS without the underscore
		return true
	}
	return false
}
