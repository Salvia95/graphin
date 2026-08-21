// diagnose.go — diagnose_index reports the index's own health from the live
// engine, in-process. A separate CLI process could not do this safely:
// graph.Open truncates the delta log and drops corrupt shards, so there is no
// way to attach to a running workspace read-only.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Salvia95/graphin/internal/graph"
	"github.com/Salvia95/graphin/internal/mcp"
	"github.com/Salvia95/graphin/internal/provision"
	"github.com/Salvia95/graphin/internal/semantic"
	"github.com/Salvia95/graphin/internal/workspace"
)

// diagSampleMax bounds the dangling/partial samples. The counts beside them
// always cover everything — only the listed rows are capped.
const diagSampleMax = 10

// storageDirs are the .graphin subdirectories broken out by size.
var storageDirs = []string{"search", "graph", "runtime", "usage"}

// diagnoseHandler answers "is the index trustworthy right now". It is
// deliberately not behind the notBootstrapped guard: the situations worth
// diagnosing are exactly the ones where the workspace is not in its happy
// state, and the graph facade returns zero values before bootstrap by design.
func diagnoseHandler(ws *workspace.Workspace) mcp.ToolHandler {
	return func(_ context.Context, _ json.RawMessage) (string, bool) {
		var sb strings.Builder
		writeStatusPrefix(&sb, ws)

		st := ws.Status()
		fmt.Fprintf(&sb, "<index_diagnostics state=%q bootstrapped=\"%t\">\n",
			mcp.EscapeAttr(st.State), ws.Bootstrapped())

		stats := ws.GraphStats()
		fmt.Fprintf(&sb, "  <graph nodes=\"%d\" edges=\"%d\" shards=\"%d\" />\n",
			stats.Nodes, stats.Edges, len(stats.Shards))

		totals := writeDangling(&sb, ws)
		partial := writePartial(&sb, ws)

		rs := ws.GraphReverseStats()
		fmt.Fprintf(&sb, "  <reverse targets=\"%d\" edges=\"%d\" log_records=\"%d\" log_tombstones=\"%d\" redirects=\"%d\" />\n",
			rs.Targets, rs.Edges, rs.LogRecords, rs.LogTombstones, rs.Redirects)

		// An empty model type is not "unspecified": bootstrap falls back to the
		// built-in default, so report the value that actually takes effect or
		// the default comparison below lies.
		cfg := ws.EffectiveConfig()
		if cfg.ModelType == "" {
			cfg.ModelType = workspace.DefaultConfig().ModelType
		}
		hdr := ws.SemanticHeader()
		expected, mismatch := modelExpectation(cfg, hdr)
		writeSemantic(&sb, ws, st, hdr, expected, mismatch)
		writeConfig(&sb, cfg)
		writeStorage(&sb, ws.Dir)

		for _, h := range redirectHint(rs, stats.Nodes) {
			fmt.Fprintf(&sb, "  <hint>%s</hint>\n", mcp.EscapeText(h))
		}
		for _, h := range diagHints(hdr, expected, mismatch, totals, partial) {
			fmt.Fprintf(&sb, "  <hint>%s</hint>\n", mcp.EscapeText(h))
		}

		sb.WriteString("</index_diagnostics>")
		return sb.String(), false
	}
}

// writeDangling reports edges whose target is not on the read path. Samples
// list code targets first: DB targets dangle by design (§7a) and are the less
// actionable half, so they must not crowd code rows out of a capped list.
func writeDangling(sb *strings.Builder, ws *workspace.Workspace) graph.DanglingTotals {
	rows, totals := ws.GraphDangling(diagSampleMax * 4)
	if totals.Sum() == 0 {
		sb.WriteString("  <dangling code=\"0\" db=\"0\" />\n")
		return totals
	}
	fmt.Fprintf(sb, "  <dangling code=\"%d\" db=\"%d\">\n", totals.Code, totals.DB)
	n := 0
	for _, wantDB := range []bool{false, true} {
		for _, d := range rows {
			if d.DBDomain != wantDB || n >= diagSampleMax {
				continue
			}
			fmt.Fprintf(sb, "    <edge source=%q type=%q confidence=\"%.2f\" target=%q db=\"%t\" />\n",
				mcp.EscapeAttr(d.SourceID), graph.EdgeTypeName(d.Edge.Type),
				d.Edge.Confidence, mcp.EscapeAttr(d.Edge.TargetID), d.DBDomain)
			n++
		}
	}
	sb.WriteString("  </dangling>\n")
	return totals
}

