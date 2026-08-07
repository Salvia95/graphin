package usage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// logLine renders one event the way ingest writes it.
func logLine(t *testing.T, ts, session, tool string) string {
	t.Helper()
	b, err := json.Marshal(Event{V: 1, TS: ts, SessionID: session, Tool: tool})
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// logDir writes files[name] = lines and returns the directory.
func logDir(t *testing.T, files map[string][]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, lines := range files {
		body := ""
		for _, l := range lines {
			body += l + "\n"
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func mustCutoff(t *testing.T, s string) time.Time {
	t.Helper()
	c, err := parseSince(s, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestPruneRemovesFullyStaleRotatedFile(t *testing.T) {
	dir := logDir(t, map[string][]string{
		"events-20260701T000000Z.jsonl": {
			logLine(t, "2026-06-30T10:00:00Z", "s1", "Grep"),
			logLine(t, "2026-06-30T11:00:00Z", "s1", "Read"),
		},
		"events.jsonl": {logLine(t, "2026-08-05T10:00:00Z", "s2", "Grep")},
	})
	res, err := Prune(dir, mustCutoff(t, "2026-07-01"), false)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Removed) != 1 || res.Dropped != 2 || res.Kept != 0 {
		t.Fatalf("res = %+v", res)
	}
	if _, err := os.Stat(filepath.Join(dir, "events-20260701T000000Z.jsonl")); !os.IsNotExist(err) {
		t.Fatal("stale rotated file survived")
	}
	// The active file held nothing stale, so it must not have been touched.
	if res.Rotated != "" {
		t.Fatalf("active file was rotated for nothing: %+v", res)
	}
	if _, err := os.Stat(filepath.Join(dir, "events.jsonl")); err != nil {
		t.Fatalf("active file went missing: %v", err)
	}
}

func TestPruneRewritesPartiallyStaleFile(t *testing.T) {
	dir := logDir(t, map[string][]string{
		"events-20260701T000000Z.jsonl": {
			logLine(t, "2026-06-30T10:00:00Z", "s1", "Grep"),
			logLine(t, "2026-07-02T10:00:00Z", "s1", "Read"),
		},
	})
	res, err := Prune(dir, mustCutoff(t, "2026-07-01"), false)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Rewritten) != 1 || res.Kept != 1 || res.Dropped != 1 {
		t.Fatalf("res = %+v", res)
	}
	b, err := os.ReadFile(filepath.Join(dir, "events-20260701T000000Z.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "2026-06-30") || !strings.Contains(string(b), "2026-07-02") {
		t.Fatalf("wrong survivors:\n%s", b)
	}
	// Whatever survives must still load as events.
	events, problems, err := Load(dir)
	if err != nil || len(events) != 1 || len(problems) != 0 {
		t.Fatalf("reload: %d events, %v problems, err %v", len(events), problems, err)
	}
}

// The active log is rotated, not filtered in place — filtering it would race
// the O_APPEND writers that hooks/usage.sh runs on every tool call.
func TestPruneRotatesTheActiveLogBeforeFiltering(t *testing.T) {
	dir := logDir(t, map[string][]string{
		"events.jsonl": {
			logLine(t, "2026-06-30T10:00:00Z", "s1", "Grep"),
			logLine(t, "2026-08-05T10:00:00Z", "s1", "Read"),
		},
	})
	res, err := Prune(dir, mustCutoff(t, "2026-07-01"), false)
	if err != nil {
		t.Fatal(err)
	}
	if res.Rotated == "" {
		t.Fatalf("active log was not rotated: %+v", res)
	}
	if _, err := os.Stat(filepath.Join(dir, "events.jsonl")); !os.IsNotExist(err) {
		t.Fatal("events.jsonl should be gone; the next append recreates it")
	}
	if res.Kept != 1 || res.Dropped != 1 {
		t.Fatalf("res = %+v", res)
	}
	events, _, err := Load(dir)
	if err != nil || len(events) != 1 || events[0].TS != "2026-08-05T10:00:00Z" {
		t.Fatalf("events = %+v (err %v)", events, err)
	}
}

func TestPruneDryRunChangesNothingButCounts(t *testing.T) {
	dir := logDir(t, map[string][]string{
		"events.jsonl": {
			logLine(t, "2026-06-30T10:00:00Z", "s1", "Grep"),
			logLine(t, "2026-08-05T10:00:00Z", "s1", "Read"),
		},
		"events-20260601T000000Z.jsonl": {logLine(t, "2026-05-01T10:00:00Z", "s0", "Grep")},
	})
	before, err := os.ReadFile(filepath.Join(dir, "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	res, err := Prune(dir, mustCutoff(t, "2026-07-01"), true)
	if err != nil {
		t.Fatal(err)
	}
	// Dry run must still account for the active file even though it was not
	// rotated — it is the file the user most wants previewed.
	if res.Dropped != 2 || res.Kept != 1 {
		t.Fatalf("res = %+v", res)
	}
	if len(res.Removed) != 1 || res.Rotated == "" {
		t.Fatalf("dry run did not preview the plan: %+v", res)
	}
	// The preview names the active file as what it will become, not as what
	// it is now — one file, one operation.
	if len(res.Rewritten) != 1 || res.Rewritten[0] != res.Rotated {
		t.Fatalf("preview names the wrong file: rewritten=%v rotated=%s", res.Rewritten, res.Rotated)
	}
	after, err := os.ReadFile(filepath.Join(dir, "events.jsonl"))
	if err != nil || string(after) != string(before) {
		t.Fatalf("dry run modified the active log")
	}
	if _, err := os.Stat(filepath.Join(dir, "events-20260601T000000Z.jsonl")); err != nil {
		t.Fatal("dry run deleted a file")
	}
}

// A line with no usable timestamp cannot be shown to be recent, so a prune
// drops it — and says how many, rather than letting the count drift from what
// a report would have loaded.
func TestPruneDropsUndatableLines(t *testing.T) {
	dir := logDir(t, map[string][]string{
		"events-20260701T000000Z.jsonl": {
			`{"v":1,"tool":"Grep"}`,
			"not json at all",
			logLine(t, "2026-08-05T10:00:00Z", "s1", "Read"),
		},
	})
	res, err := Prune(dir, mustCutoff(t, "2026-07-01"), false)
	if err != nil {
		t.Fatal(err)
	}
	if res.Malformed != 2 || res.Kept != 1 || res.Dropped != 0 {
		t.Fatalf("res = %+v", res)
	}
	if len(res.Rewritten) != 1 {
		t.Fatalf("file should have been rewritten: %+v", res)
	}
}

func TestPruneNoOpWhenEverythingIsRecent(t *testing.T) {
	dir := logDir(t, map[string][]string{
		"events.jsonl": {logLine(t, "2026-08-05T10:00:00Z", "s1", "Read")},
	})
	res, err := Prune(dir, mustCutoff(t, "2026-07-01"), false)
	if err != nil {
		t.Fatal(err)
	}
	if res.Rotated != "" || len(res.Removed) != 0 || len(res.Rewritten) != 0 || res.Dropped != 0 {
		t.Fatalf("prune churned a log it had no work in: %+v", res)
	}
}

func TestPruneRejectsAFile(t *testing.T) {
	dir := logDir(t, map[string][]string{"events.jsonl": {logLine(t, "2026-08-05T10:00:00Z", "s1", "Read")}})
	if _, err := Prune(filepath.Join(dir, "events.jsonl"), mustCutoff(t, "2026-07-01"), false); err == nil {
		t.Fatal("prune accepted a file path")
	}
}

func TestRunPruneRequiresBefore(t *testing.T) {
	var out, errb strings.Builder
	if code := Run([]string{"prune"}, nil, &out, &errb); code != 2 {
		t.Fatalf("exit = %d, stderr = %s", code, errb.String())
	}
	if !strings.Contains(errb.String(), "--before is required") {
		t.Fatalf("stderr = %s", errb.String())
	}
}

func TestRunPruneJSON(t *testing.T) {
	dir := logDir(t, map[string][]string{
		"events-20260701T000000Z.jsonl": {logLine(t, "2026-06-30T10:00:00Z", "s1", "Grep")},
	})
	var out, errb strings.Builder
	code := Run([]string{"prune", "--log", dir, "--before", "2026-07-01", "--json"}, nil, &out, &errb)
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %s", code, errb.String())
	}
	var res PruneResult
	if err := json.Unmarshal([]byte(out.String()), &res); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out.String())
	}
	if res.Dropped != 1 || len(res.Removed) != 1 {
		t.Fatalf("res = %+v", res)
	}
}
