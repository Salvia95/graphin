package wiki

import (
	"sort"
	"strings"
)

// minTaskMatches is how many distinct task words a set must contain before a
// task alone pulls it in.
//
// One is too loose: a single shared common word would attach any set to any
// task, and a manifest that names everything teaches a reader to skip it.
// The exception below — a direct hit on the set's own name — exists because
// "how do I cut a release" naming the `release` set is not a coincidence.
const minTaskMatches = 2

// Selection is the outcome of matching a task to the wiki.
type Selection struct {
	// Sets is the reading list, prerequisites first.
	Sets []*Set
	// Terms are glossary entries the task's own wording touched.
	Terms []*Term
	// Pushed names the sets included because the role always gets them.
	Pushed []string
	// Matched names the sets the task text pulled in.
	Matched []string
	// Missing names prerequisites that no set defines — an authoring bug,
	// surfaced rather than swallowed.
	Missing []string
}

// Empty reports a coverage miss: nothing in the wiki applies to this work.
//
// Terms count. A task that used a project word and got its definition was
// answered, even if no set matched.
//
// This is a normal, successful answer, not a failure. Every agent is gated,
// so most preflights for most tasks must return nothing quickly — and each
// one that does is a signal about where the wiki is thin.
func (s Selection) Empty() bool { return len(s.Sets) == 0 && len(s.Terms) == 0 }

// Select decides which sets a piece of work needs.
//
// Two mechanisms, deliberately different in kind. Role tagging is a standing
// decision by the wiki's authors that a class of agent always needs something
// — the reader cannot be expected to know it is missing. Task matching is a
// guess about this particular job, and it has to be conservative because a
// wrong guess costs the reader attention on every delegation.
func (st *Store) Select(role, task string) Selection {
	var sel Selection
	wanted := map[string]bool{}

	for _, s := range st.ForRole(role) {
		if !wanted[s.Name] {
			wanted[s.Name] = true
			sel.Pushed = append(sel.Pushed, s.Name)
		}
	}

	for _, s := range st.SetList() {
		if wanted[s.Name] {
			continue
		}
		if matchesTask(s, task) {
			wanted[s.Name] = true
			sel.Matched = append(sel.Matched, s.Name)
		}
	}

	names := make([]string, 0, len(wanted))
	for n := range wanted {
		names = append(names, n)
	}
	sort.Strings(names)

	sel.Sets, sel.Missing = st.Expand(names)
	sel.Terms = st.termsIn(task)
	return sel
}

// termsIn finds glossary entries the task's own wording touched, in name
// order. Matching is on the canonical form and its aliases only: a definition
// offered because the task shared a word with its body would be noise, and
// noise in a glossary is worse than absence — it teaches the reader to skip
// the whole block.
func (st *Store) termsIn(task string) []*Term {
	if strings.TrimSpace(task) == "" {
		return nil
	}
	names := make([]string, 0, len(st.Terms))
	for n := range st.Terms {
		names = append(names, n)
	}
	sort.Strings(names)

	var out []*Term
	for _, n := range names {
		t := st.Terms[n]
		if t.Status == StatusDraft {
			continue
		}
		keys := keySet(strings.Join(append([]string{t.Canonical}, t.Aliases...), " "))
		if countMatchingWords(task, keys) > 0 {
			out = append(out, t)
		}
	}
	return out
}

// matchesTask scores one set against the task description.
func matchesTask(s *Set, task string) bool {
	// The set's own name is a stronger signal than any single label word: a
	// task that says "release" and a set called "release" are about the same
	// thing far more often than not. Both the filename and the heading count,
	// because one is frequently an ASCII slug for the other and a reader
	// naming their work will have used whichever reads naturally to them.
	if countMatchingWords(task, keySet(s.Name+" "+s.Title)) > 0 {
		return true
	}
	return countMatchingWords(task, keySet(setText(s))) >= minTaskMatches
}

