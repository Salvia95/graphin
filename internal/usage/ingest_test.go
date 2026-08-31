package usage

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mkWorkspace creates dir with the index marker and returns it.
func mkWorkspace(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".graphin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, markerRel), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func hookJSON(t *testing.T, fields map[string]any) io.Reader {
	t.Helper()
	base := map[string]any{
		"hook_event_name": "PostToolUse",
		"session_id":      "s1",
		"prompt_id":       "p1",
		"tool_use_id":     "tu1",
	}
	for k, v := range fields {
		base[k] = v
	}
	b, err := json.Marshal(base)
	if err != nil {
		t.Fatal(err)
	}
	return strings.NewReader(string(b))
}

func noEnv(string) string { return "" }

func readEvents(t *testing.T, root string) []Event {
	t.Helper()
	events, problems, err := Load(filepath.Join(root, ".graphin", "usage"))
	if err != nil {
		t.Fatal(err)
	}
	if len(problems) > 0 {
		t.Fatalf("problems: %v", problems)
	}
	return events
}

func TestIngestAppendsEventInMarkedWorkspace(t *testing.T) {
	root := mkWorkspace(t)
	in := hookJSON(t, map[string]any{
		"cwd": root, "tool_name": "Grep",
		"tool_input": map[string]any{"pattern": "persistIndexes", "path": filepath.Join(root, "internal")},
	})
	if code := Ingest(in, io.Discard, noEnv); code != 0 {
		t.Fatalf("exit %d", code)
	}
	evs := readEvents(t, root)
	if len(evs) != 1 {
		t.Fatalf("got %d events", len(evs))
	}
	ev := evs[0]
	if ev.V != 1 || ev.SessionID != "s1" || ev.PromptID != "p1" || ev.Tool != "Grep" {
		t.Fatalf("event = %+v", ev)
	}
	if ev.P["pattern"] != "persistIndexes" || ev.P["path"] != "internal" {
		t.Fatalf("payload = %v", ev.P)
	}
}

func TestIngestRoutesToNearestRootViaWalkUp(t *testing.T) {
	root := mkWorkspace(t)
	sub := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	in := hookJSON(t, map[string]any{"cwd": sub, "tool_name": "Read",
		"tool_input": map[string]any{"file_path": filepath.Join(sub, "x.go")}})
	Ingest(in, io.Discard, noEnv)
	evs := readEvents(t, root)
	if len(evs) != 1 || evs[0].CWD != filepath.Join("a", "b") {
		t.Fatalf("events = %+v", evs)
	}
	if evs[0].P["file_path"] != filepath.Join("a", "b", "x.go") {
		t.Fatalf("payload = %v", evs[0].P)
	}
}

func TestIngestNoMarkerNoEnvWritesNothing(t *testing.T) {
	dir := t.TempDir()
	in := hookJSON(t, map[string]any{"cwd": dir, "tool_name": "Grep",
		"tool_input": map[string]any{"pattern": "x"}})
	if code := Ingest(in, io.Discard, noEnv); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if _, err := os.Stat(filepath.Join(dir, ".graphin")); !os.IsNotExist(err) {
		t.Fatal(".graphin must not be created in unindexed projects")
	}
}

func TestIngestFallsBackToEnvRoot(t *testing.T) {
	root := mkWorkspace(t)
	elsewhere := t.TempDir()
	in := hookJSON(t, map[string]any{"cwd": elsewhere, "tool_name": "Glob",
		"tool_input": map[string]any{"pattern": "**/*.go"}})
	env := func(k string) string {
		if k == "GRAPHIN_WS_ROOT" {
			return root
		}
		return ""
	}
	Ingest(in, io.Discard, env)
	if evs := readEvents(t, root); len(evs) != 1 || evs[0].Tool != "Glob" {
		t.Fatalf("events = %+v", evs)
	}
}

func TestIngestIgnoresMalformedAndForeignEvents(t *testing.T) {
	root := mkWorkspace(t)
	if code := Ingest(strings.NewReader("not json"), io.Discard, noEnv); code != 0 {
		t.Fatal("malformed stdin must still exit 0")
	}
	in := hookJSON(t, map[string]any{"cwd": root, "tool_name": "Grep",
		"hook_event_name": "PreToolUse"})
	Ingest(in, io.Discard, noEnv)
	if _, err := os.Stat(filepath.Join(root, ".graphin", "usage")); !os.IsNotExist(err) {
		t.Fatal("nothing should be written for non-PostToolUse or bad input")
	}
}

func TestIngestSearchHybridExtractsResultIDs(t *testing.T) {
	root := mkWorkspace(t)
	xml := `<results semantic_ready="true">` +
		`<node id="a.B#m" display_name="B.m" rank="1" match_type="lexical" />` +
		`<node id="c.D" display_name="D" rank="2" match_type="semantic" />` +
		`</results>`
	in := hookJSON(t, map[string]any{
		"cwd": root, "tool_name": "mcp__mykey__search_hybrid",
		"tool_input":    map[string]any{"query": "order cancel", "top_k": float64(5)},
		"tool_response": map[string]any{"content": []any{map[string]any{"type": "text", "text": xml}}},
	})
	Ingest(in, io.Discard, noEnv)
	evs := readEvents(t, root)
	if len(evs) != 1 {
		t.Fatalf("got %d events", len(evs))
	}
	p := evs[0].P
	if p["query"] != "order cancel" || p["result_count"] != float64(2) {
		t.Fatalf("payload = %v", p)
	}
	ids, _ := p["result_ids"].([]any)
	if len(ids) != 2 || ids[0] != "a.B#m" {
		t.Fatalf("result_ids = %v", p["result_ids"])
	}
}

