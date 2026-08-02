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
// (search_hybrid/explore_graph/read_code) — the funnel the headline metrics
// track. bootstrap/benchmark are graphin tools but not navigation.
func (c Class) IsGraphinNav() bool {
	return c == ClassGSearch || c == ClassGExplore || c == ClassGRead
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
	case "search_hybrid":
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
