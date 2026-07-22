package main

// `graphin eval swe-explore` — Phase 7c harness CLI (docs/phase7-spec.md §3).
// Like dbimport, this path never touches the MCP transport, so stdout is
// safe for progress output.

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/Salvia95/graphin/internal/eval/sweexplore"
)

func runEval(args []string) int {
	if len(args) == 0 || args[0] != "swe-explore" {
		fmt.Fprintln(os.Stderr, "usage: graphin eval swe-explore --bench <jsonl> --repos <dir> --out <dir> [flags]")
		return 2
	}
	fs := flag.NewFlagSet("eval swe-explore", flag.ExitOnError)
	var (
		bench   = fs.String("bench", "", "SWE-Explore-Bench JSONL file (required)")
		repos   = fs.String("repos", "", "root directory holding repo snapshots (required)")
		out     = fs.String("out", "eval-out", "output directory for submissions + summary")
		policy  = fs.String("policy", "graphin", "explorer policy: graphin | grep")
		topK    = fs.Int("top-k", 5, "search results per derived query")
		rrfK    = fs.Int("rrf-k", 60, "RRF constant")
		minConf = fs.Float64("min-confidence", 0.5, "explore_graph confidence floor")
		queries = fs.Int("queries", 3, "derived queries per issue")

		maxRegions     = fs.Int("max-regions", 100, "ranked region list cap")
		maxRegionLines = fs.Int("max-region-lines", 400, "skip nodes spanning more lines")
		grepCtx        = fs.Int("grep-context", 20, "grep baseline ±context lines")
		maxTasks       = fs.Int("tasks", 0, "limit task count (0 = all)")
		sweep          = fs.Bool("sweep", false, "run the spec §3.3 config matrix instead of one config")

		semantic  = fs.Bool("semantic", false, "hybrid mode: wait for the vector engine per task")
		modelType = fs.String("model-type", "multilingual_cjk", "embedding model (semantic mode)")
		modelDir  = fs.String("model-dir", "", "local ONNX model directory")
		ortLib    = fs.String("ort-lib", "", "onnxruntime shared library path")
		offline   = fs.Bool("offline", false, "never download model artifacts")
		keepIndex = fs.Bool("keep-index", false, "leave <repo>/.graphin behind for inspection")
	)
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}
	if *bench == "" || *repos == "" {
		fmt.Fprintln(os.Stderr, "graphin eval: --bench and --repos are required")
		return 2
	}

	base := sweexplore.Defaults()
	base.TopK, base.RRFK, base.MinConf = *topK, *rrfK, float32(*minConf)
	base.Queries = *queries
	base.MaxRegions, base.MaxRegionLines = *maxRegions, *maxRegionLines
	base.Semantic, base.ModelType, base.ModelDir = *semantic, *modelType, *modelDir
	base.OrtLib, base.Offline, base.KeepIndex = *ortLib, *offline, *keepIndex

	ro := sweexplore.RunOptions{
		BenchPath:   *bench,
		ReposRoot:   *repos,
		OutDir:      *out,
		Policy:      *policy,
		GrepContext: *grepCtx,
		MaxTasks:    *maxTasks,
		Base:        base,
	}
	if *sweep {
		ro.Configs = sweexplore.DefaultSweep()
	}
	if err := sweexplore.Run(context.Background(), ro, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "graphin eval: %v\n", err)
		return 1
	}
	fmt.Printf("done → %s\n", *out)
	return 0
}