// writePartial counts files parsed with syntax errors. The scan is O(nodes) —
// it is the one section of this tool that is not a cheap lookup.
func writePartial(sb *strings.Builder, ws *workspace.Workspace) int {
	var total int
	var sample []graph.NodeInfo
	ws.GraphForEach(func(n graph.NodeInfo) bool {
		if !n.Partial {
			return true
		}
		total++
		if len(sample) < diagSampleMax {
			sample = append(sample, n)
		}
		return true
	})
	if total == 0 {
		sb.WriteString("  <partial count=\"0\" />\n")
		return 0
	}
	fmt.Fprintf(sb, "  <partial count=\"%d\">\n", total)
	for _, n := range sample {
		fmt.Fprintf(sb, "    <node id=%q file=%q />\n",
			mcp.EscapeAttr(n.ID), mcp.EscapeAttr(n.FilePath))
	}
	sb.WriteString("  </partial>\n")
	return total
}

func writeSemantic(sb *strings.Builder, ws *workspace.Workspace, st mcp.Status,
	hdr *semantic.Header, expected string, mismatch bool) {
	pending, depth, drained := ws.SemanticQueue()
	fmt.Fprintf(sb, "  <semantic ready=\"%t\" gated=\"%t\" pending=\"%d\" queue_depth=\"%d\" drained=\"%t\"",
		st.SemanticReady, st.SemanticGated, pending, depth, drained)
	if hdr != nil {
		fmt.Fprintf(sb, " stored_model_id=%q vector_files=\"%d\"",
			mcp.EscapeAttr(hdr.ModelID), len(hdr.Files))
	}
	if expected != "" {
		fmt.Fprintf(sb, " expected_model_id=%q mismatch=\"%t\"", mcp.EscapeAttr(expected), mismatch)
	}
	if nodes, max, gated := ws.GateInfo(); gated {
		fmt.Fprintf(sb, " gate_nodes=\"%d\" gate_max=\"%d\"", nodes, max)
	}
	if fail := ws.SemUnavailable(); fail != "" {
		fmt.Fprintf(sb, " failure=%q", mcp.EscapeAttr(fail))
	}
	sb.WriteString(" />\n")
}

// writeConfig renders the effective startup flags. changed="true" is a value
// comparison against the built-in default, not a record of what was typed:
// passing a flag explicitly with its default value still reads as unchanged.
func writeConfig(sb *strings.Builder, cfg workspace.ConfigView) {
	def := workspace.DefaultConfig()
	sb.WriteString("  <config")
	fmt.Fprintf(sb, " workspace=%q", mcp.EscapeAttr(cfg.Root))
	writeCfgAttr(sb, "model_type", cfg.ModelType, def.ModelType)
	writeCfgAttr(sb, "workers", strconv.Itoa(cfg.Workers), strconv.Itoa(def.Workers))
	writeCfgAttr(sb, "semantic_max_nodes", strconv.Itoa(cfg.SemanticMaxNodes), strconv.Itoa(def.SemanticMaxNodes))
	writeCfgAttr(sb, "offline", strconv.FormatBool(cfg.Offline), strconv.FormatBool(def.Offline))
	writeCfgAttr(sb, "model_dir", cfg.ModelDir, def.ModelDir)
	writeCfgAttr(sb, "ort_lib", cfg.OrtLib, def.OrtLib)
	fmt.Fprintf(sb, " ort_version=%q />\n", provision.ORTVersion)
}

