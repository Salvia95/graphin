// graphin is a local MCP server that lets AI coding agents navigate a
// codebase through progressive disclosure: search_hybrid → explore_graph →
// read_code.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/Salvia95/graphin/internal/dbimport"
	"github.com/Salvia95/graphin/internal/mcp"
	"github.com/Salvia95/graphin/internal/mcp/tools"
	"github.com/Salvia95/graphin/internal/obs"
	"github.com/Salvia95/graphin/internal/workspace"
)

const version = "0.1.0-dev"

func main() {
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

	var (
		root      = flag.String("workspace", "", "path to the workspace to index (required)")
		modelType = flag.String("model-type", "multilingual_cjk", "embedding model: english_optimal | multilingual_cjk")
		offline   = flag.Bool("offline", false, "never download; use local runtime/ artifacts only")
		modelDir  = flag.String("model-dir", "", "local directory containing the ONNX model")
		ortLib    = flag.String("ort-lib", "", "path to the onnxruntime shared library")
		workers   = flag.Int("workers", runtime.NumCPU(), "parser worker pool size")
		semMaxNodes = flag.Int("semantic-max-nodes", 40000,
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

	srv := mcp.NewServer(os.Stdin, os.Stdout, reg, version, lg)
	if err := srv.Serve(context.Background()); err != nil && err != context.Canceled {
		lg.Event("serve_error", map[string]any{"error": err.Error()})
		os.Exit(1)
	}
}

// fatalf writes to stderr only — stdout belongs to the MCP transport.
func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "graphin: "+format+"\n", args...)
	os.Exit(2)
}
