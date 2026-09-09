// scope.go — what the index is made of, and how to take something out of it.
//
// The exclusion contract is deliberately one-sided: writing a pattern changes
// nothing that is already indexed. Search keeps answering from the current
// index until the server next starts, which is when initialScan reconciles the
// walk against the merkle tree and drops what is no longer walked. That keeps
// the read path filter-free (the same reason opRedirect stays out of the edge
// map) and leaves exactly one moment where the index changes shape.
//
// "The server next starts" means a fresh process, not another
// bootstrap_workspace call: Bootstrap returns immediately when the workspace is
// already bootstrapped, so a second call on a live server rescans nothing.
package workspace

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Salvia95/graphin/internal/ignore"
	"github.com/Salvia95/graphin/internal/parse"
	"github.com/Salvia95/graphin/internal/scan"
	"github.com/Salvia95/graphin/internal/store"
)

// scopeTopN bounds the rows in one grouping; the totals beside them always
// cover everything.
const scopeTopN = 12

// rootGroup labels files that sit directly under the workspace root.
const rootGroup = "(root)"

// ScopeEntry is one grouping row — a top-level directory or an extension.
type ScopeEntry struct {
	Name       string
	Files      int
	Nodes      int // 0 when Estimated and the language is not markdown
	MDHeadings int // markdown section nodes, counted without parsing
}

// ScopeReport describes what the index holds (or would hold).
//
// Estimated distinguishes the two sources. After bootstrap the numbers come
// from the merkle tree and are exact: it records every indexed file with its
// node IDs, so a per-file node count costs one map walk over files, not over
// nodes. Before bootstrap there is no merkle tree, so a fresh scan.Walk gives
// exact file counts while node counts are unknown for code — only markdown
// headings can be counted without parsing, and those are the ones that make
// node counts jump.
type ScopeReport struct {
	Estimated bool
	Files     int
	Nodes     int
	Headings  int
	Dirs      []ScopeEntry
	Exts      []ScopeEntry
	Excluded  []string
}

// ScopeImpact is what a set of patterns would take out.
type ScopeImpact struct {
	Estimated bool
	Patterns  []string
	Files     int
	Nodes     int
	Sample    []string // up to scopeTopN affected paths
	Warnings  []string
}

// Scope reports the current index composition. It reads the merkle tree, so
// it is exact and cheap; before bootstrap it falls back to ScopePreview.
func (w *Workspace) Scope() (*ScopeReport, error) {
	if !w.Bootstrapped() {
		return w.ScopePreview()
	}
	dirs := map[string]*ScopeEntry{}
	exts := map[string]*ScopeEntry{}
	rep := &ScopeReport{}

	w.indexMu.Lock()
	for rel, fe := range w.merkle.Files {
		n := len(fe.Nodes)
		rep.Files++
		rep.Nodes += n
		d := groupOf(dirs, topSegment(rel))
		d.Files++
		d.Nodes += n
		e := groupOf(exts, extOf(rel))
		e.Files++
		e.Nodes += n
		if parse.DetectLanguage(rel) == parse.LangMarkdown {
			// Every markdown node is a heading section; the file node the
			// parser also emits is counted with them and is off by one per
			// file, which no decision here turns on.
			rep.Headings += n
			d.MDHeadings += n
			e.MDHeadings += n
		}
	}
	w.indexMu.Unlock()

	rep.Dirs = topEntries(dirs)
	rep.Exts = topEntries(exts)
	rep.Excluded = w.excludedPatterns()
	return rep, nil
}

// ScopePreview reports what a bootstrap would index right now, without
// indexing it. Node counts are exact for markdown (headings are countable by
// reading, not parsing) and unknown for code, so Estimated is set.
func (w *Workspace) ScopePreview() (*ScopeReport, error) {
	res, err := scan.Walk(w.Root, w.Log)
	if err != nil {
		return nil, err
	}
	dirs := map[string]*ScopeEntry{}
	exts := map[string]*ScopeEntry{}
	rep := &ScopeReport{Estimated: true}

	for _, f := range res.Files {
		rep.Files++
		d := groupOf(dirs, topSegment(f.RelPath))
		d.Files++
		e := groupOf(exts, extOf(f.RelPath))
		e.Files++
		if parse.DetectLanguage(f.RelPath) == parse.LangMarkdown {
			h := countHeadings(f.AbsPath)
			rep.Headings += h
			rep.Nodes += h
			d.Nodes += h
			d.MDHeadings += h
			e.Nodes += h
			e.MDHeadings += h
		}
	}
	rep.Dirs = topEntries(dirs)
	rep.Exts = topEntries(exts)
	rep.Excluded = w.excludedPatterns()
	return rep, nil
}

