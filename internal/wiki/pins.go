package wiki

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Salvia95/graphin/internal/parse"
)

// PinsFile is the committed lockfile name, relative to the wiki directory.
const PinsFile = "pins.lock"

// Pins records what each set's entries looked like when they were admitted.
//
// It is committed rather than derived at runtime, and that is load-bearing:
// .gitignore excludes the whole runtime data directory, so a pin kept there
// would vanish on clone and every entry would silently re-pin to whatever the
// document says now. Drift detection would then never fire — the failure it
// exists to prevent, arrived at by storing its own evidence in the wrong place.
type Pins struct {
	V int `json:"v"`
	// Pins is set name → node ID → what that entry looked like.
	Pins map[string]map[string]Pin `json:"pins"`
}

// Pin is one entry's recorded state.
//
// Two hashes, because a heading rename and a rewrite are different events and
// only one of them invalidates a summary. Hash covers the section including
// its heading line, so a retitle changes it; Rename covers the body alone and
// does not. Recording only Hash made every rename report as "content changed",
// and a drift flag that cries wolf is worse than none — readers learn to skip
// it, including the time the text really did change.
type Pin struct {
	Hash   string `json:"h"`
	Rename string `json:"r,omitempty"`
}

// NewPins returns an empty, versioned lockfile.
func NewPins() *Pins { return &Pins{V: 1, Pins: map[string]map[string]Pin{}} }

// LoadPins reads the lockfile. A missing file is not an error: a workspace
// with sets and no pins yet is the normal state before the first admission.
func LoadPins(path string) (*Pins, error) {
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return NewPins(), nil
	}
	if err != nil {
		return nil, err
	}
	var p Pins
	if err := json.Unmarshal(raw, &p); err != nil {
		// Say the next action, not just the fault. This file is generated,
		// so every way it can be unreadable has the same one-command fix,
		// and a raw JSON error sends the reader to edit it by hand instead.
		return nil, fmt.Errorf("%s is unreadable (%w) — regenerate it with `graphin wiki repin`", path, err)
	}
	if p.V != 1 {
		return nil, fmt.Errorf("%s was written by a different version (v%d) — regenerate it with `graphin wiki repin`", path, p.V)
	}
	if p.Pins == nil {
		p.Pins = map[string]map[string]Pin{}
	}
	return &p, nil
}

// Save writes the lockfile with stable key order so a re-pin that changed
// nothing produces no diff.
func (p *Pins) Save(path string) error {
	raw, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(raw, '\n'), 0o644)
}

// Get returns the recorded pin for one entry.
func (p *Pins) Get(set, nodeID string) (Pin, bool) {
	pin, ok := p.Pins[set][nodeID]
	return pin, ok
}

// Set records one entry's pin.
func (p *Pins) Set(set, nodeID string, pin Pin) {
	if p.Pins[set] == nil {
		p.Pins[set] = map[string]Pin{}
	}
	p.Pins[set][nodeID] = pin
}

// FormatHash renders a subtree hash the way the lockfile stores it.
func FormatHash(h [32]byte) string { return "b3:" + hex.EncodeToString(h[:]) }

// Hasher resolves a node ID to its content hash straight from the file, with
// no index and no running server.
//
// This is what makes the lockfile checkable in CI. A section's hash is
// BLAKE3 over its exact source slice (internal/parse), so re-parsing the
// document reproduces byte-for-byte what the indexer recorded — the same
// property that lets the existing anchor guard run as a pure text check.
type Hasher struct {
	root  string
	cache map[string]map[string]Pin // relPath → nodeID → pin
	fail  map[string]error
}

// NewHasher returns a Hasher rooted at a workspace directory.
func NewHasher(root string) *Hasher {
	return &Hasher{
		root:  root,
		cache: map[string]map[string]Pin{},
		fail:  map[string]error{},
	}
}

// Pin returns the node's current state. ok is false when the file is
// unreadable, unparseable, or no longer contains that node.
func (h *Hasher) Pin(nodeID string) (Pin, bool) {
	rel, _, _ := strings.Cut(nodeID, "#")
	nodes, err := h.file(rel)
	if err != nil {
		return Pin{}, false
	}
	v, ok := nodes[nodeID]
	return v, ok
}

