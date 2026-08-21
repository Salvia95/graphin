package wiki

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// FrictionFile is the wiki's own event log, under the runtime directory.
//
// It is not the usage log, and that is a decision rather than an oversight.
// The usage pipeline's unit is "a tool call the PostToolUse hook observed",
// and its report turns those into the adoption metrics. A coverage verdict is
// not a tool call shape — the hook would have to parse a tool's XML response
// to derive one — and synthetic rows in events.jsonl would move numbers that
// are supposed to count real calls. Two logs, two questions.
const FrictionFile = "friction.jsonl"

// frictionRotateBytes bounds the log. It is diagnostic input, not a ledger:
// losing the oldest misses costs a little history and nothing else.
const frictionRotateBytes = 8 << 20

// maxTaskLen truncates the recorded task description. The log is meant to be
// read by a person deciding what to write, so a paragraph helps nobody, and a
// prompt kept whole is a privacy surface with no upside.
const maxTaskLen = 300

// FrictionKind classifies one recorded event.
type FrictionKind string

const (
	// FrictionMiss is preflight finding nothing. This is the generation
	// trigger: the wiki is grown from work that wanted knowledge and did not
	// get it, never from a retroactive sweep of the documents.
	FrictionMiss FrictionKind = "coverage_miss"
	// FrictionHit is preflight matching, recorded so that a set nobody ever
	// resolves can be told from one nobody ever matches.
	FrictionHit FrictionKind = "coverage_hit"
	// FrictionResolve is knowledge actually loaded. The gap between hits and
	// resolves is the push/pull demotion signal.
	FrictionResolve FrictionKind = "resolve"
	// FrictionDrift is an entry served with a stale pin: the re-verification
	// queue, captured where it is noticed rather than where it is fixed.
	FrictionDrift FrictionKind = "drift"
)

// FrictionEvent is one line of the log.
type FrictionEvent struct {
	V    int          `json:"v"`
	TS   string       `json:"ts"`
	Kind FrictionKind `json:"kind"`
	Task string       `json:"task,omitempty"`
	Role string       `json:"role,omitempty"`
	Sets []string     `json:"sets,omitempty"`
	Node string       `json:"node,omitempty"`
}

// FrictionPath is the log's location.
func FrictionPath(root string) string {
	return filepath.Join(root, filepath.FromSlash(RuntimeSubdir), FrictionFile)
}

// AppendFriction records one event. It never returns an error to the caller's
// detriment: this runs inside tool handlers, and a full disk must not turn a
// knowledge lookup into a failure.
func AppendFriction(root string, ev FrictionEvent) {
	ev.V = 1
	if ev.TS == "" {
		ev.TS = time.Now().UTC().Format(time.RFC3339)
	}
	ev.Task = truncate(ev.Task, maxTaskLen)

	path := FrictionPath(root)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	if fi, err := os.Stat(path); err == nil && fi.Size() > frictionRotateBytes {
		_ = os.Rename(path, path+".1")
	}
	line, err := json.Marshal(ev)
	if err != nil {
		return
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(append(line, '\n'))
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	cut := n
	for cut > 0 && (s[cut]&0xC0) == 0x80 {
		cut--
	}
	return s[:cut]
}

// ReadFriction loads the log, oldest first. A malformed line is skipped
// rather than fatal — the log is diagnostic, and refusing to report because
// one line is broken helps nobody.
func ReadFriction(root string) []FrictionEvent {
	var out []FrictionEvent
	for _, p := range []string{FrictionPath(root) + ".1", FrictionPath(root)} {
		f, err := os.Open(p)
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 0, 64<<10), 1<<20)
		for sc.Scan() {
			var ev FrictionEvent
			if json.Unmarshal(sc.Bytes(), &ev) == nil && ev.Kind != "" {
				out = append(out, ev)
			}
		}
		f.Close()
	}
	return out
}

// FrictionReport summarizes the log into the three questions it exists to
// answer: what to write, what to demote, and what to re-verify.
type FrictionReport struct {
	// Misses are the tasks the wiki had nothing for, most recent first.
	// This is the generation queue.
	Misses []FrictionEvent
	// Matched counts how often each set was offered in a catalogue.
	Matched map[string]int
	// Resolved counts how often each set's content was actually loaded.
	// A set matched often and resolved never is a catalogue line nobody
	// finds worth opening.
	Resolved map[string]int
	// Drifted counts stale-pin servings per node: the re-verification queue.
	Drifted map[string]int
}

// Summarize turns raw events into the report.
func Summarize(events []FrictionEvent) FrictionReport {
	r := FrictionReport{
		Matched:  map[string]int{},
		Resolved: map[string]int{},
		Drifted:  map[string]int{},
	}
	for _, ev := range events {
		switch ev.Kind {
		case FrictionMiss:
			r.Misses = append(r.Misses, ev)
		case FrictionHit:
			for _, s := range ev.Sets {
				r.Matched[s]++
			}
		case FrictionResolve:
			for _, s := range ev.Sets {
				r.Resolved[s]++
			}
		case FrictionDrift:
			r.Drifted[ev.Node]++
		}
	}
	sort.SliceStable(r.Misses, func(i, j int) bool { return r.Misses[i].TS > r.Misses[j].TS })
	return r
}

// Unread lists sets that keep being offered and never opened, worst first.
// §5.1 demotes these from the push block to the catalogue; a set nobody opens
// is costing every delegation and returning nothing.
func (r FrictionReport) Unread() []string {
	var out []string
	for set, matched := range r.Matched {
		if matched >= 3 && r.Resolved[set] == 0 {
			out = append(out, set)
		}
	}
	sort.Strings(out)
	return out
}
