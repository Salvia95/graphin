package wiki

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// fileReader serves node bodies straight off disk, standing in for the index.
type fileReader struct{ root string }

func (r fileReader) Read(ids []string) []Block {
	h := NewHasher(r.root)
	out := make([]Block, 0, len(ids))
	for _, id := range ids {
		if _, ok := h.Pin(id); !ok {
			out = append(out, Block{NodeID: id, Err: errors.New("node not found")})
			continue
		}
		out = append(out, Block{NodeID: id, Text: "body of " + id, StartLine: 1, EndLine: 2})
	}
	return out
}

func resolveFixture(t *testing.T, mode string) (*Store, fileReader, string) {
	t.Helper()
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "docs", "target.md"), targetDoc)
	fm := ""
	if mode != "" {
		fm = "mode: " + mode + "\n"
	}
	writeSet(t, root, "s", fm)

	store, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	pins, _ := Repin(root, store.SetList())
	if err := pins.Save(store.PinsPath()); err != nil {
		t.Fatal(err)
	}
	store, _ = Load(root)
	return store, fileReader{root}, root
}

func TestResolveServesContentAndNoDrift(t *testing.T) {
	store, r, _ := resolveFixture(t, "")
	res := store.Resolve(r, []string{"s"})

	if len(res.Sets) != 1 || len(res.Sets[0].Entries) != 1 {
		t.Fatalf("res = %+v", res)
	}
	e := res.Sets[0].Entries[0]
	if e.Drift != DriftNone {
		t.Errorf("Drift = %q, want none", e.Drift)
	}
	if e.Block.Text == "" {
		t.Error("expected content")
	}
	if len(res.Drifted()) != 0 {
		t.Errorf("Drifted = %v", res.Drifted())
	}
}

func TestResolveFlagsDriftButStillServes(t *testing.T) {
	store, r, root := resolveFixture(t, "")
	mustWrite(t, filepath.Join(root, "docs", "target.md"),
		"# Target\n\n## Section one\n\nRewritten.\n")

	res := store.Resolve(r, []string{"s"})
	e := res.Sets[0].Entries[0]
	if e.Drift != DriftChanged {
		t.Fatalf("Drift = %q, want changed", e.Drift)
	}
	// A set that refuses to answer is worse than one that answers with a
	// caveat: the reader can judge, but only if they get the text.
	if e.Block.Text == "" {
		t.Error("live mode must still serve the current content")
	}
	if len(res.Drifted()) != 1 {
		t.Errorf("Drifted = %v", res.Drifted())
	}
}

func TestResolvePinnedRefusesDriftedContent(t *testing.T) {
	store, r, root := resolveFixture(t, "pinned")
	mustWrite(t, filepath.Join(root, "docs", "target.md"),
		"# Target\n\n## Section one\n\nRewritten.\n")

	res := store.Resolve(r, []string{"s"})
	e := res.Sets[0].Entries[0]
	if e.Drift != DriftChanged {
		t.Fatalf("Drift = %q", e.Drift)
	}
	// Reproducibility is the entire value of a pinned set, so serving the
	// new text would destroy what the reader came for.
	if e.Block.Text != "" {
		t.Error("pinned mode must not serve rewritten content")
	}
	if !errors.Is(e.Block.Err, ErrPinnedDrift) {
		t.Errorf("Err = %v, want ErrPinnedDrift", e.Block.Err)
	}
}

func TestResolveMarksGoneAfterRename(t *testing.T) {
	store, r, root := resolveFixture(t, "")
	mustWrite(t, filepath.Join(root, "docs", "target.md"),
		"# Target\n\n## Section renamed\n\nSame body.\n")

	res := store.Resolve(r, []string{"s"})
	if got := res.Sets[0].Entries[0].Drift; got != DriftGone {
		t.Fatalf("Drift = %q, want gone", got)
	}
}

func TestResolveReportsUnknownSets(t *testing.T) {
	store, r, _ := resolveFixture(t, "")
	res := store.Resolve(r, []string{"s", "nope"})
	if len(res.Missing) != 1 || res.Missing[0] != "nope" {
		t.Fatalf("Missing = %v", res.Missing)
	}
	if len(res.Sets) != 1 {
		t.Fatalf("Sets = %d, want the real one still served", len(res.Sets))
	}
}

