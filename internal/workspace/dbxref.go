package workspace

// Phase 7b (docs/phase7-spec.md §1.6/§2): when a watcher batch moves the DB
// registry — snapshots added, tables renamed, files removed — existing code
// nodes hold stale cross-domain edges until they re-resolve. This module
// finds the affected code files (BM25 posting lookup by table name, reverse
// index for removals) and re-parses them for resolution only: bytes are
// unchanged, so embeddings, lexical docs and merkle state stay untouched.

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/Salvia95/graphin/internal/lexical"
	"github.com/Salvia95/graphin/internal/merkle"
	"github.com/Salvia95/graphin/internal/nodeid"
	"github.com/Salvia95/graphin/internal/parse"
)

// dbXrefDelta accumulates one batch's DB-registry movement. It exists only
// while handleBatch runs (single writer under indexMu).
type dbXrefDelta struct {
	names   map[string]bool // added/changed table+alias lookup names
	removed []string        // removed db.* node IDs
}

func newDBXrefDelta() *dbXrefDelta {
	return &dbXrefDelta{names: map[string]bool{}}
}

func (d *dbXrefDelta) empty() bool {
	return d == nil || len(d.names) == 0 && len(d.removed) == 0
}

// collectDBXrefDelta records one applied DB-schema file's registry movement.
func (w *Workspace) collectDBXrefDelta(res *parse.FileResult, diff merkle.FileDiff) {
	d := w.xrefDelta
	if d == nil || res.Lang != parse.LangDBSchema {
		return
	}
	for _, n := range diff.Changed {
		if n.Kind != nodeid.KindTable && n.Kind != nodeid.KindView {
			continue
		}
		d.names[n.SimpleName] = true
		for _, a := range n.Aliases {
			if a != "" {
				d.names[a] = true
			}
		}
	}
	for _, id := range diff.Removed {
		if strings.HasPrefix(id, "db.") {
			d.removed = append(d.removed, id)
		}
	}
}

// collectDBXrefRemovals records DB node IDs dropped with a deleted file.
func (w *Workspace) collectDBXrefRemovals(ids []string) {
	if w.xrefDelta == nil {
		return
	}
	for _, id := range ids {
		if strings.HasPrefix(id, "db.") {
			w.xrefDelta.removed = append(w.xrefDelta.removed, id)
		}
	}
}

// maxXrefInvalidateHits bounds the BM25 sweep per affected table name.
const maxXrefInvalidateHits = 200

// invalidateDBXrefsLocked re-parses the code files whose cross-domain edges
// may have moved with this batch's registry delta. Callers hold w.indexMu.
func (w *Workspace) invalidateDBXrefsLocked() int {
	d := w.xrefDelta
	if d.empty() || w.graph == nil {
		return 0
	}
	files := map[string]bool{}
	addNode := func(id string) {
		if m, ok := w.nodeMeta(id); ok && !nodeid.IsDBKind(m.Kind) && m.Kind != nodeid.KindFile {
			files[m.RelPath] = true
		}
	}
	for _, name := range sortedStrings(d.names) {
		for _, hit := range w.Lex.Search(lexical.Tokenize(name), maxXrefInvalidateHits) {
			addNode(hit.DocID)
		}
	}
	for _, id := range d.removed {
		for _, src := range w.graph.UsedBySources(id) {
			addNode(src)
		}
	}
	count := 0
	for _, rel := range sortedStrings(files) {
		if w.reresolveFileLocked(rel) {
			count++
		}
	}
	if count > 0 {
		w.Log.Event("db_xref_invalidate", map[string]any{
			"names": len(d.names), "removed": len(d.removed), "files": count,
		})
	}
	return count
}

// reresolveFileLocked re-parses one file and refreshes its graph records as
// if every node changed: edges recompute at the next Flush, Track B
// consumers (embeddings, lexical) stay untouched — the bytes are identical.
func (w *Workspace) reresolveFileLocked(rel string) bool {
	abs := filepath.Join(w.Root, filepath.FromSlash(rel))
	src, err := os.ReadFile(abs)
	if err != nil {
		return false
	}
	r, err := w.parseFile(rel, src)
	if err != nil {
		return false
	}
	w.graph.ApplyFile(r, merkle.FileDiff{RelPath: rel, Changed: r.Nodes})
	return true
}
