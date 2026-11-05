package workspace

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/Salvia95/graphin/internal/lexical"
	"github.com/Salvia95/graphin/internal/nodeid"
	"github.com/Salvia95/graphin/internal/parse"
)

// Section chunks (parse.SectionChunk) are index entries, not nodes: the
// merkle tree, the graph and the node map never see them. This file keeps the
// three places that do — BM25, the vector index and the chunk line table —
// in step with the sections they belong to.

// setChunkMetaLocked records where a section's chunks start, replacing what
// was recorded before. Runs for every parsed node (Track A), because lines
// move whenever anything above them does. Callers hold nodesMu.
func (w *Workspace) setChunkMetaLocked(n parse.Node) {
	old := w.chunkCount[n.ID]
	for ord := len(n.Chunks) + 2; ord <= old+1; ord++ {
		delete(w.chunkLines, nodeid.ChunkID(n.ID, ord))
	}
	if len(n.Chunks) == 0 {
		delete(w.chunkCount, n.ID)
		return
	}
	for _, c := range n.Chunks {
		w.chunkLines[nodeid.ChunkID(n.ID, c.Ord)] = c.StartLine
	}
	w.chunkCount[n.ID] = len(n.Chunks)
}

// dropChunkMetaLocked forgets a removed section's chunk lines. Callers hold
// nodesMu.
func (w *Workspace) dropChunkMetaLocked(id string) {
	for ord := 2; ord <= w.chunkCount[id]+1; ord++ {
		delete(w.chunkLines, nodeid.ChunkID(id, ord))
	}
	delete(w.chunkCount, id)
}

// indexChunks writes a changed section's chunks to BM25 and drops the ones a
// shorter version of the section no longer has. Each chunk document carries
// the heading's tokens as well as its own, so a query that names the
// subject and a detail from deep in the section meets both in one document.
func (w *Workspace) indexChunks(n parse.Node) {
	heading := lexical.Tokenize(n.SimpleName)
	for _, c := range n.Chunks {
		toks := make([]string, 0, len(heading)+len(c.Tokens))
		toks = append(append(toks, heading...), c.Tokens...)
		w.Lex.Upsert(nodeid.ChunkID(n.ID, c.Ord), toks)
	}
	w.dropChunkDocs(n.ID, len(n.Chunks)+2)
}

// dropChunkDocs removes chunk documents and vectors of section id from
// ordinal from onward. Chunk ordinals are contiguous, so BM25 holding the
// document is the record of how far they go — which also covers chunks left
// by a run whose in-memory table is gone.
func (w *Workspace) dropChunkDocs(id string, from int) {
	for ord := from; ; ord++ {
		cid := nodeid.ChunkID(id, ord)
		if !w.Lex.HasDoc(cid) {
			return
		}
		w.Lex.Delete(cid)
		if w.semSink != nil {
			w.semSink.Remove(cid)
		}
	}
}

// ChunkLine returns the line a section chunk starts at, for reporting where
// in a section a search hit matched.
func (w *Workspace) ChunkLine(chunkID string) (int, bool) {
	w.nodesMu.RLock()
	defer w.nodesMu.RUnlock()
	l, ok := w.chunkLines[chunkID]
	return int(l), ok
}

// docsIndexVersion names what markdown files put into the ranked indexes.
// Raise it when that changes, so an index built before the change gets the
// new entries without being rebuilt from scratch:
//
//	1 — section excerpts in the vector summary, continuation chunks
//	    (parse.SectionChunk)
const docsIndexVersion = "1"

func (w *Workspace) docsMarkerPath() string {
	return filepath.Join(w.Dir, "search", "docs.version")
}

// reindexDocsIfStale makes the initial scan treat every markdown file as new
// when the index was built under an older docsIndexVersion. Unchanged files
// are otherwise OffsetOnly and never reach BM25 or the embedder again, so a
// restored index would keep heading-only vectors and no chunks forever.
//
// Only markdown is touched: re-embedding the code nodes too, which a lexical
// snapshot version bump would do, costs far more and changes nothing. The
// old documents are removed here rather than left to be overwritten, so a
// section renamed while no server ran does not linger in the index.
func (w *Workspace) reindexDocsIfStale() {
	if b, err := os.ReadFile(w.docsMarkerPath()); err == nil && strings.TrimSpace(string(b)) == docsIndexVersion {
		return
	}
	w.indexMu.Lock()
	defer w.indexMu.Unlock()
	var docs []string
	for rel := range w.merkle.Files {
		if parse.DetectLanguage(rel) == parse.LangMarkdown {
			docs = append(docs, rel)
		}
	}
	for _, rel := range docs {
		for _, id := range w.merkle.Remove(rel) {
			w.Lex.Delete(id)
			w.Sym.Delete(id)
			w.dropChunkDocs(id, 2)
		}
	}
	if len(docs) > 0 {
		w.Log.Event("docs_reindex", map[string]any{"files": len(docs), "version": docsIndexVersion})
	}
}

// writeDocsMarker records that the index now holds what docsIndexVersion
// describes. Called once the initial scan has been applied and persisted;
// a crash before that leaves the marker stale and the reindex repeats.
func (w *Workspace) writeDocsMarker() {
	if err := os.WriteFile(w.docsMarkerPath(), []byte(docsIndexVersion+"\n"), 0o644); err != nil {
		w.Log.Event("persist_error", map[string]any{"file": "docs.version", "error": err.Error()})
	}
}
