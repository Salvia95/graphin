package merkle

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Salvia95/graphin/internal/parse"
)

const javaFixture = "../../testdata/fixtures/java/src/main/java/com/example/order/domain/OrderService.java"

func parseSrc(t *testing.T, rel string, src []byte) *parse.FileResult {
	t.Helper()
	res, err := parse.File(rel, src)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

type mockEmbedder struct{ calls []string }

func (m *mockEmbedder) EmbedNode(n parse.Node) { m.calls = append(m.calls, n.ID) }

type mockEdgeSink struct {
	updated []string
	removed []string
}

func (m *mockEdgeSink) UpdateEdges(n parse.Node) { m.updated = append(m.updated, n.ID) }
func (m *mockEdgeSink) RemoveNode(id string)     { m.removed = append(m.removed, id) }

// TestTrackA_OffsetRefreshAfterTopImportInsert is the spec's core regression
// test (§7-P2-①): adding one import line shifts every byte offset; unchanged
// methods must land in OffsetOnly with offsets that still slice the exact
// original text.
func TestTrackA_OffsetRefreshAfterTopImportInsert(t *testing.T) {
	rel := "src/OrderService.java"
	src, err := os.ReadFile(filepath.Join(javaFixture))
	if err != nil {
		t.Fatal(err)
	}

	tree := NewTree()
	first := parseSrc(t, rel, src)
	tree.Update(first)

	// Insert an import right after the package line.
	insert := "import java.util.ArrayList;\n"
	idx := strings.Index(string(src), "import com.example.common.Logger;")
	if idx < 0 {
		t.Fatal("fixture layout changed")
	}
	edited := []byte(string(src[:idx]) + insert + string(src[idx:]))

	second := parseSrc(t, rel, edited)
	diff := Diff(tree, second)

	if len(diff.Changed) != 0 || len(diff.Removed) != 0 {
		t.Fatalf("import-only edit must not change any subtree: changed=%v removed=%v",
			nodeIDs(diff.Changed), diff.Removed)
	}
	if len(diff.OffsetOnly) == 0 {
		t.Fatal("expected OffsetOnly nodes")
	}

	target := "com.example.order.domain.OrderService.cancelPayment(long,String)"
	var before, after *parse.Node
	for i := range first.Nodes {
		if first.Nodes[i].ID == target {
			before = &first.Nodes[i]
		}
	}
	for i := range diff.OffsetOnly {
		if diff.OffsetOnly[i].ID == target {
			after = &diff.OffsetOnly[i]
		}
	}
	if before == nil || after == nil {
		t.Fatal("target method missing")
	}
	if after.StartByte == before.StartByte {
		t.Fatal("offsets did not shift — Track A refresh would be a no-op test")
	}
	oldText := string(src[before.StartByte:before.EndByte])
	newText := string(edited[after.StartByte:after.EndByte])
	if oldText != newText {
		t.Fatalf("refreshed offsets slice wrong text:\nold: %q\nnew: %q", oldText, newText)
	}
}

// TestTrackB_ZeroEmbedZeroEdgeCallsForUnchangedSubtrees proves §7-P2-②: mock
// consumers observe zero calls when nothing changed semantically.
func TestTrackB_ZeroEmbedZeroEdgeCallsForUnchangedSubtrees(t *testing.T) {
	rel := "src/OrderService.java"
	src, err := os.ReadFile(javaFixture)
	if err != nil {
		t.Fatal(err)
	}
	tree := NewTree()
	tree.Update(parseSrc(t, rel, src))

	edited := append([]byte("// a leading comment shifts every offset\n"), src...)
	diff := Diff(tree, parseSrc(t, rel, edited))

	emb, sink := &mockEmbedder{}, &mockEdgeSink{}
	Apply(diff, emb, sink)

	if len(emb.calls) != 0 {
		t.Fatalf("embedding invoked %d times for unchanged subtrees: %v", len(emb.calls), emb.calls)
	}
	if len(sink.updated) != 0 || len(sink.removed) != 0 {
		t.Fatalf("edge engine invoked for unchanged subtrees: %+v", sink)
	}
}

// TestChangedSubtreeTriggersBothTracks: editing one method re-runs Track B
// for exactly the touched subtrees (method + enclosing class span).
func TestChangedSubtreeTriggersBothTracks(t *testing.T) {
	rel := "src/OrderService.java"
	src, err := os.ReadFile(javaFixture)
	if err != nil {
		t.Fatal(err)
	}
	tree := NewTree()
	tree.Update(parseSrc(t, rel, src))

	edited := strings.Replace(string(src),
		`logger.info("cancel payment for " + orderId + ": " + reason);`,
		`logger.info("CANCELLING " + orderId);`, 1)
	if edited == string(src) {
		t.Fatal("fixture layout changed")
	}
	diff := Diff(tree, parseSrc(t, rel, []byte(edited)))

	emb := &mockEmbedder{}
	Apply(diff, emb, nil)

	wantChanged := map[string]bool{
		"com.example.order.domain.OrderService.cancelPayment(long,String)": true,
		"com.example.order.domain.OrderService":                            true, // enclosing span
	}
	if len(diff.Changed) != len(wantChanged) {
		t.Fatalf("changed = %v, want exactly %v", nodeIDs(diff.Changed), wantChanged)
	}
	for _, n := range diff.Changed {
		if !wantChanged[n.ID] {
			t.Fatalf("unexpected changed node %s", n.ID)
		}
	}
	if len(emb.calls) != len(wantChanged) {
		t.Fatalf("embedder calls = %v", emb.calls)
	}
}

func TestRemovedNodesReported(t *testing.T) {
	rel := "src/OrderService.java"
	src, err := os.ReadFile(javaFixture)
	if err != nil {
		t.Fatal(err)
	}
	tree := NewTree()
	tree.Update(parseSrc(t, rel, src))

	edited := strings.Replace(string(src),
		"    public void cancelPayment(long orderId, String reason) {\n"+
			"        logger.info(\"cancel payment for \" + orderId + \": \" + reason);\n"+
			"        paymentPort.refund(orderId);\n"+
			"    }\n\n", "", 1)
	if edited == string(src) {
		t.Fatal("fixture layout changed")
	}
	diff := Diff(tree, parseSrc(t, rel, []byte(edited)))

	found := false
	for _, id := range diff.Removed {
		if id == "com.example.order.domain.OrderService.cancelPayment(long,String)" {
			found = true
		}
	}
	if !found {
		t.Fatalf("deleted method not in Removed: %v", diff.Removed)
	}

	sink := &mockEdgeSink{}
	Apply(diff, nil, sink)
	if len(sink.removed) != len(diff.Removed) {
		t.Fatal("RemoveNode not called for all removed IDs")
	}
}

func nodeIDs(nodes []parse.Node) []string {
	out := make([]string, len(nodes))
	for i, n := range nodes {
		out[i] = n.ID
	}
	return out
}
