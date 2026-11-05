package workspace

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Salvia95/graphin/internal/nodeid"
	"github.com/Salvia95/graphin/internal/obs"
)

// chunkWord spells n in lowercase letters: one word, one BM25 token.
func chunkWord(n int) string {
	s := "q"
	for {
		s += string(rune('a' + n%26))
		n /= 26
		if n == 0 {
			return s
		}
	}
}

// longDoc is a section of paras paragraphs of 60 distinct words each, so it
// runs well past one index entry and every word is findable on its own.
func longDoc(paras int) string {
	var b strings.Builder
	b.WriteString("# 운영 메모\n\n")
	for p := 0; p < paras; p++ {
		for w := 0; w < 60; w++ {
			b.WriteString(chunkWord(p*100 + w))
			b.WriteByte(' ')
		}
		b.WriteString("\n\n")
	}
	return b.String()
}

const chunkSec = "docs/ops.md#운영-메모"

func bootstrapDocs(t *testing.T, root string) *Workspace {
	t.Helper()
	w := New(Config{Root: root, Log: obs.Nop(), OrtLib: "/nonexistent-ort"})
	if _, err := w.Bootstrap(context.Background(), "", false); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) && w.FSM.Phase() < PhaseLexicalReady {
		time.Sleep(20 * time.Millisecond)
	}
	if w.FSM.Phase() < PhaseLexicalReady {
		t.Fatal("initial scan never finished")
	}
	return w
}

func writeDoc(t *testing.T, root, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "ops.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// A word from the back of a long section finds the section, reported as the
// section with the line of the chunk that matched; the chunk entries follow
// the section through a shrink and a delete.
func TestSectionChunksLifecycle(t *testing.T) {
	root := t.TempDir()
	writeDoc(t, root, longDoc(12))
	w := bootstrapDocs(t, root)
	defer w.Close()

	tail := chunkWord(11*100 + 30) // paragraph 12 of 12
	res := w.Router.Search(tail, 5)
	if len(res) == 0 || res[0].NodeID != chunkSec || res[0].Via == "" {
		t.Fatalf("tail word: %+v", res)
	}
	line, ok := w.ChunkLine(res[0].Via)
	src := longDoc(12)
	wantLine := 1 + strings.Count(src[:strings.Index(src, tail)], "\n")
	if !ok || line > wantLine || line < wantLine-12 {
		t.Fatalf("chunk line %d (ok=%v), the word is on line %d", line, ok, wantLine)
	}
	if !w.Lex.HasDoc(nodeid.ChunkID(chunkSec, 2)) {
		t.Fatal("no chunk documents were indexed")
	}

	// Shrink: the section drops below one entry, so every chunk goes.
	writeDoc(t, root, longDoc(1))
	indexOneFile(t, w, "docs/ops.md")
	if w.Lex.HasDoc(nodeid.ChunkID(chunkSec, 2)) {
		t.Fatal("a stale chunk survived the section shrinking")
	}
	if _, ok := w.ChunkLine(res[0].Via); ok {
		t.Fatal("a stale chunk line survived the section shrinking")
	}

	// Grow again, then delete the file.
	writeDoc(t, root, longDoc(12))
	indexOneFile(t, w, "docs/ops.md")
	if !w.Lex.HasDoc(nodeid.ChunkID(chunkSec, 3)) {
		t.Fatal("chunks did not come back with the section")
	}
	w.indexMu.Lock()
	w.removeFileLocked("docs/ops.md")
	w.indexMu.Unlock()
	if w.Lex.HasDoc(nodeid.ChunkID(chunkSec, 2)) || len(w.Router.Search(tail, 5)) != 0 {
		t.Fatal("chunks outlived their file")
	}
}

// An index built before chunks existed is restored on restart with its
// markdown unchanged — OffsetOnly, which would never reach BM25 again. The
// docs version marker is what makes the scan index those files afresh.
func TestDocsReindexOnStaleMarker(t *testing.T) {
	root := t.TempDir()
	writeDoc(t, root, longDoc(12))
	w := bootstrapDocs(t, root)
	// Make it look like an index from before chunks: no chunk docs, no marker.
	w.indexMu.Lock()
	w.dropChunkDocs(chunkSec, 2)
	w.persistIndexesLocked()
	w.indexMu.Unlock()
	w.Close()
	if err := os.Remove(filepath.Join(root, ".graphin", "search", "docs.version")); err != nil {
		t.Fatal(err)
	}

	w = bootstrapDocs(t, root)
	defer w.Close()
	if !w.Lex.HasDoc(nodeid.ChunkID(chunkSec, 2)) {
		t.Fatal("restart under a stale marker did not index the chunks")
	}
	if b, err := os.ReadFile(filepath.Join(root, ".graphin", "search", "docs.version")); err != nil ||
		strings.TrimSpace(string(b)) != docsIndexVersion {
		t.Fatalf("marker not rewritten: %q %v", b, err)
	}
}
