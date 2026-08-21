package wiki

import (
	"fmt"
	"io"
	"sort"
)

// QueueReport is everything `graphin wiki queue` knows, as a value.
//
// The command used to print straight to an io.Writer. That made a second
// consumer — a console serving the same view over HTTP — impossible to build
// without reimplementing the question, and two implementations of one question
// drift until they disagree in front of a user. So the shape is the contract:
// the command renders it, --json marshals it, and anything else serves that
// JSON unchanged (docs/console-spec.md §5).
//
// It carries no limit and no cursor. Truncation is a terminal's problem; a
// reader with a scrollbar wants the whole list.
type QueueReport struct {
	Glossary GlossaryUsage    `json:"glossary"`
	Awaiting []QueuedProposal `json:"awaiting_review"`
	// Misses are the tasks the wiki had no answer for, newest first. This is
	// the list new knowledge gets written from — there is no retroactive sweep.
	Misses []FrictionEvent `json:"misses"`
	// Unread are sets offered in catalogue after catalogue and never opened.
	Unread []UnreadSet `json:"unread_sets"`
	// Drifted are sections served while their pin no longer matched.
	Drifted []DriftedNode `json:"drifted"`
}

// GlossaryUsage is how full the glossary is. The cap is a decision point
// rather than a limit to raise: displacing an entry is a judgement about which
// knowledge matters more, and that is left to a person.
type GlossaryUsage struct {
	Count int `json:"count"`
	Cap   int `json:"cap"`
}

// QueuedProposal is one candidate waiting for a person.
//
// File travels with it because approving *is* moving that file into
// docs/wiki/glossary/. A consumer that can only name a candidate cannot act on
// one, and the whole point of the queue is that something happens next.
type QueuedProposal struct {
	Canonical string   `json:"canonical"`
	File      string   `json:"file"`
	Seen      int      `json:"seen"`
	Evidence  []string `json:"evidence"`
}

// UnreadSet is a set that keeps being offered and never resolved.
type UnreadSet struct {
	Set     string `json:"set"`
	Offered int    `json:"offered"`
}

// DriftedNode is a section that was served against a stale pin.
type DriftedNode struct {
	Node   string `json:"node"`
	Served int    `json:"served"`
}

// BuildQueueReport gathers the queue without opening the index.
//
// Every source here is a plain file — proposals under docs/wiki, the friction
// log as JSONL — which is what lets this run while the MCP server holds the
// workspace. Opening the graph engine truncates its delta log, so a second
// process that wanted these numbers from the index could not safely have them
// (see the package comment, and internal/mcp/tools/diagnose.go for the same
// constraint met from the other side).
func BuildQueueReport(root string) (QueueReport, error) {
	store, err := Load(root)
	if err != nil {
		return QueueReport{}, err
	}
	proposals, err := store.Queue()
	if err != nil {
		return QueueReport{}, err
	}
	friction := Summarize(ReadFriction(root))

	// Every slice starts empty rather than nil. This report is marshalled to
	// JSON for a browser, and there the difference is not cosmetic: `null`
	// turns an ordinary `.map()` over an empty queue into a crash, and an
	// empty queue is the state the console spends most of its life in.
	q := QueueReport{
		Glossary: GlossaryUsage{Count: len(store.Terms), Cap: GlossaryCap},
		Awaiting: []QueuedProposal{},
		Misses:   []FrictionEvent{},
		Unread:   []UnreadSet{},
		Drifted:  []DriftedNode{},
	}
	if friction.Misses != nil {
		q.Misses = friction.Misses
	}
	for _, p := range proposals {
		evidence := p.Evidence
		if evidence == nil {
			evidence = []string{}
		}
		q.Awaiting = append(q.Awaiting, QueuedProposal{
			Canonical: p.Canonical,
			File:      p.File,
			Seen:      p.Seen,
			Evidence:  evidence,
		})
	}
	for _, set := range friction.Unread() {
		q.Unread = append(q.Unread, UnreadSet{Set: set, Offered: friction.Matched[set]})
	}
	nodes := make([]string, 0, len(friction.Drifted))
	for n := range friction.Drifted {
		nodes = append(nodes, n)
	}
	sort.Strings(nodes)
	for _, n := range nodes {
		q.Drifted = append(q.Drifted, DriftedNode{Node: n, Served: friction.Drifted[n]})
	}
	return q, nil
}

// RenderQueue writes the human-facing form. misses caps the coverage-miss list
// because it is the only section that grows without bound; the others are
// short by construction.
func RenderQueue(w io.Writer, q QueueReport, misses int) {
	fmt.Fprintf(w, "glossary: %d of %d\n\n", q.Glossary.Count, q.Glossary.Cap)

	fmt.Fprintf(w, "awaiting review (%d)\n", len(q.Awaiting))
	for _, p := range q.Awaiting {
		fmt.Fprintf(w, "  %-24s seen %d, %d citation(s)\n", p.Canonical, p.Seen, len(p.Evidence))
	}
	if len(q.Awaiting) == 0 {
		fmt.Fprintln(w, "  (nothing)")
	}

	fmt.Fprintf(w, "\nwork the wiki had no answer for (%d, newest first)\n", len(q.Misses))
	for i, m := range q.Misses {
		if i >= misses {
			fmt.Fprintf(w, "  … %d more\n", len(q.Misses)-misses)
			break
		}
		role := m.Role
		if role == "" {
			role = "-"
		}
		fmt.Fprintf(w, "  [%s] %s\n", role, m.Task)
	}
	if len(q.Misses) == 0 {
		fmt.Fprintln(w, "  (nothing)")
	}

	if len(q.Unread) > 0 {
		fmt.Fprintf(w, "\noffered but never opened (%d)\n", len(q.Unread))
		for _, s := range q.Unread {
			fmt.Fprintf(w, "  %-24s offered %d times, resolved 0\n", s.Set, s.Offered)
		}
	}

	if len(q.Drifted) > 0 {
		fmt.Fprintf(w, "\nserved with a stale pin (%d)\n", len(q.Drifted))
		for _, d := range q.Drifted {
			fmt.Fprintf(w, "  %s (%d)\n", d.Node, d.Served)
		}
		fmt.Fprintln(w, "  re-read each, confirm the summary still holds, then `graphin wiki repin`")
	}
}