// ScopeImpactOf computes what `patterns` would remove, without writing them.
func (w *Workspace) ScopeImpactOf(patterns []string) (*ScopeImpact, error) {
	m := ignore.NewMatcher()
	m.AddPatterns("", patterns)
	imp := &ScopeImpact{Patterns: patterns}

	hit := func(rel string, nodes int) {
		imp.Files++
		imp.Nodes += nodes
		if len(imp.Sample) < scopeTopN {
			imp.Sample = append(imp.Sample, rel)
		}
	}

	var total int
	dbHit := 0
	if w.Bootstrapped() {
		w.indexMu.Lock()
		for _, fe := range w.merkle.Files {
			total += len(fe.Nodes)
		}
		rels := make([]string, 0, len(w.merkle.Files))
		for rel := range w.merkle.Files {
			rels = append(rels, rel)
		}
		sort.Strings(rels) // deterministic sample
		for _, rel := range rels {
			if !ignoredPath(m, rel) {
				continue
			}
			hit(rel, len(w.merkle.Files[rel].Nodes))
			if parse.DetectLanguage(rel) == parse.LangDBSchema {
				dbHit++
			}
		}
		w.indexMu.Unlock()
	} else {
		imp.Estimated = true
		res, err := scan.Walk(w.Root, w.Log)
		if err != nil {
			return nil, err
		}
		for _, f := range res.Files {
			if !ignoredPath(m, f.RelPath) {
				continue
			}
			// Markdown is the one language whose node count is knowable
			// without parsing, and it is the one that dominates these
			// numbers — counting it makes the estimate useful rather than
			// uniformly zero.
			n := 0
			if parse.DetectLanguage(f.RelPath) == parse.LangMarkdown {
				n = countHeadings(f.AbsPath)
			}
			hit(f.RelPath, n)
			if parse.DetectLanguage(f.RelPath) == parse.LangDBSchema {
				dbHit++
			}
		}
	}

	imp.Warnings = w.scopeWarnings(m, imp, total, dbHit)
	return imp, nil
}

// scopeWarnings names the things a person should see before confirming. It
// never refuses: the three cases below are legitimate in some projects, and
// the tool's job is to make the cost visible, not to decide it.
func (w *Workspace) scopeWarnings(m *ignore.Matcher, imp *ScopeImpact, total, dbHit int) []string {
	var out []string
	if docs := w.pinnedFilesCut(m); docs > 0 {
		// Counted per document, not per pin: one file can hold several pinned
		// sections, and saying "N pins" when N is a file count reads as a
		// smaller loss than it is.
		out = append(out, fmt.Sprintf(
			"위키가 핀한 문서 %d개가 걸립니다 — 그 문서의 섹션을 가리키는 세트 항목이 끊깁니다", docs))
	}
	if dbHit > 0 {
		out = append(out, fmt.Sprintf(
			"DB 스키마 스냅샷 %d개가 걸립니다 — 테이블·FK 노드가 사라집니다", dbHit))
	}
	if total > 0 && imp.Nodes*10 > total*3 {
		out = append(out, fmt.Sprintf(
			"전체 노드의 %d%%가 걸립니다 (%d / %d)", imp.Nodes*100/total, imp.Nodes, total))
	}
	return out
}

// pinnedFilesCut counts distinct wiki-pinned documents the patterns would
// remove. Pins are read straight from the lockfile: it is the checkable
// record and needs no running wiki store.
func (w *Workspace) pinnedFilesCut(m *ignore.Matcher) int {
	b, err := os.ReadFile(filepath.Join(w.Root, "docs", "wiki", "pins.lock"))
	if err != nil {
		return 0
	}
	seen := map[string]bool{}
	// Node IDs appear as JSON keys "<relpath>#<anchor>"; the path half is all
	// this needs, so scan for them rather than decoding the whole structure.
	for _, tok := range strings.Split(string(b), "\"") {
		i := strings.IndexByte(tok, '#')
		if i <= 0 {
			continue
		}
		rel := tok[:i]
		if !strings.Contains(rel, "/") || seen[rel] {
			continue
		}
		seen[rel] = true
	}
	n := 0
	for rel := range seen {
		if ignoredPath(m, rel) {
			n++
		}
	}
	return n
}

