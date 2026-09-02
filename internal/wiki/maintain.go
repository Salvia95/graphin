package wiki

import (
	"fmt"
	"os"
	pathpkg "path"
	"path/filepath"
	"strings"
	"unicode"
)

// Maintenance is the part of curating a set that makes no new claim.
//
// Three of the decision kinds a person is asked to end — a dangling entry, a
// drifted summary, a set nobody opens — are answered by finding, reading and
// rewording, not by asserting anything the documents do not already say. That
// is the work an agent does best and the work with the least to lose, so it
// is the first of the wiki's chores handed over (docs/wiki-plan.md P2).
//
// The handover rests on three things. Every write goes through a mechanical
// judgement first, the way a glossary candidate does, so a broken anchor or a
// duplicate never reaches the file. Every write is surgical, so the review is
// an ordinary diff (EditSetFront's rule, kept here). And every write marks the
// set `reviewed: false`, which is the user's chosen control: apply now, review
// after. The mark travels with each served section and sits in the queue
// until a person ends it.

// Rules for a set entry: the admission test's counterpart for the reading
// list. Anything here can be decided from files alone, which is what makes it
// a gate rather than a note for the reviewer.
const (
	// RuleAnchor: no such heading in that file.
	RuleAnchor RuleID = "anchor"
	// RuleStructure: the node is code. The index already answers it, and a
	// set that names it duplicates what search returns (package comment).
	RuleStructure RuleID = "structure"
	// RuleDuplicate: the set already lists this node.
	RuleDuplicate RuleID = "duplicate"
	// RuleSummary: an entry without a sentence is a table-of-contents row.
	RuleSummary RuleID = "summary"
	// RuleFormat: the text would not survive the file format — a title with
	// "]", a node id with a space — and an entry the parser cannot read back
	// is an entry silently lost, pin and mark already written.
	RuleFormat RuleID = "format"
)

// JudgeEntry runs the mechanical rules over one entry as it would stand after
// an edit. current is the node the entry lists now, so a repoint is not
// flagged as a duplicate of itself; it is "" for an entry being added.
func JudgeEntry(root string, s *Set, current, nodeID, title, summary string) Verdict {
	var v Verdict
	if strings.ContainsAny(title, "]\n\r") || strings.TrimSpace(title) == "" {
		v.Findings = append(v.Findings, Finding{RuleFormat, "a title is one line and cannot contain ']'"})
	}
	if strings.ContainsAny(nodeID, " \t\n\r)") {
		v.Findings = append(v.Findings, Finding{RuleFormat, "a node id cannot contain whitespace or ')'"})
	}
	rel, _, _ := strings.Cut(nodeID, "#")
	if !strings.HasSuffix(strings.ToLower(rel), ".md") {
		v.Findings = append(v.Findings, Finding{RuleStructure,
			fmt.Sprintf("%s is code, not documentation — the index already answers it", nodeID)})
	}
	if _, ok := NewHasher(root).Pin(nodeID); !ok {
		v.Findings = append(v.Findings, Finding{RuleAnchor,
			fmt.Sprintf("%s does not resolve — no such heading in that file", nodeID)})
	}
	if nodeID != current && s.cites(nodeID) {
		v.Findings = append(v.Findings, Finding{RuleDuplicate,
			fmt.Sprintf("%s already lists %s", s.Name, nodeID)})
	}
	if strings.TrimSpace(summary) == "" {
		v.Findings = append(v.Findings, Finding{RuleSummary,
			"an entry without a sentence is a table-of-contents row"})
	}
	return v
}

// EntryEdit is what may change on one entry. An empty field keeps what the
// author wrote — the same convention as approving a candidate.
type EntryEdit struct {
	// NodeID is the new target: the answer to a dangling entry.
	NodeID string
	// Title is the link text.
	Title string
	// Summary is the one line the catalogue offers the section by: the
	// answer to a drifted entry once the section has been re-read.
	Summary string
}

