package wiki

import (
	"flag"
	"fmt"
	"io"
)

const usageLine = `usage: graphin wiki check [--root <dir>]
       graphin wiki repin [--root <dir>] [--dry-run]
       graphin wiki gate            # PreToolUse hook sink, reads JSON on stdin
       graphin wiki mark            # SubagentStart/PostToolUse hook sink`

// Run executes the `graphin wiki` subcommand and returns a process exit code.
//
// Every verb here is index-free by construction. That is not an accident of
// scope: opening the graph engine truncates its delta log, so a command a CI
// step or a hook may run while a server holds the workspace must never need
// it. Section hashes come from re-parsing the documents, which is exactly
// what the indexer did to produce them.
func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, usageLine)
		return 2
	}
	switch args[0] {
	case "check":
		return runCheck(args[1:], stdout, stderr)
	case "repin":
		return runRepin(args[1:], stdout, stderr)
	case "gate":
		return runGate(stdin, stderr)
	case "mark":
		return runMark(stdin, stderr)
	default:
		fmt.Fprintf(stderr, "graphin wiki: unknown verb %q (check | repin | gate | mark)\n", args[0])
		return 2
	}
}

func runCheck(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("wiki check", flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", ".", "workspace root containing "+DirName)
	if err := fs.Parse(args); err != nil {
		return 2
	}

	store, err := Load(*root)
	if err != nil {
		fmt.Fprintf(stderr, "graphin wiki check: %v\n", err)
		return 1
	}
	sets := store.SetList()
	if len(sets) == 0 {
		fmt.Fprintf(stdout, "no knowledge sets under %s\n", DirName)
		return 0
	}

	problems := Check(*root, sets, store.Pins)
	entries := 0
	for _, s := range sets {
		entries += len(s.Entries())
	}
	if len(problems) == 0 {
		fmt.Fprintf(stdout, "ok: %d sets, %d entries, all pinned and current\n", len(sets), entries)
		return 0
	}
	for _, p := range problems {
		fmt.Fprintln(stdout, p)
	}
	fmt.Fprintf(stdout, "\n%d problem(s) across %d sets, %d entries\n", len(problems), len(sets), entries)
	fmt.Fprintln(stdout, "run `graphin wiki repin` after confirming each summary still matches its section")
	return 1
}

func runRepin(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("wiki repin", flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", ".", "workspace root containing "+DirName)
	dry := fs.Bool("dry-run", false, "report what would change without writing")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	store, err := Load(*root)
	if err != nil {
		fmt.Fprintf(stderr, "graphin wiki repin: %v\n", err)
		return 1
	}
	sets := store.SetList()
	pins, problems := Repin(*root, sets)

	added, changed := 0, 0
	for set, byNode := range pins.Pins {
		for id, h := range byNode {
			switch old, ok := store.Pins.Get(set, id); {
			case !ok:
				added++
			case old != h:
				changed++
			}
		}
	}
	// A pin that exists for an entry no longer in any set is dropped, because
	// Repin rebuilds rather than merges. Saying so matters: an author who
	// deleted an entry should see that its record went with it.
	dropped := 0
	for set, byNode := range store.Pins.Pins {
		for id := range byNode {
			if _, ok := pins.Get(set, id); !ok {
				dropped++
			}
		}
	}

	for _, p := range problems {
		fmt.Fprintln(stdout, p)
	}
	fmt.Fprintf(stdout, "%d added, %d updated, %d dropped\n", added, changed, dropped)
	if *dry {
		fmt.Fprintln(stdout, "(dry run: nothing written)")
		return 0
	}
	if err := pins.Save(store.PinsPath()); err != nil {
		fmt.Fprintf(stderr, "graphin wiki repin: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "wrote %s\n", store.PinsPath())
	if len(problems) > 0 {
		return 1
	}
	return 0
}
