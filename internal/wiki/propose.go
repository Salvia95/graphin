package wiki

import (
	"fmt"
	"os"
	pathpkg "path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// GlossaryCap is the hard ceiling on glossary entries per project.
//
// A cap is the point, not a side effect. A glossary that grows without bound
// becomes a dictionary nobody reads, and the discipline of having to displace
// something is what keeps the entries worth their place. The number is
// tunable; that there is one is not.
const GlossaryCap = 30

// minEvidenceFiles is the cross-context test: a term used in one place is one
// author's coinage, and defining it teaches a vocabulary the project does not
// actually speak. Distinct files stand in for distinct contexts because that
// is what the evidence can prove.
const minEvidenceFiles = 2

// Proposal is a candidate awaiting review.
//
// Work agents write here and nowhere else. The separation is deliberate: an
// agent that can publish to the glossary will publish the artifact it just
// built, and no amount of instruction has been enough to stop that — only a
// mechanical judgement and a human gate were.
type Proposal struct {
	Term
	// File is where the proposal lives, which is also its identity.
	File string
	// Seen counts how many times this candidate has been submitted. A term
	// that keeps coming back is one the project really does speak.
	Seen int
}

// RuleID names one admission rule.
type RuleID string

const (
	// RuleIdentifier: the index already resolves this word. Structure, not
	// meaning — and a second definition of it will drift from the first.
	RuleIdentifier RuleID = "identifier"
	// RuleEvidence: too few distinct contexts to call it shared vocabulary.
	RuleEvidence RuleID = "evidence"
	// RuleCap: the glossary is full; this has to displace something.
	RuleCap RuleID = "cap"
)

// Finding is one rule's objection.
type Finding struct {
	Rule   RuleID
	Detail string
}

// Verdict is the mechanical part of admission — everything that can be
// decided without judgement. What it never does is admit: passing every rule
// makes a candidate reviewable, not accepted.
type Verdict struct {
	Findings []Finding
	// Status is what the entry would carry if a person approves it.
	Status Status
}

// Blocked reports whether any rule objected.
func (v Verdict) Blocked() bool { return len(v.Findings) > 0 }

func (v Verdict) String() string {
	if !v.Blocked() {
		return "reviewable (" + string(v.Status) + ")"
	}
	parts := make([]string, 0, len(v.Findings))
	for _, f := range v.Findings {
		parts = append(parts, fmt.Sprintf("%s: %s", f.Rule, f.Detail))
	}
	return strings.Join(parts, "; ")
}

// Definer answers whether the code index already resolves a word. It is the
// admission test's first rule, and the reason the wiki can state its boundary
// as a function rather than a instruction.
type Definer interface {
	Defines(word string) (nodeID, kind string, ok bool)
}

// Judge runs the mechanical admission rules over a candidate.
//
// d may be nil, and then the identifier rule is skipped rather than silently
// passed — the caller is told, because "no index available" and "not an
// identifier" are very different states to approve on.
func (st *Store) Judge(t *Term, d Definer) Verdict {
	v := Verdict{Status: StatusUnverified}

	if d != nil {
		for _, word := range append([]string{t.Canonical}, t.Aliases...) {
			if id, kind, ok := d.Defines(word); ok {
				v.Findings = append(v.Findings, Finding{RuleIdentifier,
					fmt.Sprintf("%q is already %s %s — ask graphin, not the wiki", word, kind, id)})
				break
			}
		}
	}

	files := map[string]bool{}
	for _, e := range t.Evidence {
		rel, _, _ := strings.Cut(e, "#")
		files[rel] = true
	}
	if len(files) < minEvidenceFiles {
		v.Findings = append(v.Findings, Finding{RuleEvidence,
			fmt.Sprintf("cited in %d context(s), need %d", len(files), minEvidenceFiles)})
	} else if len(files) >= minEvidenceFiles+1 {
		// Corroborated well past the floor: no reason to make a reader treat
		// it as provisional.
		v.Status = StatusActive
	}

	if _, existing := st.Terms[t.Canonical]; !existing && len(st.Terms) >= GlossaryCap {
		v.Findings = append(v.Findings, Finding{RuleCap,
			fmt.Sprintf("glossary holds %d of %d — this must displace an entry, which is a decision for a person",
				len(st.Terms), GlossaryCap)})
	}
	return v
}

// ProposalPath is where a candidate for canonical lives.
func ProposalPath(root, canonical string) string {
	return filepath.Join(root, filepath.FromSlash(pathpkg.Join(DirName, proposeSubdir)),
		safeName(canonical)+".md")
}

// Propose files a candidate, merging with one already queued under the same
// name. Merging rather than overwriting is what makes recurrence visible: the
// second submission is evidence about the term, not just a repeat of it.
func (st *Store) Propose(t *Term) (*Proposal, error) {
	path := ProposalPath(st.Root, t.Canonical)
	p := &Proposal{Term: *t, File: path, Seen: 1}

	if raw, err := os.ReadFile(path); err == nil {
		prev, perr := ParseTerm(path, raw)
		if perr == nil {
			p.Seen = prevSeen(raw) + 1
			p.Evidence = mergeStrings(prev.Evidence, t.Evidence)
			p.Aliases = mergeStrings(prev.Aliases, t.Aliases)
			if p.Body == "" {
				p.Body = prev.Body
			}
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	return p, os.WriteFile(path, []byte(p.render()), 0o644)
}

// render writes the proposal back out in the authored format, so approving it
// is a file move and the review is an ordinary diff.
func (p *Proposal) render() string {
	var b strings.Builder
	b.WriteString("---\ntype: glossary\n")
	fmt.Fprintf(&b, "canonical: %s\n", p.Canonical)
	writeList(&b, "aliases", p.Aliases)
	if p.DerivesFrom != "" {
		fmt.Fprintf(&b, "derives_from: %s\n", p.DerivesFrom)
	}
	if len(p.Confusions) > 0 {
		b.WriteString("not_to_be_confused_with:\n")
		for _, c := range p.Confusions {
			fmt.Fprintf(&b, "  - %s — %s\n", c.Term, c.Why)
		}
	}
	writeList(&b, "scope", p.Scope)
	writeList(&b, "evidence", p.Evidence)
	fmt.Fprintf(&b, "status: %s\n", StatusProposed)
	fmt.Fprintf(&b, "seen: %d\n", p.Seen)
	fmt.Fprintf(&b, "last_verified: %s\n---\n\n", time.Now().UTC().Format("2006-01-02"))
	b.WriteString(strings.TrimSpace(p.Body))
	b.WriteString("\n")
	return b.String()
}

func writeList(b *strings.Builder, key string, items []string) {
	if len(items) == 0 {
		fmt.Fprintf(b, "%s: []\n", key)
		return
	}
	fmt.Fprintf(b, "%s:\n", key)
	for _, it := range items {
		fmt.Fprintf(b, "  - %s\n", it)
	}
}

func prevSeen(raw []byte) int {
	fm, _ := splitFrontmatter(raw)
	n := 0
	_, _ = fmt.Sscanf(parseFrontmatter(fm).Get("seen"), "%d", &n)
	if n < 1 {
		return 1
	}
	return n
}

func mergeStrings(a, b []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, s := range append(append([]string{}, a...), b...) {
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

// Queue lists everything awaiting review, in name order.
func (st *Store) Queue() ([]*Proposal, error) {
	dir := filepath.Join(st.Root, filepath.FromSlash(pathpkg.Join(DirName, proposeSubdir)))
	ents, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(ents))
	for _, e := range ents {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	out := make([]*Proposal, 0, len(names))
	for _, n := range names {
		path := filepath.Join(dir, n)
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		t, err := ParseTerm(pathpkg.Join(DirName, proposeSubdir, n), raw)
		if err != nil {
			return nil, err
		}
		out = append(out, &Proposal{Term: *t, File: path, Seen: prevSeen(raw)})
	}
	return out, nil
}
