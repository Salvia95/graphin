package workspace

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	pathpkg "path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Salvia95/graphin/internal/ignore"
	"github.com/Salvia95/graphin/internal/lexical"
	"github.com/Salvia95/graphin/internal/merkle"
	"github.com/Salvia95/graphin/internal/nodeid"
	"github.com/Salvia95/graphin/internal/parse"
	"github.com/Salvia95/graphin/internal/scan"
	"github.com/Salvia95/graphin/internal/semantic"
	"github.com/Salvia95/graphin/internal/watch"
)

// Sentinel errors surfaced as §3.0 tool error codes.
var (
	ErrNodeNotFound = errors.New("node not found")
	ErrNodeGone     = errors.New("node gone after reparse")
)

// NodeMeta is the per-node location record backing read_code (§3.4).
type NodeMeta struct {
	RelPath     string
	StartByte   uint32
	EndByte     uint32
	FileHash    string // hex BLAKE3 of the whole file at index time
	Kind        string
	DisplayName string
	Partial     bool
}

// nodeMeta reads one node's metadata.
func (w *Workspace) nodeMeta(id string) (NodeMeta, bool) {
	w.nodesMu.RLock()
	defer w.nodesMu.RUnlock()
	m, ok := w.nodes[id]
	return m, ok
}

// DisplayName returns the §3.2 display_name for a node ("" if unknown).
func (w *Workspace) DisplayName(id string) string {
	m, _ := w.nodeMeta(id)
	return m.DisplayName
}

// applyFileResult is the single-writer application of one parsed file: 2-Track
// classification, merkle update, metadata/symbol/lexical maintenance.
// Callers must hold w.indexMu.
func (w *Workspace) applyFileResult(res *parse.FileResult) merkle.FileDiff {
	diff := merkle.Diff(w.merkle, res)
	if w.graph != nil {
		w.graph.ApplyFile(res, diff)
	}
	if w.semSink != nil { // Track B + crash-recovery force set (§2.2)
		force := w.forceEmbed[res.RelPath]
		changedIDs := make(map[string]bool, len(diff.Changed))
		for _, n := range diff.Changed {
			changedIDs[n.ID] = true
		}
		for _, n := range res.Nodes {
			if force || changedIDs[n.ID] {
				w.semSink.Enqueue(n.ID, semantic.Summarize(res.Package, n))
			}
		}
		for _, id := range diff.Removed {
			w.semSink.Remove(id)
		}
		delete(w.forceEmbed, res.RelPath)
	}
	w.merkle.Update(res)

	fileHex := merkle.Hex(res.FileHash)
	w.nodesMu.Lock()
	for _, n := range res.Nodes { // Track A: offsets refresh unconditionally
		// graphindb 가드: 스키마별 파일 분할은 합법이지만 같은 테이블 FQN이
		// 두 파일에서 오면 last-writer-wins로 조용히 덮이므로 경고만 남긴다.
		if old, ok := w.nodes[n.ID]; ok && old.RelPath != res.RelPath && nodeid.IsDBKind(n.Kind) {
			w.Log.Event("db_duplicate_fqn", map[string]any{
				"node": n.ID, "kept": res.RelPath, "previous": old.RelPath,
			})
		}
		w.nodes[n.ID] = NodeMeta{
			RelPath:     res.RelPath,
			StartByte:   n.StartByte,
			EndByte:     n.EndByte,
			FileHash:    fileHex,
			Kind:        n.Kind,
			DisplayName: n.DisplayName,
			Partial:     res.Partial,
		}
	}
	for _, id := range diff.Removed {
		delete(w.nodes, id)
	}
	w.nodesMu.Unlock()

	for _, n := range diff.Changed { // Track B consumers: content changed only
		w.Sym.Put(n.ID, n.SimpleName)
		tokens := lexical.BuildDocTokens(n.SimpleName, n.ID, signatureText(n))
		w.Lex.Upsert(n.ID, append(tokens, n.BodyTokens...)) // §보완 B
	}
	for _, id := range diff.Removed {
		w.Sym.Delete(id)
		w.Lex.Delete(id)
	}
	return diff
}

func signatureText(n parse.Node) string {
	parts := append([]string{}, n.Params...)
	parts = append(parts, n.Supers...)
	parts = append(parts, n.Kind)
	return strings.Join(parts, " ")
}