// EditResult says what an edit did and where.
type EditResult struct {
	File string `json:"file"`
	Set  string `json:"set"`
	// NodeID is the entry's node after the edit.
	NodeID string `json:"node_id,omitempty"`
	Line   int    `json:"line,omitempty"`
	// Repinned is true when the entry's pin now records the current section.
	Repinned bool `json:"repinned"`
	// AlsoIn names other sets that list the same node. Not a rule — a
	// section may belong to several sets by design — but a reviewer wants
	// to know the edit touched shared ground.
	AlsoIn []string `json:"also_in,omitempty"`
}

// EditSetEntry rewrites one entry in place, judges it first, and marks the
// set unreviewed. A blocked verdict writes nothing and is not an error: the
// caller renders the findings, the way wiki_propose does.
func EditSetEntry(root, set, nodeID string, e EntryEdit) (EditResult, Verdict, error) {
	store, err := Load(root)
	if err != nil {
		return EditResult{}, Verdict{}, err
	}
	target, ok := store.Sets[set]
	if !ok {
		return EditResult{}, Verdict{}, fmt.Errorf("%w: %s", ErrNoSet, set)
	}
	res := EditResult{File: target.RelPath, Set: set}
	var entry *Entry
	for _, g := range target.Groups {
		for i := range g.Entries {
			if g.Entries[i].NodeID == nodeID {
				entry = &g.Entries[i]
				break
			}
		}
		if entry != nil {
			break
		}
	}
	if entry == nil {
		return res, Verdict{}, fmt.Errorf("%w: %s does not list %s", ErrNoEntry, set, nodeID)
	}

	newID, title := e.NodeID, e.Title
	// A summary is one line in the file whatever shape it arrived in.
	summary := strings.Join(strings.Fields(e.Summary), " ")
	if newID == "" {
		newID = nodeID
	}
	if title == "" {
		title = entry.Title
	}
	if summary == "" {
		summary = entry.Summary
	}
	res.NodeID, res.Line = newID, entry.Line

	v := JudgeEntry(root, target, nodeID, newID, title, summary)
	if v.Blocked() {
		return res, v, nil
	}

	path := filepath.Join(root, filepath.FromSlash(target.RelPath))
	src, err := os.ReadFile(path)
	if err != nil {
		return res, v, err
	}
	lines := strings.Split(string(src), "\n")
	start := entry.Line - 1
	if start < 0 || start >= len(lines) || !entryRe.MatchString(lines[start]) {
		return res, v, fmt.Errorf("%s:%d is not the entry line the parser reported", target.RelPath, entry.Line)
	}
	end := entrySpanEnd(lines, start)

	newSummary := ""
	if e.Summary != "" {
		newSummary = summary
	}
	repl := renderEntry(lines[start:end], pathpkg.Dir(target.RelPath), title, newID,
		newID != nodeID, newSummary)
	lines = append(lines[:start:start], append(repl, lines[end:]...)...)

	out, err := markUnreviewed(strings.Join(lines, "\n"))
	if err != nil {
		return res, v, err
	}
	// Read it back before writing it. The rules above cover the shapes that
	// are known to break the line; this covers the ones that are not, so an
	// entry can never vanish from the set with its pin and mark in place.
	if again, perr := ParseSet(target.RelPath, []byte(out)); perr != nil || !again.cites(newID) {
		return res, v, fmt.Errorf("the edited entry would not parse back — nothing written")
	}
	if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
		return res, v, err
	}

	// Pins follow the edit. The old node's pin goes unless another entry in
	// the set still names it; the new node is pinned as it stands, because
	// the summary was just written (or confirmed) against that text.
	if newID != nodeID {
		still := false
		for _, other := range target.Entries() {
			if other.NodeID == nodeID && other.Line != entry.Line {
				still = true
			}
		}
		if !still {
			store.Pins.Delete(set, nodeID)
		}
	}
	if cur, ok := NewHasher(root).Pin(newID); ok {
		store.Pins.Set(set, newID, cur)
		res.Repinned = true
	}
	if err := store.Pins.Save(store.PinsPath()); err != nil {
		return res, v, err
	}
	for _, other := range store.SetList() {
		if other.Name != set && other.cites(newID) {
			res.AlsoIn = append(res.AlsoIn, other.Name)
		}
	}
	return res, v, nil
}

