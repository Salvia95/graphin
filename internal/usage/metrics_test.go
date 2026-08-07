package usage

import "testing"

func gsearch(session, prompt, use, query string, ids ...any) Event {
	p := map[string]any{"query": query}
	if len(ids) > 0 {
		p["result_ids"] = ids
		p["result_count"] = float64(len(ids))
	}
	return ev(session, "", prompt, use, "mcp__k__search_hybrid", false, p)
}

func compute(events ...Event) Report {
	return Compute(events, nil, Options{})
}

func TestMetricsAdoptionViaRead(t *testing.T) {
	rep := compute(
		gsearch("s1", "p1", "u1", "order cancel"),
		ev("s1", "", "p1", "u2", "Read", false, map[string]any{"file_path": "a.go"}),
	)
	g := rep.Groups["all"]
	if g.Adoptions != 1 || g.Fallbacks != 0 || g.Inconclusive != 0 {
		t.Fatalf("group = %+v", g)
	}
}

func TestMetricsAdoptionViaAction(t *testing.T) {
	rep := compute(
		gsearch("s1", "p1", "u1", "order cancel"),
		ev("s1", "", "p1", "u2", "Edit", false, map[string]any{"file_path": "a.go"}),
	)
	if g := rep.Groups["all"]; g.Adoptions != 1 {
		t.Fatalf("group = %+v", g)
	}
}

func TestMetricsAdoptionViaGReadAtWindowEnd(t *testing.T) {
	rep := compute(
		gsearch("s1", "p1", "u1", "order cancel"),
		ev("s1", "", "p1", "u2", "mcp__k__read_code", false, map[string]any{"node_id": "n1"}),
	)
	if g := rep.Groups["all"]; g.Adoptions != 1 || g.Inconclusive != 0 {
		t.Fatalf("group = %+v", g)
	}
}

func TestMetricsSameIntentFallback(t *testing.T) {
	rep := compute(
		gsearch("s1", "p1", "u1", "where is order cancellation handled"),
		ev("s1", "", "p1", "u2", "Grep", false, map[string]any{"pattern": "cancelOrder"}),
	)
	g := rep.Groups["all"]
	if g.Fallbacks != 1 || g.SameIntentFallbacks != 1 {
		t.Fatalf("group = %+v", g)
	}
	if len(rep.FallbackPairs) != 1 || !rep.FallbackPairs[0].SameIntent {
		t.Fatalf("pairs = %+v", rep.FallbackPairs)
	}
}

func TestMetricsNewIntentFallback(t *testing.T) {
	rep := compute(
		gsearch("s1", "p1", "u1", "order cancellation"),
		ev("s1", "", "p1", "u2", "Grep", false, map[string]any{"pattern": "tokenizerConfig"}),
	)
	g := rep.Groups["all"]
	if g.Fallbacks != 1 || g.SameIntentFallbacks != 0 {
		t.Fatalf("group = %+v", g)
	}
}

func TestMetricsBatchTieBreakSearchWins(t *testing.T) {
	// Parallel batch after the run mixes Read with Grep: pessimistic → fallback.
	rep := compute(
		gsearch("s1", "p1", "u1", "order cancel"),
		ev("s1", "", "p1", "u2", "Read", true, map[string]any{"file_path": "a.go"}),
		ev("s1", "", "p1", "u3", "Grep", true, map[string]any{"pattern": "cancelOrder"}),
	)
	g := rep.Groups["all"]
	if g.Fallbacks != 1 || g.Adoptions != 0 {
		t.Fatalf("group = %+v", g)
	}
}

func TestMetricsLateSwitch(t *testing.T) {
	rep := compute(
		ev("s1", "", "p1", "u1", "Grep", false, map[string]any{"pattern": "a"}),
		ev("s1", "", "p1", "u2", "Grep", false, map[string]any{"pattern": "b"}),
		gsearch("s1", "p1", "u3", "order cancel"),
		ev("s1", "", "p1", "u4", "Read", false, map[string]any{"file_path": "a.go"}),
	)
	if g := rep.Groups["all"]; g.LateSwitches != 1 {
		t.Fatalf("group = %+v", g)
	}
}

func TestMetricsDiscoveryFailure(t *testing.T) {
	rep := compute(
		ev("s1", "", "p1", "u1", "Grep", false, map[string]any{"pattern": "a"}),
		ev("s1", "", "p1", "u2", "Grep", false, map[string]any{"pattern": "b"}),
		ev("s1", "", "p1", "u3", "Bash", false, map[string]any{"search": true, "pattern": "c"}),
	)
	g := rep.Groups["all"]
	if g.DiscoveryFailures != 1 || g.WindowsWithSearch != 1 {
		t.Fatalf("group = %+v", g)
	}
	if rep.SessionsWithGraphin != 0 || rep.MedianCallsToFirstNav != -1 {
		t.Fatalf("report = %+v", rep)
	}
}

