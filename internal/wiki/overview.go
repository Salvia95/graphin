package wiki

import (
	"fmt"
	pathpkg "path"
	"sort"
	"time"
)

// DecisionKind is why something is waiting on a person.
//
// These are deliberately not "problems". A drifted pin and an unopened set are
// both fine states for a wiki to be in for a while; what they have in common
// is that only a person can end them, and that nothing else in the system will.
type DecisionKind string

const (
	// DecisionDangling is a set entry whose node no longer resolves — a
	// renamed heading or a moved file. The set is broken, not merely stale:
	// a reader who selects it gets less than the catalogue promised.
	DecisionDangling DecisionKind = "dangling"
	// DecisionGlossaryFull ranks high because it blocks approvals. Nothing
	// else can proceed until someone decides what matters less.
	DecisionGlossaryFull DecisionKind = "glossary_full"
	// DecisionExpired is a set or term past its own stale_after. Separate
	// from drift on purpose: nothing changed, which is exactly the worry.
	DecisionExpired DecisionKind = "expired"
	// DecisionDrift is a pinned section whose content moved under it. The
	// text is still served; what may no longer hold is the one-line summary
	// the catalogue offers it by.
	DecisionDrift DecisionKind = "drift"
	// DecisionApprove is a queued candidate.
	DecisionApprove DecisionKind = "approve"
	// DecisionStaleSkill is a generated role block that no longer matches
	// what it was generated from.
	DecisionStaleSkill DecisionKind = "stale_skill"
	// DecisionUnreadSet is a set offered in catalogue after catalogue and
	// never opened. It costs every delegation and returns nothing.
	DecisionUnreadSet DecisionKind = "unread_set"
	// DecisionUncovered is work the wiki had no answer for. The only kind
	// here that asks for writing rather than judging.
	DecisionUncovered DecisionKind = "uncovered"
	// DecisionUnreviewed is a set an agent changed that no person has looked
	// at since. Agent maintenance applies first and is controlled after, and
	// this is the control: the flag stays until someone reads the diff.
	DecisionUnreviewed DecisionKind = "unreviewed"
)

// severity orders the queue. Lower is more urgent, and the order is a claim
// about consequence rather than about how loud the signal is: a broken link
// costs a reader something today, an unopened set costs tokens, an uncovered
// task costs nothing until someone hits it again.
var severity = map[DecisionKind]int{
	DecisionDangling:     0,
	DecisionGlossaryFull: 1,
	DecisionExpired:      2,
	DecisionDrift:        3,
	// Above approve: an unreviewed change is already being served, while a
	// candidate is not served until someone says so.
	DecisionUnreviewed: 4,
	DecisionApprove:    5,
	DecisionStaleSkill: 6,
	DecisionUnreadSet:  7,
	DecisionUncovered:  8,
}

// Decision is one thing waiting on a person, in one shape whatever it came from.
//
// Deriving these in Go rather than in the interface is the same rule the queue
// report follows: which states count as decisions, and which of them is more
// urgent, is a judgement this project makes once. A console that sorted them
// itself would eventually disagree with the command about what matters.
type Decision struct {
	Kind     DecisionKind `json:"kind"`
	Severity int          `json:"severity"`
	// Title names the thing; Detail says what is wrong with it; Action says
	// what ends it. A decision that cannot state its action is a notification.
	Title  string `json:"title"`
	Detail string `json:"detail"`
	Action string `json:"action"`
	// Set, NodeID and Canonical locate it, and are what an action posts back.
	Set       string `json:"set,omitempty"`
	NodeID    string `json:"node_id,omitempty"`
	Canonical string `json:"canonical,omitempty"`
	// File and Line are where the fix is typed, workspace-relative.
	//
	// Always inside the wiki, never in the document a set points at, and that
	// is not an omission. Every action here changes what the wiki claims — fix
	// this link, confirm this summary, move this date, demote this set — so the
	// file to open is the set or the candidate. The document itself is one
	// ctrl-click away from the entry line this lands on.
	File string `json:"file,omitempty"`
	Line int    `json:"line,omitempty"`
	// Count is how many times the same thing has been seen: proposals
	// resubmitted, sections served stale, tasks that found nothing. One
	// occurrence is an anecdote.
	Count int `json:"count,omitempty"`
	// FirstSeen and LastSeen are dates (YYYY-MM-DD) bounding the occurrences
	// Count sums. Three misses last week and three misses in March are not the
	// same decision, and a count on its own cannot tell them apart.
	FirstSeen string `json:"first_seen,omitempty"`
	LastSeen  string `json:"last_seen,omitempty"`
	// Evidence carries the citations for a candidate.
	Evidence []string `json:"evidence,omitempty"`
	// Role is the delegate's role for an uncovered task.
	Role string `json:"role,omitempty"`
}

