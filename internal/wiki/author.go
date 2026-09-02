package wiki

import (
	"errors"
	"fmt"
	"os"
	pathpkg "path"
	"path/filepath"
	"strings"
)

// Authoring is the one chore that makes a new claim, which is why it comes
// after maintenance (docs/wiki-plan.md P3) and why it is fenced three ways.
//
// The trigger is the friction log, not the context. An agent that "extracts
// knowledge" from whatever it just did writes a set per task, and a wiki
// grown that way is a sweep of the documents nobody asked for. A set is
// written for a task that ran a preflight and got nothing — and that is
// checked, not requested.
//
// There is a cap, for the glossary's reason: a catalogue that grows without
// bound is a table of contents nobody reads, and having to displace something
// is what keeps every line worth its place. Sets have a second reason the
// glossary does not: matching quality moves with the set count (the common
// key threshold, the set files in search results), so growth is not free
// even before anyone reads the catalogue.
//
// And the set has to be reachable. A set written for a task that a preflight
// for that same task would not select is dead weight from the day it lands —
// the miss it was meant to close stays open. That, too, is checked.

// SetCap is the ceiling on knowledge sets per project. The number is tunable;
// that there is one is not.
const SetCap = 30

// ErrNoWiki means the workspace never adopted the wiki. Creating docs/wiki is
// how a project opts in — and arms the delegation gate — so an agent must
// not do it as a side effect of writing one set.
var ErrNoWiki = errors.New("this workspace has no docs/wiki — a person creates it to opt in")

const (
	// RuleName: the name is not a valid filename stem, or a set has it.
	RuleName RuleID = "name"
	// RuleUnasked: no preflight ever missed for this task. The wiki grows
	// from work that wanted knowledge and did not get it, never from a
	// retroactive sweep.
	RuleUnasked RuleID = "unasked"
	// RuleUnreachable: a preflight for the task this set answers would not
	// select it. Writing it would close nothing.
	RuleUnreachable RuleID = "unreachable"
)

// SetDraft is what an agent hands in: the whole set, plus the task it is for.
//
// There are no roles. A role tag pushes a set into every delegation of that
// role whether or not the task mentions it, and that is a standing decision
// by the wiki's authors — the one thing an agent writing at the end of a task
// is not positioned to make. Agent-written sets are pull-only.
type SetDraft struct {
	Name        string       `json:"name"`
	Title       string       `json:"title"`
	Description string       `json:"description"`
	Aliases     []string     `json:"aliases,omitempty"`
	Tags        []string     `json:"tags,omitempty"`
	Intro       string       `json:"intro,omitempty"`
	Groups      []DraftGroup `json:"groups"`
	// Task is the sentence given to wiki_preflight, verbatim. It is what the
	// friction log is searched for and what reachability is tested against.
	Task string `json:"task"`
}

// DraftGroup is one `##` section of the set to be.
type DraftGroup struct {
	Title   string       `json:"title"`
	Entries []DraftEntry `json:"entries"`
}

// DraftEntry is one line of it.
type DraftEntry struct {
	Title   string `json:"title"`
	NodeID  string `json:"node_id"`
	Summary string `json:"summary"`
}

func (d *SetDraft) entries() []DraftEntry {
	var out []DraftEntry
	for _, g := range d.Groups {
		out = append(out, g.Entries...)
	}
	return out
}

