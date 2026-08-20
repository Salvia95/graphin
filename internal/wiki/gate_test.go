package wiki

import (
	"encoding/json"
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
	code, _ := runVerb(t, "gate", hookJSON(t, map[string]any{
		"tool_name": "Task", "cwd": root, "session_id": "s1",
		"tool_input": map[string]any{"subagent_type": "graphin-explorer", "prompt": "where is X"},
	}))
	if code != exitAllow {
		t.Fatalf("exit = %d, want allow for an exempt agent", code)
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

func TestMarkSpawnClearsOnValidToken(t *testing.T) {
	root := gateWorkspace(t, true)
	tok := validToken(t, root)
	runVerb(t, "mark", hookJSON(t, map[string]any{
		"hook_event_name": "SubagentStart", "cwd": root,
		"session_id": "s1", "agent_id": "a1", "agent_type": "backend-dev",
		"agent_prompt": "work to do. token: " + tok,
	}))

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

func TestMarkSpawnLeavesBreadcrumbOnBadToken(t *testing.T) {
	root := gateWorkspace(t, true)
	runVerb(t, "mark", hookJSON(t, map[string]any{
		"hook_event_name": "SubagentStart", "cwd": root,
		"session_id": "s1", "agent_id": "a1", "agent_type": "backend-dev",
		"agent_prompt": "no manifest here",
	}))

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
	runVerb(t, "mark", hookJSON(t, map[string]any{
		"hook_event_name": "SubagentStart", "cwd": root,
		"session_id": "s1", "agent_id": "a1", "agent_type": "graphin-explorer",
		"agent_prompt": "where is X",
	}))
	f, _ := ReadFlag(root, "s1", "a1")
	if f.Status != StatusCleared || f.Producer != "exempt" {
		t.Fatalf("flag = %+v, want cleared by exemption", f)
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
