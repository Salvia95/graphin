package e2e

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Salvia95/graphin/internal/wiki"
)

const wikiHandlerRel = "../plugin/graphin/hooks/wiki.sh"

// TestWikiGateSmoke runs the shipped shell handlers against the real binary.
//
// The gate is the one part of this system that can stop someone's work, and
// it is assembled from three pieces that are edited separately — a hook
// matcher, a shell script and a Go verb. Unit tests cover the verb; only this
// covers whether the three still agree.
func TestWikiGateSmoke(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not on PATH")
	}
	handler, err := filepath.Abs(wikiHandlerRel)
	if err != nil {
		t.Fatal(err)
	}
	bin := buildGraphin(t)
	root := gatedWorkspace(t)

	hook := func(t *testing.T, verb string, payload map[string]any) (int, string) {
		t.Helper()
		payload["cwd"] = root
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command("sh", handler, verb)
		cmd.Stdin = bytes.NewReader(raw)
		cmd.Env = []string{
			"CLAUDE_PROJECT_DIR=" + root,
			"GRAPHIN_BIN=" + bin,
			"PATH=" + os.Getenv("PATH"),
			"HOME=" + os.Getenv("HOME"),
		}
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		err = cmd.Run()
		code := 0
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else if err != nil {
			t.Fatalf("run handler: %v", err)
		}
		// Claude Code parses a hook's stdout as a decision document. These
		// handlers speak through the exit code and stderr, and anything on
		// stdout would be read as something else entirely.
		if stdout.Len() != 0 {
			t.Fatalf("handler wrote stdout: %q", stdout.String())
		}
		return code, stderr.String()
	}

	token := mintToken(t, root)

	t.Run("delegation without a manifest is blocked", func(t *testing.T) {
		code, msg := hook(t, "gate", map[string]any{
			"tool_name":  "Task",
			"session_id": "smoke",
			"tool_input": map[string]any{"subagent_type": "backend-dev", "prompt": "do it"},
		})
		if code != 2 {
			t.Fatalf("exit = %d, want 2", code)
		}
		if !strings.Contains(msg, "wiki_preflight") {
			t.Fatalf("no next action named:\n%s", msg)
		}
	})

	t.Run("the normal path costs no blocked call", func(t *testing.T) {
		// This is the only test that proves clearing at spawn was worth
		// building: without it every gated agent pays one blocked edit.
		if code, msg := hook(t, "gate", map[string]any{
			"tool_name":  "Task",
			"session_id": "smoke",
			"tool_input": map[string]any{"subagent_type": "backend-dev", "prompt": "do it. token: " + token},
		}); code != 0 {
			t.Fatalf("delegation blocked (%d): %s", code, msg)
		}
		// No prompt here: SubagentStart carries agent_id and agent_type only.
		// The clearance comes from the note the delegation gate just left.
		hook(t, "mark", map[string]any{
			"hook_event_name": "SubagentStart",
			"session_id":      "smoke", "agent_id": "agent-1", "agent_type": "backend-dev",
		})
		if code, msg := hook(t, "gate", map[string]any{
			"tool_name": "Edit", "session_id": "smoke", "agent_id": "agent-1",
		}); code != 0 {
			t.Fatalf("first edit blocked (%d): %s", code, msg)
		}
	})

	t.Run("removing the flag re-arms the gate", func(t *testing.T) {
		if err := os.Remove(wiki.FlagPath(root, "smoke", "agent-1")); err != nil {
			t.Fatal(err)
		}
		code, msg := hook(t, "gate", map[string]any{
			"tool_name": "Edit", "session_id": "smoke", "agent_id": "agent-1",
		})
		if code != 2 {
			t.Fatalf("exit = %d, want 2 once the record is gone", code)
		}
		// A vanished record for a subagent means the spawn hook never ran,
		// which is an install fault and must not read as a careless caller.
		if !strings.Contains(msg, "SubagentStart") {
			t.Fatalf("wrong diagnosis:\n%s", msg)
		}
	})

	t.Run("a read-only agent is never stopped", func(t *testing.T) {
		// graphin ships investigators that hold Bash and never edit. A gate
		// keyed on tools alone would stop them for knowledge they never use.
		hook(t, "mark", map[string]any{
			"hook_event_name": "SubagentStart",
			"session_id":      "smoke", "agent_id": "agent-ro", "agent_type": "graphin-explorer",
		})
		if code, msg := hook(t, "gate", map[string]any{
			"tool_name": "Bash", "session_id": "smoke", "agent_id": "agent-ro",
		}); code != 0 {
			t.Fatalf("exempt agent blocked (%d): %s", code, msg)
		}
	})

	t.Run("resolve clears the main context", func(t *testing.T) {
		if code, _ := hook(t, "gate", map[string]any{
			"tool_name": "Bash", "session_id": "main-sess",
		}); code != 2 {
			t.Fatal("the orchestrator must be gated too")
		}
		hook(t, "mark", map[string]any{
			"hook_event_name": "PostToolUse", "session_id": "main-sess",
			"tool_name": "mcp__graphin__wiki_resolve",
		})
		if code, msg := hook(t, "gate", map[string]any{
			"tool_name": "Bash", "session_id": "main-sess",
		}); code != 0 {
			t.Fatalf("still blocked after resolve (%d): %s", code, msg)
		}
	})

	t.Run("the escape hatch works", func(t *testing.T) {
		// Arming by wiki presence protects against unwanted policy, not
		// against a bug in the gate. Someone blocked wrongly needs a way out
		// that is not deleting their knowledge base.
		raw, _ := json.Marshal(map[string]any{
			"tool_name": "Edit", "session_id": "no-such", "cwd": root,
		})
		cmd := exec.Command("sh", handler, "gate")
		cmd.Stdin = bytes.NewReader(raw)
		cmd.Env = []string{
			"CLAUDE_PROJECT_DIR=" + root, "GRAPHIN_BIN=" + bin,
			"PATH=" + os.Getenv("PATH"), "HOME=" + os.Getenv("HOME"),
			"GRAPHIN_WIKI_GATE=off",
		}
		if err := cmd.Run(); err != nil {
			t.Fatalf("GRAPHIN_WIKI_GATE=off must allow: %v", err)
		}
	})
}

