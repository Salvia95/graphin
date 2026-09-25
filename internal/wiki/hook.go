package wiki

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	maxHookStdin = 8 << 20
	// markerRel marks a workspace that has actually been indexed. The
	// directory alone appears the moment a server starts; this file only
	// after the first scan persisted.
	markerRel = ".graphin/merkle.json"
	maxWalkUp = 8
)

// hookInput is the subset of Claude Code's hook payload the gate reads.
//
// internal/usage declares its own view of the same payload. That is not
// duplication to consolidate: the payload is a product contract neither
// package owns, and each reading only the fields it acts on keeps a new field
// in one from silently becoming a dependency of the other.
//
// There is deliberately no delegation-prompt field. SubagentStart does not
// carry one, and declaring it would invite code that reads an always-empty
// string and concludes the manifest was missing. See PendingPath.
type hookInput struct {
	HookEventName string         `json:"hook_event_name"`
	SessionID     string         `json:"session_id"`
	AgentID       string         `json:"agent_id"`
	AgentType     string         `json:"agent_type"`
	CWD           string         `json:"cwd"`
	ToolName      string         `json:"tool_name"`
	ToolInput     map[string]any `json:"tool_input"`
}

// readHook parses one hook payload from stdin.
func readHook(stdin io.Reader) (hookInput, error) {
	raw, err := io.ReadAll(io.LimitReader(stdin, maxHookStdin))
	if err != nil {
		return hookInput{}, err
	}
	var in hookInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return hookInput{}, err
	}
	return in, nil
}

// str pulls a string field out of tool_input.
func (in hookInput) str(key string) string {
	if v, ok := in.ToolInput[key].(string); ok {
		return v
	}
	return ""
}

// findRoot walks up from dir for an indexed workspace.
func findRoot(dir string) string {
	if dir == "" || !filepath.IsAbs(dir) {
		return ""
	}
	d := filepath.Clean(dir)
	for i := 0; i <= maxWalkUp; i++ {
		if fi, err := os.Stat(filepath.Join(d, markerRel)); err == nil && !fi.IsDir() {
			return d
		}
		parent := filepath.Dir(d)
		if parent == d {
			return ""
		}
		d = parent
	}
	return ""
}

// Flag records what the gate knows about one agent in one session.
type Flag struct {
	// Status is "seen" or "cleared". The distinction is the point: a missing
	// file means the spawn hook never ran at all, which is an installation
	// fault rather than a misbehaving caller, and the two need different
	// answers. Without the breadcrumb a broken hook looks exactly like an
	// agent that skipped preflight, and the recovery loop hides it forever.
	Status   string   `json:"status"`
	Producer string   `json:"producer"`
	Sets     []string `json:"sets,omitempty"`
	Token    string   `json:"token,omitempty"`
	TS       string   `json:"ts"`
}

const (
	StatusSeen    = "seen"
	StatusCleared = "cleared"
)

// mainAgent is the flag name for the orchestrator, which has no agent id.
const mainAgent = "main"

// flagTTL bounds how long a session's flags survive. SubagentStop is not
// guaranteed to fire — a crash skips it — and a flag left behind would clear
// a later agent that never preflighted.
const flagTTL = 24 * time.Hour

// FlagPath is where one agent's gate state lives.
func FlagPath(root, sessionID, agentID string) string {
	if agentID == "" {
		agentID = mainAgent
	}
	return filepath.Join(root, filepath.FromSlash(RuntimeSubdir), "flags",
		safeName(sessionID), safeName(agentID)+".json")
}

// safeName keeps an id from escaping the flags directory. Session and agent
// ids are opaque strings from outside this process; treating them as path
// components without this would let one address any file on disk.
func safeName(s string) string {
	if s == "" {
		return "unknown"
	}
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	return b.String()
}

// PendingPath is where a verified delegation waits for the agent it spawned.
//
// This exists because of a product fact that is easy to assume away: the
// SubagentStart payload is {common fields, agent_id, agent_type} and carries
// NO delegation prompt (verified against Claude Code 2.1.238). The spawn hook
// therefore cannot see the manifest token, and clearing an agent at spawn —
// the thing that keeps the normal path free of a blocked call — has to be
// arranged by whoever did see it.
//
// So the delegation gate, which verifies the token and knows the session and
// the target agent type, leaves a note here. The spawn hook consumes it.
func PendingPath(root, sessionID, agentType string) string {
	return filepath.Join(root, filepath.FromSlash(RuntimeSubdir), "flags",
		safeName(sessionID), "pending", safeName(agentType)+".json")
}

