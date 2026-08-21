package wiki

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
)

const usageLine = `usage: graphin wiki check [--root <dir>]
       graphin wiki repin [--root <dir>] [--dry-run]
       graphin wiki queue [--root <dir>] [--misses N] [--json]
       graphin wiki skills [--root <dir>] [--out <dir>] [--check]
       graphin wiki export --okf --out <dir> [--root <dir>]
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
	case "queue":
		return runQueue(args[1:], stdout, stderr)
	case "skills":
		return runSkills(args[1:], stdout, stderr)
	case "export":
		return runExport(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "graphin wiki: unknown verb %q (check | repin | queue | skills | export | gate | mark)\n", args[0])
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

	res, err := RepinAll(*root, *dry)
	if err != nil {
		fmt.Fprintf(stderr, "graphin wiki repin: %v\n", err)
		return 1
	}
	for _, p := range res.Problems {
		fmt.Fprintln(stdout, p)
	}
	fmt.Fprintf(stdout, "%d added, %d updated, %d dropped\n", res.Added, res.Updated, res.Dropped)
	if !res.Wrote {
		fmt.Fprintln(stdout, "(dry run: nothing written)")
		return 0
	}
	fmt.Fprintf(stdout, "wrote %s\n", res.Path)
	if len(res.Problems) > 0 {
		return 1
	}
	return 0
}

// runQueue shows what is waiting for a person: candidates to approve, work the
// wiki could not answer, sets nobody opens, and entries to re-verify.
//
// One command for all four because they are one decision. Reading the queue
// without the misses invites approving whatever happened to be submitted,
// rather than writing what the work actually wanted.
func runQueue(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("wiki queue", flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", ".", "workspace root containing "+DirName)
	limit := fs.Int("misses", 10, "how many recent coverage misses to list")
	asJSON := fs.Bool("json", false, "emit the raw QueueReport as JSON instead of text")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	q, err := BuildQueueReport(*root)
	if err != nil {
		fmt.Fprintf(stderr, "graphin wiki queue: %v\n", err)
		return 1
	}
	if *asJSON {
		b, err := json.MarshalIndent(q, "", "  ")
		if err != nil {
			fmt.Fprintf(stderr, "graphin wiki queue: %v\n", err)
			return 1
		}
		fmt.Fprintln(stdout, string(b))
		return 0
	}
	RenderQueue(stdout, q, *limit)
	return 0
}

// DefaultSkillDir is where a project's own skills live, relative to the root.
const DefaultSkillDir = ".claude/skills"

// runSkills regenerates the per-role convention blocks.
//
// These are push knowledge: injected whole at the start of a session because
// the reader cannot detect that they are missing. Everything an agent CAN
// notice it needs stays in the catalogue instead, where it costs nothing until
// it is asked for.
func runSkills(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("wiki skills", flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", ".", "workspace root containing "+DirName)
	out := fs.String("out", DefaultSkillDir, "directory to write the generated skills into")
	check := fs.Bool("check", false, "report staleness without writing (exit 1 if stale)")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	store, err := Load(*root)
	if err != nil {
		fmt.Fprintf(stderr, "graphin wiki skills: %v\n", err)
		return 1
	}
	dir := filepath.Join(*root, filepath.FromSlash(*out))

	if len(store.Roles()) == 0 {
		fmt.Fprintf(stdout, "no role tags in %s — nothing to generate\n", DirName)
		return 0
	}

	if *check {
		stale := store.StaleSkills(dir)
		if len(stale) == 0 {
			fmt.Fprintln(stdout, "generated skills are up to date")
			return 0
		}
		for _, s := range stale {
			fmt.Fprintf(stdout, "stale: %s\n", s)
		}
		// A generated artifact that drifts from its source is worse than an
		// absent one: it still reads as authoritative.
		fmt.Fprintln(stdout, "run `graphin wiki skills` and commit the result")
		return 1
	}

	written, err := store.WriteSkills(dir)
	if err != nil {
		fmt.Fprintf(stderr, "graphin wiki skills: %v\n", err)
		return 1
	}
	for _, g := range written {
		fmt.Fprintf(stdout, "wrote %s\n", filepath.Join(dir, g.Name, "SKILL.md"))
		for _, d := range g.Dropped {
			fmt.Fprintf(stdout, "  capped out: %s\n", d)
		}
	}

	// Reported, never applied. Agent definitions are the user's files, and a
	// generator that rewrites them makes "regenerate" unsafe to run blind.
	needing := store.AgentsNeeding()
	if len(needing) > 0 {
		fmt.Fprintln(stdout, "\ndeclare these in the matching agent definitions:")
		skills := make([]string, 0, len(needing))
		for s := range needing {
			skills = append(skills, s)
		}
		sort.Strings(skills)
		for _, s := range skills {
			fmt.Fprintf(stdout, "  skills: [%s]  →  %s\n", s, strings.Join(needing[s], ", "))
		}
	}
	return 0
}

// runExport writes the wiki out in an interchange format.
//
// Export, not adopt. The authored form addresses a heading inside a document;
// the Open Knowledge Format addresses a file. Producing a bundle beside the
// wiki keeps that granularity and costs nothing until someone wants to read
// one, which is the same trade the portability decision made originally.
func runExport(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("wiki export", flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", ".", "workspace root containing "+DirName)
	out := fs.String("out", "", "directory to write the bundle into (required)")
	okf := fs.Bool("okf", false, "write an Open Knowledge Format bundle")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if !*okf {
		fmt.Fprintln(stderr, "graphin wiki export: --okf is the only format; pass it explicitly")
		return 2
	}
	if *out == "" {
		fmt.Fprintln(stderr, "graphin wiki export: --out is required")
		return 2
	}

	store, err := Load(*root)
	if err != nil {
		fmt.Fprintf(stderr, "graphin wiki export: %v\n", err)
		return 1
	}
	if !store.Present {
		fmt.Fprintf(stderr, "graphin wiki export: no wiki under %s\n", DirName)
		return 1
	}
	written, err := store.ExportOKF(*out)
	if err != nil {
		fmt.Fprintf(stderr, "graphin wiki export: %v\n", err)
		return 1
	}
	for _, w := range written {
		fmt.Fprintf(stdout, "  %s\n", w)
	}
	fmt.Fprintf(stdout, "wrote %d files to %s (OKF v%s)\n", len(written), *out, OKFVersion)
	return 0
}
