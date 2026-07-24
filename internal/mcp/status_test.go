package mcp

import (
	"strings"
	"testing"
)

// embed_pending is surfaced only while embedding is in progress and semantic
// is not yet ready — so an agent sees advancement, not a hang (§7c 후속 ①).
func TestStatusEmbedPendingVisibility(t *testing.T) {
	cases := []struct {
		name    string
		st      Status
		wantSub string
		absent  bool
	}{
		{"warming with backlog",
			Status{State: "indexing", LexicalReady: true, EmbedPending: 1234},
			`embed_pending="1234"`, false},
		{"ready hides it",
			Status{State: "ready", LexicalReady: true, SemanticReady: true, EmbedPending: 5},
			`embed_pending`, true},
		{"idle backlog hidden",
			Status{State: "indexing", LexicalReady: true, EmbedPending: 0},
			`embed_pending`, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			xml := c.st.XML()
			has := strings.Contains(xml, c.wantSub)
			if c.absent && has {
				t.Fatalf("%q should be absent: %s", c.wantSub, xml)
			}
			if !c.absent && !has {
				t.Fatalf("%q missing: %s", c.wantSub, xml)
			}
		})
	}
}
