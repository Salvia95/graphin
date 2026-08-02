package usage

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// LogGlob matches the active log plus rotated siblings (spec §2.3).
const LogGlob = "events*.jsonl"

// Load reads usage events from every events*.jsonl under dir (a .graphin/usage
// directory or a single file path). Malformed or future-versioned lines are
// collected into problems, never fatal — the log is written concurrently by
// live sessions, so a torn final line is normal (spec §5).
func Load(path string) ([]Event, []string, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return nil, nil, err
	}
	files := []string{path}
	if fi.IsDir() {
		files, err = filepath.Glob(filepath.Join(path, LogGlob))
		if err != nil {
			return nil, nil, err
		}
		// Rotated files carry UTC stamps, so lexical order is chronological.
		// The active file must come last (it holds the newest events); that
		// already falls out of sorting ("events-" < "events."), but pin it
		// explicitly so append order never depends on byte-order trivia.
		sort.Strings(files)
		if len(files) > 1 {
			active := filepath.Join(path, "events.jsonl")
			for i, f := range files {
				if f == active {
					files = append(append(files[:i:i], files[i+1:]...), active)
					break
				}
			}
		}
	}

	var events []Event
	var problems []string
	for _, f := range files {
		evs, probs, err := loadFile(f)
		if err != nil {
			return nil, nil, err
		}
		events = append(events, evs...)
		problems = append(problems, probs...)
	}
	return events, problems, nil
}

func loadFile(path string) ([]Event, []string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()

	var events []Event
	var problems []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	line := 0
	base := filepath.Base(path)
	for sc.Scan() {
		line++
		raw := sc.Bytes()
		if len(raw) == 0 {
			continue
		}
		var ev Event
		if err := json.Unmarshal(raw, &ev); err != nil {
			problems = append(problems, fmt.Sprintf("%s:%d: invalid JSON: %v", base, line, err))
			continue
		}
		if ev.V > 1 {
			problems = append(problems, fmt.Sprintf("%s:%d: schema v%d > 1, skipped", base, line, ev.V))
			continue
		}
		if ev.SessionID == "" || ev.Tool == "" {
			problems = append(problems, fmt.Sprintf("%s:%d: missing session_id/tool", base, line))
			continue
		}
		events = append(events, ev)
	}
	if err := sc.Err(); err != nil {
		return nil, nil, err
	}
	return events, problems, nil
}