func TestResolveNodesFindsPinAcrossSets(t *testing.T) {
	store, r, _ := resolveFixture(t, "")
	got := store.ResolveNodes(r, []string{"docs/target.md#section-one"})
	if len(got) != 1 {
		t.Fatalf("got %d entries", len(got))
	}
	// The reader asked by id and did not say which set they came from, so a
	// set that still vouches for the content makes it current.
	if got[0].Drift != DriftNone {
		t.Errorf("Drift = %q, want none", got[0].Drift)
	}
}

func TestResolveNodesUnknownIsGone(t *testing.T) {
	store, r, _ := resolveFixture(t, "")
	got := store.ResolveNodes(r, []string{"docs/target.md#no-such-heading"})
	if got[0].Drift != DriftGone {
		t.Errorf("Drift = %q, want gone", got[0].Drift)
	}
}

func TestResolveUnpinnedIsSaidOutLoud(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "docs", "target.md"), targetDoc)
	writeSet(t, root, "s", "")
	store, _ := Load(root)
	if _, err := os.Stat(store.PinsPath()); !os.IsNotExist(err) {
		t.Fatal("fixture should have no lockfile")
	}

	res := store.Resolve(fileReader{root}, []string{"s"})
	// Silence here would read as "verified unchanged", which is the one
	// thing an unpinned entry cannot promise.
	if got := res.Sets[0].Entries[0].Drift; got != DriftUnpinned {
		t.Errorf("Drift = %q, want unpinned", got)
	}
}

// stubRedirect stands in for the index's redirect table.
type stubRedirect map[string]string

func (s stubRedirect) ResolveID(id string) string {
	if to, ok := s[id]; ok {
		return to
	}
	return id
}

func TestRetitleIsNotDrift(t *testing.T) {
	store, r, root := resolveFixture(t, "")
	// Only the heading moves; every byte of the body stays.
	mustWrite(t, filepath.Join(root, "docs", "target.md"),
		"# Target\n\n## Section one, restated\n\nOriginal body.\n")
	store.SetRedirector(stubRedirect{
		"docs/target.md#section-one": "docs/target.md#section-one-restated",
	})

	res := store.Resolve(r, []string{"s"})
	e := res.Sets[0].Entries[0]
	if e.RedirectedTo != "docs/target.md#section-one-restated" {
		t.Fatalf("RedirectedTo = %q", e.RedirectedTo)
	}
	// The summary describes what the section claims, and a retitle does not
	// change that. Flagging it would train readers to ignore the flag — and
	// the next real rewrite with it.
	if e.Drift != DriftNone {
		t.Fatalf("Drift = %q, want none for a pure retitle", e.Drift)
	}
	if e.Block.Text == "" {
		t.Error("a redirected entry must still serve its content")
	}
}

func TestRetitlePlusRewriteIsStillDrift(t *testing.T) {
	store, r, root := resolveFixture(t, "")
	mustWrite(t, filepath.Join(root, "docs", "target.md"),
		"# Target\n\n## Section one, restated\n\nAnd rewritten.\n")
	store.SetRedirector(stubRedirect{
		"docs/target.md#section-one": "docs/target.md#section-one-restated",
	})

	// Following a redirect must not become a way to launder a rewrite.
	if got := store.Resolve(r, []string{"s"}).Sets[0].Entries[0].Drift; got != DriftChanged {
		t.Fatalf("Drift = %q, want changed", got)
	}
}

func TestCheckStillReportsDanglingWithoutTheIndex(t *testing.T) {
	store, _, root := resolveFixture(t, "")
	mustWrite(t, filepath.Join(root, "docs", "target.md"),
		"# Target\n\n## Section one, restated\n\nOriginal body.\n")

	// The CLI check runs with no index and therefore no redirects. That is
	// deliberate: the redirect keeps readers working, but the set still links
	// to a heading that is gone, and a green check would let it rot there.
	probs := Check(root, store.SetList(), store.Pins)
	if len(probs) != 1 || probs[0].Kind != ProblemDangling {
		t.Fatalf("problems = %v, want one dangling", probs)
	}
}
