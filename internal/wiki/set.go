package wiki

import (
	pathpkg "path"
	"regexp"
	"strings"

	"github.com/Salvia95/graphin/internal/parse"
)

// entryRe matches one set line: `- [title](target) — summary`. The summary is
// optional here and validated separately, because an entry without one is a
// specific, reportable mistake rather than an unrecognized line.
var entryRe = regexp.MustCompile(`^\s*-\s+\[([^\]]+)\]\(([^)\s]+)\)\s*(.*)$`)

// headingRe matches an ATX heading and captures its level and text.
var headingRe = regexp.MustCompile(`^\s{0,3}(#{1,6})\s+(.*)$`)

// fenceRe matches a code fence opener/closer.
var fenceRe = regexp.MustCompile("^\\s{0,3}(`{3,}|~{3,})")

// ParseSet reads one knowledge set file.
//
// Group node IDs are not derived here. The set file is markdown, so the
// parser already assigns every heading the exact ID the index uses, and
// re-deriving the slug rule is how the CI guard for these files acquired its
// longest comment: a second implementation disagreed with the first on
// non-ASCII headings and both accepted and rejected the wrong entries.
// Matching on byte offset instead of on title also keeps two groups with the
// same name from collapsing into one.
func ParseSet(relPath string, src []byte) (*Set, error) {
	fm, body := splitFrontmatter(src)
	front := parseFrontmatter(fm)

	set := &Set{
		Name:          strings.TrimSuffix(pathpkg.Base(relPath), ".md"),
		RelPath:       relPath,
		Title:         front.Get("title"),
		Description:   front.Get("description"),
		Tags:          front.List("tags"),
		StaleAfter:    front.Get("stale_after"),
		Roles:         front.List("roles"),
		Prerequisites: front.List("prerequisites"),
		Mode:          Mode(front.Get("mode")),
	}
	if set.Mode == "" {
		set.Mode = ModeLive
	}

	// Offsets are needed against the whole file, so the body scan starts at
	// the offset the frontmatter ended on rather than at zero.
	bodyStart := len(src) - len(body)
	dir := pathpkg.Dir(relPath)

	sections := sectionIDsByOffset(relPath, src)

	var (
		cur      *Group
		intro    []string
		lastLine *strings.Builder // summary being accumulated
	)
	flush := func() {
		if cur != nil {
			set.Groups = append(set.Groups, *cur)
			cur = nil
		}
	}

	inFence := false
	var fence string
	off := bodyStart
	for lineNo, raw := range strings.Split(string(body), "\n") {
		line := strings.TrimRight(raw, "\r")
		lineStart := off
		off += len(raw) + 1

		if m := fenceRe.FindStringSubmatch(line); m != nil {
			// Fences matter: this repository's own documents quote example
			// sets, and their sample entries must not become real ones.
			if !inFence {
				inFence, fence = true, m[1]
			} else if strings.HasPrefix(m[1], fence[:1]) && len(m[1]) >= len(fence) {
				inFence, fence = false, ""
			}
			lastLine = nil
			continue
		}
		if inFence {
			continue
		}

		if m := headingRe.FindStringSubmatch(line); m != nil {
			lastLine = nil
			if len(m[1]) == 1 {
				// A declared title wins: the heading is prose the author may
				// rewrite, the field is what other documents were told.
				if set.Title == "" {
					set.Title = strings.TrimSpace(strings.TrimRight(strings.TrimSpace(m[2]), "#"))
				}
				continue
			}
			flush()
			cur = &Group{
				Title:  strings.TrimSpace(strings.TrimRight(strings.TrimSpace(m[2]), "#")),
				NodeID: sections[uint32(lineStart)],
			}
			continue
		}

		if m := entryRe.FindStringSubmatch(line); m != nil {
			target, anchor, _ := strings.Cut(m[2], "#")
			id := pathpkg.Join(dir, target)
			if anchor != "" {
				id += "#" + anchor
			}
			if cur == nil {
				// Entries before any `##`: keep them rather than drop them.
				// Losing an entry silently is the one outcome this file
				// cannot have.
				cur = &Group{Title: "", NodeID: sections[uint32(bodyStart)]}
			}
			cur.Entries = append(cur.Entries, Entry{
				Title:   strings.TrimSpace(m[1]),
				NodeID:  id,
				Summary: trimSummary(m[3]),
				Line:    lineNo + 1,
			})
			sb := &strings.Builder{}
			sb.WriteString(cur.Entries[len(cur.Entries)-1].Summary)
			lastLine = sb
			continue
		}

		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			lastLine = nil
			continue
		}
		// An indented, non-empty line continues the previous entry's summary.
		// The format puts summaries on the line below the link often enough
		// that treating continuations as noise would drop most of them.
		if lastLine != nil && (strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")) {
			if lastLine.Len() > 0 {
				lastLine.WriteString(" ")
			}
			lastLine.WriteString(trimSummary(trimmed))
			cur.Entries[len(cur.Entries)-1].Summary = lastLine.String()
			continue
		}
		if cur == nil {
			intro = append(intro, trimmed)
		}
		lastLine = nil
	}
	flush()
	set.Intro = strings.Join(intro, " ")
	return set, nil
}

// trimSummary strips the separator that joins a link to its sentence.
func trimSummary(s string) string {
	s = strings.TrimSpace(s)
	for _, sep := range []string{"—", "–", "-", ":"} {
		if strings.HasPrefix(s, sep) {
			return strings.TrimSpace(strings.TrimPrefix(s, sep))
		}
	}
	return s
}

// sectionIDsByOffset maps each heading's byte offset to the node ID the
// index gives it. A parse failure yields an empty map rather than an error:
// group IDs are a convenience for partial reads, and a set whose entries all
// resolve is still usable without them.
func sectionIDsByOffset(relPath string, src []byte) map[uint32]string {
	out := map[uint32]string{}
	res, err := parse.File(relPath, src)
	if err != nil {
		return out
	}
	for _, n := range res.Nodes {
		out[n.StartByte] = n.ID
	}
	return out
}
