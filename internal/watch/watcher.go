package watch

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/rjeczalik/notify"

	"github.com/llls2542/graphin/internal/obs"
)

// GraphinDirName is the workspace data directory, excluded from watching
// unconditionally: index writes must never re-trigger indexing.
const GraphinDirName = ".graphin"

// Watcher feeds filesystem events under root into a Debouncer.
type Watcher struct {
	root string
	c    chan notify.EventInfo
	deb  *Debouncer
	log  *obs.Logger
}

// NewWatcher starts a recursive watch on root. Call Run to pump events.
func NewWatcher(root string, deb *Debouncer, lg *obs.Logger) (*Watcher, error) {
	c := make(chan notify.EventInfo, 4096)
	err := notify.Watch(filepath.Join(root, "..."), c,
		notify.Create, notify.Write, notify.Remove, notify.Rename)
	if err != nil {
		return nil, err
	}
	return &Watcher{root: root, c: c, deb: deb, log: lg}, nil
}

// Run translates and filters events until ctx is cancelled.
func (w *Watcher) Run(ctx context.Context) {
	defer notify.Stop(w.c)
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-w.c:
			if !ok {
				return
			}
			path := ev.Path()
			if InsideGraphinDir(w.root, path) {
				continue
			}
			kind := Modified
			if ev.Event()&(notify.Remove|notify.Rename) != 0 {
				kind = Removed
			}
			select {
			case w.deb.In() <- Change{Path: path, Kind: kind}:
			case <-ctx.Done():
				return
			}
		}
	}
}

// InsideGraphinDir reports whether path lies inside root's .graphin directory.
func InsideGraphinDir(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == GraphinDirName || strings.HasPrefix(rel, GraphinDirName+string(filepath.Separator))
}
