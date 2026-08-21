package wiki

import (
	"fmt"
	"io"
	"strings"
)

// Exit codes this verb speaks. The handler script translates them for the hook
// runner, which blocks on 2 and nothing else.
//
// exitBlock is deliberately not 2. This binary returns 2 for a usage error
// like every other subcommand, so a script that trusted 2 could not tell "the
// gate decided no" from "this graphin has never heard of the gate" — and the
// second one happens for real, when an upgrade leaves a handler pointed at an
// older binary for a moment. The consequence was total: every tool call on the
// machine blocked, with an instruction the caller could not follow, because
// the recovery it names is the very thing that was not reachable.
//
// A dedicated code cannot be produced by accident. Nothing that does not know
// about the gate can answer 20, so an unrecognised failure now means "we could
// not ask" and is allowed through. Both halves of a version mismatch fail open
// for the same reason, which is the correct direction for a gate: refusing to
// work is a worse failure than letting one edit past.
const (
	exitAllow = 0
	exitBlock = 20
)

// runGate implements `graphin wiki gate`, the PreToolUse handler for both
// gates. Which one runs is decided by the tool being called, so a single
// binary verb backs both matchers and they cannot drift apart.
//
// Everything that is not a deliberate block exits 0. This hook is installed
// once and fires on every tool call in every project on the machine: a fault
// in our own machinery must never be able to stop someone's work. The cost of
// that choice is that a broken gate is a silent gate, which is why the
// installation smoke test exists rather than a runtime alarm.
func runGate(stdin io.Reader, stderr io.Writer) int {
	in, err := readHook(stdin)
	if err != nil {
		fmt.Fprintf(stderr, "graphin wiki gate: %v\n", err)
		return exitAllow
	}
	root := findRoot(in.CWD)
	if root == "" {
		return exitAllow
	}
	store, err := Load(root)
	if err != nil {
		fmt.Fprintf(stderr, "graphin wiki gate: %v\n", err)
		return exitAllow
	}
	if !store.Present {
		return exitAllow
	}

	switch in.ToolName {
	case "Task", "Agent":
		return gateDelegation(store, root, in, stderr)
	default:
		return gateChange(root, in, stderr)
	}
}

// gateDelegation is gate ①: no subagent starts without a manifest.
func gateDelegation(store *Store, root string, in hookInput, stderr io.Writer) int {
	if _, gated := store.Agents.Role(in.str("subagent_type")); !gated {
		return exitAllow
	}
	secret, err := LoadOrCreateSecret(root)
	if err != nil {
		fmt.Fprintf(stderr, "graphin wiki gate: %v\n", err)
		return exitAllow
	}
	if tok := FindToken(in.str("prompt")); store.VerifyToken(secret, tok) {
		// Leave the note the spawn hook will consume. This is the only
		// moment anyone sees both the token and the agent it is for.
		if err := WritePending(root, in.SessionID, in.str("subagent_type"), Flag{
			Status: StatusCleared, Producer: "manifest", Token: tok,
		}); err != nil {
			fmt.Fprintf(stderr, "graphin wiki gate: %v\n", err)
		}
		return exitAllow
	}
	// Name the next action, not the fault. A block that only says "no" turns
	// into a retry of the same call; a block that says what to run is a loop
	// the caller can close on its own.
	fmt.Fprint(stderr, "This delegation carries no valid knowledge manifest.\n\n"+
		"Call wiki_preflight with the task and the delegate's role, then include the\n"+
		"token it returns in the delegation prompt. An empty catalogue is a normal\n"+
		"answer and still returns a token — include it and proceed.\n\n"+
		"A token minted before the wiki was last edited no longer verifies; run\n"+
		"wiki_preflight again rather than reusing one from earlier in the session.\n")
	return exitBlock
}

