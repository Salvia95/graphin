package parse

import (
	pathpkg "path"
	"strings"
	"unicode"

	"github.com/Salvia95/graphin/internal/lexical"
	"github.com/Salvia95/graphin/internal/nodeid"
	"github.com/zeebo/blake3"
)

// extractMarkdown splits an ATX-headed markdown file into one file node plus
// one node per heading (docs/markdown-spec.md §3).
//
// The file node survives (D1): file discovery already works through it, and it
// is the root of the contains hierarchy. Its span stays the whole file so
// read_code on it is unchanged — but its body tokens carry only the preamble
// (D5), or section 1's text would enter the BM25 index twice.
//
// Section spans are flat (D3): a section ends at the NEXT heading of any
// level, so `### 13.1` is not inside `## 13`'s span. Requesting a parent and a
// child together therefore never returns the same bytes twice; the hierarchy
// lives in contains edges instead.
func extractMarkdown(src []byte, res *FileResult) {
	base := pathpkg.Base(res.RelPath)
	if dir := pathpkg.Dir(res.RelPath); dir != "." && dir != "/" {
		res.Package = strings.ReplaceAll(dir, "/", ".")
	}

	heads := scanHeadings(src)

	// Preamble: everything before the first heading. With no headings at all
	// this is the whole file, which reproduces the old plain-file behaviour.
	preEnd := len(src)
	if len(heads) > 0 {
		preEnd = int(heads[0].start)
	}
	// Slugs are resolved for every heading first: the parent lookup reads the
	// parent's slug, and its disambiguation suffix must already be decided.
	slugs := map[string]int{}
	for i := range heads {
		heads[i].slug = uniqueSlug(slugify(heads[i].text), slugs)
	}

	// children[parentID] — the contains edges, built here because the parse
	// already knows every target exactly (spec §3.5). The file node is the
	// root for any heading with no shallower heading before it.
	children := map[string][]string{}
	for i := range heads {
		id := res.RelPath + "#" + heads[i].slug
		parent := res.RelPath
		if j := parentIndex(heads, i); j >= 0 {
			parent = res.RelPath + "#" + heads[j].slug
		}
		children[parent] = append(children[parent], id)
	}

	file := Node{
		ID:          res.RelPath,
		DisplayName: base,
		SimpleName:  base,
		Kind:        nodeid.KindFile,
		StartByte:   0,
		EndByte:     uint32(len(src)),
		// The preamble, not the whole file. This node's *indexed* content is
		// the preamble (D5), and the 2-Track diff uses this hash to decide
		// whether to re-tokenize and re-embed — so hashing anything wider
		// would re-embed the file node every time any section changed.
		//
		// It also makes the upgrade land: a workspace indexed before sections
		// existed recorded the whole-file hash here, so this node now reads
		// as changed exactly once and drops its stale whole-file tokens.
		Hash:     blake3.Sum256(src[:preEnd]),
		Contains: children[res.RelPath],
	}
	// Non-nil even when empty: attachBodyTokens skips nodes that already
	// carry tokens, and nil would make it re-tokenize the whole file.
	file.BodyTokens = capTokens(src[:preEnd])
	res.Nodes = append(res.Nodes, file)

	for i, h := range heads {
		end := uint32(len(src))
		if i+1 < len(heads) {
			end = heads[i+1].start
		}
		id := res.RelPath + "#" + h.slug
		res.Nodes = append(res.Nodes, Node{
			ID:          id,
			DisplayName: h.text,
			SimpleName:  h.text,
			Kind:        nodeid.KindSection,
			Container:   containerOf(heads, i),
			StartByte:   h.start,
			EndByte:     end,
			Hash:        blake3.Sum256(src[h.start:end]),
			Contains:    children[id],
		})
	}
}

// capTokens is TokenizeCapped with a non-nil result. attachBodyTokens skips
// nodes that already carry tokens, so an empty preamble has to be an empty
// slice — nil would make it re-tokenize the whole file and undo D5.
func capTokens(b []byte) []string {
	t := lexical.TokenizeCapped(string(b), maxBodyTokens)
	if t == nil {
		return []string{}
	}
	return t
}

// heading is one ATX heading found outside fenced code.
type heading struct {
	level int
	text  string
	start uint32 // byte offset of the '#'
	slug  string // filled by extractMarkdown as it disambiguates
}

