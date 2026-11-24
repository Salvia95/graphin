package workspace

import (
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/Salvia95/graphin/internal/nodeid"
)

// What a response says about a unit without returning its body: a one-line
// signature, a few lines around a point of interest, and the line where it
// mentions another unit. These are what let one tool answer on its own — an
// id and a line number alone send the agent to a second call it often never
// makes (docs/tool-contract-spec.md §3.2).

// maxSignature caps a signature line, in runes.
const maxSignature = 200

// maxSigLines bounds how many source lines a signature may join.
const maxSigLines = 8

// maxRefText caps a reference line, in runes.
const maxRefText = 160

// sigCache memoizes signatures per node and file version. A signature is
// cheap to derive but needs the file read; neighbour lists ask for twenty at
// a time, often for the same few files.
type sigCache struct {
	mu sync.Mutex
	m  map[string]sigEntry
}

type sigEntry struct {
	hash string
	sig  string
}

// sigCacheMax bounds the cache. Reset on overflow rather than evicting: the
// working set of one agent session is far below it, and a reset costs only
// file reads.
const sigCacheMax = 8192

var signatures = &sigCache{m: map[string]sigEntry{}}

// Signature returns a unit's declaration as one line: the header of a
// function, method or class, a doc section's heading, a DB object's kind and
// name. A file has none. "" when the node is unknown or unreadable.
func (w *Workspace) Signature(id string) string {
	meta, ok := w.nodeMeta(id)
	if !ok {
		return ""
	}
	switch {
	case meta.Kind == nodeid.KindFile:
		return ""
	case nodeid.IsDBKind(meta.Kind):
		name := meta.DisplayName
		if name == "" {
			name = nodeid.Simple(id)
		}
		return meta.Kind + " " + name
	}
	key := w.Root + "\x00" + id
	signatures.mu.Lock()
	if e, ok := signatures.m[key]; ok && e.hash == meta.FileHash {
		signatures.mu.Unlock()
		return e.sig
	}
	signatures.mu.Unlock()

	cb, err := w.ReadCode(id)
	if err != nil {
		return ""
	}
	var sig string
	if meta.Kind == nodeid.KindSection {
		sig = firstLine(cb.Code)
	} else {
		sig = declarationLine(cb.Code)
	}
	if fresh, ok := w.nodeMeta(id); ok {
		meta = fresh
	}
	signatures.mu.Lock()
	if len(signatures.m) >= sigCacheMax {
		signatures.m = map[string]sigEntry{}
	}
	signatures.m[key] = sigEntry{hash: meta.FileHash, sig: sig}
	signatures.mu.Unlock()
	return sig
}

// declarationLine extracts the header of a code unit: leading annotations,
// decorators and comments skipped, then lines joined until the body opens
// ("{", a trailing ":" or "=>", "=" for an expression body) or maxSigLines
// lines have been taken — enough for a parameter list written one per line. Whitespace collapses to single spaces.
func declarationLine(code string) string {
	var parts []string
	for _, raw := range strings.Split(code, "\n") {
		line := strings.TrimSpace(raw)
		if len(parts) == 0 && (line == "" || isPreamble(line)) {
			continue
		}
		if i := strings.IndexByte(line, '{'); i >= 0 {
			if head := strings.TrimSpace(line[:i]); head != "" {
				parts = append(parts, head)
			}
			break
		}
		parts = append(parts, line)
		if strings.HasSuffix(line, ":") || strings.HasSuffix(line, "=>") ||
			strings.HasSuffix(line, "=") || strings.Contains(line, ") =") || len(parts) == maxSigLines {
			break
		}
	}
	return clip(strings.Join(strings.Fields(strings.Join(parts, " ")), " "), maxSignature)
}

// isPreamble reports lines that precede a declaration without being it.
func isPreamble(line string) bool {
	for _, p := range []string{"@", "//", "#", "/*", "*", "\"\"\"", "'''"} {
		if strings.HasPrefix(line, p) {
			return true
		}
	}
	return false
}

func firstLine(code string) string {
	for _, l := range strings.Split(code, "\n") {
		if t := strings.TrimSpace(l); t != "" {
			return clip(t, maxSignature)
		}
	}
	return ""
}

// Snippet returns up to 2*radius+1 lines of a unit centred on a file line,
// clipped to the unit. around <= 0 centres nothing: the unit's first lines
// are returned. from is the file line of lines[0].
func (w *Workspace) Snippet(id string, around, radius int) (from int, lines []string, ok bool) {
	cb, err := w.ReadCode(id)
	if err != nil {
		return 0, nil, false
	}
	all := strings.Split(strings.TrimRight(cb.Code, "\n"), "\n")
	n := 2*radius + 1
	start := 0
	if around > 0 {
		start = around - cb.StartLine - radius
	}
	if start > len(all)-n {
		start = len(all) - n
	}
	if start < 0 {
		start = 0
	}
	end := min(start+n, len(all))
	out := make([]string, 0, end-start)
	for _, l := range all[start:end] {
		out = append(out, clip(strings.TrimRight(l, " \t\r"), maxRefText))
	}
	return cb.StartLine + start, out, true
}

// RefLine finds where unit holder mentions name: the first line of its body,
// after its own declaration line, holding name as a whole identifier. It is
// how a neighbour list says where a call happens when the graph only knows
// that it does. Not found — an implicit reference such as ORM table access,
// or inheritance written on the declaration line — is reported as such,
// never as a guess.
func (w *Workspace) RefLine(holder, name string) (line int, text string, ok bool) {
	if name == "" {
		return 0, "", false
	}
	cb, err := w.ReadCode(holder)
	if err != nil {
		return 0, "", false
	}
	for i, raw := range strings.Split(cb.Code, "\n") {
		if i == 0 && strings.Count(cb.Code, "\n") > 0 {
			continue
		}
		if containsIdent(raw, name) {
			return cb.StartLine + i, clip(strings.TrimSpace(raw), maxRefText), true
		}
	}
	// A one-line unit, or a mention only on the declaration line (extends,
	// implements, a parameter type).
	if first, _, _ := strings.Cut(cb.Code, "\n"); containsIdent(first, name) {
		return cb.StartLine, clip(strings.TrimSpace(first), maxRefText), true
	}
	return 0, "", false
}

// containsIdent reports name in s with no identifier character on either
// side, so "Close" does not match "CloseAll" or "isClosed".
func containsIdent(s, name string) bool {
	for off := 0; ; {
		i := strings.Index(s[off:], name)
		if i < 0 {
			return false
		}
		i += off
		before, _ := utf8.DecodeLastRuneInString(s[:i])
		after, _ := utf8.DecodeRuneInString(s[i+len(name):])
		if !isIdentRune(before) && !isIdentRune(after) {
			return true
		}
		off = i + len(name)
	}
}

func isIdentRune(r rune) bool {
	return r == '_' || r == '$' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

func clip(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	r := []rune(s)
	return string(r[:max]) + "…"
}

// NodeLines returns the file lines a unit spans.
func (w *Workspace) NodeLines(id string) (from, to int, ok bool) {
	cb, err := w.ReadCode(id)
	if err != nil {
		return 0, 0, false
	}
	return cb.StartLine, cb.EndLine, true
}
