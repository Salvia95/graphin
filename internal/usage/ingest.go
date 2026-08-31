package usage

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	maxStdin    = 8 << 20 // hook stdin cap; read_code responses can be large
	maxLine     = 8 << 10 // keep O_APPEND writes atomic on local filesystems
	maxStr      = 300     // per-string payload truncation (privacy + size)
	maxWalkUp   = 8       // marker walk-up bound, matches the usage hook
	rotateBytes = 32 << 20
	markerRel   = ".graphin/merkle.json"
	// binpathRel is written by the server at startup (spec §2.2), before and
	// independently of any indexing. Its presence without markerRel is the
	// signature of the failure the metrics cannot count: the server ran and
	// the agent never called bootstrap_workspace.
	binpathRel = ".graphin/binpath"
)

// hookInput is the PostToolUse JSON Claude Code writes to the hook's stdin.
type hookInput struct {
	HookEventName string          `json:"hook_event_name"`
	SessionID     string          `json:"session_id"`
	PromptID      string          `json:"prompt_id"`
	ToolUseID     string          `json:"tool_use_id"`
	Parallel      bool            `json:"parallel"`
	AgentID       string          `json:"agent_id"`
	AgentType     string          `json:"agent_type"`
	CWD           string          `json:"cwd"`
	ToolName      string          `json:"tool_name"`
	ToolInput     map[string]any  `json:"tool_input"`
	ToolResponse  json.RawMessage `json:"tool_response"`
}

// Ingest implements `graphin usage ingest` (spec §2.3): parse one PostToolUse
// payload from stdin and append one event line under the owning workspace.
// It must never block or fail the session, so every path returns 0; errors go
// to stderr only (the plugin handler discards them, direct runs see them).
func Ingest(stdin io.Reader, stderr io.Writer, getenv func(string) string) int {
	raw, err := io.ReadAll(io.LimitReader(stdin, maxStdin))
	if err != nil {
		fmt.Fprintf(stderr, "usage ingest: read stdin: %v\n", err)
		return 0
	}
	var in hookInput
	if err := json.Unmarshal(raw, &in); err != nil {
		fmt.Fprintf(stderr, "usage ingest: bad JSON: %v\n", err)
		return 0
	}
	if in.HookEventName != "PostToolUse" || in.SessionID == "" || in.ToolName == "" {
		return 0
	}

	// Route to the nearest indexed workspace from the event's cwd, falling
	// back to the root the handler already verified. A session that moves
	// between workspaces logs each event at the right one.
	root := findRoot(in.CWD)
	if root == "" {
		root = getenv("GRAPHIN_WS_ROOT")
	}
	if root == "" {
		return 0
	}

	ev := Event{
		V:         1,
		TS:        time.Now().UTC().Format(time.RFC3339Nano),
		SessionID: in.SessionID,
		PromptID:  in.PromptID,
		ToolUseID: in.ToolUseID,
		Parallel:  in.Parallel,
		AgentID:   in.AgentID,
		AgentType: in.AgentType,
		CWD:       relTo(root, in.CWD),
		Tool:      in.ToolName,
		P:         extractPayload(root, in.ToolName, in.ToolInput, in.ToolResponse),
	}
	line, err := json.Marshal(ev)
	if err != nil {
		return 0
	}
	if len(line) > maxLine {
		ev.P = nil
		if line, err = json.Marshal(ev); err != nil {
			return 0
		}
	}
	if err := appendLine(filepath.Join(root, ".graphin", "usage"), line); err != nil {
		fmt.Fprintf(stderr, "usage ingest: append: %v\n", err)
	}
	return 0
}

// findRoot walks up from dir (bounded) looking for the index marker. The
// marker is merkle.json, not the .graphin dir: the dir appears the moment the
// server starts, the marker only after the initial scan persisted (spec §2.1).
func findRoot(dir string) string { return findMarker(dir, markerRel) }

// findServerRoot walks up for the startup sidecar instead of the index marker.
// A hit here with no findRoot hit means graphin ran in this tree and was never
// asked to do anything.
func findServerRoot(dir string) string { return findMarker(dir, binpathRel) }

func findMarker(dir, rel string) string {
	if dir == "" || !filepath.IsAbs(dir) {
		return ""
	}
	d := filepath.Clean(dir)
	for i := 0; i <= maxWalkUp; i++ {
		if fi, err := os.Stat(filepath.Join(d, rel)); err == nil && !fi.IsDir() {
			return d
		}
		parent := filepath.Dir(d)
		if parent == d {
			return ""
		}
		d = parent
	}
	return ""
}

func relTo(root, p string) string {
	if p == "" {
		return ""
	}
	if rel, err := filepath.Rel(root, p); err == nil && !strings.HasPrefix(rel, "..") {
		return rel
	}
	return p
}

func appendLine(dir string, line []byte) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, "events.jsonl")
	if fi, err := os.Stat(path); err == nil && fi.Size() > rotateBytes {
		stamp := time.Now().UTC().Format("20060102T150405Z")
		// Losing this rename race to a concurrent session is fine — one of
		// the writers rotates, the rest append to the fresh file.
		_ = os.Rename(path, filepath.Join(dir, "events-"+stamp+".jsonl"))
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(line, '\n'))
	return err
}

