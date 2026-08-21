// Package wiki implements the agent knowledge layer: a curated set of pages
// that answer "what does this mean and why" — the questions the code index
// cannot answer because they are not written down in any one place.
//
// The boundary with graphin proper is a judgement, not a slogan: anything the
// index can resolve is not wiki knowledge. A class name, a table, a function
// signature — those have a node, and a glossary entry for one is duplication
// that will drift. What belongs here is the vocabulary and the conventions
// that live between the files.
//
// # Dependencies
//
// This package may depend on internal/parse, which is a pure function from
// bytes to nodes, but never on internal/graph or internal/workspace. Those
// arrive through the Index interface instead. The rule exists for a concrete
// reason rather than tidiness: opening the graph engine truncates its delta
// log (internal/graph/deltalog.go), so any code path that a hook or a CI step
// might run out-of-process must be reachable without it. Parsing a markdown
// file needs none of that, which is what makes pins verifiable in CI.
package wiki

import "strings"

// Mode decides what a stale pin means for a set.
type Mode string

const (
	// ModeLive serves the current content even when the pin no longer
	// matches, flagging the drift. This is the default because a knowledge
	// set that refuses to answer is worse than one that answers with a
	// caveat — the reader can judge, but only if they get the text.
	ModeLive Mode = "live"
	// ModePinned refuses on mismatch. Reserved for knowledge whose value is
	// reproducibility rather than currency, such as a decision record.
	ModePinned Mode = "pinned"
)

// Status is where an entry sits in its life, using the Open Knowledge Format
// vocabulary so a bundle of these needs no translation.
//
// It deliberately says nothing about who vouched for the content. That was
// conflated here originally — one field carrying both "is this current" and
// "did a person approve it" — and the two answers move independently: an
// entry can be stable and unreviewed, or deprecated and human-reviewed.
// Approval lives in Reviewed.
type Status string

const (
	// StatusDraft is not ready to be served. Everything in the propose queue
	// is a draft.
	StatusDraft Status = "draft"
	// StatusStable is the default: served.
	StatusStable Status = "stable"
	// StatusDeprecated is still served, because a reader who arrives with the
	// old word needs to be told it is the old word — withholding it just
	// leaves them using it.
	StatusDeprecated Status = "deprecated"
)

// Review is one confirmation event: who vouched for this, and when.
type Review struct {
	// By follows the actor convention: "human:<id>" for people,
	// "<producer>/<version>" for agents, "process:<id>" for automation. The
	// human: prefix is load-bearing — it is what separates a person's
	// judgement from a machine's confidence.
	By string `json:"by"`
	At string `json:"at"`
}

// Trust is how much a reader should lean on an entry, derived from Reviewed
// rather than declared. Deriving it means nobody can assert it.
type Trust string

const (
	TrustUnverified Trust = "unverified"
	TrustMachine    Trust = "machine-confirmed"
	TrustHuman      Trust = "human-reviewed"
)

// Trust derives the tier. A human review outranks any number of machine ones.
func (t *Term) Trust() Trust {
	tier := TrustUnverified
	for _, r := range t.Reviewed {
		if strings.HasPrefix(r.By, "human:") {
			return TrustHuman
		}
		tier = TrustMachine
	}
	return tier
}

// Stale reports whether the entry has passed its own expiry date.
//
// This is a different question from drift, and both are needed. Drift catches
// content that changed; this catches content that did not. A decision record
// can be byte-for-byte what it was and still describe a world that is gone,
// and no hash will ever say so.
func (t *Term) Stale(today string) bool {
	return t.StaleAfter != "" && today >= t.StaleAfter
}

// Set is one knowledge set: a curriculum, not a search result. It names the
// sections a reader needs before starting a kind of work, and it names them
// by node ID so the text is never copied here to drift.
type Set struct {
	Name string // filename stem; there is no second place declaring it
	// Title is the set's `#` heading. It matters because the filename is
	// often an ASCII slug while the heading is what the subject is actually
	// called — `release.md` titled `릴리스` in this repository — and a
	// reader describing their work uses the latter.
	Title   string
	RelPath string // workspace-relative path of the set file
	Roles   []string
	// Prerequisites are other set names. Selecting a set expands these
	// deterministically, so a reader cannot end up with the advanced page
	// and not the one it assumes.
	Prerequisites []string
	Mode          Mode
	// Description, Tags and StaleAfter are the same Open Knowledge Format
	// fields the glossary carries. Description falls back to Intro: the
	// opening paragraph already served this purpose before the field existed.
	Description string
	Tags        []string
	StaleAfter  string
	Intro       string // body above the first group
	Groups      []Group
}

// Stale reports whether the set has passed its own expiry date. See
// Term.Stale for why this is a separate question from drift.
func (s *Set) Stale(today string) bool {
	return s.StaleAfter != "" && today >= s.StaleAfter
}

