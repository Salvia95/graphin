package wiki

import "errors"

// Drift is what pin comparison found for one entry.
type Drift string

const (
	// DriftNone: the section is byte-for-byte what it was when admitted.
	DriftNone Drift = ""
	// DriftChanged: the section still exists but was rewritten since. The
	// content is served anyway and the reader is told — a set that refuses
	// to answer is worse than one that answers with a caveat, because the
	// reader can judge but only if they get the text.
	DriftChanged Drift = "changed-since-registration"
	// DriftUnpinned: nothing was recorded, so nothing can be compared. Worth
	// saying out loud: silence here would read as "verified unchanged".
	DriftUnpinned Drift = "unpinned"
	// DriftGone: the node no longer resolves.
	DriftGone Drift = "gone"
)

// ErrPinnedDrift is returned in place of content for a pinned set whose
// section changed. Reproducibility is the entire value of such a set, so
// serving the new text would quietly destroy what the reader came for.
var ErrPinnedDrift = errors.New("pinned entry changed since registration")

// Resolution is a served reading list.
type Resolution struct {
	Sets []ResolvedSet
	// Missing names sets that were asked for and do not exist.
	Missing []string
}

// ResolvedSet is one set with its entries loaded.
type ResolvedSet struct {
	Name    string
	Mode    Mode
	Entries []ResolvedEntry
}

// ResolvedEntry pairs an entry with its current content and pin verdict.
type ResolvedEntry struct {
	Entry
	Block Block
	Drift Drift
	// RedirectedTo is set when the entry's recorded ID was superseded and
	// the content came from its replacement. The set still names the old id,
	// so this is what tells a reader the link needs updating.
	RedirectedTo string
	// Unreviewed carries the set's review flag down to the section, because
	// a reader sees sections, not sets. Same pattern as Drift: a caveat
	// served with the text beats text withheld for lack of a signature.
	Unreviewed bool
}

// Drifted reports whether any entry needs re-verification.
func (r Resolution) Drifted() []ResolvedEntry {
	var out []ResolvedEntry
	for _, s := range r.Sets {
		for _, e := range s.Entries {
			if e.Drift == DriftChanged || e.Drift == DriftGone {
				out = append(out, e)
			}
		}
	}
	return out
}

// Resolve loads the content behind a list of set names.
//
// Two sources are used on purpose. Bodies come from the reader, which knows
// how to repair byte offsets that went stale since indexing; pin comparison
// comes from re-parsing the file, which is the same computation the indexer
// performed and needs nothing running. Keeping them separate is what lets the
// identical verdict be produced in CI.
func (st *Store) Resolve(r Reader, setNames []string) Resolution {
	sets, missing := st.Expand(setNames)
	res := Resolution{Missing: missing}
	h := NewHasher(st.Root)

	for _, s := range sets {
		rs := ResolvedSet{Name: s.Name, Mode: s.Mode}
		entries := s.Entries()

		// Read and hash the node that is current NOW, but keep looking the
		// pin up under the id the set recorded: the pin is keyed by what the
		// author wrote, and a rename must not read as an unpinned entry.
		ids := make([]string, 0, len(entries))
		for _, e := range entries {
			ids = append(ids, st.current(e.NodeID))
		}
		blocks := r.Read(ids)

		for i, e := range entries {
			re := ResolvedEntry{Entry: e, Unreviewed: s.Unreviewed || st.unreviewedFor(e.NodeID)}
			if ids[i] != e.NodeID {
				re.RedirectedTo = ids[i]
			}
			if i < len(blocks) {
				re.Block = blocks[i]
			} else {
				re.Block = Block{NodeID: e.NodeID, Err: errors.New("not returned by reader")}
			}
			re.Drift = st.driftOf(h, s, e.NodeID)

			if s.Mode == ModePinned && re.Drift == DriftChanged {
				re.Block.Text = ""
				re.Block.Err = ErrPinnedDrift
			}
			rs.Entries = append(rs.Entries, re)
		}
		res.Sets = append(res.Sets, rs)
	}
	return res
}

// unreviewedFor reports whether any set that lists the node carries agent
// changes nobody has checked.
//
// One rule for both ways of asking. A section fetched by set and the same
// section fetched by id must carry the same caveat, and "any citing set" is
// the honest one: the reader did not say which set they trust, and a caveat
// they did not need costs less than one they did.
func (st *Store) unreviewedFor(nodeID string) bool {
	for _, s := range st.SetList() {
		if s.Unreviewed && s.cites(nodeID) {
			return true
		}
	}
	return false
}

// driftOf compares one entry against its recorded pin, following a redirect
// if the recorded ID has been superseded.
//
// A renamed heading changes the section hash without touching a word of the
// body, so comparing hashes alone would report every rename as drift. The
// summary in the set describes what the section claims, and a retitle does
// not change that — flagging it would train readers to ignore the flag, and
// the next real rewrite with it.
func (st *Store) driftOf(h *Hasher, s *Set, nodeID string) Drift {
	current := st.current(nodeID)
	cur, ok := h.Pin(current)
	if !ok {
		return DriftGone
	}
	want, pinned := st.Pins.Get(s.Name, nodeID)
	switch {
	case !pinned:
		return DriftUnpinned
	case want.Hash == cur.Hash:
		return DriftNone
	case current != nodeID && want.Rename != "" && want.Rename == cur.Rename:
		// Followed a redirect and the body is byte-for-byte what it was:
		// only the title moved. The reader is told through RedirectedTo,
		// which is the thing that actually needs fixing.
		return DriftNone
	default:
		return DriftChanged
	}
}

// ResolveNodes serves specific node IDs rather than whole sets, for a reader
// that already scanned a catalogue and wants three of its thirty entries.
//
// Pin state is looked up across every set that cites the node: the same
// section can belong to several sets, and a reader asking for it by ID has
// not told us which one they came from.
func (st *Store) ResolveNodes(r Reader, nodeIDs []string) []ResolvedEntry {
	h := NewHasher(st.Root)
	ids := make([]string, 0, len(nodeIDs))
	for _, id := range nodeIDs {
		ids = append(ids, st.current(id))
	}
	blocks := r.Read(ids)
	out := make([]ResolvedEntry, 0, len(nodeIDs))

	for i, id := range nodeIDs {
		re := ResolvedEntry{Entry: Entry{NodeID: id}}
		if i < len(blocks) {
			re.Block = blocks[i]
		}
		if cur := st.current(id); cur != id {
			re.RedirectedTo = cur
		}
		re.Drift = DriftUnpinned
		if _, ok := h.Pin(st.current(id)); !ok {
			re.Drift = DriftGone
			out = append(out, re)
			continue
		}
		re.Unreviewed = st.unreviewedFor(id)
		for _, s := range st.SetList() {
			if _, pinned := st.Pins.Get(s.Name, id); !pinned {
				continue
			}
			d := st.driftOf(h, s, id)
			// Any set that still considers the node current makes it
			// current: a stale pin in one set is that set's problem, not a
			// reason to distrust text another set vouches for.
			if d == DriftNone {
				re.Drift = DriftNone
				break
			}
			re.Drift = d
		}
		out = append(out, re)
	}
	return out
}
