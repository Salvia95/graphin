package tools

import (
	"sort"
	"strings"
	"testing"

	"github.com/Salvia95/graphin/internal/mcp"
)

// The plugin loads graphin's tools up front (.mcp.json alwaysLoad), because a
// hidden tool is never chosen: with deferred loading the mixed arm used graphin
// in 0 of 108 runs, and alwaysLoad lifted adoption by 56%p without moving
// pass rate or cost (docs/effort-adoption-bench-design.md). Up front means
// paying for every schema on every turn, so only the navigation loop is there;
// everything else stays behind tool search. Adding a tool means deciding which
// side it is on — this list is that decision.
func TestOnlyTheNavigationLoopLoadsUpFront(t *testing.T) {
	reg := mcp.NewRegistry()
	Register(reg, nil)

	var upFront []string
	for _, tool := range reg.List() {
		if !tool.Deferred {
			upFront = append(upFront, tool.Name)
		}
	}
	sort.Strings(upFront)
	want := "bootstrap_workspace explore_graph read_code search_hybrid search_keyword"
	if got := strings.Join(upFront, " "); got != want {
		t.Fatalf("tools loaded up front:\n got  %s\n want %s", got, want)
	}
}
