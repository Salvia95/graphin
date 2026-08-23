package console

import (
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Salvia95/graphin/internal/wiki"
)

// The console tells people to go and fix things, and every one of those fixes
// happens in an editor. Before this, the fix landed and the card stayed — the
// page had read the files once and had no reason to think they had moved.
//
// # Why polling rather than a filesystem watcher
//
// internal/watch exists and works, but it takes a recursive OS watch on the
// tree. This repository has node_modules under internal/console/ui, and a
// recursive inotify watch over a workspace like that is how you exhaust the
// per-user watch limit for the whole machine — including the MCP server's own
// watcher, which is the one that actually needs it.
//
// What the console reads is small and known: the wiki, the documents its
// entries point into, the generated skills, and the friction log. Stat-ing that
// set once a second while somebody is looking costs less than the watch would,
// has no limit to exhaust, and cannot leave a stale handle behind when the
// process ends. It also degrades honestly — a bigger wiki means more stats, not
// a silent failure to notice.
const pollInterval = time.Second

// heartbeat keeps intermediaries from deciding an idle stream is a dead one.
const heartbeat = 20 * time.Second

// watchState is one client's view of the files the console reads.
//
// refs is cached and only rebuilt when the wiki itself changes, because
// deriving it means parsing every set file. The wiki changing is exactly when
// the set of referenced documents can change, so nothing is missed by waiting
// for that.
type watchState struct {
	root string
	wiki string
	refs []string
}

func (s *watchState) digest() string {
	w := treeDigest(filepath.Join(s.root, filepath.FromSlash(wiki.DirName)))
	if w != s.wiki {
		s.wiki = w
		s.refs = referenced(s.root)
	}
	h := fnv.New64a()
	_, _ = io.WriteString(h, w)
	_, _ = io.WriteString(h, treeDigest(filepath.Join(s.root, wiki.DefaultSkillDir)))
	for _, p := range s.refs {
		stamp(h, p)
	}
	// The friction log is the only input that changes without anyone editing a
	// file — an agent hitting a coverage miss moves the backlog underneath a
	// person who is reading it.
	stamp(h, wiki.FrictionPath(s.root))
	stamp(h, wiki.FrictionPath(s.root)+".1")
	return strconv.FormatUint(h.Sum64(), 16)
}

// referenced lists the documents the wiki's entries point into. Drift is the
// hash of those files' sections, so a set that never changes can still stop
// being true when one of them is edited.
func referenced(root string) []string {
	store, err := wiki.Load(root)
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, s := range store.SetList() {
		for _, id := range s.NodeIDs() {
			file := id
			if i := strings.IndexByte(file, '#'); i >= 0 {
				file = file[:i]
			}
			if file == "" || seen[file] {
				continue
			}
			seen[file] = true
			out = append(out, filepath.Join(root, filepath.FromSlash(file)))
		}
	}
	return out
}

func stamp(h io.Writer, path string) {
	fi, err := os.Stat(path)
	if err != nil {
		// Absence is a state worth hashing: a deleted file has to read as a
		// change, not as "nothing to add".
		fmt.Fprintf(h, "%s|-\n", path)
		return
	}
	fmt.Fprintf(h, "%s|%d|%d\n", path, fi.ModTime().UnixNano(), fi.Size())
}

func treeDigest(dir string) string {
	h := fnv.New64a()
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		fi, err := d.Info()
		if err != nil {
			return nil
		}
		fmt.Fprintf(h, "%s|%d|%d\n", p, fi.ModTime().UnixNano(), fi.Size())
		return nil
	})
	if err != nil {
		fmt.Fprintf(h, "walk-error|%v\n", err)
	}
	return strconv.FormatUint(h.Sum64(), 16)
}

// eventsHandler streams a line whenever those files change.
//
// It sends no data of its own — only "something moved". The client already
// knows how to fetch the views it is showing, and a stream that also carried
// the payload would be a second way to answer questions the JSON endpoints
// already answer, which is the thing docs/console-spec.md §5 exists to prevent.
func eventsHandler(root string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			writeError(w, http.StatusInternalServerError, errors.New("this server cannot stream"))
			return
		}
		h := w.Header()
		h.Set("Content-Type", "text/event-stream")
		h.Set("Cache-Control", "no-cache")
		h.Set("Connection", "keep-alive")
		// The reader is a browser on the same machine and there is no proxy in
		// between, but nginx-style buffering turns a stream into a silence and
		// the header costs nothing.
		h.Set("X-Accel-Buffering", "no")
		w.WriteHeader(http.StatusOK)

		state := &watchState{root: root}
		last := state.digest()
		fmt.Fprintf(w, "event: hello\ndata: %s\n\n", last)
		flusher.Flush()

		tick := time.NewTicker(pollInterval)
		defer tick.Stop()
		idle := time.Duration(0)

		for {
			select {
			case <-r.Context().Done():
				return
			case <-tick.C:
				cur := state.digest()
				if cur != last {
					last = cur
					idle = 0
					fmt.Fprintf(w, "event: change\ndata: %s\n\n", cur)
					flusher.Flush()
					continue
				}
				if idle += pollInterval; idle >= heartbeat {
					idle = 0
					fmt.Fprint(w, ": ping\n\n")
					flusher.Flush()
				}
			}
		}
	}
}
