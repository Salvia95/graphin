// Package watch turns raw filesystem events into deduplicated batches for the
// indexer: recursive watching via rjeczalik/notify plus a 500ms debouncer
// (§3.1) that absorbs editor atomic saves and git branch switches.
package watch

import (
	"context"
	"time"
)

// ChangeKind classifies what the indexer must do with a path.
type ChangeKind int

const (
	Modified ChangeKind = iota // created or written: (re)parse
	Removed                    // removed or renamed away: drop from index
)

// Change is one coalesced filesystem event.
type Change struct {
	Path string
	Kind ChangeKind
}

// Batch maps path → latest observed change.
type Batch map[string]Change

// Debouncer coalesces a noisy event stream into batches. A batch fires after
// `quiet` with no new events, or at most `maxWait` after its first event so a
// continuous stream cannot starve consumers.
type Debouncer struct {
	in      chan Change
	out     chan Batch
	quiet   time.Duration
	maxWait time.Duration
}

// NewDebouncer builds a debouncer; non-positive durations select the defaults
// (500ms quiet window, 2s max latency).
func NewDebouncer(quiet, maxWait time.Duration) *Debouncer {
	if quiet <= 0 {
		quiet = 500 * time.Millisecond
	}
	if maxWait <= 0 {
		maxWait = 2 * time.Second
	}
	return &Debouncer{
		in:      make(chan Change, 4096),
		out:     make(chan Batch, 8),
		quiet:   quiet,
		maxWait: maxWait,
	}
}

func (d *Debouncer) In() chan<- Change { return d.in }

func (d *Debouncer) Out() <-chan Batch { return d.out }

// Run processes events until ctx is cancelled.
func (d *Debouncer) Run(ctx context.Context) {
	var (
		pending Batch
		quietT  = time.NewTimer(time.Hour)
		deadT   = time.NewTimer(time.Hour)
		quietC  <-chan time.Time
		deadC   <-chan time.Time
	)
	stopTimer := func(t *time.Timer, c *<-chan time.Time) {
		if !t.Stop() && *c != nil {
			select {
			case <-t.C:
			default:
			}
		}
		*c = nil
	}
	stopTimer(quietT, &quietC)
	stopTimer(deadT, &deadC)

	flush := func() {
		stopTimer(quietT, &quietC)
		stopTimer(deadT, &deadC)
		if len(pending) == 0 {
			return
		}
		select {
		case d.out <- pending:
			pending = nil
		case <-ctx.Done():
		}
	}

	for {
		select {
		case <-ctx.Done():
			return
		case c := <-d.in:
			if pending == nil {
				pending = Batch{}
				stopTimer(deadT, &deadC)
				deadT.Reset(d.maxWait)
				deadC = deadT.C
			}
			pending[c.Path] = c
			stopTimer(quietT, &quietC)
			quietT.Reset(d.quiet)
			quietC = quietT.C
		case <-quietC:
			quietC = nil
			flush()
		case <-deadC:
			deadC = nil
			flush()
		}
	}
}