// TestWikiGateIgnoresProjectsWithoutAWiki keeps the blast radius honest: the
// handler is installed once and fires everywhere.
func TestWikiGateIgnoresProjectsWithoutAWiki(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not on PATH")
	}
	handler, _ := filepath.Abs(wikiHandlerRel)
	bin := buildGraphin(t)

	root := t.TempDir()
	write(t, filepath.Join(root, ".graphin", "merkle.json"), "{}")

	raw, _ := json.Marshal(map[string]any{
		"tool_name": "Edit", "session_id": "s", "cwd": root,
	})
	cmd := exec.Command("sh", handler, "gate")
	cmd.Stdin = bytes.NewReader(raw)
	cmd.Env = []string{
		"CLAUDE_PROJECT_DIR=" + root, "GRAPHIN_BIN=" + bin,
		"PATH=" + os.Getenv("PATH"), "HOME=" + os.Getenv("HOME"),
	}
	if err := cmd.Run(); err != nil {
		t.Fatalf("a project with no wiki must be untouched: %v", err)
	}
}

// gatedWorkspace is an indexed workspace that has adopted the wiki.
func gatedWorkspace(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	write(t, filepath.Join(root, ".graphin", "merkle.json"), "{}")
	write(t, filepath.Join(root, "docs", "handbook.md"),
		"# Handbook\n\n## Layering rules\n\nHandlers never touch storage directly.\n")
	write(t, filepath.Join(root, wiki.DirName, "sets", "conventions.md"),
		"---\nroles: [backend]\n---\n\n# Conventions\n\nStanding rules.\n\n"+
			"## Rules\n\n- [layering](../../handbook.md#layering-rules) — Handlers never touch storage.\n")
	return root
}

func mintToken(t *testing.T, root string) string {
	t.Helper()
	store, err := wiki.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	secret, err := wiki.LoadOrCreateSecret(root)
	if err != nil {
		t.Fatal(err)
	}
	return store.MintToken(secret)
}

// TestGateCannotBlockOnItsOwnFailure is the regression for 2026-08-22, when
// every tool call on a machine stopped for fifteen minutes.
//
// The handler had propagated the binary's exit code, and the binary answers 2
// for a usage error as well as for a block. So a graphin that did not know the
// `wiki` subcommand — an older one the handler reached for a moment during an
// upgrade — refused every Bash, Edit and Task with the gate's own message,
// which names a recovery that runs through the tool that was not working. The
// instruction was unfollowable by construction.
//
// The fix is that a block has a code nothing else can produce. This test does
// not check that number; it checks the property, by handing the handler a
// binary that fails the way the real one did.
func TestGateCannotBlockOnItsOwnFailure(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not on PATH")
	}
	handler, err := filepath.Abs(wikiHandlerRel)
	if err != nil {
		t.Fatal(err)
	}
	root := gatedWorkspace(t)

	for _, tc := range []struct {
		name, script string
	}{
		{"usage error", "#!/bin/sh\necho 'graphin: --workspace is required' >&2\nexit 2\n"},
		{"crash", "#!/bin/sh\nexit 1\n"},
		{"panic-ish", "#!/bin/sh\necho boom >&2\nexit 3\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := filepath.Join(t.TempDir(), "graphin")
			if err := os.WriteFile(fake, []byte(tc.script), 0o755); err != nil {
				t.Fatal(err)
			}
			raw, err := json.Marshal(map[string]any{
				"hook_event_name": "PreToolUse",
				"session_id":      "regression",
				"cwd":             root,
				"tool_name":       "Bash",
				"tool_input":      map[string]any{"command": "ls"},
			})
			if err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command("sh", handler, "gate")
			cmd.Stdin = bytes.NewReader(raw)
			cmd.Env = []string{
				"CLAUDE_PROJECT_DIR=" + root,
				"GRAPHIN_BIN=" + fake,
				"PATH=" + os.Getenv("PATH"),
				"HOME=" + os.Getenv("HOME"),
			}
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			code := 0
			if err := cmd.Run(); err != nil {
				ee, ok := err.(*exec.ExitError)
				if !ok {
					t.Fatalf("run handler: %v", err)
				}
				code = ee.ExitCode()
			}
			if code != 0 {
				t.Fatalf("exit = %d, want 0 — a graphin that cannot answer must not block:\n%s",
					code, stderr.String())
			}
		})
	}
}
