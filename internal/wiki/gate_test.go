package wiki

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// gateWorkspace builds an indexed workspace, optionally holding a wiki.
func gateWorkspace(t *testing.T, withWiki bool) string {
	t.Helper()
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, markerRel), "{}")
	if withWiki {
		mustWrite(t, filepath.Join(root, "docs", "target.md"), targetDoc)
		writeSet(t, root, "s", "roles: [backend]\n")
	}
	return root
}

func hookJSON(t *testing.T, fields map[string]any) io.Reader {
	t.Helper()
	raw, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	return strings.NewReader(string(raw))
}

func runVerb(t *testing.T, verb string, in io.Reader) (int, string) {
	t.Helper()
	var errb strings.Builder
	code := Run([]string{verb}, in, io.Discard, &errb)
	return code, errb.String()
}

func validToken(t *testing.T, root string) string {
	t.Helper()
	store, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	secret, err := LoadOrCreateSecret(root)
	if err != nil {
		t.Fatal(err)
	}
	return store.MintToken(secret)
}

func TestGateAllowsWhenNoWorkspace(t *testing.T) {
	// The hooks are installed once and fire in every project on the machine.
	code, _ := runVerb(t, "gate", hookJSON(t, map[string]any{
		"tool_name": "Edit", "cwd": t.TempDir(), "session_id": "s1",
	}))
	if code != exitAllow {
		t.Fatalf("exit = %d, want allow outside an indexed workspace", code)
	}
}

func TestGateAllowsWhenNoWiki(t *testing.T) {
	// A project that never adopted the wiki must be untouched by it, or the
	// gate charges every edit everywhere for nothing.
	root := gateWorkspace(t, false)
	code, _ := runVerb(t, "gate", hookJSON(t, map[string]any{
		"tool_name": "Edit", "cwd": root, "session_id": "s1",
	}))
	if code != exitAllow {
		t.Fatalf("exit = %d, want allow with no wiki present", code)
	}
}

func TestGateBlocksDelegationWithoutManifest(t *testing.T) {
	root := gateWorkspace(t, true)
	code, msg := runVerb(t, "gate", hookJSON(t, map[string]any{
		"tool_name": "Task", "cwd": root, "session_id": "s1",
		"tool_input": map[string]any{"subagent_type": "backend-dev", "prompt": "go do the thing"},
	}))
	if code != exitBlock {
		t.Fatalf("exit = %d, want block", code)
	}
	// A block that only says "no" becomes a retry of the same call.
	if !strings.Contains(msg, "wiki_preflight") {
		t.Fatalf("block message names no next action:\n%s", msg)
	}
}

func TestGateAllowsDelegationWithToken(t *testing.T) {
	root := gateWorkspace(t, true)
	tok := validToken(t, root)
	code, msg := runVerb(t, "gate", hookJSON(t, map[string]any{
		"tool_name": "Task", "cwd": root, "session_id": "s1",
		"tool_input": map[string]any{
			"subagent_type": "backend-dev",
			"prompt":        "Do the thing.\n\nKnowledge manifest token: " + tok,
		},
	}))
	if code != exitAllow {
		t.Fatalf("exit = %d (%s), want allow", code, msg)
	}
}

func TestGateAllowsExemptAgentWithoutToken(t *testing.T) {
	root := gateWorkspace(t, true)
	// graphin ships read-only investigators that hold Bash. Gating them would
	// stop them for knowledge they will never use.
	//
	// The namespaced form is what a plugin agent actually arrives as, and it
	// is the one that matters: the bare name is what the table is written
	// with, but nothing outside our own tests ever sends it.
	for _, name := range []string{"graphin-explorer", "graphin-guide:graphin-explorer"} {
		code, _ := runVerb(t, "gate", hookJSON(t, map[string]any{
			"tool_name": "Task", "cwd": root, "session_id": "s1",
			"tool_input": map[string]any{"subagent_type": name, "prompt": "where is X"},
		}))
		if code != exitAllow {
			t.Fatalf("exit = %d, want allow for exempt agent %q", code, name)
		}
	}
}