func writeCfgAttr(sb *strings.Builder, name, value, def string) {
	fmt.Fprintf(sb, " %s=%q", name, mcp.EscapeAttr(value))
	if value != def {
		fmt.Fprintf(sb, " %s_changed=\"true\"", name)
	}
}

func writeStorage(sb *strings.Builder, dir string) {
	var total int64
	var rows strings.Builder
	for _, sub := range storageDirs {
		n := duDir(filepath.Join(dir, sub))
		total += n
		fmt.Fprintf(&rows, "    <dir name=%q bytes=\"%d\" />\n", sub, n)
	}
	fmt.Fprintf(sb, "  <storage path=%q total_bytes=\"%d\">\n", mcp.EscapeAttr(dir), total)
	sb.WriteString(rows.String())
	sb.WriteString("  </storage>\n")
}

// duDir sums file sizes under dir (0 when absent).
func duDir(dir string) int64 {
	var total int64
	_ = filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if info, err := d.Info(); err == nil {
			total += info.Size()
		}
		return nil
	})
	return total
}

// modelExpectation resolves the configured model type to its pinned ModelID
// and flags a vectors.bin written by a different model (mixed-index hazard).
func modelExpectation(cfg workspace.ConfigView, hdr *semantic.Header) (string, bool) {
	spec, ok := provision.Models[cfg.ModelType]
	if !ok {
		return "", false
	}
	return spec.ID, hdr != nil && hdr.ModelID != spec.ID
}

// diagHints turns the numbers into the reading an operator would otherwise
// have to know by heart. A healthy index emits none.
func diagHints(hdr *semantic.Header, expected string, mismatch bool,
	totals graph.DanglingTotals, partial int) []string {
	var out []string
	if mismatch {
		out = append(out, fmt.Sprintf(
			"vectors.bin was written by model %q but %q is configured. The index now mixes two "+
				"vector spaces and semantic ranking degrades. Act on this one: stop the server, "+
				"delete .graphin/search/vectors.bin, restart.", hdr.ModelID, expected))
	}
	if totals.Code > 0 {
		out = append(out, fmt.Sprintf(
			"%d code edges point at a node that is not in the index — a deleted target or an "+
				"unresolved reference. Re-indexing usually clears them; a small, stable count is "+
				"normal in a large tree.", totals.Code))
	}
	if totals.DB > 0 {
		out = append(out, fmt.Sprintf(
			"%d DB edges dangle. That is often intentional: a reference to something outside the "+
				"snapshot, such as Supabase's auth.users. Check what they point at before treating "+
				"the count as a defect.", totals.DB))
	}
	if partial > 0 {
		out = append(out, fmt.Sprintf(
			"%d nodes come from files parsed with syntax errors, so edges inside the broken span "+
				"may be missing and exploration around them is incomplete. Fix the syntax error and "+
				"the watcher re-indexes the file.", partial))
	}
	return out
}

// redirectCarryFrac is where carrying superseded IDs stops being free.
//
// Redirects are upsert-class, so compaction preserves them and the table only
// grows. The GC that would collect them is deferred by design (§4.3), and this
// is the number that says when deferring it stops being the right call.
const redirectCarryFrac = 0.20

// redirectHint reports the carry ratio once it is worth acting on.
func redirectHint(rs graph.ReverseStats, nodes int) []string {
	if rs.Redirects == 0 || nodes == 0 {
		return nil
	}
	frac := float64(rs.Redirects) / float64(nodes)
	if frac < redirectCarryFrac {
		return nil
	}
	return []string{fmt.Sprintf(
		"the reverse log carries %d redirects against %d live nodes (%.0f%%). "+
			"They are never dropped at compaction, so this only grows: at this ratio "+
			"the deferred redirect GC is worth implementing.",
		rs.Redirects, nodes, frac*100)}
}
