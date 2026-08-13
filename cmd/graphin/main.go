// graphin is a local MCP server that lets AI coding agents navigate a
// codebase through progressive disclosure: search_hybrid → explore_graph →
// read_code.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"

	"github.com/Salvia95/graphin/internal/dbimport"
	"github.com/Salvia95/graphin/internal/mcp"
	"github.com/Salvia95/graphin/internal/mcp/tools"
	"github.com/Salvia95/graphin/internal/obs"
	"github.com/Salvia95/graphin/internal/provision"
	"github.com/Salvia95/graphin/internal/usage"
	"github.com/Salvia95/graphin/internal/workspace"
)

// Stamped by the release build via -ldflags -X. These MUST stay `var`:
// -ldflags cannot write to a const and fails silently when you try, which is
// how release binaries end up reporting a development version.
var (
	version   = "dev"
	commit    = ""
	buildDate = ""
)

func main() {
	// Subcommand: `graphin version [--json]` — the plugin installer and
	// /graphin:doctor identify a binary through this. It has to be a
	// subcommand, not a flag: --workspace is validated right after
	// flag.Parse, so a flag.Bool("version") would exit 2 before printing.
	if len(os.Args) > 1 && (os.Args[1] == "version" || os.Args[1] == "--version") {
		os.Exit(printVersion(os.Args[2:], os.Stdout, os.Stderr))
	}
	// Subcommand: `graphin dbimport …` converts/scaffolds graphindb snapshot
	// files and exits — no MCP transport involved, stdout is safe to use.
	if len(os.Args) > 1 && os.Args[1] == "dbimport" {
		os.Exit(dbimport.Run(os.Args[2:], os.Stdin, os.Stdout, os.Stderr))
	}
	// Subcommand: `graphin eval swe-explore …` runs the Phase 7c exploration
	// benchmark harness and exits.
	if len(os.Args) > 1 && os.Args[1] == "eval" {
		os.Exit(runEval(os.Args[2:]))
	}
	// Subcommand: `graphin usage ingest|report` — adoption instrumentation
	// (docs/usage-spec.md). ingest is the graphin plugin's PostToolUse sink.
	if len(os.Args) > 1 && os.Args[1] == "usage" {
		os.Exit(usage.Run(os.Args[2:], os.Stdin, os.Stdout, os.Stderr))
	}

	// Flag defaults live in workspace.DefaultConfig so diagnose_index can say
	// which settings were changed without keeping a second copy of them.
	def := workspace.DefaultConfig()
	var (
		root        = flag.String("workspace", "", "path to the workspace to index (required)")
		modelType   = flag.String("model-type", def.ModelType, "embedding model: english_optimal | multilingual_cjk")
		offline     = flag.Bool("offline", def.Offline, "never download; use local runtime/ artifacts only")
		modelDir    = flag.String("model-dir", def.ModelDir, "local directory containing the ONNX model")
		ortLib      = flag.String("ort-lib", def.OrtLib, "path to the onnxruntime shared library")
		workers     = flag.Int("workers", def.Workers, "parser worker pool size")
		semMaxNodes = flag.Int("semantic-max-nodes", def.SemanticMaxNodes,
			"disable semantic search above this node count; lexical stays on (0 = no limit). "+
				"Default from docs/eval cold-start: ~1.4GB peak / ~4.6min warmup at 40k on 8GB.")
		verbose = flag.Bool("verbose", false, "mirror JSONL logs to stderr")
	)
	flag.Parse()

	if *root == "" {
		fatalf("--workspace is required")
	}
	abs, err := filepath.Abs(*root)
	if err != nil {
		fatalf("resolve workspace: %v", err)
	}
	fi, err := os.Stat(abs)
	if err != nil || !fi.IsDir() {
		fatalf("workspace is not a directory: %s", abs)
	}

	lg, err := obs.New(filepath.Join(abs, workspace.DataDirName, "agent-nav.log"), *verbose)
	if err != nil {
		fatalf("open log: %v", err)
	}
	defer lg.Close()

	// binpath sidecar (docs/usage-spec.md §2.2): MCP configs register graphin
	// by absolute path, so the usage plugin's hook can't count on PATH — it
	// resolves the binary through this file instead.
	if exe, err := os.Executable(); err == nil {
		_ = os.WriteFile(filepath.Join(abs, workspace.DataDirName, "binpath"), []byte(exe+"\n"), 0o644)
	}

	ws := workspace.New(workspace.Config{
		Root:             abs,
		Workers:          *workers,
		ModelType:        *modelType,
		Offline:          *offline,
		ModelDir:         *modelDir,
		OrtLib:           *ortLib,
		SemanticMaxNodes: *semMaxNodes,
		Log:              lg,
	})
	defer ws.Close()

	reg := mcp.NewRegistry()
	tools.Register(reg, ws)

	// One resolved identity for every surface: `graphin version` and the MCP
	// handshake must never disagree about which binary is running.
	ver, _ := buildIdentity()

	// Process-start marker. Nothing else in agent-nav.log says "a server came
	// up here, at this time", and adoption measurement needs that boundary to
	// tell one run's events from the next — the 2026-08-11 remeasure took it
	// from the admin listener's event, and the admin page is gone.
	lg.Event("server_start", map[string]any{"version": ver, "workspace": abs})

	srv := mcp.NewServer(os.Stdin, os.Stdout, reg, ver, lg)
	if err := srv.Serve(context.Background()); err != nil && err != context.Canceled {
		lg.Event("serve_error", map[string]any{"error": err.Error()})
		os.Exit(1)
	}
}

