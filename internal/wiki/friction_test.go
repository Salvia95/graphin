package wiki

import (
	"os"
	"strings"
	"testing"
)

func TestFrictionRoundTrip(t *testing.T) {
	root := t.TempDir()
	AppendFriction(root, FrictionEvent{Kind: FrictionMiss, Task: "cut a release", Role: "all"})
	AppendFriction(root, FrictionEvent{Kind: FrictionHit, Sets: []string{"release"}})

	got := ReadFriction(root)
	if len(got) != 2 {
		t.Fatalf("read %d events", len(got))
	}
	if got[0].Kind != FrictionMiss || got[0].Task != "cut a release" {
		t.Fatalf("first = %+v", got[0])
	}
	if got[0].TS == "" {
		t.Error("events must be timestamped: the queue is read newest first")
	}
}

func TestFrictionSkipsBrokenLines(t *testing.T) {
	root := t.TempDir()
	AppendFriction(root, FrictionEvent{Kind: FrictionMiss, Task: "one"})
	f, err := os.OpenFile(FrictionPath(root), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("{not json\n")
	f.Close()
	AppendFriction(root, FrictionEvent{Kind: FrictionMiss, Task: "two"})

	// This log is diagnostic. Refusing to report because one line is broken
	// helps nobody.
	if got := ReadFriction(root); len(got) != 2 {
		t.Fatalf("read %d events, want the two good ones", len(got))
	}
}

func TestFrictionTruncatesWithoutSplittingARune(t *testing.T) {
	root := t.TempDir()
	long := strings.Repeat("가나다라마바사", 100)
	AppendFriction(root, FrictionEvent{Kind: FrictionMiss, Task: long})

	got := ReadFriction(root)
	if len(got) != 1 {
		t.Fatal("event lost")
	}
	if len(got[0].Task) > maxTaskLen {
		t.Errorf("task not truncated: %d bytes", len(got[0].Task))
	}
	for _, r := range got[0].Task {
		if r == '�' {
			t.Fatal("truncation split a UTF-8 rune")
		}
	}
}

func TestSummarizeBucketsByKind(t *testing.T) {
	events := []FrictionEvent{
		{Kind: FrictionMiss, TS: "2026-08-20T10:00:00Z", Task: "older"},
		{Kind: FrictionMiss, TS: "2026-08-21T10:00:00Z", Task: "newer"},
		{Kind: FrictionHit, Sets: []string{"release", "adoption"}},
		{Kind: FrictionResolve, Sets: []string{"release"}},
		{Kind: FrictionDrift, Node: "docs/a.md#x"},
		{Kind: FrictionDrift, Node: "docs/a.md#x"},
	}
	r := Summarize(events)

	// The generation queue reads newest first: what the work wanted most
	// recently is what is worth writing next.
	if len(r.Misses) != 2 || r.Misses[0].Task != "newer" {
		t.Fatalf("misses = %+v", r.Misses)
	}
	if r.Matched["release"] != 1 || r.Matched["adoption"] != 1 {
		t.Fatalf("matched = %v", r.Matched)
	}
	if r.Resolved["release"] != 1 {
		t.Fatalf("resolved = %v", r.Resolved)
	}
	if r.Drifted["docs/a.md#x"] != 2 {
		t.Fatalf("drifted = %v", r.Drifted)
	}
}

func TestUnreadFindsSetsNobodyOpens(t *testing.T) {
	events := []FrictionEvent{
		{Kind: FrictionHit, Sets: []string{"ignored"}},
		{Kind: FrictionHit, Sets: []string{"ignored"}},
		{Kind: FrictionHit, Sets: []string{"ignored"}},
		{Kind: FrictionHit, Sets: []string{"used"}},
		{Kind: FrictionHit, Sets: []string{"used"}},
		{Kind: FrictionHit, Sets: []string{"used"}},
		{Kind: FrictionResolve, Sets: []string{"used"}},
	}
	// A set offered on every delegation and opened by nobody is costing
	// attention and returning nothing — that is the demotion signal.
	got := Summarize(events).Unread()
	if len(got) != 1 || got[0] != "ignored" {
		t.Fatalf("Unread = %v", got)
	}
}

func TestUnreadIgnoresSmallSamples(t *testing.T) {
	events := []FrictionEvent{{Kind: FrictionHit, Sets: []string{"new"}}}
	// One unopened offer is not evidence of anything.
	if got := Summarize(events).Unread(); len(got) != 0 {
		t.Fatalf("Unread = %v", got)
	}
}