// Summary is the one line that describes the set, preferring the declared
// description over the opening prose.
func (s *Set) Summary() string {
	if s.Description != "" {
		return s.Description
	}
	return s.Intro
}

// Group is one `##` section of a set. Groups are not decoration: the set file
// is markdown, so each group is itself a section node and an agent can load
// one group instead of the whole set.
type Group struct {
	Title string
	// NodeID of the group's own heading, taken from the parser rather than
	// re-derived, so it is the same string the index uses.
	NodeID  string
	Entries []Entry
}

// Entry is one line of a set: a node ID plus the one sentence that lets a
// reader decide whether to load it.
type Entry struct {
	Title   string // link text as written
	NodeID  string // link target resolved against the set file's directory
	Summary string
	Line    int // 1-based line in the set file, for diagnostics
}

// Entries flattens the set in document order.
func (s *Set) Entries() []Entry {
	var out []Entry
	for _, g := range s.Groups {
		out = append(out, g.Entries...)
	}
	return out
}

// NodeIDs returns every node the set points at, in document order, without
// duplicates. This is the set's footprint for pinning and for refcounting.
func (s *Set) NodeIDs() []string {
	seen := map[string]bool{}
	out := []string{}
	for _, e := range s.Entries() {
		if !seen[e.NodeID] {
			seen[e.NodeID] = true
			out = append(out, e.NodeID)
		}
	}
	return out
}

// Term is one glossary entry: a canonical word plus everything needed to keep
// people from using a different one for the same thing.
// Term is one glossary entry.
//
// The JSON names are the frontmatter keys, deliberately. A reviewer's form is
// the file's fields, so keeping one set of names means the API, the file and
// the form cannot disagree about what something is called. Which of these an
// approval may actually change is decided by applyEdits, not by what a client
// is able to send.
type Term struct {
	Canonical string `json:"canonical"`
	RelPath   string `json:"rel_path,omitempty"`
	// Title and Description are the human-facing labels. Both are Open
	// Knowledge Format fields and both are flat, so adopting them cost
	// nothing and made these files readable by anything that speaks OKF.
	Title       string   `json:"title,omitempty"`
	Description string   `json:"description,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	// StaleAfter is an absolute date (YYYY-MM-DD) after which this should be
	// re-read regardless of whether anything changed.
	StaleAfter string `json:"stale_after,omitempty"`
	// Reviewed records confirmations. See Trust.
	Reviewed []Review `json:"reviewed,omitempty"`
	// Aliases are interchangeable in every context in this project. Partial
	// overlap is not an alias — that is a separate term with a stated
	// relation, and merging the two hides the difference that mattered.
	Aliases []string `json:"aliases,omitempty"`
	// DerivesFrom names a root term whose definition this one inherits.
	// Compounds are not defined twice.
	DerivesFrom string `json:"derives_from,omitempty"`
	// Confusions are the near-misses worth naming outright, written as
	// "other term — why they differ".
	Confusions []Confusion `json:"not_to_be_confused_with,omitempty"`
	Scope      []string    `json:"scope,omitempty"`
	// Evidence are node IDs showing the term used across separate contexts.
	// Without them a candidate is one author's coinage, and admission fails.
	Evidence     []string `json:"evidence,omitempty"`
	Status       Status   `json:"status,omitempty"`
	LastVerified string   `json:"last_verified,omitempty"`
	Body         string   `json:"body,omitempty"`
}

// Confusion is one "not to be confused with" pair.
type Confusion struct {
	Term string `json:"term"`
	Why  string `json:"why"`
}

// Reader is the narrow view of the code index that serving a set needs. It
// exists so this package states its requirement instead of reaching into the
// engine, and so the policy above it can run against a test double.
//
// It is deliberately one method. Pin comparison does not appear here because
// Hasher answers it from the file, which is what lets the same check run in
// CI with no server; and nothing in this package searches yet. An interface
// with methods no caller uses is a claim about a design that has not been
// tested, so these get added when something needs them.
type Reader interface {
	// Read returns each node's current content, in the order requested.
	// A node that cannot be read is reported in Block.Err rather than
	// failing the batch: one broken entry must not cost a reader the other
	// nine sections they asked for.
	Read(nodeIDs []string) []Block
}

// Redirector maps a node ID that was superseded to the one that replaced it.
//
// It is optional, and what happens without one is the point. The CLI check
// runs with no index and therefore no redirects, so a renamed heading shows up
// there as a dangling entry and an author fixes the link. At serve time the
// redirect keeps the reader working in the meantime. A cushion, not a repair —
// if the check quietly passed on redirected entries, links would rot behind a
// green build until the redirect was eventually collected.
type Redirector interface {
	ResolveID(nodeID string) string
}

// Block is one node's content as served to a reader.
type Block struct {
	NodeID    string
	RelPath   string
	StartLine int
	EndLine   int
	Text      string
	// Err explains why Text is empty, if it is.
	Err error
}