func TestIngestSearchHybridRecordsTargetOnlyWhenSet(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input map[string]any
		want  any
	}{
		{"filtered", map[string]any{"query": "how the lock is released", "target": "code"}, "code"},
		{"unfiltered", map[string]any{"query": "how the lock is released"}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := mkWorkspace(t)
			in := hookJSON(t, map[string]any{
				"cwd": root, "tool_name": "mcp__mykey__search_hybrid",
				"tool_input":    tc.input,
				"tool_response": map[string]any{"text": `<results semantic_ready="true"></results>`},
			})
			Ingest(in, io.Discard, noEnv)
			evs := readEvents(t, root)
			if len(evs) != 1 {
				t.Fatalf("got %d events", len(evs))
			}
			if got := evs[0].P["target"]; got != tc.want {
				t.Fatalf("target = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestIngestBashSearchKeepsPatternNotCommand(t *testing.T) {
	root := mkWorkspace(t)
	in := hookJSON(t, map[string]any{
		"cwd": root, "tool_name": "Bash",
		"tool_input": map[string]any{"command": `rg -A 3 'tokenLimit' internal/ | head -20`},
	})
	Ingest(in, io.Discard, noEnv)
	evs := readEvents(t, root)
	p := evs[0].P
	if p["search"] != true || p["pattern"] != "tokenLimit" {
		t.Fatalf("payload = %v", p)
	}
	raw, err := os.ReadFile(filepath.Join(root, ".graphin", "usage", "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "head -20") || strings.Contains(string(raw), "internal/") {
		t.Fatalf("full command leaked into log: %s", raw)
	}
}

func TestIngestOversizedPayloadDropped(t *testing.T) {
	root := mkWorkspace(t)
	// 5 ids x 2000 chars pushes the marshaled line past maxLine, forcing the
	// payload-drop re-marshal.
	in := hookJSON(t, map[string]any{
		"cwd": root, "tool_name": "mcp__k__search_hybrid",
		"tool_input":    map[string]any{"query": "q"},
		"tool_response": map[string]any{"text": "<results>" + strings.Repeat(`<node id="`+strings.Repeat("x", 2000)+`" `, 5) + "</results>"},
	})
	Ingest(in, io.Discard, noEnv)
	evs := readEvents(t, root)
	if len(evs) != 1 {
		t.Fatalf("got %d events", len(evs))
	}
	if evs[0].P != nil {
		t.Fatalf("oversized payload should be dropped, got %d keys", len(evs[0].P))
	}
	raw, _ := os.ReadFile(filepath.Join(root, ".graphin", "usage", "events.jsonl"))
	if len(raw) > maxLine+1 {
		t.Fatalf("line not capped: %d bytes", len(raw))
	}
}

func TestClassifyBashVariants(t *testing.T) {
	cases := []struct {
		cmd     string
		search  bool
		pattern string
	}{
		{`grep -rn "drainSignal" internal/`, true, "drainSignal"},
		{`git grep -e explore_graph`, true, "explore_graph"},
		{`find . -name '*.fbs'`, true, "*.fbs"},
		{`ls -la && rg confidence`, true, "confidence"},
		{`go test ./...`, false, ""},
		{`echo "grep is not run here"`, false, ""},
		{`/usr/bin/rg foo`, true, "foo"},
	}
	for _, tc := range cases {
		search, pattern := classifyBash(tc.cmd)
		if search != tc.search || pattern != tc.pattern {
			t.Fatalf("classifyBash(%q) = (%v, %q), want (%v, %q)",
				tc.cmd, search, pattern, tc.search, tc.pattern)
		}
	}
}

func TestIngestRotationAtSizeCap(t *testing.T) {
	root := mkWorkspace(t)
	dir := filepath.Join(root, ".graphin", "usage")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	big, err := os.Create(filepath.Join(dir, "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if err := big.Truncate(rotateBytes + 1); err != nil {
		t.Fatal(err)
	}
	big.Close()

	in := hookJSON(t, map[string]any{"cwd": root, "tool_name": "Grep",
		"tool_input": map[string]any{"pattern": "x"}})
	Ingest(in, io.Discard, noEnv)

	rotated, err := filepath.Glob(filepath.Join(dir, "events-*.jsonl"))
	if err != nil || len(rotated) != 1 {
		t.Fatalf("rotated files = %v (err %v)", rotated, err)
	}
	fi, err := os.Stat(filepath.Join(dir, "events.jsonl"))
	if err != nil || fi.Size() > maxLine {
		t.Fatalf("fresh active file expected, got size %v err %v", fi, err)
	}
}

func TestFindRootBounded(t *testing.T) {
	root := mkWorkspace(t)
	deep := root
	for i := 0; i < maxWalkUp+2; i++ {
		deep = filepath.Join(deep, fmt.Sprintf("d%d", i))
	}
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := findRoot(deep); got != "" {
		t.Fatalf("walk-up beyond bound should fail, got %q", got)
	}
	near := filepath.Join(root, "a")
	if err := os.MkdirAll(near, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := findRoot(near); got != root {
		t.Fatalf("findRoot(%q) = %q, want %q", near, got, root)
	}
}
