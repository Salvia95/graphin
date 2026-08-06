package e2e

// Black-box acceptance for the instrumentation hook (docs/usage-spec.md §7):
// exec the real hook with fixture PostToolUse stdin against temp workspaces.
// Every case asserts the hook contract — exit 0, empty stdout.
//
// The hook moved from the retired graphin-usage plugin into plugin/graphin
// (plugin-distribution §6.7); this suite followed it rather than staying
// behind on a dead script. Behaviour specific to the move — resolution order
// and workspace_subdir — is covered in plugin_test.go.

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const (
	usageFixtures = "../testdata/fixtures/usage"
	handlerRel    = "../plugin/graphin/hooks/usage.sh"
)

func TestUsageHookHandler(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not on PATH")
	}
	handler, err := filepath.Abs(handlerRel)
	if err != nil {
		t.Fatal(err)
	}
	bin := buildGraphin(t)

	// run executes the hook with the fixture (its __CWD__ rewritten to cwd)
	// and the given env overrides, asserting the never-block contract.
	run := func(t *testing.T, fixture, cwd, projectDir string, extraEnv ...string) {
		t.Helper()
		raw, err := os.ReadFile(filepath.Join(usageFixtures, fixture))
		if err != nil {
			t.Fatal(err)
		}
		stdin := strings.ReplaceAll(string(raw), "__CWD__", cwd)
		cmd := exec.Command("sh", handler)
		cmd.Stdin = strings.NewReader(stdin)
		env := append([]string{"CLAUDE_PROJECT_DIR=" + projectDir}, extraEnv...)
		hasPath := false
		for _, e := range extraEnv {
			if strings.HasPrefix(e, "PATH=") {
				hasPath = true
			}
		}
		if !hasPath {
			env = append(env, "PATH="+os.Getenv("PATH"))
		}
		cmd.Env = env
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		if err := cmd.Run(); err != nil {
			t.Fatalf("handler must always exit 0: %v (stderr: %s)", err, stderr.String())
		}
		if stdout.Len() != 0 {
			t.Fatalf("handler must never write stdout (it would be parsed as a hook decision): %q", stdout.String())
		}
	}

	logPath := func(root string) string {
		return filepath.Join(root, ".graphin", "usage", "events.jsonl")
	}

	t.Run("no marker writes nothing", func(t *testing.T) {
		dir := t.TempDir()
		run(t, "hook-stdin-grep.json", dir, dir, "GRAPHIN_BIN="+bin)
		if _, err := os.Stat(filepath.Join(dir, ".graphin")); !os.IsNotExist(err) {
			t.Fatal(".graphin must not appear in unindexed projects")
		}
	})

	t.Run("marker at project dir appends one event", func(t *testing.T) {
		root := mkIndexedWorkspace(t)
		run(t, "hook-stdin-grep.json", root, root, "GRAPHIN_BIN="+bin)
		line := readSingleLine(t, logPath(root))
		for _, want := range []string{
			`"v":1`, `"session_id":"e2e-session"`, `"prompt_id":"e2e-prompt"`,
			`"tool":"Grep"`, `"pattern":"persistIndexesLocked"`,
		} {
			if !strings.Contains(line, want) {
				t.Fatalf("event missing %s: %s", want, line)
			}
		}
	})

	t.Run("marker in parent found by walk-up", func(t *testing.T) {
		root := mkIndexedWorkspace(t)
		sub := filepath.Join(root, "services", "api")
		if err := os.MkdirAll(sub, 0o755); err != nil {
			t.Fatal(err)
		}
		run(t, "hook-stdin-grep.json", sub, sub, "GRAPHIN_BIN="+bin)
		line := readSingleLine(t, logPath(root))
		if !strings.Contains(line, `"cwd":"services/api"`) {
			t.Fatalf("event should land at parent root with relative cwd: %s", line)
		}
	})

	t.Run("malformed stdin still exits clean", func(t *testing.T) {
		root := mkIndexedWorkspace(t)
		run(t, "hook-stdin-malformed.json", root, root, "GRAPHIN_BIN="+bin)
		if _, err := os.Stat(logPath(root)); !os.IsNotExist(err) {
			t.Fatal("malformed input must not be logged")
		}
	})

	t.Run("bash search logs pattern not command", func(t *testing.T) {
		root := mkIndexedWorkspace(t)
		run(t, "hook-stdin-bash-search.json", root, root, "GRAPHIN_BIN="+bin)
		line := readSingleLine(t, logPath(root))
		if !strings.Contains(line, `"search":true`) || !strings.Contains(line, `"pattern":"drainSignal"`) {
			t.Fatalf("bash search payload wrong: %s", line)
		}
		if strings.Contains(line, "head -20") || strings.Contains(line, "internal/semantic") {
			t.Fatalf("full command line leaked: %s", line)
		}
	})

	t.Run("sequential invocations append", func(t *testing.T) {
		root := mkIndexedWorkspace(t)
		run(t, "hook-stdin-grep.json", root, root, "GRAPHIN_BIN="+bin)
		run(t, "hook-stdin-graphin.json", root, root, "GRAPHIN_BIN="+bin)
		raw, err := os.ReadFile(logPath(root))
		if err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
		if len(lines) != 2 {
			t.Fatalf("got %d lines, want 2", len(lines))
		}
	})

	t.Run("search_hybrid response ids extracted", func(t *testing.T) {
		root := mkIndexedWorkspace(t)
		run(t, "hook-stdin-graphin.json", root, root, "GRAPHIN_BIN="+bin)
		line := readSingleLine(t, logPath(root))
		for _, want := range []string{
			`"tool":"mcp__mykey__search_hybrid"`, `"result_count":2`,
			`"result_ids":["workspace.persistIndexesLocked","workspace.Indexer"]`,
		} {
			if !strings.Contains(line, want) {
				t.Fatalf("event missing %s: %s", want, line)
			}
		}
	})

	t.Run("binary resolved via binpath sidecar", func(t *testing.T) {
		root := mkIndexedWorkspace(t)
		if err := os.WriteFile(filepath.Join(root, ".graphin", "binpath"), []byte(bin+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		run(t, "hook-stdin-grep.json", root, root) // no GRAPHIN_BIN
		readSingleLine(t, logPath(root))
	})

	t.Run("unresolvable binary is a silent no-op", func(t *testing.T) {
		root := mkIndexedWorkspace(t)
		// No GRAPHIN_BIN, no binpath, and a PATH that keeps sh's helpers
		// (cat) but cannot contain a stray dev install of graphin.
		run(t, "hook-stdin-grep.json", root, root, "PATH=/usr/bin:/bin")
		if _, err := os.Stat(logPath(root)); !os.IsNotExist(err) {
			t.Fatal("nothing should be written when graphin cannot be resolved")
		}
	})
}

// mkIndexedWorkspace creates a temp dir carrying the index marker the guard
// requires (.graphin/merkle.json — dir existence alone is not enough).
func mkIndexedWorkspace(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".graphin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".graphin", "merkle.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func readSingleLine(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 1 {
		t.Fatalf("got %d lines, want 1: %s", len(lines), raw)
	}
	return lines[0]
}

// buildGraphin compiles the real binary once for the handler to exec.
func buildGraphin(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not on PATH")
	}
	bin := filepath.Join(t.TempDir(), "graphin")
	cmd := exec.Command("go", "build", "-o", bin, "../cmd/graphin")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	return bin
}