func TestGateInheritingAgentFollowsItsCaller(t *testing.T) {
	root := gateWorkspace(t, true)

	// The caller has loaded nothing. A fork of it inherits nothing, so it is
	// gated like any other delegation — the hole this closes is an
	// orchestrator that reads its way through a repository (Read, Grep and
	// Glob are not gated), forks, and lets the fork do every edit.
	code, msg := runVerb(t, "gate", hookJSON(t, map[string]any{
		"tool_name": "Task", "cwd": root, "session_id": "s1",
		"tool_input": map[string]any{"subagent_type": "fork", "prompt": "carry on"},
	}))
	if code != exitBlock {
		t.Fatalf("exit = %d (%s), want block: an uncleared caller has no clearance to hand down", code, msg)
	}

	// Now the caller is cleared. The fork begins holding what the caller
	// held, so it needs nothing of its own.
	mustWrite(t, FlagPath(root, "s1", ""), `{"status":"cleared","producer":"resolve"}`)
	if code, msg := runVerb(t, "gate", hookJSON(t, map[string]any{
		"tool_name": "Task", "cwd": root, "session_id": "s1",
		"tool_input": map[string]any{"subagent_type": "fork", "prompt": "carry on"},
	})); code != exitAllow {
		t.Fatalf("exit = %d (%s), want allow for a fork of a cleared caller", code, msg)
	}

	// And the clearance is recorded as inherited, not as a manifest that was
	// never verified.
	spawn(t, root, "s1", "f1", "fork")
	if f, _ := ReadFlag(root, "s1", "f1"); f.Status != StatusCleared || f.Producer != "inherited" {
		t.Fatalf("flag = %+v, want cleared by inheritance", f)
	}
}

func TestGateBlockNamesTheRoleFromTheAgentsPage(t *testing.T) {
	root := gateWorkspace(t, true)
	mustWrite(t, filepath.Join(root, DirName, "agents.md"),
		"---\nagents:\n  - backend-dev — backend\n---\n")

	// The table was just asked which role this delegate needs. Saying it is
	// the one thing the caller would otherwise have to guess.
	_, msg := runVerb(t, "gate", hookJSON(t, map[string]any{
		"tool_name": "Task", "cwd": root, "session_id": "s1",
		"tool_input": map[string]any{"subagent_type": "backend-dev", "prompt": "fix it"},
	}))
	if !strings.Contains(msg, `"backend"`) {
		t.Errorf("block message does not name the role:\n%s", msg)
	}

	// An agent the table has never heard of has no role to name, and
	// inventing one would be worse than staying quiet.
	_, msg = runVerb(t, "gate", hookJSON(t, map[string]any{
		"tool_name": "Task", "cwd": root, "session_id": "s1",
		"tool_input": map[string]any{"subagent_type": "who-dis", "prompt": "fix it"},
	}))
	if strings.Contains(msg, "agents page puts this delegate") {
		t.Errorf("block message invented a role for an unlisted agent:\n%s", msg)
	}
}

func TestGateRejectsTokenMintedBeforeAWikiEdit(t *testing.T) {
	root := gateWorkspace(t, true)
	tok := validToken(t, root)

	mustWrite(t, filepath.Join(root, DirName, setsSubdir, "s.md"),
		"# s\n\n## G\n\n- [x](../../target.md#section-one) — a summary\n"+
			"- [y](../../target.md#section-one) — a second entry\n")

	// The manifest the delegate is carrying no longer describes the wiki.
	// Failing here is safe: the caller preflights again.
	code, msg := runVerb(t, "gate", hookJSON(t, map[string]any{
		"tool_name": "Task", "cwd": root, "session_id": "s1",
		"tool_input": map[string]any{"subagent_type": "backend-dev", "prompt": "token: " + tok},
	}))
	if code != exitBlock {
		t.Fatalf("exit = %d, want block for a stale token", code)
	}
	if !strings.Contains(msg, "no longer verifies") {
		t.Errorf("message does not explain staleness:\n%s", msg)
	}
}

func TestGateAllowsChangeWhenCleared(t *testing.T) {
	root := gateWorkspace(t, true)
	if err := WriteFlag(root, "s1", "a1", Flag{Status: StatusCleared}); err != nil {
		t.Fatal(err)
	}
	code, msg := runVerb(t, "gate", hookJSON(t, map[string]any{
		"tool_name": "Edit", "cwd": root, "session_id": "s1", "agent_id": "a1",
	}))
	if code != exitAllow {
		t.Fatalf("exit = %d (%s), want allow", code, msg)
	}
}

