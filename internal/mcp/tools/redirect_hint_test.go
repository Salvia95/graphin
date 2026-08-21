package tools

import (
	"strings"
	"testing"

	"github.com/Salvia95/graphin/internal/graph"
)

func TestRedirectHintStaysQuietUntilItMatters(t *testing.T) {
	if got := redirectHint(graph.ReverseStats{Redirects: 0}, 100); got != nil {
		t.Fatalf("hint with no redirects: %v", got)
	}
	// A handful of renames in a large tree is the system working, not a
	// problem to report.
	if got := redirectHint(graph.ReverseStats{Redirects: 5}, 1000); got != nil {
		t.Fatalf("hint below the threshold: %v", got)
	}
}

func TestRedirectHintFiresAtTheCarryRatio(t *testing.T) {
	got := redirectHint(graph.ReverseStats{Redirects: 25}, 100)
	if len(got) != 1 {
		t.Fatalf("hint = %v", got)
	}
	// The hint has to say what to do about it, not just that a number is big.
	if !strings.Contains(got[0], "GC") {
		t.Fatalf("no action named: %q", got[0])
	}
}
