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

	"github.com/Salvia95/graphin/internal/admin"
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
	// (docs/usage-spec.md). ingest is the graphin-usage plugin's hook sink.
	if len(os.Args) > 1 && os.Args[1] == "usage" {
		os.Exit(usage.Run(os.Args[2:], os.Stdin, os.Stdout, os.Stderr))
	}

	var (
		root        = flag.String("workspace", "", "path to the workspace to index (required)")
		modelType   = flag.String("model-type", "multilingual_cjk", "embedding model: english_optimal | multilingual_cjk")
		offline     = flag.Bool("offline", false, "never download; use local runtime/ artifacts only")
		modelDir    = flag.String("model-dir", "", "local directory containing the ONNX model")
		ortLib      = flag.String("ort-lib", "", "path to the onnxruntime shared library")
		workers     = flag.Int("workers", runtime.NumCPU(), "parser worker pool size")
		semMaxNodes = flag.Int("semantic-max-nodes", 40000,
			"disable semantic search above this node count; lexical stays on (0 = no limit). "+
				"Default from docs/eval cold-start: ~1.4GB peak / ~4.6min warmup at 40k on 8GB.")
		verbose   = flag.Bool("verbose", false, "mirror JSONL logs to stderr")
		adminAddr = flag.String("admin-addr", "",
			"serve the read-only local admin page at this loopback address "+
				"(e.g. 127.0.0.1:7466); empty = disabled")
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

	// One resolved identity for both surfaces: the admin footer and the MCP
	// handshake must never disagree about which binary is running.
	ver, _ := buildIdentity()

	// Admin page rides in-process so it shares the live workspace instead of
	// fighting the single-writer lock. It must be shut down before ws.Close —
	// the deferred stop below runs first (LIFO) and waits for handlers.
	adminCtx, stopAdmin := context.WithCancel(context.Background())
	adminDone := make(chan struct{})
	if *adminAddr != "" {
		go func() {
			defer close(adminDone)
			if err := admin.Serve(adminCtx, ws, *adminAddr, ver, lg); err != nil {
				// The MCP server keeps running without the page — never fatal.
				lg.Event("admin_serve_error", map[string]any{"error": err.Error()})
				// A bare bind error leaves the operator with no next move, and
				// the address usually comes from a plugin option rather than
				// this flag — name both places it can be changed.
				fmt.Fprintf(os.Stderr, "graphin: admin page unavailable: %v\n"+
					"graphin:   %s is in use — pick another address with --admin-addr, "+
					"or in the graphin plugin's admin_addr option (/plugin → Manage → graphin → Configure), "+
					"or per project in %s\n",
					err, *adminAddr, filepath.Join(abs, workspace.DataDirName, "admin-addr"))
			}
		}()
		fmt.Fprintf(os.Stderr, "graphin: admin page at http://%s\n", *adminAddr)
	} else {
		close(adminDone)
	}
	defer func() { stopAdmin(); <-adminDone }()

	srv := mcp.NewServer(os.Stdin, os.Stdout, reg, ver, lg)
	if err := srv.Serve(context.Background()); err != nil && err != context.Canceled {
		lg.Event("serve_error", map[string]any{"error": err.Error()})
		stopAdmin()
		<-adminDone
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
