package tools

import (
	"context"
	"testing"
	"time"
)

func TestIsDataPathIsAboutExtensionOnly(t *testing.T) {
	for _, p := range []string{"docs/eval/x/scores.json", "go.sum", "pnpm-lock.lock", "data/rows.CSV", "a/b.jsonl", "db/main.graphindb.json"} {
		if !isDataPath(p) {
			t.Errorf("%s should read as data", p)
		}
	}
	for _, p := range []string{"internal/keyword/keyword.go", "docs/spec.md", "schema.sql", "prisma/schema.prisma", "package.json.md", "x.ts"} {
		if isDataPath(p) {
			t.Errorf("%s should not read as data", p)
		}
	}
}

func TestWaitSemanticReturnsOnReadyStopOrDeadline(t *testing.T) {
	ctx := context.Background()
	if !waitSemantic(ctx, time.Second, func() bool { return true }, func() bool { return false }) {
		t.Fatal("ready at once must return true")
	}
	if waitSemantic(ctx, time.Second, func() bool { return false }, func() bool { return true }) {
		t.Fatal("a permanent failure must not be waited out")
	}
	start := time.Now()
	if waitSemantic(ctx, 250*time.Millisecond, func() bool { return false }, func() bool { return false }) {
		t.Fatal("never-ready must time out false")
	}
	if el := time.Since(start); el < 200*time.Millisecond || el > 2*time.Second {
		t.Fatalf("deadline not honoured: %v", el)
	}
	// Readiness arriving mid-wait is picked up on the next poll.
	flip := time.Now().Add(150 * time.Millisecond)
	if !waitSemantic(ctx, 2*time.Second, func() bool { return time.Now().After(flip) }, func() bool { return false }) {
		t.Fatal("readiness during the wait must return true")
	}
}
