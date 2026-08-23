package wiki

import (
	"errors"
	"fmt"
	"os"
	pathpkg "path"
	"path/filepath"
	"strings"
	"time"
)

// Errors a caller is expected to tell apart. Approving is the one place in the
// wiki where a person's action can fail for reasons that are not bugs, and a
// caller with a form to render needs to say which one happened.
var (
	// ErrNoProposal means nothing is queued under that name.
	ErrNoProposal = errors.New("no such candidate in the queue")
	// ErrAlreadyInGlossary means approving would overwrite a live entry.
	ErrAlreadyInGlossary = errors.New("the glossary already has this term")
	// ErrGlossaryFull means the cap is reached. Displacing an entry is a
	// judgement about which knowledge matters more, so it is never automatic.
	ErrGlossaryFull = errors.New("the glossary is full")
	// ErrNotHuman means the review was not attributed to a person.
	ErrNotHuman = errors.New(`approval must be attributed as "human:<id>"`)
	// ErrNoEntry means the set does not list that node. Repinning one entry is
	// the only operation that takes a (set, node) pair from outside, so it is
	// the only one that can be handed a pair nothing backs.
	ErrNoEntry = errors.New("no such entry in that set")
)

// GlossaryPath is where an approved term lives.
func GlossaryPath(root, canonical string) string {
	return filepath.Join(root, filepath.FromSlash(pathpkg.Join(DirName, glossarySubdir)),
		safeName(canonical)+".md")
}

// Approve moves a candidate from the queue into the glossary.
//
// # It writes the working tree and stops there
//
// Nothing here stages, commits or pushes. Approving is a file move because the
// review is an ordinary diff, and that property survives exactly as long as the
// change stays uncommitted — moving the file is not what would destroy it,
// committing on the reviewer's behalf is (docs/console-spec.md §8).
//
// # What edits may change
//
// The prose fields, and not Evidence. Evidence is the record of why the
// candidate cleared the admission rules; letting the approval step rewrite it
// would make the rule that demands two independent citations satisfiable by
// typing two. Canonical is identity and is likewise fixed — a different word is
// a different term.
func (st *Store) Approve(canonical string, edits *Term, by string) (*Term, error) {
	// The human: prefix is what Trust reads to separate a person's judgement
	// from a machine's confidence, so an unattributed approval would quietly
	// mint human-reviewed trust for nobody.
	if !strings.HasPrefix(by, "human:") || strings.TrimSpace(by) == "human:" {
		return nil, ErrNotHuman
	}

	src := ProposalPath(st.Root, canonical)
	raw, err := os.ReadFile(src)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNoProposal
	}
	if err != nil {
		return nil, err
	}
	t, err := ParseTerm(pathpkg.Join(DirName, proposeSubdir, filepath.Base(src)), raw)
	if err != nil {
		return nil, err
	}

	dst := GlossaryPath(st.Root, canonical)
	if _, err := os.Stat(dst); err == nil {
		return nil, ErrAlreadyInGlossary
	}
	if len(st.Terms) >= GlossaryCap {
		return nil, fmt.Errorf("%w: %d of %d — displace an entry first", ErrGlossaryFull, len(st.Terms), GlossaryCap)
	}

	applyEdits(t, edits)
	// A queued file is a draft by construction; an approved one is served.
	t.Status = StatusStable
	t.Reviewed = append(t.Reviewed, Review{By: by, At: time.Now().UTC().Format("2006-01-02")})

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(dst, []byte(renderGlossary(t)), 0o644); err != nil {
		return nil, err
	}
	// Write, then remove. A failure between the two leaves the term in both
	// places, which the queue shows and a person can fix; the other order can
	// lose the text somebody wrote.
	if err := os.Remove(src); err != nil {
		return t, err
	}
	return t, nil
}

// Discard drops a candidate without publishing it.
//
// A queue that can only be approved from is a queue that only grows, and the
// friction this whole surface exists to remove is the friction of deciding —
// which includes deciding no. It deletes the proposal file and nothing else:
// the term can be proposed again, and Seen will start over, which is correct.
// A candidate that keeps coming back after a rejection is evidence, not noise.
func (st *Store) Discard(canonical string) error {
	err := os.Remove(ProposalPath(st.Root, canonical))
	if errors.Is(err, os.ErrNotExist) {
		return ErrNoProposal
	}
	return err
}

// applyEdits overlays the reviewer's form onto the queued candidate. A field
// left empty keeps what the proposer wrote rather than erasing it: the form
// arrives prefilled, and a blank there means "unchanged", not "delete".
func applyEdits(t *Term, e *Term) {
	if e == nil {
		return
	}
	if e.Title != "" {
		t.Title = e.Title
	}
	if e.Description != "" {
		t.Description = e.Description
	}
	if e.Body != "" {
		t.Body = e.Body
	}
	if e.DerivesFrom != "" {
		t.DerivesFrom = e.DerivesFrom
	}
	if e.StaleAfter != "" {
		t.StaleAfter = e.StaleAfter
	}
	if len(e.Tags) > 0 {
		t.Tags = e.Tags
	}
	if len(e.Aliases) > 0 {
		t.Aliases = e.Aliases
	}
	if len(e.Scope) > 0 {
		t.Scope = e.Scope
	}
	if len(e.Confusions) > 0 {
		t.Confusions = e.Confusions
	}
}

// renderGlossary is the approved form: the shared frontmatter, a real status,
// and who vouched.
func renderGlossary(t *Term) string {
	var b strings.Builder
	writeTermFront(&b, t)
	fmt.Fprintf(&b, "status: %s\n", t.Status)
	if len(t.Reviewed) > 0 {
		b.WriteString("reviewed:\n")
		for _, r := range t.Reviewed {
			fmt.Fprintf(&b, "  - %s — %s\n", r.By, r.At)
		}
	}
	fmt.Fprintf(&b, "last_verified: %s\n---\n\n", time.Now().UTC().Format("2006-01-02"))
	b.WriteString(strings.TrimSpace(t.Body))
	b.WriteString("\n")
	return b.String()
}