// printVersion writes the build identity and exits. Writing to stdout is
// safe here: the MCP transport never starts on this path.
func printVersion(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	for _, a := range args {
		if a != "--json" {
			fmt.Fprintf(stderr, "graphin version: unknown argument %q (only --json)\n", a)
			return 2
		}
		asJSON = true
	}

	ver, sha := buildIdentity()
	if !asJSON {
		line := fmt.Sprintf("graphin %s %s/%s", ver, runtime.GOOS, runtime.GOARCH)
		if detail := strings.Join(nonEmpty(sha, buildDate), ", "); detail != "" {
			line += " (" + detail + ")"
		}
		if !provision.SemanticSupported(runtime.GOOS, runtime.GOARCH) {
			line += " [lexical only: no onnxruntime " + provision.ORTVersion + " build for this platform]"
		}
		fmt.Fprintln(stdout, line)
		return 0
	}

	b, err := json.Marshal(map[string]any{
		"version":    ver,
		"commit":     sha,
		"build_date": buildDate,
		"os":         runtime.GOOS,
		"arch":       runtime.GOARCH,
		"ort":        provision.ORTVersion,
		// Whether this platform can do semantic search at all — the one line
		// that separates "the model failed to download" from "it never can".
		"semantic_supported": provision.SemanticSupported(runtime.GOOS, runtime.GOARCH),
	})
	if err != nil {
		fmt.Fprintf(stderr, "graphin version: %v\n", err)
		return 1
	}
	fmt.Fprintln(stdout, string(b))
	return 0
}

// buildIdentity resolves the version and commit to report.
//
// -ldflags stamps them for release builds, but a binary from
// `go install …/cmd/graphin@v1.0.0` carries no ldflags at all — the module
// version is only in the embedded build info. Without this fallback the
// installer's post-install check reads "dev" and rejects a perfectly good
// binary (docs/plugin-distribution.md §6.2 4단계).
func buildIdentity() (ver, sha string) {
	ver, sha = version, commit
	if ver == "" {
		ver = "dev"
	}
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return ver, sha
	}
	if ver == "dev" && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		ver = bi.Main.Version
	}
	if sha == "" {
		for _, s := range bi.Settings {
			if s.Key == "vcs.revision" {
				sha = s.Value
				if len(sha) > 7 {
					sha = sha[:7]
				}
				break
			}
		}
	}
	return ver, sha
}

func nonEmpty(vals ...string) []string {
	out := vals[:0:0]
	for _, v := range vals {
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}

// fatalf writes to stderr only — stdout belongs to the MCP transport.
func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "graphin: "+format+"\n", args...)
	os.Exit(2)
}
