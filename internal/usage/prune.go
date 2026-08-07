package usage

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// PruneResult is what a prune did (or, in dry-run, would do).
type PruneResult struct {
	Removed   []string `json:"removed"`   // files whose every event was older than the cutoff
	Rewritten []string `json:"rewritten"` // files that lost some events but not all
	Rotated   string   `json:"rotated,omitempty"`
	Kept      int      `json:"kept"`
	Dropped   int      `json:"dropped"`
	Malformed int      `json:"malformed"` // unparseable/undated lines, dropped with the rest
}

// Prune deletes events older than cutoff from a usage log directory.
//
// Why the active file is rotated first instead of filtered in place:
// hooks/usage.sh runs `usage ingest` on every tool call of every live session,
// and appendLine takes no lock — it opens O_APPEND, writes one line, closes.
// Reading events.jsonl and renaming a filtered copy over it would lose any
// line written in between, and that is a race this package does not otherwise
// have.
//
// Renaming the active file first has the same shape as the 32MiB rotation in
// appendLine, whose comment already accepts the one race it leaves: a writer
// that opened its fd before the rename keeps appending to the renamed file.
// So this reuses an accepted race instead of introducing a new one. The
// worst case is one straggler event surviving in a pruned file, and the next
// prune takes it.
//
// The active file is only rotated when it actually holds something older than
// the cutoff — a prune that has nothing to do must not churn the log layout.
func Prune(dir string, cutoff time.Time, dryRun bool) (PruneResult, error) {
	var res PruneResult

	fi, err := os.Stat(dir)
	if err != nil {
		return res, err
	}
	if !fi.IsDir() {
		return res, fmt.Errorf("%s is not a directory (prune works on a .graphin/usage dir)", dir)
	}

	active := filepath.Join(dir, "events.jsonl")
	if stale, err := hasEventsBefore(active, cutoff); err != nil {
		if !os.IsNotExist(err) {
			return res, err
		}
	} else if stale {
		rotated := filepath.Join(dir, "events-"+cutoff.UTC().Format("20060102T150405Z")+"-pruned.jsonl")
		res.Rotated = filepath.Base(rotated)
		if !dryRun {
			if err := os.Rename(active, rotated); err != nil {
				return res, err
			}
		}
	}

	files, err := filepath.Glob(filepath.Join(dir, LogGlob))
	if err != nil {
		return res, err
	}
	sort.Strings(files)

	for _, f := range files {
		// In dry-run the active file was never rotated, so it still has to be
		// counted — otherwise the preview under-reports exactly the file the
		// user most wants to know about.
		if !dryRun && f == active {
			continue
		}
		kept, dropped, malformed, err := partitionByTime(f, cutoff)
		if err != nil {
			return res, err
		}
		res.Kept += len(kept)
		res.Dropped += dropped
		res.Malformed += malformed
		if dropped == 0 && malformed == 0 {
			continue
		}
		base := filepath.Base(f)
		// In a real run the active file has already become res.Rotated by the
		// time it is filtered. Name it that way in the preview too, or the
		// dry run reads as "rotate it AND rewrite it" — two operations on one
		// file, which is not what happens.
		if dryRun && f == active && res.Rotated != "" {
			base = res.Rotated
		}
		if len(kept) == 0 {
			res.Removed = append(res.Removed, base)
			if !dryRun {
				if err := os.Remove(f); err != nil {
					return res, err
				}
			}
			continue
		}
		res.Rewritten = append(res.Rewritten, base)
		if !dryRun {
			if err := writeAtomic(f, kept); err != nil {
				return res, err
			}
		}
	}
	return res, nil
}

// hasEventsBefore stops at the first stale line instead of reading the file,
// because the answer only decides whether to rotate.
func hasEventsBefore(path string, cutoff time.Time) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64<<10), 8<<20)
	for sc.Scan() {
		if before, ok := lineIsBefore(sc.Bytes(), cutoff); ok && before {
			return true, nil
		}
	}
	return false, sc.Err()
}

// partitionByTime splits one log file's lines into those at/after the cutoff
// and those before it. Undated and unparseable lines count as malformed and
// are dropped: a line with no timestamp cannot be shown to be recent, and a
// prune that keeps what it cannot date is not a prune.
func partitionByTime(path string, cutoff time.Time) (kept [][]byte, dropped, malformed int, err error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, 0, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64<<10), 8<<20)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		before, ok := lineIsBefore(line, cutoff)
		switch {
		case !ok:
			malformed++
		case before:
			dropped++
		default:
			kept = append(kept, append([]byte(nil), line...))
		}
	}
	return kept, dropped, malformed, sc.Err()
}

// lineIsBefore reports whether one JSONL event predates the cutoff. ok is
// false for anything it cannot date.
func lineIsBefore(line []byte, cutoff time.Time) (before, ok bool) {
	var probe struct {
		TS string `json:"ts"`
	}
	if err := json.Unmarshal(line, &probe); err != nil || probe.TS == "" {
		return false, false
	}
	t, err := time.Parse(time.RFC3339Nano, probe.TS)
	if err != nil {
		return false, false
	}
	return t.Before(cutoff), true
}

// writeAtomic replaces path with lines via a temp file in the same directory,
// so an interrupted prune leaves the original log intact rather than a
// truncated one.
func writeAtomic(path string, lines [][]byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".prune-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name) // no-op once the rename succeeded

	w := bufio.NewWriter(tmp)
	for _, l := range lines {
		if _, err := w.Write(append(l, '\n')); err != nil {
			tmp.Close()
			return err
		}
	}
	if err := w.Flush(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}