// file parses one document once per Hasher.
func (h *Hasher) file(rel string) (map[string]Pin, error) {
	if nodes, ok := h.cache[rel]; ok {
		return nodes, nil
	}
	if err, ok := h.fail[rel]; ok {
		return nil, err
	}
	src, err := os.ReadFile(filepath.Join(h.root, filepath.FromSlash(rel)))
	if err != nil {
		h.fail[rel] = err
		return nil, err
	}
	res, err := parse.File(rel, src)
	if err != nil {
		h.fail[rel] = err
		return nil, err
	}
	nodes := make(map[string]Pin, len(res.Nodes))
	for _, n := range res.Nodes {
		nodes[n.ID] = Pin{Hash: FormatHash(n.Hash), Rename: FormatHash(n.RenameKey)}
	}
	h.cache[rel] = nodes
	return nodes, nil
}

// ProblemKind classifies what went wrong with one entry.
type ProblemKind string

const (
	// ProblemDangling: the node ID does not resolve. A renamed heading does
	// this silently, which is why it is checked rather than discovered.
	ProblemDangling ProblemKind = "dangling"
	// ProblemDrift: the node still exists but its content changed since the
	// entry was admitted, so the one-line summary may now be a lie.
	ProblemDrift ProblemKind = "drift"
	// ProblemUnpinned: the entry has no recorded hash, so drift cannot be
	// detected for it at all.
	ProblemUnpinned ProblemKind = "unpinned"
	// ProblemNoSummary: an entry without a sentence is a table of contents
	// row, and the reader still has to open it to find out what it says.
	ProblemNoSummary ProblemKind = "no-summary"
)

// Problem is one thing wrong with one entry.
type Problem struct {
	Kind   ProblemKind
	Set    string
	NodeID string
	Line   int
	Detail string
}

func (p Problem) String() string {
	loc := fmt.Sprintf("%s:%d", p.Set, p.Line)
	if p.Detail == "" {
		return fmt.Sprintf("%-11s %s  %s", p.Kind, loc, p.NodeID)
	}
	return fmt.Sprintf("%-11s %s  %s — %s", p.Kind, loc, p.NodeID, p.Detail)
}

// Check verifies every entry of every set against the files on disk and the
// recorded pins. It reports rather than repairs: an entry that drifted needs
// a person to decide whether the summary or the document is now wrong.
func Check(root string, sets []*Set, pins *Pins) []Problem {
	h := NewHasher(root)
	var out []Problem
	for _, s := range sets {
		for _, e := range s.Entries() {
			if e.Summary == "" {
				out = append(out, Problem{ProblemNoSummary, s.Name, e.NodeID, e.Line, ""})
			}
			cur, ok := h.Pin(e.NodeID)
			if !ok {
				out = append(out, Problem{ProblemDangling, s.Name, e.NodeID, e.Line,
					"no such node — heading renamed or file moved"})
				continue
			}
			want, pinned := pins.Get(s.Name, e.NodeID)
			switch {
			case !pinned:
				out = append(out, Problem{ProblemUnpinned, s.Name, e.NodeID, e.Line, ""})
			case want.Hash != cur.Hash:
				out = append(out, Problem{ProblemDrift, s.Name, e.NodeID, e.Line,
					"content changed since registration"})
			}
		}
	}
	return out
}

// Repin rebuilds the lockfile from the current documents. Entries that do not
// resolve are left out and reported: pinning a dangling ID would record a
// hash for nothing and hide the break behind a green check.
func Repin(root string, sets []*Set) (*Pins, []Problem) {
	h := NewHasher(root)
	pins := NewPins()
	var out []Problem
	names := make([]string, 0, len(sets))
	byName := map[string]*Set{}
	for _, s := range sets {
		names = append(names, s.Name)
		byName[s.Name] = s
	}
	sort.Strings(names)
	for _, name := range names {
		s := byName[name]
		for _, id := range s.NodeIDs() {
			cur, ok := h.Pin(id)
			if !ok {
				out = append(out, Problem{ProblemDangling, s.Name, id, 0,
					"no such node — not pinned"})
				continue
			}
			pins.Set(s.Name, id, cur)
		}
	}
	return pins, out
}