func TestMetricsInconclusiveRun(t *testing.T) {
	rep := compute(
		gsearch("s1", "p1", "u1", "order cancel"),
		ev("s1", "", "p1", "u2", "mcp__k__explore_graph", false, map[string]any{"node_id": "n"}),
		// window ends: no decisive follow-up, run does not end in read_code
	)
	if g := rep.Groups["all"]; g.Inconclusive != 1 || g.Adoptions+g.Fallbacks != 0 {
		t.Fatalf("group = %+v", g)
	}
}

func TestMetricsMergedRunJudgedOnce(t *testing.T) {
	// nav, other, nav, Read: the first run defers to the second — one adoption.
	rep := compute(
		gsearch("s1", "p1", "u1", "order cancel"),
		ev("s1", "", "p1", "u2", "Bash", false, map[string]any{"search": false}),
		gsearch("s1", "p1", "u3", "order cancel again"),
		ev("s1", "", "p1", "u4", "Read", false, map[string]any{"file_path": "a.go"}),
	)
	g := rep.Groups["all"]
	if g.Adoptions != 1 || g.Fallbacks != 0 || g.Inconclusive != 0 {
		t.Fatalf("group = %+v", g)
	}
}

func TestMetricsFunnelAdherence(t *testing.T) {
	rep := compute(
		gsearch("s1", "p1", "u1", "order cancel", "node.A", "node.B"),
		ev("s1", "", "p1", "u2", "mcp__k__explore_graph", false, map[string]any{"node_id": "node.A"}),
		gsearch("s1", "p2", "u3", "unrelated", "node.C"),
		ev("s1", "", "p2", "u4", "mcp__k__read_code", false, map[string]any{"node_id": "node.ZZZ"}),
	)
	g := rep.Groups["all"]
	if g.FunnelSearches != 2 || g.FunnelAdherent != 1 {
		t.Fatalf("group = %+v", g)
	}
}

func TestMetricsSubagentSplit(t *testing.T) {
	rep := compute(
		gsearch("s1", "p1", "u1", "q"),
		ev("s1", "", "p1", "u2", "Read", false, map[string]any{"file_path": "a.go"}),
		ev("s1", "sub", "p1", "u3", "Grep", false, map[string]any{"pattern": "x"}),
		ev("s1", "sub", "p1", "u4", "Grep", false, map[string]any{"pattern": "y"}),
		ev("s1", "sub", "p1", "u5", "Grep", false, map[string]any{"pattern": "z"}),
	)
	if g := rep.Groups["main"]; g.Adoptions != 1 || g.DiscoveryFailures != 0 {
		t.Fatalf("main = %+v", g)
	}
	if g := rep.Groups["subagent"]; g.DiscoveryFailures != 1 {
		t.Fatalf("subagent = %+v", g)
	}
	if g := rep.Groups["all"]; g.Adoptions != 1 || g.DiscoveryFailures != 1 {
		t.Fatalf("all = %+v", g)
	}
}

func TestMetricsSessionLevelAndMedian(t *testing.T) {
	rep := compute(
		// s1: nav on 3rd call
		ev("s1", "", "p1", "u1", "Grep", false, map[string]any{"pattern": "a"}),
		ev("s1", "", "p1", "u2", "Read", false, nil),
		gsearch("s1", "p1", "u3", "q"),
		// s2: no graphin at all
		ev("s2", "", "p1", "u4", "Grep", false, map[string]any{"pattern": "b"}),
	)
	if rep.Sessions != 2 || rep.SessionsWithGraphin != 1 {
		t.Fatalf("report = %+v", rep)
	}
	if rep.MedianCallsToFirstNav != 2 {
		t.Fatalf("median = %d, want 2", rep.MedianCallsToFirstNav)
	}
}

func TestMetricsBigrams(t *testing.T) {
	rep := compute(
		gsearch("s1", "p1", "u1", "q"),
		ev("s1", "", "p1", "u2", "Grep", false, map[string]any{"pattern": "x"}),
		ev("s1", "", "p1", "u3", "Read", false, nil),
	)
	if rep.Bigrams["g_search"]["search"] != 1 || rep.Bigrams["search"]["read"] != 1 {
		t.Fatalf("bigrams = %v", rep.Bigrams)
	}
}