// gateChange is gate ②: no edits by an agent that was never cleared.
//
// This is the hottest path in the system — it fires on every Edit, Write and
// Bash of every agent — so it stats one file and reads no configuration. Every
// decision that needs the wiki was made once already, at spawn.
func gateChange(root string, in hookInput, stderr io.Writer) int {
	flag, found := ReadFlag(root, in.SessionID, in.AgentID)
	if found && flag.Status == StatusCleared {
		return exitAllow
	}

	switch {
	case found:
		// The spawn hook ran and declined to clear: the delegation arrived
		// without a manifest.
		fmt.Fprint(stderr, "This agent started without a knowledge manifest.\n\n"+
			"Call wiki_resolve for the sets your task needs, then retry. If the\n"+
			"delegation prompt names no sets, call wiki_preflight first.\n")
	case in.AgentID != "":
		// A subagent with no breadcrumb at all: SubagentStart never ran.
		// Saying so is the whole reason the breadcrumb exists — otherwise a
		// broken installation is indistinguishable from a careless caller,
		// and the recovery loop below papers over it session after session.
		fmt.Fprint(stderr, "No knowledge gate record exists for this agent.\n\n"+
			"That usually means graphin's SubagentStart hook did not run — check\n"+
			"`/hooks` and the graphin plugin install, and report it if it persists.\n\n"+
			"To continue now: call wiki_resolve for the sets your task needs, then retry.\n")
	default:
		// The orchestrator has no spawn event, so it clears itself the first
		// time it loads knowledge. One block per session, in the place where
		// a person is watching.
		fmt.Fprint(stderr, "Load the project's knowledge before editing.\n\n"+
			"Call wiki_preflight to see what applies to this work, then wiki_resolve\n"+
			"for the sets you need. If nothing applies, wiki_resolve with the sets\n"+
			"named in the manifest — or an empty catalogue — still clears this.\n")
	}
	return exitBlock
}

// runMark implements `graphin wiki mark`, the recorder for both non-blocking
// events. SubagentStart decides whether a spawn is cleared; PostToolUse
// notices a resolve and clears whoever ran it.
func runMark(stdin io.Reader, stderr io.Writer) int {
	in, err := readHook(stdin)
	if err != nil {
		fmt.Fprintf(stderr, "graphin wiki mark: %v\n", err)
		return exitAllow
	}
	root := findRoot(in.CWD)
	if root == "" {
		return exitAllow
	}
	store, err := Load(root)
	if err != nil || !store.Present {
		return exitAllow
	}
	GCFlags(root)

	switch in.HookEventName {
	case "SubagentStart":
		return markSpawn(store, root, in, stderr)
	case "PostToolUse":
		return markResolve(root, in, stderr)
	}
	return exitAllow
}

// markSpawn writes the breadcrumb, then decides.
func markSpawn(store *Store, root string, in hookInput, stderr io.Writer) int {
	// The breadcrumb goes down before any logic that could fail. Its whole
	// job is to prove this hook ran, so anything that happens first can cost
	// the proof.
	if err := WriteFlag(root, in.SessionID, in.AgentID, Flag{
		Status: StatusSeen, Producer: "subagent_start",
	}); err != nil {
		fmt.Fprintf(stderr, "graphin wiki mark: %v\n", err)
		return exitAllow
	}

	if _, gated := store.Agents.Role(in.AgentType); !gated {
		_ = WriteFlag(root, in.SessionID, in.AgentID, Flag{
			Status: StatusCleared, Producer: "exempt",
		})
		return exitAllow
	}

	// The delegation gate left a note if it verified a token for this agent
	// type. Consuming it clears the spawn, so the normal path costs no
	// blocked call at all.
	//
	// The token cannot be checked here: SubagentStart carries agent_id and
	// agent_type and no prompt, so this hook never sees a manifest.
	if pending, ok := ConsumePending(root, in.SessionID, in.AgentType); ok {
		_ = WriteFlag(root, in.SessionID, in.AgentID, Flag{
			Status: StatusCleared, Producer: "manifest", Token: pending.Token,
		})
		return exitAllow
	}
	// Leave it at "seen". Gate ② will block the first change and say what to
	// run; this hook cannot block, so declining to clear is the only signal
	// it has.
	//
	// The bet the clearing path makes is that a delegate handed a catalogue
	// will read what it needs. A delegate that never resolves shows up as a
	// set with no reads, which is the same statistic that demotes an unused
	// set — so the leak feeds a metric instead of hiding.
	return exitAllow
}

// markResolve clears whoever just loaded knowledge.
func markResolve(root string, in hookInput, stderr io.Writer) int {
	// MCP tool names arrive namespaced (mcp__graphin__wiki_resolve), and the
	// prefix depends on how the server was registered, so match on the verb.
	if !strings.Contains(in.ToolName, "wiki_resolve") {
		return exitAllow
	}
	// This hook is the only place that knows the agent id: a subagent cannot
	// report its own, so the command it runs could never write this itself.
	if err := WriteFlag(root, in.SessionID, in.AgentID, Flag{
		Status: StatusCleared, Producer: "resolve",
	}); err != nil {
		fmt.Fprintf(stderr, "graphin wiki mark: %v\n", err)
	}
	return exitAllow
}