// WritePending records that a delegation to agentType passed the gate.
func WritePending(root, sessionID, agentType string, f Flag) error {
	if f.TS == "" {
		f.TS = time.Now().UTC().Format(time.RFC3339)
	}
	path := PendingPath(root, sessionID, agentType)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.Marshal(f)
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}

// ConsumePending takes the note left for agentType, if there is one.
//
// Consuming rather than reading is what keeps it honest: one verified
// delegation clears one spawn. Leaving the note in place would let a later
// spawn of the same agent type — one that arrived with no manifest at all —
// walk through on someone else's credential.
func ConsumePending(root, sessionID, agentType string) (Flag, bool) {
	path := PendingPath(root, sessionID, agentType)
	raw, err := os.ReadFile(path)
	if err != nil {
		return Flag{}, false
	}
	_ = os.Remove(path)
	var f Flag
	if err := json.Unmarshal(raw, &f); err != nil {
		return Flag{}, false
	}
	return f, true
}

// ReadFlag loads one agent's gate state.
func ReadFlag(root, sessionID, agentID string) (Flag, bool) {
	raw, err := os.ReadFile(FlagPath(root, sessionID, agentID))
	if err != nil {
		return Flag{}, false
	}
	var f Flag
	if err := json.Unmarshal(raw, &f); err != nil {
		return Flag{}, false
	}
	return f, true
}

// WriteFlag records one agent's gate state, creating the session directory.
func WriteFlag(root, sessionID, agentID string, f Flag) error {
	if f.TS == "" {
		f.TS = time.Now().UTC().Format(time.RFC3339)
	}
	path := FlagPath(root, sessionID, agentID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.Marshal(f)
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}

// Session markers. They sit next to the per-agent flags, and their names
// carry no .json suffix, which FlagPath always appends — so no agent id can
// ever address one.
//
// Together they answer one question the flags cannot: can this session reach
// the recovery the gate asks for at all? Every block names an MCP call
// (wiki_preflight, wiki_resolve), and the gate arms on disk state alone —
// docs/wiki and an indexed workspace — which says nothing about whether this
// session's graphin server is connected. On 2026-09-25 it was not (Claude
// Code 2.1.282 rejected our tools/list), and every Bash, Edit and Agent call
// was blocked with an instruction no tool existed to follow.
const (
	markerBlocked = "blocked" // the gate has blocked in this session
	markerReached = "reached" // a wiki MCP tool answered in this session
)

func sessionMarkerPath(root, sessionID, name string) string {
	return filepath.Join(root, filepath.FromSlash(RuntimeSubdir), "flags",
		safeName(sessionID), name)
}

func touchSessionMarker(root, sessionID, name string) error {
	path := sessionMarkerPath(root, sessionID, name)
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(time.Now().UTC().Format(time.RFC3339)+"\n"), 0o644)
}

func hasSessionMarker(root, sessionID, name string) bool {
	_, err := os.Stat(sessionMarkerPath(root, sessionID, name))
	return err == nil
}

// recoveryUnreachable reports whether this session was already told what to
// run and has never once reached a tool that runs it.
//
// One block is always spent first. That block is the normal, working path's
// only cost (장면 10), and it is also the only way to tell the two sessions
// apart: a connected session answers it with a wiki call, a disconnected one
// cannot. What this lets through is a caller that has the tools and ignores
// the block — a real cost, and the one the gate's own rule prices below
// stopping a session that has no way to comply.
func recoveryUnreachable(root, sessionID string) bool {
	return hasSessionMarker(root, sessionID, markerBlocked) &&
		!hasSessionMarker(root, sessionID, markerReached)
}

// GCFlags removes session directories older than flagTTL.
func GCFlags(root string) {
	base := filepath.Join(root, filepath.FromSlash(RuntimeSubdir), "flags")
	ents, err := os.ReadDir(base)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-flagTTL)
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil || info.ModTime().After(cutoff) {
			continue
		}
		_ = os.RemoveAll(filepath.Join(base, e.Name()))
	}
}
