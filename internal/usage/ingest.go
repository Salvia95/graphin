package usage

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
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
//
// response_bytes is added to every payload here rather than inside the
// branches, and that placement is the point: it is measured the same way for
// a graphin call and for the grep that replaced it. Token economy is a
// comparison, and a comparison needs one ruler. The server also reports its
// own <cost bytes>, but only for its own tools — the hook is the only place
// that can price the whole loop.
func extractPayload(root, tool string, input map[string]any, resp json.RawMessage) map[string]any {
	p := basePayload(root, tool, input, resp)
	if p == nil {
		return nil
	}
	if n := responseTextLen(resp); n > 0 {
		p["response_bytes"] = n
	}
	return p
}

// responseTextLen is how many bytes of text the tool put into the agent's
// context: the strings of the response, not the JSON that carried them.
func responseTextLen(resp json.RawMessage) int {
	if len(resp) == 0 {
		return 0
	}
	var v any
	if json.Unmarshal(resp, &v) != nil {
		return len(resp)
	}
	var texts []string
	collectStrings(v, &texts, 0)
	n := 0
	for _, t := range texts {
		n += len(t)
	}
	return n
}

func basePayload(root, tool string, input map[string]any, resp json.RawMessage) map[string]any {
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
		// Which retriever earned the slots, and how large a pool it chose
		// from. Contribution per retriever cannot be asked of the log any
		// other way: the result ids alone do not say whether the vector index
		// found them or BM25 did.
		if mt := responseAttrCounts(resp, matchTypeAttr); len(mt) > 0 {
			p["match_types"] = mt
		}
		if c, ok := responseAttrInt(resp, candidatesAttr); ok {
			p["candidates"] = c
		}
		if h, ok := responseHintKind(resp); ok {
			p["hint"] = h
		}
		return p
	case "search_keyword":
		p := map[string]any{"pattern": str("pattern")}
		if v, ok := input["regex"].(bool); ok && v {
			p["regex"] = true
		}
		if v := str("path"); v != "" {
			p["path"] = v
		}
		if f, ok := responseAttrInt(resp, filesAttr); ok {
			p["files"] = f
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
var (
	matchTypeAttr  = regexp.MustCompile(`match_type="([^"]+)"`)
	candidatesAttr = regexp.MustCompile(`candidates="(\d+)"`)
	filesAttr      = regexp.MustCompile(`<results[^>]*files="(\d+)"`)
	hintText       = regexp.MustCompile(`<hint>([^<]{0,60})`)
)

// resultsText returns the <results> body of a graphin tool response.
func resultsText(resp json.RawMessage) (string, bool) {
	if len(resp) == 0 {
		return "", false
	}
	var v any
	if json.Unmarshal(resp, &v) != nil {
		return "", false
	}
	var texts []string
	collectStrings(v, &texts, 0)
	for _, t := range texts {
		if strings.Contains(t, "<results") {
			return t, true
		}
	}
	return "", false
}

func responseAttrCounts(resp json.RawMessage, re *regexp.Regexp) map[string]int {
	t, ok := resultsText(resp)
	if !ok {
		return nil
	}
	out := map[string]int{}
	for _, m := range re.FindAllStringSubmatch(t, -1) {
		out[m[1]]++
	}
	return out
}

func responseAttrInt(resp json.RawMessage, re *regexp.Regexp) (int, bool) {
	t, ok := resultsText(resp)
	if !ok {
		return 0, false
	}
	m := re.FindStringSubmatch(t)
	if m == nil {
		return 0, false
	}
	n, err := strconv.Atoi(m[1])
	return n, err == nil
}

// responseHintKind records that the search redirected the caller, and which
// way, without storing the sentence. The kinds are the loop's branch points:
// a name that is not here, a name that is not a symbol, a query too broad to
// have decided anything.
func responseHintKind(resp json.RawMessage) (string, bool) {
	t, ok := resultsText(resp)
	if !ok {
		return "", false
	}
	m := hintText.FindStringSubmatch(t)
	if m == nil {
		return "", false
	}
	switch h := m[1]; {
	case strings.Contains(h, "no indexed symbol is named it"):
		return "unnamed_ident", true
	case strings.Contains(h, "no indexed symbol spells"):
		return "absent_ident", true
	case strings.Contains(h, "touched"):
		return "broad_query", true
	case strings.Contains(h, "no ranked result"):
		return "empty", true
	}
	return "other", true
}

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
	return classifyBashDepth(cmd, 0)
}

// shells whose -c argument is itself a command line: `bash -c 'cd x && rg y'`
// runs rg although the whole script is one quoted word.
var shells = map[string]bool{"bash": true, "sh": true, "zsh": true, "dash": true}

func classifyBashDepth(cmd string, depth int) (bool, string) {
	for _, seg := range splitSegments(cmd) {
		argv := fields(seg)
		if len(argv) == 0 {
			continue
		}
		if depth < 2 {
			if script, ok := shellScript(argv); ok {
				if s, p := classifyBashDepth(script, depth+1); s {
					return s, p
				}
				continue
			}
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

// shellScript finds a `<shell> -c <script>` anywhere in argv — directly, or
// behind a wrapper (`timeout 60 bash -c …`, `job.sh run x -- bash -c …`) —
// and returns the script. Combined flags count when they end in c (-lc, -ec).
func shellScript(argv []string) (string, bool) {
	for k := 0; k+2 < len(argv); k++ {
		if !shells[filepath.Base(argv[k])] {
			continue
		}
		f := argv[k+1]
		if len(f) >= 2 && f[0] == '-' && f[1] != '-' && strings.HasSuffix(f, "c") {
			return argv[k+2], true
		}
	}
	return "", false
}

// splitSegments cuts a command line into the simple commands the shell would
// run: at list and pipeline operators (&&, ||, |, ;, newline) outside quotes
// and not behind a backslash, and around each $(...) — a command
// substitution is a command of its own even inside double quotes
// (`echo "n: $(rg -o x | wc -l)"` runs rg). Heredoc bodies are dropped: they
// are input to a program (`python3 - <<'EOF'`), not commands. Segments keep
// their quotes and escapes for fields to resolve.
//
// It used to split on the raw text first. That cut `grep "a\|b"` inside its
// quotes, so the event kept only the first alternative with a stray backslash
// — and more than half of real Bash searches carry a `|` in the pattern
// (docs/usage-spec.md §3).
func splitSegments(cmd string) []string {
	var segs []string
	var cur strings.Builder
	cut := func() {
		segs = append(segs, cur.String())
		cur.Reset()
	}
	// One frame per open $(: the quote state it interrupted, and how many
	// plain parentheses are open inside it so that only its own ) closes it.
	type frame struct {
		quote  rune
		parens int
	}
	var stack []frame
	var quote rune
	var heredocs []string // delimiters whose bodies start at the next newline
	rs := []rune(cmd)
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		switch {
		case quote == '\'': // single quotes: no escapes, only the closing quote
			if r == '\'' {
				quote = 0
			}
		case r == '\\':
			cur.WriteRune(r)
			if i+1 < len(rs) {
				i++
				r = rs[i]
			}
		case r == '$' && i+1 < len(rs) && rs[i+1] == '(':
			cut()
			stack = append(stack, frame{quote: quote})
			quote = 0
			i++
			continue
		case quote == '"':
			if r == '"' {
				quote = 0
			}
		case r == '\'' || r == '"':
			quote = r
		case r == '(' && len(stack) > 0:
			stack[len(stack)-1].parens++
		case r == ')' && len(stack) > 0:
			top := &stack[len(stack)-1]
			if top.parens > 0 {
				top.parens--
				break
			}
			cut()
			quote = top.quote
			stack = stack[:len(stack)-1]
			continue
		case r == '<' && i+1 < len(rs) && rs[i+1] == '<' && (i+2 >= len(rs) || rs[i+2] != '<'):
			if delim, end := heredocDelim(rs, i+2); delim != "" {
				heredocs = append(heredocs, delim)
				cur.WriteString(string(rs[i:end]))
				i = end - 1
				continue
			}
		case r == '\n' && len(heredocs) > 0:
			cut()
			i = skipHeredocs(rs, i+1, heredocs) - 1
			heredocs = nil
			continue
		case r == '|' || r == ';' || r == '\n' || r == '&' && i+1 < len(rs) && rs[i+1] == '&':
			cut()
			if (r == '|' || r == '&') && i+1 < len(rs) && rs[i+1] == r {
				i++ // the second rune of || or &&
			}
			continue
		}
		cur.WriteRune(r)
	}
	cut()
	return segs
}

// heredocDelim reads the delimiter word of a heredoc whose `<<` ends just
// before rs[i]: an optional `-`, blanks, then a bare, 'quoted' or "quoted"
// word. It returns the word and the index just past it, or "" when no word
// follows.
func heredocDelim(rs []rune, i int) (string, int) {
	if i < len(rs) && rs[i] == '-' {
		i++
	}
	for i < len(rs) && (rs[i] == ' ' || rs[i] == '\t') {
		i++
	}
	var w strings.Builder
	var quote rune
	for ; i < len(rs); i++ {
		r := rs[i]
		if quote != 0 {
			if r == quote {
				quote = 0
			} else {
				w.WriteRune(r)
			}
			continue
		}
		if r == '\'' || r == '"' {
			quote = r
			continue
		}
		if r == ' ' || r == '\t' || r == '\n' || strings.ContainsRune("|&;<>()", r) {
			break
		}
		w.WriteRune(r)
	}
	return w.String(), i
}

// skipHeredocs returns the index just past the bodies of the given heredocs,
// which start at rs[i] and follow one another; each ends at a line that is
// its delimiter (leading tabs allowed, for <<-). An unterminated body runs to
// the end of the command, as in the shell.
func skipHeredocs(rs []rune, i int, delims []string) int {
	for _, d := range delims {
		for i < len(rs) {
			j := i
			for j < len(rs) && rs[j] != '\n' {
				j++
			}
			line := strings.TrimLeft(string(rs[i:j]), "\t")
			i = j + 1
			if line == d {
				break
			}
		}
	}
	if i > len(rs) {
		return len(rs)
	}
	return i
}

// fields is a shell-aware-enough tokenizer: whitespace split, quotes grouping
// and stripping, and backslashes resolved the way the shell resolves them —
// so the pattern is the argument grep actually received. Outside quotes a
// backslash makes the next rune literal; inside double quotes it does so only
// for " \ $ and `, and stays otherwise (`"a\|b"` reaches grep as `a\|b`);
// inside single quotes it is an ordinary rune. No expansion.
func fields(s string) []string {
	var out []string
	var cur strings.Builder
	var quote rune
	started := false // a quoted empty string ("") is still an argument
	flush := func() {
		if started {
			out = append(out, cur.String())
			cur.Reset()
			started = false
		}
	}
	rs := []rune(s)
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		switch {
		case quote == '\'':
			if r == '\'' {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case quote == '"':
			switch {
			case r == '"':
				quote = 0
			case r == '\\' && i+1 < len(rs) && strings.ContainsRune("\"\\$`", rs[i+1]):
				i++
				cur.WriteRune(rs[i])
			default:
				cur.WriteRune(r)
			}
		case r == '\\':
			started = true
			if i+1 < len(rs) {
				i++
				cur.WriteRune(rs[i])
			}
		case r == '\'' || r == '"':
			quote = r
			started = true
		case r == ' ' || r == '\t':
			flush()
		default:
			cur.WriteRune(r)
			started = true
		}
	}
	flush()
	return out
}