// ConfirmEntry records that a drifted entry was re-read and its summary still
// holds: the set is marked for review and the pin moves to the current text.
// It is RepinEntry plus the mark, and the mark is the difference between a
// person's repin and an agent's.
//
// A dangling entry cannot be confirmed — there is no text to have re-read —
// and that is a verdict, not a silent no-op: RepinEntry reports the problem
// without failing, and an agent told "applied" would move on.
func ConfirmEntry(root, set, nodeID string) (EditResult, Verdict, error) {
	store, err := Load(root)
	if err != nil {
		return EditResult{}, Verdict{}, err
	}
	target, ok := store.Sets[set]
	if !ok {
		return EditResult{}, Verdict{}, fmt.Errorf("%w: %s", ErrNoSet, set)
	}
	res := EditResult{File: target.RelPath, Set: set, NodeID: nodeID}
	if !target.cites(nodeID) {
		return res, Verdict{}, fmt.Errorf("%w: %s does not list %s", ErrNoEntry, set, nodeID)
	}
	var v Verdict
	if _, ok := NewHasher(root).Pin(nodeID); !ok {
		v.Findings = append(v.Findings, Finding{RuleAnchor,
			fmt.Sprintf("%s does not resolve — repoint it; there is nothing to confirm", nodeID)})
		return res, v, nil
	}
	// Mark first, then pin. A failure between the two leaves a marked set
	// with a stale pin, which the queue still shows as drift; the other
	// order would leave an agent's pin with no mark at all.
	if err := markSet(root, target); err != nil {
		return res, v, err
	}
	pin, err := RepinEntry(root, set, nodeID)
	if err != nil {
		return res, v, err
	}
	res.Repinned = pin.Wrote && len(pin.Problems) == 0
	return res, v, nil
}

// DescribeSet rewrites the catalogue line — the answer to a set that is
// offered and never opened — and marks the set for review.
func DescribeSet(root, set, description string) (EditResult, Verdict, error) {
	var v Verdict
	d := strings.TrimSpace(description)
	switch {
	case d == "":
		v.Findings = append(v.Findings, Finding{RuleSummary, "a catalogue line cannot be empty"})
	case strings.ContainsAny(d, "\n\r"):
		v.Findings = append(v.Findings, Finding{RuleSummary, "a catalogue line is one line"})
	}
	if v.Blocked() {
		return EditResult{Set: set}, v, nil
	}
	store, err := Load(root)
	if err != nil {
		return EditResult{Set: set}, v, err
	}
	target, ok := store.Sets[set]
	if !ok {
		return EditResult{Set: set}, v, fmt.Errorf("%w: %s", ErrNoSet, set)
	}
	res := EditResult{File: target.RelPath, Set: set}
	path := filepath.Join(root, filepath.FromSlash(target.RelPath))
	src, err := os.ReadFile(path)
	if err != nil {
		return res, v, err
	}
	fm, body := splitFrontmatter(src)
	var lines []string
	if fm != nil {
		lines = strings.Split(strings.TrimRight(string(fm), "\n"), "\n")
	} else {
		body = src
	}
	lines = setScalar(lines, "description", frontScalar(d))
	lines = setScalar(lines, "reviewed", "false")
	out := "---\n" + strings.Join(lines, "\n") + "\n---\n" + string(body)
	return res, v, os.WriteFile(path, []byte(out), 0o644)
}

// markSet writes reviewed: false into a set file, adding a header if there
// is none. See markUnreviewed for why an agent write gets a header where a
// person's edit would be refused.
func markSet(root string, s *Set) error {
	path := filepath.Join(root, filepath.FromSlash(s.RelPath))
	src, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	out, err := markUnreviewed(string(src))
	if err != nil {
		return err
	}
	return os.WriteFile(path, []byte(out), 0o644)
}