// JudgeSet runs every rule that files alone can decide over a draft.
//
// Like Judge and JudgeEntry it never admits — a clean verdict means the set
// may be written, marked, and served with the mark until a person looks.
func (st *Store) JudgeSet(d *SetDraft) Verdict {
	var v Verdict
	name := strings.TrimSpace(d.Name)
	switch {
	case name == "" || name != safeName(name) || name == "unknown":
		v.Findings = append(v.Findings, Finding{RuleName,
			fmt.Sprintf("%q is not a set name — letters, digits, - and _ only", d.Name)})
	case st.Sets[name] != nil:
		v.Findings = append(v.Findings, Finding{RuleName,
			fmt.Sprintf("a set named %s exists — repair it with wiki_edit_set instead", name)})
	}
	if strings.TrimSpace(d.Title) == "" || strings.TrimSpace(d.Description) == "" {
		v.Findings = append(v.Findings, Finding{RuleSummary, "a set needs a title and a one-line description"})
	}
	if strings.ContainsAny(d.Title, "\n\r") || strings.ContainsAny(d.Description, "\n\r") {
		v.Findings = append(v.Findings, Finding{RuleFormat, "the title and the description are each one line"})
	}
	for _, item := range append(append([]string{}, d.Aliases...), d.Tags...) {
		if strings.ContainsAny(item, ",]\n\r") || strings.TrimSpace(item) == "" {
			v.Findings = append(v.Findings, Finding{RuleFormat,
				fmt.Sprintf("%q cannot be an alias or tag — no commas, brackets or line breaks", item)})
		}
	}

	entries := d.entries()
	if len(entries) == 0 {
		v.Findings = append(v.Findings, Finding{RuleSummary, "a set with no entries is a catalogue line for nothing"})
	}
	h := NewHasher(st.Root)
	seen := map[string]bool{}
	for _, e := range entries {
		rel, _, _ := strings.Cut(e.NodeID, "#")
		if !strings.HasSuffix(strings.ToLower(rel), ".md") {
			v.Findings = append(v.Findings, Finding{RuleStructure,
				fmt.Sprintf("%s is code, not documentation — the index already answers it", e.NodeID)})
			continue
		}
		if _, ok := h.Pin(e.NodeID); !ok {
			v.Findings = append(v.Findings, Finding{RuleAnchor,
				fmt.Sprintf("%s does not resolve — no such heading in that file", e.NodeID)})
		}
		if seen[e.NodeID] {
			v.Findings = append(v.Findings, Finding{RuleDuplicate, e.NodeID + " is listed twice"})
		}
		seen[e.NodeID] = true
		if strings.TrimSpace(e.Summary) == "" || strings.TrimSpace(e.Title) == "" {
			v.Findings = append(v.Findings, Finding{RuleSummary,
				e.NodeID + " has no title or no summary — a row without a sentence is a table of contents"})
		}
		if strings.ContainsAny(e.Title, "]\n\r") || strings.ContainsAny(e.NodeID, " \t\n\r)") {
			v.Findings = append(v.Findings, Finding{RuleFormat,
				e.NodeID + ": a title cannot contain ']' and a node id cannot contain whitespace or ')'"})
		}
	}
	// A set whose every section an existing set already lists is that set
	// with a new name. Partial overlap is fine — sections belong to several
	// sets by design — and is left for the reviewer to see in the diff.
	// Only sections that passed the structure rule count: a draft of code
	// nodes has nothing to compare and must not read as a duplicate.
	if len(seen) > 0 {
		for _, s := range st.SetList() {
			all := true
			for id := range seen {
				if !s.cites(id) {
					all = false
					break
				}
			}
			if all {
				v.Findings = append(v.Findings, Finding{RuleDuplicate,
					fmt.Sprintf("every section here is already in %s — extend that set or write a different one", s.Name)})
				break
			}
		}
	}

	if len(st.Sets) >= SetCap {
		v.Findings = append(v.Findings, Finding{RuleCap,
			fmt.Sprintf("the wiki holds %d of %d sets — this must displace one, which is a decision for a person", len(st.Sets), SetCap)})
	}

	task := truncate(d.Task, maxTaskLen)
	if task == "" {
		v.Findings = append(v.Findings, Finding{RuleUnasked, "task is required — the sentence given to wiki_preflight, verbatim"})
	} else {
		missed := false
		for _, ev := range ReadFriction(st.Root) {
			if ev.Kind == FrictionMiss && ev.Task == task {
				missed = true
				break
			}
		}
		if !missed {
			v.Findings = append(v.Findings, Finding{RuleUnasked,
				"no preflight missed for this task — the wiki grows from work that asked and got nothing, not from a sweep; pass the task exactly as given to wiki_preflight"})
		}
		// Reachability is tested with the set in place, stop keys and all:
		// adding a set moves the common-key threshold, and a test against
		// the old store would pass a set the real preflight then skipped.
		// Skipped when the name is bad, because the trial would be keyed by
		// a name the file could never have.
		if !hasRule(v, RuleName) {
			if !st.wouldSelect(d, task) {
				v.Findings = append(v.Findings, Finding{RuleUnreachable,
					"a preflight for this task would not select this set — name it, or alias it, the way the task names the subject"})
			}
		}
	}
	return v
}

// hasRule reports whether a rule already objected.
func hasRule(v Verdict, r RuleID) bool {
	for _, f := range v.Findings {
		if f.Rule == r {
			return true
		}
	}
	return false
}