// removeFileLocked drops one file's nodes. Callers hold w.indexMu.
func (w *Workspace) removeFileLocked(rel string) {
	ids := w.merkle.Remove(rel)
	if len(ids) == 0 {
		return
	}
	w.nodesMu.Lock()
	for _, id := range ids {
		delete(w.nodes, id)
	}
	w.nodesMu.Unlock()
	for _, id := range ids {
		w.Sym.Delete(id)
		w.Lex.Delete(id)
		if w.semSink != nil {
			w.semSink.Remove(id)
		}
	}
	if w.graph != nil {
		w.graph.RemoveNodes(ids)
	}
}

// removePrefixLocked drops every indexed file under a removed directory.
func (w *Workspace) removePrefixLocked(relDir string) {
	prefix := relDir + "/"
	var doomed []string
	for rel := range w.merkle.Files {
		if strings.HasPrefix(rel, prefix) {
			doomed = append(doomed, rel)
		}
	}
	for _, rel := range doomed {
		w.removeFileLocked(rel)
	}
}

// initialScan is the first full index pass: parallel parse fan-out, serial
// single-writer application, staged availability flip.
func (w *Workspace) initialScan(ctx context.Context) {
	start := time.Now()
	res, err := scan.Walk(w.Root, w.Log)
	if err != nil {
		w.Log.Event("scan_error", map[string]any{"error": err.Error()})
		return
	}
	w.matcherMu.Lock()
	w.matcher = res.Matcher
	w.matcherMu.Unlock()

	total := len(res.Files)
	jobs := make(chan scan.FileInfo)
	out := make(chan *parse.FileResult, w.cfg.Workers)

	var wg sync.WaitGroup
	for i := 0; i < w.cfg.Workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for f := range jobs {
				src, err := os.ReadFile(f.AbsPath)
				if err != nil {
					continue
				}
				r, err := w.parseFile(f.RelPath, src)
				if err != nil {
					w.Log.Event("parse_error", map[string]any{"path": f.RelPath, "error": err.Error()})
					continue
				}
				if r.Partial {
					w.Log.Event("parse_partial", map[string]any{"path": f.RelPath})
				}
				select {
				case out <- r:
				case <-ctx.Done():
					return
				}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, f := range res.Files {
			select {
			case jobs <- f:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() { wg.Wait(); close(out) }()

	done, offsetOnly, changed := 0, 0, 0
	for r := range out {
		w.indexMu.Lock()
		d := w.applyFileResult(r)
		w.indexMu.Unlock()
		offsetOnly += len(d.OffsetOnly)
		changed += len(d.Changed)
		done++
		if total > 0 {
			w.FSM.SetProgress(done * 100 / total)
		}
	}
	if w.graph != nil {
		w.indexMu.Lock()
		w.graph.Flush()
		w.indexMu.Unlock()
	}
	w.persistIndexes()
	w.FSM.SetProgress(100)
	w.FSM.Set(PhaseLexicalReady)
	w.Log.Event("initial_scan_done", map[string]any{
		"files": total, "nodes": w.Sym.Len(),
		"nodes_changed": changed, "nodes_offset_only": offsetOnly,
		"ms": time.Since(start).Milliseconds(),
	})
}

// handleBatch applies one debounced watcher batch (§3.1).
func (w *Workspace) handleBatch(b watch.Batch) {
	w.matcherMu.Lock()
	matcher := w.matcher
	w.matcherMu.Unlock()

	w.indexMu.Lock()
	defer w.indexMu.Unlock()
	touched := 0
	manifestEvent := false
	for abs, ch := range b {
		rel, err := filepath.Rel(w.Root, abs)
		if err != nil {
			continue
		}
		rel = filepath.ToSlash(rel)
		if rel == "." || strings.HasPrefix(rel, "..") || strings.HasPrefix(rel, watch.GraphinDirName) {
			continue
		}
		if parse.IsDBManifest(rel) {
			manifestEvent = true
		}

		if ch.Kind == watch.Removed {
			w.removeFileLocked(rel)
			w.removePrefixLocked(rel)
			touched++
			continue
		}
		fi, err := os.Stat(abs)
		if err != nil { // raced with a delete
			w.removeFileLocked(rel)
			w.removePrefixLocked(rel)
			touched++
			continue
		}
		if fi.IsDir() { // e.g. git checkout materializing a subtree
			touched += w.indexDirLocked(rel, abs, matcher)
			continue
		}
		if w.indexPathLocked(rel, abs, fi.Size(), matcher) {
			touched++
		}
	}
	// 매니페스트 변경: 라우팅 diff → 영향받은 SSOT 파일 강제 재인덱싱
	if manifestEvent {
		touched += w.reloadDBRoutesLocked(matcher)
	}
	if touched > 0 {
		if w.graph != nil {
			w.graph.Flush()
		}
		w.persistIndexesLocked()
		w.Log.Event("watch_batch", map[string]any{"events": len(b), "indexed": touched})
	}
}

// indexPathLocked parses and applies one file if it is indexable and its
// content hash actually moved. Callers hold w.indexMu.
func (w *Workspace) indexPathLocked(rel, abs string, size int64, m *ignore.Matcher) bool {
	if !scan.Indexable(w.Root, rel, m) {
		return false
	}
	if size > scan.DefaultMaxFileSize {
		w.Log.Event("skip_large_file", map[string]any{"path": rel, "bytes": size})
		return false
	}
	src, err := os.ReadFile(abs)
	if err != nil {
		return false
	}
	if w.merkle.FileHash(rel) == merkle.Hex(merkle.Sum(src)) {
		return false // editor touch without content change
	}
	r, err := w.parseFile(rel, src)
	if err != nil {
		return false
	}
	w.applyFileResult(r)
	return true
}

func (w *Workspace) indexDirLocked(relDir, absDir string, m *ignore.Matcher) int {
	count := 0
	_ = filepath.WalkDir(absDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if scan.DefaultExcludedDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		rel := pathpkg.Join(relDir, filepath.ToSlash(strings.TrimPrefix(p, absDir+string(filepath.Separator))))
		info, err := d.Info()
		if err != nil {
			return nil
		}
		if w.indexPathLocked(rel, p, info.Size(), m) {
			count++
		}
		return nil
	})
	return count
}

// persistIndexes snapshots lexical.idx and merkle.json atomically.
func (w *Workspace) persistIndexes() {
	w.indexMu.Lock()
	defer w.indexMu.Unlock()
	w.persistIndexesLocked()
}

func (w *Workspace) persistIndexesLocked() {
	searchDir := filepath.Join(w.Dir, "search")
	if err := os.MkdirAll(searchDir, 0o755); err != nil {
		w.Log.Event("persist_error", map[string]any{"error": err.Error()})
		return
	}
	if err := lexical.Save(filepath.Join(searchDir, "lexical.idx"), w.Lex, w.Sym); err != nil {
		w.Log.Event("persist_error", map[string]any{"file": "lexical.idx", "error": err.Error()})
	}
	if err := w.merkle.Save(filepath.Join(w.Dir, "merkle.json")); err != nil {
		w.Log.Event("persist_error", map[string]any{"file": "merkle.json", "error": err.Error()})
	}
}

// CodeBlock is the read_code payload (§3.4).
type CodeBlock struct {
	ID        string
	RelPath   string
	StartLine int
	EndLine   int
	Code      string
	Reparsed  bool
	Partial   bool
}

// ReadCode slices a node's exact source. If the file hash no longer matches
// the index, the file is reparsed inline and the fresh slice returned with
// Reparsed=true; NODE_GONE only if the node vanished (§2.1.2 Auto-Reparse).
func (w *Workspace) ReadCode(id string) (*CodeBlock, error) {
	meta, ok := w.nodeMeta(id)
	if !ok {
		return nil, ErrNodeNotFound
	}
	abs := filepath.Join(w.Root, filepath.FromSlash(meta.RelPath))
	src, err := os.ReadFile(abs)
	if err != nil {
		w.indexMu.Lock()
		w.removeFileLocked(meta.RelPath)
		w.indexMu.Unlock()
		return nil, ErrNodeGone
	}

	reparsed := false
	if merkle.Hex(merkle.Sum(src)) != meta.FileHash {
		r, perr := w.parseFile(meta.RelPath, src)
		if perr != nil {
			return nil, ErrNodeGone
		}
		w.indexMu.Lock()
		w.applyFileResult(r)
		if w.graph != nil {
			w.graph.Flush()
		}
		w.indexMu.Unlock()
		reparsed = true
		if meta, ok = w.nodeMeta(id); !ok || meta.RelPath != r.RelPath {
			return nil, ErrNodeGone
		}
	}

	if int(meta.EndByte) > len(src) || meta.StartByte > meta.EndByte {
		return nil, ErrNodeGone
	}
	slice := src[meta.StartByte:meta.EndByte]
	startLine := 1 + bytes.Count(src[:meta.StartByte], []byte("\n"))
	endLine := startLine + bytes.Count(slice, []byte("\n"))
	return &CodeBlock{
		ID:        id,
		RelPath:   meta.RelPath,
		StartLine: startLine,
		EndLine:   endLine,
		Code:      string(slice),
		Reparsed:  reparsed,
		Partial:   meta.Partial,
	}, nil
}