// entrySpanEnd finds where an entry's lines stop: at the first blank line or
// the first line back at column zero, which is the parser's continuation rule.
func entrySpanEnd(lines []string, start int) int {
	j := start + 1
	for j < len(lines) {
		l := strings.TrimRight(lines[j], "\r")
		if strings.TrimSpace(l) == "" || !(strings.HasPrefix(l, " ") || strings.HasPrefix(l, "\t")) {
			break
		}
		j++
	}
	return j
}

// renderEntry rewrites an entry's lines, changing only what was asked.
//
// The link text and target are rewritten inside the first line and the rest
// of that line is kept byte for byte; the continuation lines are replaced
// only when a new summary was given. Re-wrapping a summary nobody changed
// would turn a one-word diff into a paragraph, which is the rewrite this
// editor exists to avoid.
func renderEntry(orig []string, setDir, title, nodeID string, retarget bool, newSummary string) []string {
	first := orig[0]
	open := strings.Index(first, "[")
	mid := strings.Index(first, "](")
	closeAt := strings.Index(first[mid:], ")") + mid
	target := first[mid+2 : closeAt]
	if retarget {
		target = relativeTarget(setDir, nodeID)
	}
	tail := first[closeAt+1:]
	head := first[:open] + "[" + title + "](" + target + ")"
	if newSummary == "" {
		return append([]string{head + tail}, orig[1:]...)
	}

	// Keep the author's separator and their choice of inline versus
	// continuation-line summary.
	sep := "—"
	rest := strings.TrimSpace(tail)
	for _, s := range []string{"—", "–", "-", ":"} {
		if strings.HasPrefix(rest, s) {
			sep = s
			break
		}
	}
	inline := trimSummary(rest) != ""
	if inline {
		return []string{head + " " + sep + " " + newSummary}
	}
	return append([]string{head + " " + sep}, wrapIndented(newSummary, "  ", 80)...)
}

// relativeTarget writes a node id the way an entry links it: relative to the
// set file's directory, anchor kept.
func relativeTarget(setDir, nodeID string) string {
	file, anchor, _ := strings.Cut(nodeID, "#")
	rel, err := filepath.Rel(filepath.FromSlash(setDir), filepath.FromSlash(file))
	if err != nil {
		rel = file
	}
	out := filepath.ToSlash(rel)
	if anchor != "" {
		out += "#" + anchor
	}
	return out
}

// wrapIndented breaks a summary into continuation lines at word boundaries,
// counting East Asian characters as two columns the way a terminal does.
func wrapIndented(text, indent string, width int) []string {
	var out []string
	var line strings.Builder
	cols := 0
	for _, w := range strings.Fields(text) {
		ww := displayWidth(w)
		if cols > 0 && len(indent)+cols+1+ww > width {
			out = append(out, indent+line.String())
			line.Reset()
			cols = 0
		}
		if cols > 0 {
			line.WriteString(" ")
			cols++
		}
		line.WriteString(w)
		cols += ww
	}
	if cols > 0 {
		out = append(out, indent+line.String())
	}
	return out
}

func displayWidth(s string) int {
	n := 0
	for _, r := range s {
		if unicode.Is(unicode.Hangul, r) || unicode.Is(unicode.Han, r) ||
			unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r) {
			n += 2
		} else {
			n++
		}
	}
	return n
}

// markUnreviewed sets `reviewed: false` in a set file's frontmatter.
//
// A file with no header gets one, which EditSetFront refuses to do. The
// difference is who is writing: a person editing a catalogue line can decide
// that a header is not wanted, but an agent's write is not allowed to land
// unmarked, so the header is the lesser harm.
func markUnreviewed(src string) (string, error) {
	fm, body := splitFrontmatter([]byte(src))
	if fm == nil {
		return "---\nreviewed: false\n---\n" + src, nil
	}
	lines := strings.Split(strings.TrimRight(string(fm), "\n"), "\n")
	lines = setScalar(lines, "reviewed", "false")
	return "---\n" + strings.Join(lines, "\n") + "\n---\n" + string(body), nil
}