func TestGateBlocksChangeWhenOnlySeen(t *testing.T) {
	root := gateWorkspace(t, true)
	if err := WriteFlag(root, "s1", "a1", Flag{Status: StatusSeen}); err != nil {
		t.Fatal(err)
	}
	_, msg := runVerb(t, "gate", hookJSON(t, map[string]any{
		"tool_name": "Edit", "cwd": root, "session_id": "s1", "agent_id": "a1",
	}))
	if !strings.Contains(msg, "started without a knowledge manifest") {
		t.Fatalf("wrong diagnosis for a seen-but-uncleared agent:\n%s", msg)
	}
}

func TestGateNamesTheHookWhenNoRecordExists(t *testing.T) {
	root := gateWorkspace(t, true)
	// No breadcrumb at all for a subagent means SubagentStart never ran.
	// Without this distinction a broken install is indistinguishable from a
	// careless caller, and the recovery loop hides it session after session.
	_, msg := runVerb(t, "gate", hookJSON(t, map[string]any{
		"tool_name": "Write", "cwd": root, "session_id": "s1", "agent_id": "a1",
	}))
	if !strings.Contains(msg, "SubagentStart") {
		t.Fatalf("a missing breadcrumb must be reported as an install fault:\n%s", msg)
	}
}

func TestGateBlocksMainContextOncePerSession(t *testing.T) {
	root := gateWorkspace(t, true)
	// The orchestrator has no spawn event, so it clears itself the first time
	// it loads knowledge.
	code, msg := runVerb(t, "gate", hookJSON(t, map[string]any{
		"tool_name": "Bash", "cwd": root, "session_id": "s1",
	}))
	if code != exitBlock {
		t.Fatalf("exit = %d, want block", code)
	}
	if strings.Contains(msg, "SubagentStart") {
		t.Fatalf("the main context has no spawn hook; that cannot be the diagnosis:\n%s", msg)
	}
	if !strings.Contains(msg, "wiki_preflight") {
		t.Fatalf("no next action named:\n%s", msg)
	}

	if err := WriteFlag(root, "s1", "", Flag{Status: StatusCleared, Producer: "resolve"}); err != nil {
		t.Fatal(err)
	}
	if code, _ := runVerb(t, "gate", hookJSON(t, map[string]any{
		"tool_name": "Bash", "cwd": root, "session_id": "s1",
	})); code != exitAllow {
		t.Fatal("the main context must stay cleared for the rest of the session")
	}
}

// delegate runs the real two-step: the delegation gate verifies the token and
// leaves a note, then the spawn hook consumes it.
//
// SubagentStart carries agent_id and agent_type and NO prompt, so the spawn
// hook can never see a manifest itself. Any test that hands it one is testing
// a payload the product does not send.
func delegate(t *testing.T, root, session, agentType, prompt string) int {
	t.Helper()
	code, _ := runVerb(t, "gate", hookJSON(t, map[string]any{
		"tool_name": "Task", "cwd": root, "session_id": session,
		"tool_input": map[string]any{"subagent_type": agentType, "prompt": prompt},
	}))
	return code
}

func spawn(t *testing.T, root, session, agentID, agentType string) {
	t.Helper()
	runVerb(t, "mark", hookJSON(t, map[string]any{
		"hook_event_name": "SubagentStart", "cwd": root,
		"session_id": session, "agent_id": agentID, "agent_type": agentType,
	}))
}

func TestVerifiedDelegationClearsTheSpawn(t *testing.T) {
	root := gateWorkspace(t, true)
	tok := validToken(t, root)

	if code := delegate(t, root, "s1", "backend-dev", "work. token: "+tok); code != exitAllow {
		t.Fatalf("delegation blocked (%d)", code)
	}
	spawn(t, root, "s1", "a1", "backend-dev")

	f, ok := ReadFlag(root, "s1", "a1")
	if !ok || f.Status != StatusCleared {
		t.Fatalf("flag = %+v ok=%v, want cleared", f, ok)
	}
	// Clearing at spawn is the whole point: the normal path costs no blocked
	// call at all.
	if code, msg := runVerb(t, "gate", hookJSON(t, map[string]any{
		"tool_name": "Edit", "cwd": root, "session_id": "s1", "agent_id": "a1",
	})); code != exitAllow {
		t.Fatalf("first edit was blocked (%s)", msg)
	}
}

