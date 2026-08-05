package e2e

// Black-box acceptance for the graphin plugin's shell surface
// (docs/plugin-distribution.md §6): the launcher that turns plugin env into a
// command line, and the relocated instrumentation hook.
//
// Shares mkIndexedWorkspace / readSingleLine / buildGraphin with
// usage_hook_test.go.

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const (
	launcherRel  = "../plugin/graphin/bin/graphin-launch.sh"
	usageHookRel = "../plugin/graphin/hooks/usage.sh"
	pluginRel    = "../plugin/graphin"
)

// argvStub writes a fake graphin that prints the argv it was handed, so the
// launcher's command line can be asserted exactly.
func argvStub(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "stub")
	body := "#!/bin/sh\nfor a in \"$@\"; do printf '%s\\n' \"$a\"; done\n"
	if err := os.WriteFile(p, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestLauncherArgv covers the reason .mcp.json carries no `args`: a static
// JSON config cannot omit --admin-addr when it is empty, and cannot express
// --offline at all. The launcher is what makes those conditional.
func TestLauncherArgv(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not on PATH")
	}
	launcher, err := filepath.Abs(launcherRel)
	if err != nil {
		t.Fatal(err)
	}
	root, err := filepath.Abs(pluginRel)
	if err != nil {
		t.Fatal(err)
	}
	stub := argvStub(t)

	run := func(t *testing.T, projectDir string, env ...string) []string {
		t.Helper()
		cmd := exec.Command("sh", launcher)
		cmd.Env = append([]string{
			"PATH=" + os.Getenv("PATH"),
			"CLAUDE_PLUGIN_ROOT=" + root,
			"CLAUDE_PLUGIN_DATA=" + t.TempDir(),
			"GRAPHIN_PROJECT_DIR=" + projectDir,
			"GRAPHIN_BINARY_PATH=" + stub,
		}, env...)
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		if err := cmd.Run(); err != nil {
			t.Fatalf("launcher failed: %v (stderr: %s)", err, stderr.String())
		}
		out := strings.TrimSpace(stdout.String())
		if out == "" {
			return nil
		}
		return strings.Split(out, "\n")
	}

	eq := func(t *testing.T, got, want []string) {
		t.Helper()
		if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
			t.Fatalf("argv =\n  %q\nwant\n  %q", got, want)
		}
	}

	t.Run("unset options add no flags", func(t *testing.T) {
		dir := t.TempDir()
		// Every declared-but-unconfigured user_config option renders as an
		// empty string, which must not become an empty flag value.
		got := run(t, dir,
			"GRAPHIN_ADMIN_ADDR=", "GRAPHIN_MODEL_TYPE=", "GRAPHIN_OFFLINE=",
			"GRAPHIN_MODEL_DIR=", "GRAPHIN_SEMANTIC_MAX_NODES=", "GRAPHIN_WORKSPACE_SUBDIR=")
		eq(t, got, []string{"--workspace", dir})
	})

	t.Run("unsubstituted literal is treated as unset", func(t *testing.T) {
		dir := t.TempDir()
		got := run(t, dir, "GRAPHIN_ADMIN_ADDR=${user_config.admin_addr}")
		eq(t, got, []string{"--workspace", dir})
	})

	t.Run("booleans", func(t *testing.T) {
		dir := t.TempDir()
		eq(t, run(t, dir, "GRAPHIN_OFFLINE=true"), []string{"--workspace", dir, "--offline"})
		eq(t, run(t, dir, "GRAPHIN_OFFLINE=false"), []string{"--workspace", dir})
	})

	t.Run("workspace_subdir descends", func(t *testing.T) {
		dir := t.TempDir()
		got := run(t, dir, "GRAPHIN_WORKSPACE_SUBDIR=backend")
		eq(t, got, []string{"--workspace", filepath.Join(dir, "backend")})
	})

	// D5: plugin options live in user settings only, so admin_addr is one
	// global value. Without a per-project override every project would fight
	// over the same port the moment it is set.
	t.Run("project admin-addr file beats the global option", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(dir, ".graphin"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, ".graphin", "admin-addr"),
			[]byte("  127.0.0.1:7777\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		got := run(t, dir, "GRAPHIN_ADMIN_ADDR=127.0.0.1:7466")
		eq(t, got, []string{"--workspace", dir, "--admin-addr", "127.0.0.1:7777"})
	})

	t.Run("global option applies with no project file", func(t *testing.T) {
		dir := t.TempDir()
		got := run(t, dir, "GRAPHIN_ADMIN_ADDR=127.0.0.1:7466")
		eq(t, got, []string{"--workspace", dir, "--admin-addr", "127.0.0.1:7466"})
	})
}

