package graph

import (
	"encoding/binary"
	"hash/crc32"
	"math"
	"testing"

	"github.com/Salvia95/graphin/internal/merkle"
	"github.com/Salvia95/graphin/internal/nodeid"
	"github.com/Salvia95/graphin/internal/obs"
	"github.com/Salvia95/graphin/internal/parse"
)

// sectionWithBody models a real markdown section: Hash covers the heading
// line, so retitling changes it, while RenameKey covers only the body and
// survives. A fixture that shared Hash across a rename would test something
// the parser never produces.
func sectionWithBody(id, display string, body [32]byte) parse.Node {
	return parse.Node{
		ID: id, DisplayName: display, SimpleName: display,
		Kind:      nodeid.KindSection,
		Hash:      merkle.Sum([]byte(id + "|" + display)),
		RenameKey: body,
	}
}

func TestRedirectRecordRoundTrip(t *testing.T) {
	want := record{Op: opRedirect, TargetID: "docs/a.md#old", SourceID: "docs/a.md#new", Epoch: 1755600000}
	buf := encodeRecord(nil, want)

	got, n, ok := decodeRecord(buf)
	if !ok {
		t.Fatal("redirect record did not decode")
	}
	if n != len(buf) {
		t.Errorf("consumed %d of %d bytes", n, len(buf))
	}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

// TestEdgeRecordBytesAreUnchanged pins the backward-compatibility claim: a log
// written before redirects existed must replay byte for byte, which is what
// lets this ship without a format version.
func TestEdgeRecordBytesAreUnchanged(t *testing.T) {
	// The pre-redirect layout, spelled out rather than produced by the code
	// under test — otherwise the test would agree with any change to it.
	var old []byte
	old = append(old, opUpsert)
	old = binary.LittleEndian.AppendUint32(old, uint32(len("target")))
	old = append(old, "target"...)
	old = binary.LittleEndian.AppendUint32(old, uint32(len("source")))
	old = append(old, "source"...)
	old = append(old, byte(EdgeCall))
	old = binary.LittleEndian.AppendUint32(old, math.Float32bits(0.95))
	old = binary.LittleEndian.AppendUint32(old, crc32.ChecksumIEEE(old))

	got, n, ok := decodeRecord(old)
	if !ok || n != len(old) {
		t.Fatalf("pre-redirect record did not decode: ok=%v n=%d len=%d", ok, n, len(old))
	}
	if got.TargetID != "target" || got.SourceID != "source" || got.Type != EdgeCall {
		t.Fatalf("decoded %+v", got)
	}
	if got.Epoch != 0 {
		t.Errorf("Epoch = %d, want 0 for an edge record", got.Epoch)
	}

	fresh := encodeRecord(nil, record{
		Op: opUpsert, TargetID: "target", SourceID: "source",
		Type: EdgeCall, Confidence: 0.95,
	})
	if string(fresh) != string(old) {
		t.Error("edge records no longer encode to the pre-redirect bytes")
	}
}

func TestRedirectStaysOutOfUsedBy(t *testing.T) {
	e := newEngine(t)
	e.rev.Upsert("docs/a.md#new", "caller", EdgeReference, 1.0)
	e.rev.Redirect("docs/a.md#old", "docs/a.md#new", 100)

	// UsedBySources queries with minConf 0 and feeds invalidation, so a
	// redirect leaking into the edge map would surface as a phantom caller.
	if got := e.UsedBySources("docs/a.md#new"); len(got) != 1 || got[0] != "caller" {
		t.Fatalf("UsedBySources = %v, want just the real caller", got)
	}
	if got := e.UsedBySources("docs/a.md#old"); len(got) != 0 {
		t.Fatalf("the old id has no incoming edges, got %v", got)
	}
}

func TestRedirectSurvivesCompaction(t *testing.T) {
	dir := t.TempDir()
	r, err := OpenReverse(dir, obs.Nop())
	if err != nil {
		t.Fatal(err)
	}
	r.Upsert("docs/a.md#new", "caller", EdgeReference, 1.0)
	r.Redirect("docs/a.md#old", "docs/a.md#new", 4242)
	r.Compact()
	r.Close()

	// Compaction serializes upserts only, which is exactly why a redirect
	// must not be tombstone-class: a redirect that evaporated here would
	// break every reference relying on it, silently and only after a rename.
	back, err := OpenReverse(dir, obs.Nop())
	if err != nil {
		t.Fatal(err)
	}
	defer back.Close()
	if got := back.ResolveID("docs/a.md#old"); got != "docs/a.md#new" {
		t.Fatalf("ResolveID after compaction = %q", got)
	}
	if got, ok := back.Redirected("docs/a.md#old"); !ok || got != "docs/a.md#new" {
		t.Fatalf("Redirected = %q ok=%v", got, ok)
	}
}

func TestResolveIDCompressesChains(t *testing.T) {
	dir := t.TempDir()
	r, err := OpenReverse(dir, obs.Nop())
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	r.Redirect("a", "b", 10)
	r.Redirect("b", "c", 20)
	if got := r.ResolveID("a"); got != "c" {
		t.Fatalf("ResolveID(a) = %q, want c", got)
	}
	// The walk must have been shortened, or a document renamed repeatedly
	// grows a hop per rename forever.
	if got, _ := r.Redirected("a"); got != "c" {
		t.Fatalf("chain not flattened: a → %q", got)
	}
}

func TestResolveIDDatesShortcutByNewestHop(t *testing.T) {
	dir := t.TempDir()
	r, _ := OpenReverse(dir, obs.Nop())
	defer r.Close()

	r.Redirect("a", "b", 10)
	r.Redirect("b", "c", 900)
	r.ResolveID("a")

	// Dating the shortcut by the oldest hop would make it look collectable
	// while the rename that created it is still recent, and collecting it
	// breaks every reference relying on the chain.
	r.mu.RLock()
	got := r.redirects["a"]
	r.mu.RUnlock()
	if got.epoch != 900 {
		t.Fatalf("shortcut epoch = %d, want the newest hop (900)", got.epoch)
	}
}

func TestResolveIDSurvivesCycle(t *testing.T) {
	dir := t.TempDir()
	r, _ := OpenReverse(dir, obs.Nop())
	defer r.Close()

	// Only a corrupt or hand-edited log produces this, but hanging inside a
	// read lock is not an acceptable response to bad input.
	r.Redirect("a", "b", 1)
	r.Redirect("b", "a", 2)
	if got := r.ResolveID("a"); got != "a" && got != "b" {
		t.Fatalf("ResolveID(a) = %q", got)
	}
}

func TestRedirectIgnoresSelfAndEmpty(t *testing.T) {
	dir := t.TempDir()
	r, _ := OpenReverse(dir, obs.Nop())
	defer r.Close()

	r.Redirect("a", "a", 1)
	r.Redirect("", "b", 1)
	r.Redirect("c", "", 1)
	if got := r.ResolveID("a"); got != "a" {
		t.Fatalf("a self-redirect must not be stored, got %q", got)
	}
}

func TestRenamedHeadingProducesRedirect(t *testing.T) {
	e := newEngine(t)
	e.clock = func() uint64 { return 777 }
	tree := merkle.NewTree()
	apply := func(res *parse.FileResult) {
		e.ApplyFile(res, merkle.Diff(tree, res))
		tree.Update(res)
		e.Flush()
	}

	const doc = "docs/spec.md"
	body := merkle.Sum([]byte("the section body, unchanged"))

	apply(mdFile(doc, sectionWithBody(doc+"#old-name", "Old name", body)))
	apply(mdFile(doc, sectionWithBody(doc+"#new-name", "New name", body)))

	// A node that vanished while one with identical content appeared in the
	// same file is a rename, and references to the old id must keep working.
	if got := e.ResolveID(doc + "#old-name"); got != doc+"#new-name" {
		t.Fatalf("ResolveID = %q, want the new heading", got)
	}
}

func TestRewrittenSectionIsNotARename(t *testing.T) {
	e := newEngine(t)
	tree := merkle.NewTree()
	apply := func(res *parse.FileResult) {
		e.ApplyFile(res, merkle.Diff(tree, res))
		tree.Update(res)
		e.Flush()
	}

	const doc = "docs/spec.md"
	apply(mdFile(doc, sectionWithBody(doc+"#old", "Old", merkle.Sum([]byte("body one")))))
	apply(mdFile(doc, sectionWithBody(doc+"#new", "New", merkle.Sum([]byte("body two")))))

	// Identical content is the whole test. A heading rewritten together with
	// its body is not recoverable, and guessing would point references at a
	// section that no longer says what the reference was about.
	if got := e.ResolveID(doc + "#old"); got != doc+"#old" {
		t.Fatalf("ResolveID = %q, want no redirect", got)
	}
}

func TestAmbiguousRenameIsNotGuessed(t *testing.T) {
	e := newEngine(t)
	tree := merkle.NewTree()
	apply := func(res *parse.FileResult) {
		e.ApplyFile(res, merkle.Diff(tree, res))
		tree.Update(res)
		e.Flush()
	}

	const doc = "docs/spec.md"
	dup := merkle.Sum([]byte("identical body"))
	apply(mdFile(doc, sectionWithBody(doc+"#old", "Old", dup)))
	apply(mdFile(doc,
		sectionWithBody(doc+"#new-a", "New A", dup),
		sectionWithBody(doc+"#new-b", "New B", dup),
	))

	// Two candidates and no way to choose: pointing every reference at the
	// wrong one is worse than letting it dangle where a person will see it.
	if got := e.ResolveID(doc + "#old"); got != doc+"#old" {
		t.Fatalf("ResolveID = %q, want no guess", got)
	}
}

func TestRenameAcrossFilesIsNotARedirect(t *testing.T) {
	e := newEngine(t)
	tree := merkle.NewTree()
	apply := func(res *parse.FileResult) {
		e.ApplyFile(res, merkle.Diff(tree, res))
		tree.Update(res)
		e.Flush()
	}

	body := merkle.Sum([]byte("shared body"))
	apply(mdFile("docs/a.md", sectionWithBody("docs/a.md#s", "S", body)))
	apply(mdFile("docs/b.md", sectionWithBody("docs/b.md#s", "S", body)))

	// Detection is scoped to one file's own re-parse. Two documents that
	// happen to quote the same paragraph are not a rename of each other.
	if got := e.ResolveID("docs/a.md#s"); got != "docs/a.md#s" {
		t.Fatalf("ResolveID = %q, want no cross-file redirect", got)
	}
}

func TestRenameKeySurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	const doc = "docs/spec.md"
	body := merkle.Sum([]byte("a body that outlives its heading"))

	e, err := Open(dir, obs.Nop())
	if err != nil {
		t.Fatal(err)
	}
	tree := merkle.NewTree()
	res := mdFile(doc, sectionWithBody(doc+"#old", "Old", body))
	e.ApplyFile(res, merkle.Diff(tree, res))
	tree.Update(res)
	e.Flush()
	e.Close()

	// Reopen from shards. Without the key on disk the old record comes back
	// without one, and the first rename after any restart reads as a plain
	// deletion — which is most real renames.
	e2, err := Open(dir, obs.Nop())
	if err != nil {
		t.Fatal(err)
	}
	defer e2.Close()

	renamed := mdFile(doc, sectionWithBody(doc+"#new", "New", body))
	e2.ApplyFile(renamed, merkle.Diff(tree, renamed))
	e2.Flush()

	if got := e2.ResolveID(doc + "#old"); got != doc+"#new" {
		t.Fatalf("ResolveID after restart = %q, want the new heading", got)
	}
}
