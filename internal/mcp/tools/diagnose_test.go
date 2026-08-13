package tools

import (
	"strings"
	"testing"

	"github.com/Salvia95/graphin/internal/graph"
	"github.com/Salvia95/graphin/internal/provision"
	"github.com/Salvia95/graphin/internal/semantic"
	"github.com/Salvia95/graphin/internal/workspace"
)

func TestModelExpectationFlagsMixedIndex(t *testing.T) {
	spec, ok := provision.Models["multilingual_cjk"]
	if !ok {
		t.Fatal("multilingual_cjk missing from provision.Models")
	}
	cfg := workspace.ConfigView{ModelType: "multilingual_cjk"}

	if id, mismatch := modelExpectation(cfg, nil); id != spec.ID || mismatch {
		t.Errorf("no header: got (%q, %v), want (%q, false)", id, mismatch, spec.ID)
	}
	if _, mismatch := modelExpectation(cfg, &semantic.Header{ModelID: spec.ID}); mismatch {
		t.Error("matching header reported as a mismatch")
	}
	if _, mismatch := modelExpectation(cfg, &semantic.Header{ModelID: "some-other-model"}); !mismatch {
		t.Error("header from a different model not flagged")
	}
	// An unknown model type has no pinned ID to compare against, so it must
	// not manufacture a mismatch.
	if id, mismatch := modelExpectation(workspace.ConfigView{ModelType: "nope"},
		&semantic.Header{ModelID: "x"}); id != "" || mismatch {
		t.Errorf("unknown model type: got (%q, %v), want (\"\", false)", id, mismatch)
	}
}

func TestDiagHintsOnlyOnProblems(t *testing.T) {
	if h := diagHints(nil, "expected-id", false, graph.DanglingTotals{}, 0); len(h) != 0 {
		t.Errorf("healthy index produced hints: %v", h)
	}

	hdr := &semantic.Header{ModelID: "stored-id"}
	got := diagHints(hdr, "expected-id", true, graph.DanglingTotals{Code: 3, DB: 2}, 1)
	if len(got) != 4 {
		t.Fatalf("want a hint per problem (4), got %d: %v", len(got), got)
	}
	joined := strings.Join(got, "\n")
	for _, want := range []string{"stored-id", "expected-id", "vectors.bin", "3 code edges", "2 DB edges", "1 nodes"} {
		if !strings.Contains(joined, want) {
			t.Errorf("hints missing %q:\n%s", want, joined)
		}
	}
	// DB dangling must not be phrased as a defect — it dangles by design.
	if !strings.Contains(joined, "often intentional") {
		t.Error("DB dangling hint does not say it is often intentional")
	}
}

func TestWriteConfigMarksOnlyChangedValues(t *testing.T) {
	def := workspace.DefaultConfig()

	var sb strings.Builder
	writeConfig(&sb, workspace.ConfigView{
		Root: "/tmp/ws", ModelType: def.ModelType, Workers: def.Workers,
		SemanticMaxNodes: def.SemanticMaxNodes, Offline: def.Offline,
	})
	if got := sb.String(); strings.Contains(got, "_changed") {
		t.Errorf("all-default config reported a change:\n%s", got)
	}

	sb.Reset()
	writeConfig(&sb, workspace.ConfigView{
		Root: "/tmp/ws", ModelType: "english_optimal", Workers: def.Workers,
		SemanticMaxNodes: def.SemanticMaxNodes, Offline: true,
	})
	got := sb.String()
	for _, want := range []string{`model_type_changed="true"`, `offline_changed="true"`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %s in:\n%s", want, got)
		}
	}
	if strings.Contains(got, `workers_changed`) {
		t.Errorf("unchanged workers marked as changed:\n%s", got)
	}
}