// TestPluginUsageHook covers what changed when the hook moved into this
// plugin (§6.7): the binary resolution order and workspace_subdir awareness.
func TestPluginUsageHook(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not on PATH")
	}
	hook, err := filepath.Abs(usageHookRel)
	if err != nil {
		t.Fatal(err)
	}
	bin := buildGraphin(t)

	run := func(t *testing.T, cwd, projectDir string, env ...string) {
		t.Helper()
		raw, err := os.ReadFile(filepath.Join(usageFixtures, "hook-stdin-grep.json"))
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command("sh", hook)
		cmd.Stdin = strings.NewReader(strings.ReplaceAll(string(raw), "__CWD__", cwd))
		cmd.Env = append([]string{
			"PATH=/usr/bin:/bin", // no stray dev install of graphin
			"CLAUDE_PROJECT_DIR=" + projectDir,
		}, env...)
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		if err := cmd.Run(); err != nil {
			t.Fatalf("hook must always exit 0: %v (stderr: %s)", err, stderr.String())
		}
		if stdout.Len() != 0 {
			t.Fatalf("hook must never write stdout: %q", stdout.String())
		}
	}

	logPath := func(root string) string {
		return filepath.Join(root, ".graphin", "usage", "events.jsonl")
	}

	// The subtle one. os.Executable() resolves symlinks, so a server started
	// through $DATA/bin/graphin records the *versioned* file in binpath; the
	// next upgrade prunes it and binpath dangles. Preferring the symlink is
	// what makes instrumentation survive an upgrade — if the order were
	// reversed, the stale binpath below would win and nothing would be logged.
	t.Run("plugin symlink outranks a stale binpath", func(t *testing.T) {
		root := mkIndexedWorkspace(t)
		data := t.TempDir()
		if err := os.MkdirAll(filepath.Join(data, "bin"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(bin, filepath.Join(data, "bin", "graphin")); err != nil {
			t.Fatal(err)
		}
		stale := filepath.Join(data, "bin", "graphin-0.0.1-linux-amd64")
		if err := os.WriteFile(filepath.Join(root, ".graphin", "binpath"),
			[]byte(stale+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		run(t, root, root, "CLAUDE_PLUGIN_DATA="+data)
		readSingleLine(t, logPath(root)) // fails if the stale path won
	})

	t.Run("binary_path option is honoured", func(t *testing.T) {
		root := mkIndexedWorkspace(t)
		run(t, root, root, "CLAUDE_PLUGIN_OPTION_BINARY_PATH="+bin)
		readSingleLine(t, logPath(root))
	})

	// Walking up from the project root can never find a marker that lives
	// below it, so the hook has to read the same subdir setting the server does.
	t.Run("workspace_subdir marker is found", func(t *testing.T) {
		project := t.TempDir()
		sub := filepath.Join(project, "backend")
		if err := os.MkdirAll(filepath.Join(sub, ".graphin"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(sub, ".graphin", "merkle.json"),
			[]byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
		run(t, sub, project, "GRAPHIN_BIN="+bin, "CLAUDE_PLUGIN_OPTION_WORKSPACE_SUBDIR=backend")
		readSingleLine(t, logPath(sub))
	})

	t.Run("without the subdir setting nothing is logged", func(t *testing.T) {
		project := t.TempDir()
		sub := filepath.Join(project, "backend")
		if err := os.MkdirAll(filepath.Join(sub, ".graphin"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(sub, ".graphin", "merkle.json"),
			[]byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
		run(t, project, project, "GRAPHIN_BIN="+bin)
		if _, err := os.Stat(logPath(sub)); !os.IsNotExist(err) {
			t.Fatal("the guard must not reach below the project root on its own")
		}
	})
}
