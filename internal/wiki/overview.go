package wiki

import (
	"fmt"
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
	DecisionApprove:      4,
	DecisionStaleSkill:   5,
	DecisionUnreadSet:    6,
	DecisionUncovered:    7,
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
	// Count is how many times the same thing has been seen: proposals
	// resubmitted, sections served stale, tasks that found nothing. One
	// occurrence is an anecdote.
	Count int `json:"count,omitempty"`
	// Evidence carries the citations for a candidate.
	Evidence []string `json:"evidence,omitempty"`
	// Role is the delegate's role for an uncovered task.
	Role string `json:"role,omitempty"`
}

// SetView is a set as the map shows it.
type SetView struct {
	Name          string   `json:"name"`
	Title         string   `json:"title"`
	Summary       string   `json:"summary"`
	RelPath       string   `json:"rel_path"`
	Roles         []string `json:"roles"`
	Tags          []string `json:"tags"`
	Prerequisites []string `json:"prerequisites"`
	Mode          Mode     `json:"mode"`
	Entries       int      `json:"entries"`
	// Offered and Opened are the catalogue's own return on cost: a line
	// shown in every matching preflight and never resolved is paying rent.
	Offered  int  `json:"offered"`
	Opened   int  `json:"opened"`
	Expired  bool `json:"expired"`
	Dangling int  `json:"dangling"`
	Drifted  int  `json:"drifted"`
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
	Sets      int `json:"sets"`
	Entries   int `json:"entries"`
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

	danglingBySet := map[string]int{}
	driftBySet := map[string]int{}
	for _, p := range problems {
		switch p.Kind {
		case ProblemDangling:
			danglingBySet[p.Set]++
			o.Decisions = append(o.Decisions, Decision{
				Kind: DecisionDangling, Set: p.Set, NodeID: p.NodeID,
				Title:  p.NodeID,
				Detail: "The target is gone — a heading was renamed or a file moved. The set now delivers less than its catalogue promised.",
				Action: "Fix the link in the set file (" + p.Set + ":" + itoa(p.Line) + ")",
			})
		case ProblemDrift:
			driftBySet[p.Set]++
			o.Decisions = append(o.Decisions, Decision{
				Kind: DecisionDrift, Set: p.Set, NodeID: p.NodeID,
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
				Title:  s.Name,
				Detail: "The set is past its own stale_after. That nothing changed is exactly the reason to check.",
				Action: "Re-verify the content and move stale_after forward",
			})
		}
		o.Sets = append(o.Sets, SetView{
			Name: s.Name, Title: s.Title, Summary: firstLine(s.Summary()),
			RelPath: s.RelPath, Roles: nonNil(s.Roles), Tags: nonNil(s.Tags),
			Prerequisites: nonNil(s.Prerequisites), Mode: s.Mode,
			Entries: len(s.Entries()),
			Offered: friction.Matched[s.Name], Opened: friction.Resolved[s.Name],
			Expired: expired, Dangling: danglingBySet[s.Name], Drifted: driftBySet[s.Name],
		})
	}

	for _, name := range friction.Unread() {
		o.Decisions = append(o.Decisions, Decision{
			Kind: DecisionUnreadSet, Set: name, Count: friction.Matched[name],
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
			Evidence: nonNil(p.Evidence),
			Title:    p.Canonical,
			Detail:   fmt.Sprintf("Proposed %d times, %d citations.", p.Seen, len(p.Evidence)),
			Action:   "Approving moves it into the glossary (nothing is committed)",
		})
	}

	if skillDir != "" {
		for _, name := range store.StaleSkills(skillDir) {
			o.Decisions = append(o.Decisions, Decision{
				Kind: DecisionStaleSkill, Title: name,
				Detail: "The generated role block no longer matches what it was generated from.",
				Action: "Run `graphin wiki skills` again and commit the result",
			})
		}
	}

	// Uncovered work, folded by task. One miss is an anecdote; the same
	// sentence three times is the strongest signal this system produces about
	// what to write next.
	seen := map[string]int{}
	order := []FrictionEvent{}
	for _, m := range friction.Misses {
		if seen[m.Task] == 0 {
			order = append(order, m)
		}
		seen[m.Task]++
	}
	for _, m := range order {
		o.Decisions = append(o.Decisions, Decision{
			Kind: DecisionUncovered, Count: seen[m.Task], Role: m.Role,
			Title:  m.Task,
			Detail: "The wiki had no answer for this work.",
			Action: "Write a set or a term — the only way the wiki grows",
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
	o.Health = Health{
		Decisions: len(o.Decisions),
		Dangling:  countKind(o.Decisions, DecisionDangling),
		Drifted:   countKind(o.Decisions, DecisionDrift),
		Expired:   countKind(o.Decisions, DecisionExpired),
		Awaiting:  len(proposals),
		Sets:      len(o.Sets), Entries: entries,
	}
	return o, nil
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
