package usage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeLog(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadValidAndTornLines(t *testing.T) {
	dir := t.TempDir()
	writeLog(t, dir, "events.jsonl",
		`{"v":1,"ts":"2026-08-02T10:00:00Z","session_id":"s1","tool":"Grep"}`+"\n"+
			"\n"+ // empty line: skipped silently
			`{"v":1,"ts":"2026-08-02T10:00:01Z","session_id":"s1","tool":"Read"}`+"\n"+
			`{"v":1,"ts":"2026-08-02T10:00:02Z","session_id":"s1","to`) // torn tail

	events, problems, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("got %d events, want 2", len(events))
	}
	if len(problems) != 1 || !strings.Contains(problems[0], "invalid JSON") {
		t.Fatalf("problems = %v, want one invalid-JSON entry", problems)
	}
}

func TestLoadSkipsFutureSchemaVersion(t *testing.T) {
	dir := t.TempDir()
	writeLog(t, dir, "events.jsonl",
		`{"v":2,"ts":"t","session_id":"s1","tool":"Grep"}`+"\n"+
			`{"v":1,"ts":"t","session_id":"s1","tool":"Read"}`+"\n")
	events, problems, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Tool != "Read" {
		t.Fatalf("events = %+v, want only the v1 line", events)
	}
	if len(problems) != 1 || !strings.Contains(problems[0], "schema v2") {
		t.Fatalf("problems = %v", problems)
	}
}

func TestLoadRotatedFilesActiveLast(t *testing.T) {
	dir := t.TempDir()
	// Rotated files must be read before the active file regardless of how
	// their names happen to sort.
	writeLog(t, dir, "events-20260801T000000Z.jsonl",
		`{"v":1,"ts":"t","session_id":"s1","tool":"Grep"}`+"\n")
	writeLog(t, dir, "events.jsonl",
		`{"v":1,"ts":"t","session_id":"s1","tool":"Read"}`+"\n")
	events, _, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0].Tool != "Grep" || events[1].Tool != "Read" {
		t.Fatalf("rotation order wrong: %+v", events)
	}
}

func TestLoadSingleFilePath(t *testing.T) {
	dir := t.TempDir()
	p := writeLog(t, dir, "sample.jsonl",
		`{"v":1,"ts":"t","session_id":"s1","tool":"Grep","p":{"pattern":"foo"}}`+"\n")
	events, _, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].P["pattern"] != "foo" {
		t.Fatalf("events = %+v", events)
	}
}

func TestLoadMissingPathErrors(t *testing.T) {
	if _, _, err := Load(filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Fatal("want error for missing path")
	}
}