func TestClearanceIsConsumedOnce(t *testing.T) {
	root := gateWorkspace(t, true)
	tok := validToken(t, root)
	delegate(t, root, "s1", "backend-dev", "work. token: "+tok)

	spawn(t, root, "s1", "a1", "backend-dev")
	// A second agent of the same type that arrived with no manifest must not
	// walk through on the first one's credential.
	spawn(t, root, "s1", "a2", "backend-dev")

	if f, _ := ReadFlag(root, "s1", "a1"); f.Status != StatusCleared {
		t.Fatalf("first spawn = %+v, want cleared", f)
	}
	if f, _ := ReadFlag(root, "s1", "a2"); f.Status != StatusSeen {
		t.Fatalf("second spawn = %+v, want seen", f)
	}
}

func TestSpawnWithoutADelegationLeavesBreadcrumb(t *testing.T) {
	root := gateWorkspace(t, true)
	spawn(t, root, "s1", "a1", "backend-dev")

	f, ok := ReadFlag(root, "s1", "a1")
	if !ok {
		t.Fatal("the breadcrumb must be written before any decision")
	}
	// This hook cannot block, so declining to clear is the only signal it has.
	if f.Status != StatusSeen {
		t.Fatalf("status = %q, want seen", f.Status)
	}
}

func TestMarkSpawnClearsExemptAgent(t *testing.T) {
	root := gateWorkspace(t, true)
	// Both gates read the same table, so both have to survive the namespace
	// a plugin agent arrives with. Missing it here costs a blocked edit
	// rather than a blocked spawn, which is the harder one to trace back.
	for i, name := range []string{"graphin-explorer", "graphin-guide:graphin-explorer"} {
		agent := fmt.Sprintf("a%d", i)
		spawn(t, root, "s1", agent, name)
		f, _ := ReadFlag(root, "s1", agent)
		if f.Status != StatusCleared || f.Producer != "exempt" {
			t.Fatalf("flag for %q = %+v, want cleared by exemption", name, f)
		}
	}
}

func TestMarkResolveClearsTheCaller(t *testing.T) {
	root := gateWorkspace(t, true)
	// MCP tool names arrive namespaced, and the prefix depends on how the
	// server was registered.
	runVerb(t, "mark", hookJSON(t, map[string]any{
		"hook_event_name": "PostToolUse", "cwd": root, "session_id": "s1",
		"tool_name": "mcp__graphin__wiki_resolve",
	}))
	f, ok := ReadFlag(root, "s1", "")
	if !ok || f.Status != StatusCleared || f.Producer != "resolve" {
		t.Fatalf("flag = %+v ok=%v", f, ok)
	}
}

func TestMarkIgnoresUnrelatedTools(t *testing.T) {
	root := gateWorkspace(t, true)
	runVerb(t, "mark", hookJSON(t, map[string]any{
		"hook_event_name": "PostToolUse", "cwd": root, "session_id": "s1",
		"tool_name": "Bash",
	}))
	if _, ok := ReadFlag(root, "s1", ""); ok {
		t.Fatal("an unrelated tool must not clear the gate")
	}
}

func TestFlagPathCannotEscape(t *testing.T) {
	root := t.TempDir()
	// Session and agent ids are opaque strings from outside this process.
	got := FlagPath(root, "../../etc", "../passwd")
	if !strings.HasPrefix(got, filepath.Join(root, filepath.FromSlash(RuntimeSubdir))) {
		t.Fatalf("path escaped the runtime directory: %s", got)
	}
	if strings.Contains(got, "..") {
		t.Fatalf("path retains traversal: %s", got)
	}
}

func TestGCFlagsDropsStaleSessions(t *testing.T) {
	root := t.TempDir()
	if err := WriteFlag(root, "old", "a1", Flag{Status: StatusCleared}); err != nil {
		t.Fatal(err)
	}
	if err := WriteFlag(root, "new", "a1", Flag{Status: StatusCleared}); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(root, filepath.FromSlash(RuntimeSubdir), "flags", "old")
	old := time.Now().Add(-2 * flagTTL)
	if err := os.Chtimes(stale, old, old); err != nil {
		t.Fatal(err)
	}

	GCFlags(root)

	// SubagentStop is not guaranteed to fire, and a flag left behind would
	// clear a later agent that never preflighted.
	if _, ok := ReadFlag(root, "old", "a1"); ok {
		t.Error("a stale session's flags must not survive")
	}
	if _, ok := ReadFlag(root, "new", "a1"); !ok {
		t.Error("a live session's flags must survive")
	}
}