// EntryStatus is what a set entry's pin says about that entry right now. It is
// per-entry on purpose: a set with one broken link out of twelve is not a
// broken set, and a reader deciding whether to open it needs to see which line
// is the bad one rather than a count that could mean anything.
type EntryStatus string

const (
	EntryOK       EntryStatus = "ok"
	EntryDrift    EntryStatus = "drift"
	EntryDangling EntryStatus = "dangling"
)

// EntryView is one line of a set, with the pin state that line is in.
//
// Line travels with it for the same reason it travels with a Problem: fixing a
// dangling entry means editing that line of the set file, and a consumer that
// can only name the entry cannot say where to go.
type EntryView struct {
	Title   string      `json:"title"`
	NodeID  string      `json:"node_id"`
	Summary string      `json:"summary"`
	Line    int         `json:"line"`
	Status  EntryStatus `json:"status"`
}

// SetView is a set as the map shows it.
type SetView struct {
	Name          string   `json:"name"`
	Title         string   `json:"title"`
	Summary       string   `json:"summary"`
	RelPath       string   `json:"rel_path"`
	Roles         []string `json:"roles"`
	Tags          []string `json:"tags"`
	Aliases       []string `json:"aliases"`
	Prerequisites []string `json:"prerequisites"`
	Mode          Mode     `json:"mode"`
	Origin        string   `json:"origin,omitempty"`
	Unreviewed    bool     `json:"unreviewed"`
	Entries       int      `json:"entries"`
	// Offered and Opened are the catalogue's own return on cost: a line
	// shown in every matching preflight and never resolved is paying rent.
	Offered  int  `json:"offered"`
	Opened   int  `json:"opened"`
	Expired  bool `json:"expired"`
	Dangling int  `json:"dangling"`
	Drifted  int  `json:"drifted"`
	// Items is the set opened up. Entries is kept alongside it because a list
	// header wants the number before it wants the rows, and because a caller
	// that only counts should not have to know the list is there.
	Items []EntryView `json:"items"`
}

// TermView is a glossary entry as the map shows it. Trust is derived, never
// declared, which is why it is reported rather than stored.
type TermView struct {
	Canonical   string   `json:"canonical"`
	Title       string   `json:"title,omitempty"`
	Description string   `json:"description,omitempty"`
	RelPath     string   `json:"rel_path"`
	Status      Status   `json:"status"`
	Trust       Trust    `json:"trust"`
	Aliases     []string `json:"aliases,omitempty"`
	Evidence    int      `json:"evidence"`
	Expired     bool     `json:"expired"`
}

// Health is the top-line judgement, precomputed so a tile and a command cannot
// disagree about whether this wiki is in trouble.
type Health struct {
	Decisions int `json:"decisions"`
	Dangling  int `json:"dangling"`
	Drifted   int `json:"drifted"`
	Expired   int `json:"expired"`
	Awaiting  int `json:"awaiting"`
	// Unreviewed counts sets an agent changed that nobody has looked at.
	Unreviewed int `json:"unreviewed"`
	Sets       int `json:"sets"`
	Entries    int `json:"entries"`
	// Answered counts misses that a set now matches. They are gone from the
	// backlog, and a number that quietly shrinks with no trace is a number
	// nobody trusts twice.
	Answered int `json:"answered"`
}