// scanHeadings walks lines tracking fenced code blocks. The fence tracking is
// not cosmetic: this repository's own docs quote shell scripts, and 26 of 302
// candidate headings (9%) are `#` comments inside fences (spec §3.2).
func scanHeadings(src []byte) []heading {
	var out []heading
	var fenceChar byte
	fenceLen := 0
	off := 0
	for off < len(src) {
		nl := indexByteFrom(src, off, '\n')
		lineEnd := nl
		if lineEnd < 0 {
			lineEnd = len(src)
		}
		line := src[off:lineEnd]

		if ch, n, ok := fenceMarker(line); ok {
			if fenceLen == 0 {
				fenceChar, fenceLen = ch, n
			} else if ch == fenceChar && n >= fenceLen {
				fenceChar, fenceLen = 0, 0
			}
		} else if fenceLen == 0 {
			if lvl, txt, ok := atxHeading(line); ok {
				out = append(out, heading{level: lvl, text: txt, start: uint32(off)})
			}
		}

		if nl < 0 {
			break
		}
		off = nl + 1
	}
	return out
}

func indexByteFrom(b []byte, from int, c byte) int {
	for i := from; i < len(b); i++ {
		if b[i] == c {
			return i
		}
	}
	return -1
}

// fenceMarker reports a ``` or ~~~ run at line start (up to 3 spaces indent),
// returning the fence character and its length.
func fenceMarker(line []byte) (byte, int, bool) {
	i := 0
	for i < len(line) && i < 4 && line[i] == ' ' {
		i++
	}
	if i >= len(line) || i == 4 {
		return 0, 0, false
	}
	c := line[i]
	if c != '`' && c != '~' {
		return 0, 0, false
	}
	n := 0
	for i+n < len(line) && line[i+n] == c {
		n++
	}
	if n < 3 {
		return 0, 0, false
	}
	return c, n, true
}

// atxHeading matches `# text` … `###### text` with up to 3 spaces of indent,
// stripping an optional closing run of '#'.
func atxHeading(line []byte) (int, string, bool) {
	i := 0
	for i < len(line) && i < 4 && line[i] == ' ' {
		i++
	}
	if i >= len(line) || i == 4 || line[i] != '#' {
		return 0, "", false
	}
	lvl := 0
	for i+lvl < len(line) && line[i+lvl] == '#' {
		lvl++
	}
	if lvl > 6 {
		return 0, "", false
	}
	rest := line[i+lvl:]
	// A heading needs whitespace after the hashes: "#hashtag" is not one.
	if len(rest) == 0 {
		return lvl, "", true
	}
	if rest[0] != ' ' && rest[0] != '\t' {
		return 0, "", false
	}
	txt := strings.TrimSpace(strings.TrimRight(strings.TrimSpace(string(rest)), "#"))
	return lvl, strings.TrimSpace(txt), true
}

// containerOf returns the enclosing section's slug: the nearest PRECEDING
// heading with a strictly shallower level (spec §3.5). Defining it as
// "exactly one level up" or "h1 is the root" breaks on inputs this repository
// already has — one level skip, and three files that do not start at h1.
func containerOf(heads []heading, i int) string {
	if j := parentIndex(heads, i); j >= 0 {
		return heads[j].slug
	}
	return ""
}

// parentIndex returns the index of the enclosing heading, or -1 when the
// section hangs directly off the file node.
func parentIndex(heads []heading, i int) int {
	for j := i - 1; j >= 0; j-- {
		if heads[j].level < heads[i].level {
			return j
		}
	}
	return -1
}

// slugify reproduces GitHub's heading anchors, so a link written in a
// document (`…md#13-버저닝`) and the node ID are the same string.
//
// The rules were read off GitHub's own rendering of this repository rather
// than guessed (spec §3.3):
//
//	"# 플러그인 배포 스펙 — 바이너리 …"  → 플러그인-배포-스펙--바이너리-…
//	"### 3.4 부수 개선 ✅"              → 34-부수-개선-
//	"#### 4.1 darwin/arm64 핀 …"      → 41-darwinarm64-핀-…
//
// Two consequences are easy to get wrong and both matter here: consecutive
// hyphens are NOT collapsed, and leading/trailing hyphens are NOT trimmed.
// A dropped character leaves nothing behind, but the spaces that surrounded
// it each still become a hyphen — which is why 34% of this repository's
// headings would mismatch under a collapsing implementation.
func slugify(h string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(h) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-':
			b.WriteRune(r)
		case unicode.IsSpace(r):
			b.WriteRune('-')
		}
	}
	return b.String()
}

// uniqueSlug disambiguates repeats the way GitHub does, in document order.
// Measured 0 collisions in this repository — the rule exists for other trees.
func uniqueSlug(s string, seen map[string]int) string {
	if s == "" {
		s = "section"
	}
	n := seen[s]
	seen[s]++
	if n == 0 {
		return s
	}
	return s + "-" + itoa(n)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
