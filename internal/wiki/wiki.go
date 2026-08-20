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

// Status tracks an entry's progress through admission.
type Status string

const (
	// StatusProposed is a candidate awaiting review. Nothing serves it.
	StatusProposed Status = "proposed"
	// StatusUnverified passed admission from a single session's evidence.
	// It is served, marked, and promoted to active when it recurs — one
	// session cannot tell a convention from a coincidence.
	StatusUnverified Status = "unverified"
	// StatusActive is admitted knowledge.
	StatusActive Status = "active"
)

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
	Intro         string // body above the first group
	Groups        []Group
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
type Term struct {
	Canonical string
	RelPath   string
	// Aliases are interchangeable in every context in this project. Partial
	// overlap is not an alias — that is a separate term with a stated
	// relation, and merging the two hides the difference that mattered.
	Aliases []string
	// DerivesFrom names a root term whose definition this one inherits.
	// Compounds are not defined twice.
	DerivesFrom string
	// Confusions are the near-misses worth naming outright, written as
	// "other term — why they differ".
	Confusions []Confusion
	Scope      []string
	// Evidence are node IDs showing the term used across separate contexts.
	// Without them a candidate is one author's coinage, and admission fails.
	Evidence     []string
	Status       Status
	LastVerified string
	Body         string
}

// Confusion is one "not to be confused with" pair.
type Confusion struct {
	Term string
	Why  string
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
