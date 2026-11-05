package nodeid

import "testing"

func TestChunkIDRoundTrip(t *testing.T) {
	sec := "docs/a~b/ops.md#운영-메모"
	id := ChunkID(sec, 12)
	got, ord, ok := ChunkParent(id)
	if !ok || got != sec || ord != 12 {
		t.Fatalf("ChunkParent(%q) = %q %d %v", id, got, ord, ok)
	}
	// Node ids are never chunk ids: no '#', a Python redefinition marker,
	// a '~' only in the path, ordinal 1 (the section itself), no digits.
	for _, id := range []string{
		"pkg.Class.method(long)", "mod.f#2", "docs/a~3/ops.md#x", "docs/ops.md#x~1",
		"docs/ops.md#x~", "docs/ops.md#x~2a", "docs/ops.md",
	} {
		if _, _, ok := ChunkParent(id); ok {
			t.Errorf("%q parsed as a chunk id", id)
		}
	}
}