func TestMetricsDailyTrend(t *testing.T) {
	a := gsearch("s1", "p1", "u1", "q")
	a.TS = "2026-08-01T09:00:00Z"
	b := ev("s1", "", "p1", "u2", "Read", false, nil)
	b.TS = "2026-08-01T09:00:01Z"
	rep := compute(a, b)
	if len(rep.Daily) != 1 || rep.Daily[0].Date != "2026-08-01" || rep.Daily[0].Adoptions != 1 {
		t.Fatalf("daily = %+v", rep.Daily)
	}
}

// ── Target split: code vs db schema nodes (spec §8) ──────────────────────────

func TestTargetsSplitByNodeNamespace(t *testing.T) {
	rep := compute(
		// db run: search surfaces a schema node, agent reads it, then edits.
		gsearch("s1", "p1", "u1", "job posting table", "db.main.public.job_posting"),
		ev("s1", "", "p1", "u2", "mcp__k__read_code", false, map[string]any{"node_id": "db.main.public.job_posting"}),
		ev("s1", "", "p1", "u3", "Edit", false, map[string]any{"file_path": "a.go"}),
		// code run in a separate window: search surfaces code, agent greps.
		gsearch("s1", "p2", "u4", "order cancel", "src.order.OrderService.cancel"),
		ev("s1", "", "p2", "u5", "Grep", false, map[string]any{"pattern": "cancel"}),
	)
	db, code := rep.Targets["db"], rep.Targets["code"]
	if db.Runs != 1 || db.Adoptions != 1 || db.Fallbacks != 0 {
		t.Fatalf("db = %+v", db)
	}
	if code.Runs != 1 || code.Adoptions != 0 || code.Fallbacks != 1 {
		t.Fatalf("code = %+v", code)
	}
}

// A run that surfaced schema nodes and was then abandoned for a grep is the
// db-side fallback this split exists to find — it must be counted even though
// no db node was ever explored.
func TestTargetsSearchResultsAloneClassifyTheRun(t *testing.T) {
	rep := compute(
		gsearch("s1", "p1", "u1", "which table stores application rows", "db.main.public.application"),
		ev("s1", "", "p1", "u2", "Grep", false, map[string]any{"pattern": "application"}),
	)
	if db := rep.Targets["db"]; db.Runs != 1 || db.Fallbacks != 1 || db.SameIntentFallbacks != 1 {
		t.Fatalf("db = %+v", db)
	}
	if code := rep.Targets["code"]; code.Runs != 0 {
		t.Fatalf("code should be empty, got %+v", code)
	}
}

// code and db are overlapping populations, not a partition: a run touching
// both counts in both, so the two rows can sum past the group's run count.
func TestTargetsMixedRunCountsInBoth(t *testing.T) {
	rep := compute(
		gsearch("s1", "p1", "u1", "who writes job_posting",
			"db.main.public.job_posting", "src.job.JobWriter.save"),
		ev("s1", "", "p1", "u2", "Read", false, map[string]any{"file_path": "a.go"}),
	)
	if db := rep.Targets["db"]; db.Runs != 1 || db.Adoptions != 1 {
		t.Fatalf("db = %+v", db)
	}
	if code := rep.Targets["code"]; code.Runs != 1 || code.Adoptions != 1 {
		t.Fatalf("code = %+v", code)
	}
	if g := rep.Groups["all"]; g.Adoptions != 1 {
		t.Fatalf("the group still sees one run: %+v", g)
	}
}

// A search that returned nothing references no node id, so it belongs to
// neither population. The group still judges it.
func TestTargetsRunWithoutNodeIDsCountsNowhere(t *testing.T) {
	rep := compute(
		gsearch("s1", "p1", "u1", "nothing matches this"),
		ev("s1", "", "p1", "u2", "Grep", false, map[string]any{"pattern": "x"}),
	)
	if rep.Targets["code"].Runs != 0 || rep.Targets["db"].Runs != 0 {
		t.Fatalf("targets = %+v", rep.Targets)
	}
	if g := rep.Groups["all"]; g.Fallbacks != 1 {
		t.Fatalf("group = %+v", g)
	}
}

// outMerged is judged by the following run; counting it here would push the
// target denominators past the group's.
func TestTargetsMergedRunCountedOnce(t *testing.T) {
	rep := compute(
		gsearch("s1", "p1", "u1", "q", "db.main.public.t"),
		ev("s1", "", "p1", "u2", "Bash", false, map[string]any{}),
		gsearch("s1", "p1", "u3", "q2", "db.main.public.t"),
		ev("s1", "", "p1", "u4", "Read", false, nil),
	)
	if db := rep.Targets["db"]; db.Runs != 1 || db.Adoptions != 1 {
		t.Fatalf("db = %+v", db)
	}
}