// ScopeAdd appends patterns to .graphin/ignore with the reason recorded above
// them, and reports what they will take out at the next bootstrap.
func (w *Workspace) ScopeAdd(patterns []string, reason string) (*ScopeImpact, error) {
	imp, err := w.ScopeImpactOf(patterns)
	if err != nil {
		return nil, err
	}
	cur, _ := os.ReadFile(w.ignorePath())
	var sb bytes.Buffer
	sb.Write(cur)
	if len(cur) > 0 && !bytes.HasSuffix(cur, []byte("\n")) {
		sb.WriteByte('\n')
	}
	sb.WriteByte('\n')
	fmt.Fprintf(&sb, "# index_scope %s", time.Now().Format("2006-01-02"))
	if reason != "" {
		fmt.Fprintf(&sb, ": %s", strings.ReplaceAll(reason, "\n", " "))
	}
	sb.WriteByte('\n')
	for _, p := range patterns {
		sb.WriteString(p)
		sb.WriteByte('\n')
	}
	if err := store.WriteFileAtomic(w.ignorePath(), sb.Bytes(), 0o644); err != nil {
		return nil, err
	}
	w.Log.Event("scope_excluded", map[string]any{
		"patterns": patterns, "files": imp.Files, "nodes": imp.Nodes, "reason": reason,
	})
	return imp, nil
}

// ScopeRemove deletes pattern lines from .graphin/ignore. Comment lines are
// left alone even when they become orphans: they carry why a decision was
// made, and a stale comment is cheaper to read past than a lost reason.
func (w *Workspace) ScopeRemove(patterns []string) (*ScopeImpact, error) {
	b, err := os.ReadFile(w.ignorePath())
	if err != nil {
		return nil, fmt.Errorf("no exclusion file to edit: %w", err)
	}
	drop := map[string]bool{}
	for _, p := range patterns {
		drop[strings.TrimSpace(p)] = true
	}
	var out bytes.Buffer
	removed := make([]string, 0, len(patterns))
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		line := sc.Text()
		if drop[strings.TrimSpace(line)] {
			removed = append(removed, strings.TrimSpace(line))
			continue
		}
		out.WriteString(line)
		out.WriteByte('\n')
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(removed) == 0 {
		return nil, fmt.Errorf("none of those patterns are in .graphin/ignore")
	}
	if err := store.WriteFileAtomic(w.ignorePath(), out.Bytes(), 0o644); err != nil {
		return nil, err
	}
	w.Log.Event("scope_unexcluded", map[string]any{"patterns": removed})
	// What comes back is what those patterns had been holding out, which is
	// what the next bootstrap will index again.
	return w.ScopeImpactOf(removed)
}

func (w *Workspace) ignorePath() string { return filepath.Join(w.Dir, "ignore") }

// excludedPatterns lists the pattern lines currently in .graphin/ignore.
func (w *Workspace) excludedPatterns() []string {
	b, err := os.ReadFile(w.ignorePath())
	if err != nil {
		return nil
	}
	var out []string
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	return out
}

// ignoredPath asks the matcher about a file path, also honouring a pattern
// that names one of its parent directories — Walk prunes those before it ever
// reaches the file, so a file-only test would miss "dir/" style rules.
func ignoredPath(m *ignore.Matcher, rel string) bool {
	if m.Ignored(rel, false) {
		return true
	}
	segs := strings.Split(rel, "/")
	for i := 1; i < len(segs); i++ {
		if m.Ignored(strings.Join(segs[:i], "/"), true) {
			return true
		}
	}
	return false
}

// countHeadings counts ATX headings the way the markdown parser splits
// sections, without parsing: fenced code blocks are skipped so a "# comment"
// line inside a shell example is not mistaken for a section.
func countHeadings(abs string) int {
	f, err := os.Open(abs)
	if err != nil {
		return 0
	}
	defer f.Close()
	n, fenced := 0, false
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "```") || strings.HasPrefix(line, "~~~") {
			fenced = !fenced
			continue
		}
		if fenced || !strings.HasPrefix(line, "#") {
			continue
		}
		if h := strings.TrimLeft(line, "#"); h != line && strings.HasPrefix(h, " ") {
			n++
		}
	}
	return n
}

func topSegment(rel string) string {
	if i := strings.IndexByte(rel, '/'); i > 0 {
		return rel[:i]
	}
	return rootGroup
}

func extOf(rel string) string {
	if e := path.Ext(rel); e != "" {
		return e
	}
	return path.Base(rel)
}

func groupOf(m map[string]*ScopeEntry, key string) *ScopeEntry {
	e := m[key]
	if e == nil {
		e = &ScopeEntry{Name: key}
		m[key] = e
	}
	return e
}

// topEntries orders by node count, then files, then name — the last key makes
// the ordering total so equal rows do not shuffle between calls.
func topEntries(m map[string]*ScopeEntry) []ScopeEntry {
	out := make([]ScopeEntry, 0, len(m))
	for _, e := range m {
		out = append(out, *e)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Nodes != out[j].Nodes {
			return out[i].Nodes > out[j].Nodes
		}
		if out[i].Files != out[j].Files {
			return out[i].Files > out[j].Files
		}
		return out[i].Name < out[j].Name
	})
	if len(out) > scopeTopN {
		out = out[:scopeTopN]
	}
	return out
}
