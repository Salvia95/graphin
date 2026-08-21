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
				Detail: "이 엔트리의 대상이 사라졌다 — 헤딩이 바뀌었거나 파일이 옮겨졌다. 세트가 약속한 것보다 적게 준다.",
				Action: "세트 파일의 링크를 고친다 (" + p.Set + ":" + itoa(p.Line) + ")",
			})
		case ProblemDrift:
			driftBySet[p.Set]++
			o.Decisions = append(o.Decisions, Decision{
				Kind: DecisionDrift, Set: p.Set, NodeID: p.NodeID,
				Count:  friction.Drifted[p.NodeID],
				Title:  p.NodeID,
				Detail: "본문이 등록 이후 바뀌었다. 텍스트는 계속 나가지만 카탈로그의 한 줄 요약이 더는 안 맞을 수 있다.",
				Action: "다시 읽고 요약이 여전히 맞는지 확인한 뒤 repin",
			})
		}
	}

	if len(store.Terms) >= GlossaryCap {
		o.Decisions = append(o.Decisions, Decision{
			Kind: DecisionGlossaryFull, Count: len(store.Terms),
			Title:  fmt.Sprintf("용어집이 찼다 (%d/%d)", len(store.Terms), GlossaryCap),
			Detail: "새 용어를 승인할 수 없다. 무엇을 밀어낼지는 어느 지식이 더 중요한가에 대한 판단이라 자동으로 정해지지 않는다.",
			Action: "기존 항목 하나를 지우거나 강등한다",
		})
	}

	for _, s := range sets {
		expired := s.Stale(today)
		if expired {
			o.Decisions = append(o.Decisions, Decision{
				Kind: DecisionExpired, Set: s.Name,
				Title:  s.Name,
				Detail: "세트가 자기 stale_after를 지났다. 아무것도 안 바뀌었다는 것이 바로 확인할 이유다.",
				Action: "내용을 다시 확인하고 stale_after를 갱신한다",
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
			Detail: fmt.Sprintf("%d번 제시됐고 한 번도 열리지 않았다. 카탈로그 한 줄은 매 위임마다 비용을 물리고 아무것도 돌려주지 않는다.", friction.Matched[name]),
			Action: "강등하거나 지운다",
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
			Detail:   fmt.Sprintf("%d회 제안됨, %d개 인용.", p.Seen, len(p.Evidence)),
			Action:   "승인하면 용어집으로 옮긴다 (커밋은 하지 않는다)",
		})
	}

	if skillDir != "" {
		for _, name := range store.StaleSkills(skillDir) {
			o.Decisions = append(o.Decisions, Decision{
				Kind: DecisionStaleSkill, Title: name,
				Detail: "생성된 role 블록이 생성 근거와 더는 일치하지 않는다.",
				Action: "`graphin wiki skills`를 다시 돌리고 결과를 커밋한다",
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
			Detail: "이 작업에 위키가 답을 갖고 있지 않았다.",
			Action: "세트나 용어를 쓴다 — 위키가 자라는 유일한 경로다",
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