// Overview is the whole wiki as one value: what exists, what is wrong with it,
// and what is waiting on a person.
type Overview struct {
	Present   bool          `json:"present"`
	Health    Health        `json:"health"`
	Glossary  GlossaryUsage `json:"glossary"`
	Decisions []Decision    `json:"decisions"`
	Sets      []SetView     `json:"sets"`
	Terms     []TermView    `json:"terms"`
}

// BuildOverview gathers everything without opening the index.
//
// skillDir may be empty, and then generated blocks are not checked rather than
// reported as current — the difference matters to anyone reading the result to
// decide whether they are done.
func BuildOverview(root, skillDir string) (Overview, error) {
	store, err := Load(root)
	if err != nil {
		return Overview{}, err
	}
	o := Overview{
		Present:   store.Present,
		Glossary:  GlossaryUsage{Count: len(store.Terms), Cap: GlossaryCap},
		Decisions: []Decision{},
		Sets:      []SetView{},
		Terms:     []TermView{},
	}
	sets := store.SetList()
	friction := Summarize(ReadFriction(root))
	problems := Check(root, sets, store.Pins)
	today := time.Now().UTC().Format("2006-01-02")

	relBySet := map[string]string{}
	for _, s := range sets {
		relBySet[s.Name] = s.RelPath
	}
	danglingBySet := map[string]int{}
	driftBySet := map[string]int{}
	// Keyed by set then node so an entry can be told its own state below
	// without walking the problem list again per set.
	statusBySet := map[string]map[string]EntryStatus{}
	mark := func(set, node string, st EntryStatus) {
		if statusBySet[set] == nil {
			statusBySet[set] = map[string]EntryStatus{}
		}
		statusBySet[set][node] = st
	}
	for _, p := range problems {
		switch p.Kind {
		case ProblemDangling:
			mark(p.Set, p.NodeID, EntryDangling)
			danglingBySet[p.Set]++
			o.Decisions = append(o.Decisions, Decision{
				Kind: DecisionDangling, Set: p.Set, NodeID: p.NodeID,
				File: relBySet[p.Set], Line: p.Line,
				Title:  p.NodeID,
				Detail: "The target is gone — a heading was renamed or a file moved. The set now delivers less than its catalogue promised.",
				Action: "Fix the link in the set file (" + p.Set + ":" + itoa(p.Line) + ")",
			})
		case ProblemDrift:
			mark(p.Set, p.NodeID, EntryDrift)
			driftBySet[p.Set]++
			o.Decisions = append(o.Decisions, Decision{
				Kind: DecisionDrift, Set: p.Set, NodeID: p.NodeID,
				File: relBySet[p.Set], Line: p.Line,
				Count:  friction.Drifted[p.NodeID],
				Title:  p.NodeID,
				Detail: "The section changed since it was registered. The text is still served, but the one-line summary offering it may no longer hold.",
				Action: "Re-read it, confirm the summary still holds, then repin",
			})
		}
	}

	if len(store.Terms) >= GlossaryCap {
		o.Decisions = append(o.Decisions, Decision{
			Kind: DecisionGlossaryFull, Count: len(store.Terms),
			Title:  fmt.Sprintf("Glossary is full (%d/%d)", len(store.Terms), GlossaryCap),
			Detail: "No new term can be approved. What to displace is a judgement about which knowledge matters more, so nothing decides it for you.",
			Action: "Remove or demote an existing entry",
		})
	}

	for _, s := range sets {
		expired := s.Stale(today)
		if expired {
			o.Decisions = append(o.Decisions, Decision{
				Kind: DecisionExpired, Set: s.Name,
				File: s.RelPath, Line: 1,
				Title:  s.Name,
				Detail: "The set is past its own stale_after. That nothing changed is exactly the reason to check.",
				Action: "Re-verify the content and move stale_after forward",
			})
		}
		items := []EntryView{}
		for _, e := range s.Entries() {
			st := EntryOK
			if k, ok := statusBySet[s.Name][e.NodeID]; ok {
				st = k
			}
			items = append(items, EntryView{
				Title: e.Title, NodeID: e.NodeID, Summary: firstLine(e.Summary),
				Line: e.Line, Status: st,
			})
		}
		o.Sets = append(o.Sets, SetView{
			Name: s.Name, Title: s.Title, Summary: firstLine(s.Summary()),
			RelPath: s.RelPath, Roles: nonNil(s.Roles), Tags: nonNil(s.Tags),
			Aliases: nonNil(s.Aliases), Prerequisites: nonNil(s.Prerequisites), Mode: s.Mode,
			Origin: s.Origin, Unreviewed: s.Unreviewed,
			Entries: len(items), Items: items,
			Offered: friction.Matched[s.Name], Opened: friction.Resolved[s.Name],
			Expired: expired, Dangling: danglingBySet[s.Name], Drifted: driftBySet[s.Name],
		})
	}

	for _, s := range sets {
		if !s.Unreviewed {
			continue
		}
		o.Decisions = append(o.Decisions, Decision{
			Kind: DecisionUnreviewed, Set: s.Name,
			File: s.RelPath, Line: 1,
			Title:  s.Name,
			Detail: unreviewedDetail(s),
			Action: "Read the diff, then set reviewed: true in the frontmatter",
		})
	}

	for _, name := range friction.Unread() {
		o.Decisions = append(o.Decisions, Decision{
			Kind: DecisionUnreadSet, Set: name, Count: friction.Matched[name],
			File: relBySet[name], Line: 1,
			Title:  name,
			Detail: fmt.Sprintf("Offered %d times and never opened. A catalogue line costs every delegation and returns nothing.", friction.Matched[name]),
			Action: "Demote it or delete it",
		})
	}

	proposals, err := store.Queue()
	if err != nil {
		return Overview{}, err
	}
	for _, p := range proposals {
		o.Decisions = append(o.Decisions, Decision{
			Kind: DecisionApprove, Canonical: p.Canonical, Count: p.Seen,
			File: pathpkg.Join(DirName, proposeSubdir, safeName(p.Canonical)+".md"), Line: 1,
			Evidence: nonNil(p.Evidence),
			Title:    p.Canonical,
			Detail:   fmt.Sprintf("Proposed %d times, %d citations.", p.Seen, len(p.Evidence)),
			Action:   "Approving moves it into the glossary (nothing is committed)",
		})
	}

	if skillDir != "" {
		for _, sk := range store.SkillStates(skillDir) {
			d := Decision{Kind: DecisionStaleSkill, Title: sk.Name, Role: sk.Role}
			if sk.Missing {
				d.Detail = "The role skill has never been generated, so agents in this role are delegated without the conventions the wiki says always apply to them."
				d.Action = "Run `graphin wiki skills` and commit the result"
			} else {
				d.Detail = "The generated role block no longer matches what it was generated from."
				d.Action = "Run `graphin wiki skills` again and commit the result"
			}
			o.Decisions = append(o.Decisions, d)
		}
	}

	// Uncovered work, folded by task. One miss is an anecdote; the same
	// sentence three times is the strongest signal this system produces about
	// what to write next.
	//
	// # Why the answered ones are re-checked
	//
	// Every other kind here is derived from the current state and ends when
	// that state ends: fix the link and the dangling decision is gone. This one
	// was derived from an append-only log, so it was the only kind a person
	// could not finish — you wrote the set the card asked for and the card
	// stayed. Asking Select the same question the miss was recorded from is
	// what makes it behave like the other seven. It is not a retroactive sweep
	// over old work: nothing new is discovered here, a decision whose condition
	// stopped holding is simply retired.
	seen := map[string]int{}
	last := map[string]string{}
	first := map[string]string{}
	order := []FrictionEvent{}
	for _, m := range friction.Misses { // newest first
		if seen[m.Task] == 0 {
			order = append(order, m)
			last[m.Task] = m.TS
		}
		first[m.Task] = m.TS
		seen[m.Task]++
	}
	for _, m := range order {
		if !store.Select(m.Role, m.Task).Empty() {
			o.Health.Answered++
			continue
		}
		n := seen[m.Task]
		o.Decisions = append(o.Decisions, Decision{
			Kind: DecisionUncovered, Count: n, Role: m.Role,
			Title:     m.Task,
			FirstSeen: day(first[m.Task]), LastSeen: day(last[m.Task]),
			Detail: fmt.Sprintf("Asked %s, most recently %s. Nothing in the wiki answered it.",
				times(n), day(last[m.Task])),
			Action: "Write a set entry or a term — the only way the wiki grows",
		})
	}

	for _, t := range store.Terms {
		o.Terms = append(o.Terms, TermView{
			Canonical: t.Canonical, Title: t.Title, Description: t.Description,
			RelPath: t.RelPath, Status: t.Status, Trust: t.Trust(),
			Aliases: nonNil(t.Aliases), Evidence: len(t.Evidence),
			Expired: t.Stale(today),
		})
	}
	sort.Slice(o.Terms, func(i, j int) bool { return o.Terms[i].Canonical < o.Terms[j].Canonical })

	for i := range o.Decisions {
		o.Decisions[i].Severity = severity[o.Decisions[i].Kind]
	}
	// Stable so equal-severity decisions keep the order they were gathered in,
	// which is document order for sets and newest-first for misses.
	sort.SliceStable(o.Decisions, func(i, j int) bool {
		return o.Decisions[i].Severity < o.Decisions[j].Severity
	})

	entries := 0
	for _, s := range o.Sets {
		entries += s.Entries
	}
	answered := o.Health.Answered
	o.Health = Health{
		Answered:   answered,
		Decisions:  len(o.Decisions),
		Dangling:   countKind(o.Decisions, DecisionDangling),
		Drifted:    countKind(o.Decisions, DecisionDrift),
		Expired:    countKind(o.Decisions, DecisionExpired),
		Awaiting:   len(proposals),
		Unreviewed: countKind(o.Decisions, DecisionUnreviewed),
		Sets:       len(o.Sets), Entries: entries,
	}
	return o, nil
}