// wouldSelect asks the matcher the question the miss was recorded from, with
// the draft added to the wiki as it would be written.
func (st *Store) wouldSelect(d *SetDraft, task string) bool {
	rel := pathpkg.Join(DirName, setsSubdir, d.Name+".md")
	set, err := ParseSet(rel, []byte(renderSet(d)))
	if err != nil {
		return false
	}
	trial := &Store{Root: st.Root, Dir: st.Dir, Sets: map[string]*Set{}, Terms: st.Terms,
		Pins: st.Pins, Agents: st.Agents, Present: true}
	for k, v := range st.Sets {
		trial.Sets[k] = v
	}
	trial.Sets[set.Name] = set
	for _, n := range trial.Select("", task).Matched {
		if n == set.Name {
			return true
		}
	}
	return false
}

// CreateSet judges a draft, writes it, and pins every entry.
//
// The file lands with `origin: agent` and `reviewed: false`, which is what
// makes writing it safe to do at once: the mark travels with every section
// the set serves and sits in the queue until a person clears it. Nothing is
// committed — same boundary as every other write here.
func CreateSet(root string, d *SetDraft) (EditResult, Verdict, error) {
	store, err := Load(root)
	if err != nil {
		return EditResult{}, Verdict{}, err
	}
	if !store.Present {
		return EditResult{}, Verdict{}, ErrNoWiki
	}
	v := store.JudgeSet(d)
	res := EditResult{Set: strings.TrimSpace(d.Name)}
	if v.Blocked() {
		return res, v, nil
	}
	rel := pathpkg.Join(DirName, setsSubdir, res.Set+".md")
	res.File = rel
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return res, v, err
	}
	if err := os.WriteFile(path, []byte(renderSet(d)), 0o644); err != nil {
		return res, v, err
	}
	h := NewHasher(root)
	pinned := 0
	for _, e := range d.entries() {
		if cur, ok := h.Pin(e.NodeID); ok {
			store.Pins.Set(res.Set, e.NodeID, cur)
			pinned++
		}
	}
	res.Repinned = pinned == len(d.entries())
	if err := store.Pins.Save(store.PinsPath()); err != nil {
		return res, v, err
	}
	return res, v, nil
}

// renderSet writes a draft in the authored format, so the file an agent
// produced is indistinguishable in shape from one a person wrote and the
// same parser, checker and editors apply to it.
func renderSet(d *SetDraft) string {
	var b strings.Builder
	b.WriteString("---\ntype: knowledge_set\n")
	fmt.Fprintf(&b, "description: %s\n", frontScalar(d.Description))
	fmt.Fprintf(&b, "tags: [%s]\n", strings.Join(d.Tags, ", "))
	fmt.Fprintf(&b, "aliases: [%s]\n", strings.Join(d.Aliases, ", "))
	b.WriteString("roles: []\nprerequisites: []\nmode: live\n")
	fmt.Fprintf(&b, "origin: %s\nreviewed: false\n---\n\n", OriginAgent)
	fmt.Fprintf(&b, "# %s\n\n", strings.TrimSpace(d.Title))
	if intro := strings.TrimSpace(d.Intro); intro != "" {
		for _, l := range wrapIndented(intro, "", 80) {
			b.WriteString(l)
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	dir := pathpkg.Join(DirName, setsSubdir)
	for _, g := range d.Groups {
		if t := strings.TrimSpace(g.Title); t != "" {
			fmt.Fprintf(&b, "## %s\n\n", t)
		}
		for _, e := range g.Entries {
			fmt.Fprintf(&b, "- [%s](%s) —\n", strings.TrimSpace(e.Title), relativeTarget(dir, e.NodeID))
			for _, l := range wrapIndented(e.Summary, "  ", 80) {
				b.WriteString(l)
				b.WriteString("\n")
			}
		}
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n") + "\n"
}

// frontScalar writes a description the way the authored sets do — bare —
// unless bare would parse as something else.
func frontScalar(s string) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	// "[" would parse as a list and "#" as a comment; quotes at the edges are
	// stripped by the reader. Anything else is read back byte for byte.
	if strings.HasPrefix(s, "[") || strings.HasPrefix(s, "#") || strings.HasPrefix(s, "\"") || strings.HasPrefix(s, "'") {
		return "\"" + strings.ReplaceAll(s, "\"", "'") + "\""
	}
	return s
}
