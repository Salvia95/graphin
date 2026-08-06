package usage

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// Run executes the `graphin usage` subcommand. Returns a process exit code.
// Like dbimport/eval, this path never touches the MCP transport, so stdout is
// safe to use.
func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: graphin usage ingest | report [--log <dir|file>] [--since <YYYY-MM-DD|72h>] [--json] [--top N]")
		return 2
	}
	switch args[0] {
	case "ingest":
		return Ingest(stdin, stderr, os.Getenv)
	case "report":
		return runReport(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "graphin usage: unknown verb %q (ingest | report)\n", args[0])
		return 2
	}
}

func runReport(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("usage report", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var (
		log    = fs.String("log", "", "events dir or file (default: walk up from cwd to <ws>/.graphin/usage)")
		since  = fs.String("since", "", "only events at/after this point: YYYY-MM-DD or a Go duration (72h)")
		asJSON = fs.Bool("json", false, "emit the raw Report as JSON instead of markdown")
		top    = fs.Int("top", 20, "fallback pair sample size (most recent first)")
	)
	if err := fs.Parse(args); err != nil {
		return 2
	}

	path := *log
	if path == "" {
		cwd, err := os.Getwd()
		if err != nil {
			fmt.Fprintf(stderr, "graphin usage: %v\n", err)
			return 1
		}
		root := findRoot(cwd)
		if root == "" {
			fmt.Fprintln(stderr, "graphin usage: no indexed workspace found from cwd (.graphin/merkle.json); pass --log")
			return 1
		}
		path = filepath.Join(root, ".graphin", "usage")
		if _, err := os.Stat(path); err != nil {
			fmt.Fprintf(stderr, "graphin usage: index present but no usage events at %s —\n"+
				"  graphin 플러그인의 PostToolUse 훅이 발화 중인지 확인하라 (/graphin:doctor)\n", path)
			return 1
		}
	}

	events, problems, err := Load(path)
	if err != nil {
		fmt.Fprintf(stderr, "graphin usage: %v\n", err)
		return 1
	}
	if *since != "" {
		cutoff, err := parseSince(*since, time.Now().UTC())
		if err != nil {
			fmt.Fprintf(stderr, "graphin usage: bad --since: %v\n", err)
			return 2
		}
		kept := events[:0]
		for _, ev := range events {
			// Unparseable timestamps are kept: better over-report than
			// silently drop data.
			if t, err := time.Parse(time.RFC3339Nano, ev.TS); err == nil && t.Before(cutoff) {
				continue
			}
			kept = append(kept, ev)
		}
		events = kept
	}

	rep := Compute(events, problems, Options{TopPairs: *top})
	if *asJSON {
		b, err := json.MarshalIndent(rep, "", "  ")
		if err != nil {
			fmt.Fprintf(stderr, "graphin usage: %v\n", err)
			return 1
		}
		fmt.Fprintln(stdout, string(b))
		return 0
	}
	fmt.Fprint(stdout, Markdown(rep))
	return 0
}

// parseSince turns --since into a cutoff instant.
func parseSince(s string, now time.Time) (time.Time, error) {
	if d, err := time.ParseDuration(s); err == nil {
		return now.Add(-d), nil
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t.UTC(), nil
	}
	return time.Time{}, fmt.Errorf("%q is neither YYYY-MM-DD nor a duration", s)
}