// extractPayload keeps only what the metrics need (spec §3). Never the full
// Bash command line, never file contents, never response bodies beyond
// graphin result IDs.
func extractPayload(root, tool string, input map[string]any, resp json.RawMessage) map[string]any {
	name := tool
	if m := mcpSuffix.FindStringSubmatch(tool); m != nil {
		name = m[1]
	}
	str := func(k string) string { s, _ := input[k].(string); return trunc(s) }
	switch name {
	case "search_hybrid":
		p := map[string]any{"query": str("query")}
		if k, ok := input["top_k"].(float64); ok {
			p["top_k"] = int(k)
		}
		// The target filter is the one search knob the skill pushes hard
		// (docs/eval/2026-08-12-target-filter). Dropping it here made "does
		// the agent actually filter" unanswerable from the log — the query
		// alone cannot tell a filtered search from an unfiltered one. Only
		// recorded when set: absent means the caller searched everything.
		if v := str("target"); v != "" {
			p["target"] = v
		}
		if ids := responseNodeIDs(resp); ids != nil {
			p["result_count"] = len(ids)
			if len(ids) > 5 {
				ids = ids[:5]
			}
			p["result_ids"] = ids
		}
		return p
	case "explore_graph":
		return map[string]any{"node_id": str("node_id"), "direction": str("direction")}
	case "read_code":
		return map[string]any{"node_id": str("node_id")}
	case "bootstrap_workspace", "run_local_benchmark":
		return map[string]any{}
	case "Grep":
		p := map[string]any{"pattern": str("pattern")}
		if v := str("path"); v != "" {
			p["path"] = trunc(relTo(root, v))
		}
		if v := str("glob"); v != "" {
			p["glob"] = v
		}
		return p
	case "Glob":
		return map[string]any{"pattern": str("pattern")}
	case "Read", "Edit", "Write":
		return map[string]any{"file_path": trunc(relTo(root, str("file_path")))}
	case "Bash":
		cmd, _ := input["command"].(string)
		search, pattern := classifyBash(cmd)
		p := map[string]any{"search": search}
		if search && pattern != "" {
			p["pattern"] = trunc(pattern)
		}
		return p
	default:
		return nil
	}
}

func trunc(s string) string {
	if len(s) > maxStr {
		return s[:maxStr]
	}
	return s
}

var nodeIDAttr = regexp.MustCompile(`<node id="([^"]+)"`)

// responseNodeIDs pulls node IDs out of a search_hybrid tool_response. The
// response is MCP content whose text blocks hold graphin's XML
// (`<results>…<node id="…"/>…`); shapes vary by client, so gather every
// string field and regex the XML. Returns nil when nothing matches.
func responseNodeIDs(resp json.RawMessage) []string {
	if len(resp) == 0 {
		return nil
	}
	var any any
	if json.Unmarshal(resp, &any) != nil {
		return nil
	}
	var texts []string
	collectStrings(any, &texts, 0)
	for _, t := range texts {
		if !strings.Contains(t, "<results") {
			continue
		}
		var ids []string
		for _, m := range nodeIDAttr.FindAllStringSubmatch(t, -1) {
			ids = append(ids, m[1])
		}
		return ids
	}
	return nil
}

func collectStrings(v any, out *[]string, depth int) {
	if depth > 4 {
		return
	}
	switch t := v.(type) {
	case string:
		*out = append(*out, t)
	case []any:
		for _, e := range t {
			collectStrings(e, out, depth+1)
		}
	case map[string]any:
		for _, e := range t {
			collectStrings(e, out, depth+1)
		}
	}
}

// searchCommands are argv[0] basenames that mark a Bash segment as a search
// (spec §3); `git grep` is special-cased.
var searchCommands = map[string]bool{
	"grep": true, "rg": true, "egrep": true, "fgrep": true,
	"ag": true, "ack": true, "fd": true, "find": true,
}

// classifyBash decides whether a Bash command is search-like and extracts the
// search pattern. The command is split per pipeline/&&/;/|| segment; the full
// command line is never stored.
func classifyBash(cmd string) (bool, string) {
	for _, seg := range splitSegments(cmd) {
		argv := fields(seg)
		if len(argv) == 0 {
			continue
		}
		prog := filepath.Base(argv[0])
		rest := argv[1:]
		if prog == "git" && len(rest) > 0 && rest[0] == "grep" {
			prog, rest = "grep", rest[1:]
		}
		if !searchCommands[prog] {
			continue
		}
		if prog == "find" {
			for i, a := range rest {
				if (a == "-name" || a == "-iname" || a == "-path") && i+1 < len(rest) {
					return true, rest[i+1]
				}
			}
			return true, ""
		}
		for i := 0; i < len(rest); i++ {
			a := rest[i]
			if strings.HasPrefix(a, "-") {
				switch a {
				case "-e", "--regexp": // the flag's value IS the pattern
					if i+1 < len(rest) {
						return true, rest[i+1]
					}
				case "-A", "-B", "-C", "-m", "-g", "--glob", "-t", "--type", "-f", "--file":
					i++ // value-taking flag: skip its value, keep scanning
				}
				continue
			}
			return true, a
		}
		return true, ""
	}
	return false, ""
}

func splitSegments(cmd string) []string {
	seps := []string{"&&", "||", "|", ";", "\n"}
	segs := []string{cmd}
	for _, sep := range seps {
		var next []string
		for _, s := range segs {
			next = append(next, strings.Split(s, sep)...)
		}
		segs = next
	}
	return segs
}

// fields is a shell-aware-enough tokenizer: whitespace split with single and
// double quotes grouping and stripping. No escapes, no expansion — patterns
// are what we're after, not faithful shell semantics.
func fields(s string) []string {
	var out []string
	var cur strings.Builder
	var quote rune
	flush := func() {
		if cur.Len() > 0 {
			out = append(out, cur.String())
			cur.Reset()
		}
	}
	for _, r := range s {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote = r
		case r == ' ' || r == '\t':
			flush()
		default:
			cur.WriteRune(r)
		}
	}
	flush()
	return out
}