// unreviewedDetail says what kind of agent change is waiting. A set an agent
// wrote from nothing and a set an agent repaired one line of are the same
// decision with very different reading loads.
func unreviewedDetail(s *Set) string {
	if s.Origin == OriginAgent {
		return "An agent wrote this set and no person has reviewed it. Every section it serves carries the flag until someone does."
	}
	return "An agent changed this set — a repointed entry, a rewritten summary or catalogue line — and no person has reviewed it. The change is served now; the flag is the control."
}

func countKind(ds []Decision, k DecisionKind) int {
	n := 0
	for _, d := range ds {
		if d.Kind == k {
			n++
		}
	}
	return n
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func itoa(n int) string { return fmt.Sprintf("%d", n) }

// day trims an RFC3339 timestamp to its date. Timestamps are written by the
// hook and are not guaranteed to be well formed, so anything shorter comes
// back as it arrived rather than being cut into nonsense.
func day(ts string) string {
	if len(ts) >= 10 {
		return ts[:10]
	}
	return ts
}

// times renders a recurrence count as English, because "Asked 1 times" is the
// kind of seam that makes a reader stop trusting the rest of the sentence.
func times(n int) string {
	switch n {
	case 1:
		return "once"
	case 2:
		return "twice"
	default:
		return fmt.Sprintf("%d times", n)
	}
}

// firstLine keeps a card readable: a set's summary falls back to its whole
// intro paragraph, which is prose meant for a different context.
func firstLine(s string) string {
	for i, r := range s {
		if r == '\n' {
			return s[:i]
		}
	}
	return s
}

// RepinResult reports what a repin did, in the same numbers the command prints.
type RepinResult struct {
	Added    int       `json:"added"`
	Updated  int       `json:"updated"`
	Dropped  int       `json:"dropped"`
	Problems []Problem `json:"problems"`
	Path     string    `json:"path"`
	// Wrote is false for a dry run.
	Wrote bool `json:"wrote"`
}

// RepinEntry re-pins one entry and leaves every other pin exactly as it was.
//
// RepinAll is for the moment a person has read everything. This is for the
// moment the drift card actually describes: re-read *this* section, confirm
// *this* summary still holds, then record that you did. Repinning everything
// after verifying one entry vouches for the ones nobody opened, which is the
// opposite of what a pin is for — and it does it silently, by making their
// warnings disappear.
//
// The entry has to exist in that set. A pin written for a pair no set names
// would survive until the next RepinAll dropped it, and in between the
// lockfile would assert something no document backs.
func RepinEntry(root, set, nodeID string) (RepinResult, error) {
	store, err := Load(root)
	if err != nil {
		return RepinResult{}, err
	}
	res := RepinResult{Problems: []Problem{}, Path: store.PinsPath()}

	found := false
	for _, s := range store.SetList() {
		if s.Name != set {
			continue
		}
		for _, id := range s.NodeIDs() {
			if id == nodeID {
				found = true
			}
		}
	}
	if !found {
		return res, fmt.Errorf("%w: %s does not list %s", ErrNoEntry, set, nodeID)
	}

	cur, ok := NewHasher(root).Pin(nodeID)
	if !ok {
		// Pinning a dangling ID would record a hash for nothing and hide the
		// break behind a green check — same rule Repin follows.
		res.Problems = append(res.Problems, Problem{ProblemDangling, set, nodeID, 0,
			"no such node — not pinned"})
		return res, nil
	}
	switch old, pinned := store.Pins.Get(set, nodeID); {
	case !pinned:
		res.Added++
	case old != cur:
		res.Updated++
	}
	store.Pins.Set(set, nodeID, cur)
	if err := store.Pins.Save(store.PinsPath()); err != nil {
		return res, err
	}
	res.Wrote = true
	return res, nil
}

// RepinAll rebuilds every pin from the documents themselves.
//
// Rebuild rather than merge, which is why Dropped exists: a pin for an entry no
// longer in any set goes away, and an author who deleted that entry should see
// that its record went with it rather than discover the file quietly kept it.
//
// This writes docs/wiki/pins.lock and stops there. Same boundary as approving:
// the file lands in the working tree, the commit is the reviewer's (see
// docs/console-spec.md §8).
func RepinAll(root string, dry bool) (RepinResult, error) {
	store, err := Load(root)
	if err != nil {
		return RepinResult{}, err
	}
	sets := store.SetList()
	pins, problems := Repin(root, sets)

	res := RepinResult{Problems: problems, Path: store.PinsPath()}
	if res.Problems == nil {
		res.Problems = []Problem{}
	}
	for set, byNode := range pins.Pins {
		for id, h := range byNode {
			switch old, ok := store.Pins.Get(set, id); {
			case !ok:
				res.Added++
			case old != h:
				res.Updated++
			}
		}
	}
	for set, byNode := range store.Pins.Pins {
		for id := range byNode {
			if _, ok := pins.Get(set, id); !ok {
				res.Dropped++
			}
		}
	}
	if dry {
		return res, nil
	}
	if err := pins.Save(store.PinsPath()); err != nil {
		return res, err
	}
	res.Wrote = true
	return res, nil
}