// setText is the set's labels: its name, the paragraph saying what it is for,
// its group titles and its entry titles.
//
// Entry summaries are deliberately left out, and that was learned rather than
// assumed. Summaries are prose written to explain a section, so a set of any
// size accumulates enough ordinary words to share two with almost any task —
// "the user has to fix something" pulled a release set into a task about
// users. Labels are chosen to name a subject, so they discriminate; prose is
// written to be read, so it does not.
func setText(s *Set) string {
	var b strings.Builder
	b.WriteString(s.Name)
	b.WriteString(" ")
	b.WriteString(s.Title)
	b.WriteString(" ")
	b.WriteString(s.Summary())
	for _, g := range s.Groups {
		b.WriteString(" ")
		b.WriteString(g.Title)
		for _, e := range g.Entries {
			b.WriteString(" ")
			b.WriteString(e.Title)
		}
	}
	return b.String()
}

// Manifest is what a delegation carries: enough to decide, not enough to
// read. Set bodies are deliberately absent — the point of the catalogue is
// that the delegate loads only what it turns out to need.
//
// Glossary terms are the exception, and carry their definitions inline. A
// definition is one paragraph, and a reader who has to fetch it will not:
// the failure a glossary prevents is using the wrong word without noticing,
// which is precisely the state in which nobody goes looking.
type Manifest struct {
	Sets    []ManifestSet
	Terms   []ManifestTerm
	Token   string
	Missing []string
}

// ManifestTerm is one glossary entry as delivered.
type ManifestTerm struct {
	Canonical  string
	Aliases    []string
	Definition string
	Confusions []Confusion
}

// ManifestSet is one set as it appears in a catalogue.
type ManifestSet struct {
	Name string
	// NodeID of the set file itself, so a reader that wants the whole
	// catalogue can load it in one call.
	NodeID string
	// Summary is the set's opening prose, trimmed to one line.
	Summary string
	Groups  []ManifestGroup
	// Entries counts what resolving this set would bring in, so a reader can
	// tell a two-line set from a thirty-line one before asking for it.
	Entries int
}

// ManifestGroup names one group and its size.
type ManifestGroup struct {
	Title   string
	NodeID  string
	Entries int
}

// Manifest renders a selection as a catalogue and signs the wiki state it was
// produced from.
func (st *Store) Manifest(sel Selection, secret []byte) Manifest {
	m := Manifest{Missing: sel.Missing, Token: st.MintToken(secret)}
	for _, t := range sel.Terms {
		m.Terms = append(m.Terms, ManifestTerm{
			Canonical:  t.Canonical,
			Aliases:    t.Aliases,
			Definition: firstSentence(t.Body),
			Confusions: t.Confusions,
		})
	}
	for _, s := range sel.Sets {
		ms := ManifestSet{
			Name:    s.Name,
			NodeID:  s.RelPath,
			Summary: firstSentence(s.Summary()),
			Entries: len(s.Entries()),
		}
		for _, g := range s.Groups {
			if g.Title == "" {
				continue
			}
			ms.Groups = append(ms.Groups, ManifestGroup{
				Title: g.Title, NodeID: g.NodeID, Entries: len(g.Entries),
			})
		}
		m.Sets = append(m.Sets, ms)
	}
	return m
}

// firstSentence keeps a catalogue line to one line. A set whose intro runs to
// a paragraph still has to fit next to nine others.
func firstSentence(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	for _, end := range []string{". ", "다. ", "? ", "! "} {
		if i := strings.Index(s, end); i >= 0 {
			return strings.TrimSpace(s[:i+len(end)-1])
		}
	}
	const max = 160
	if len(s) <= max {
		return s
	}
	cut := max
	for cut > 0 && (s[cut]&0xC0) == 0x80 {
		cut-- // never split a UTF-8 rune
	}
	return strings.TrimSpace(s[:cut]) + "…"
}
