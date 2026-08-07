package graph

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/Salvia95/graphin/internal/nodeid"
	"github.com/Salvia95/graphin/internal/parse"
)

func inspectFixture(t *testing.T) (*Engine, string, string) {
	t.Helper()
	pkg := "com.a"
	callee := fileRes(parse.LangJava, "a/Svc.java", pkg, nil,
		methodNode(pkg, "Svc", "charge", []string{"long"}, 1, 1))
	caller := fileRes(parse.LangJava, "a/Flow.java", pkg, nil,
		methodNode(pkg, "Flow", "run", nil, 0, 0,
			parse.Call{Name: "charge", Args: 1, Recv: "svc"}))
	other := fileRes(parse.LangJava, "b/Thing.java", "com.b", nil,
		classNode("com.b", "Thing", nodeid.KindClass))

	e := newEngine(t)
	applyAll(e, callee, caller, other)
	chargeID := nodeid.Method(pkg, "Svc", "charge", []string{"long"})
	runID := nodeid.Method(pkg, "Flow", "run", nil)
	return e, runID, chargeID
}

func TestInspectStatsForEachInfo(t *testing.T) {
	e, runID, chargeID := inspectFixture(t)

	st := e.Stats()
	if st.Nodes != 3 {
		t.Fatalf("nodes = %d, want 3", st.Nodes)
	}
	if len(st.Shards) != 2 || st.Shards[0].Pkg != "com.a" || st.Shards[1].Pkg != "com.b" {
		t.Fatalf("shards not deterministic: %+v", st.Shards)
	}

	// ForEachNode must agree with Stats and visit deterministically.
	var ids []string
	edges := 0
	e.ForEachNode(func(n NodeInfo) bool {
		ids = append(ids, n.ID)
		edges += len(n.Uses)
		return true
	})
	if len(ids) != st.Nodes || edges != st.Edges {
		t.Fatalf("ForEachNode saw %d nodes / %d edges, Stats %d / %d",
			len(ids), edges, st.Nodes, st.Edges)
	}

	// Early stop.
	seen := 0
	e.ForEachNode(func(NodeInfo) bool { seen++; return false })
	if seen != 1 {
		t.Fatalf("early stop visited %d", seen)
	}

	info, ok := e.Info(runID)
	if !ok {
		t.Fatalf("Info miss for %s", runID)
	}
	if info.FilePath != "a/Flow.java" || info.Pkg != "com.a" || info.Kind != nodeid.KindMethod {
		t.Fatalf("Info fields: %+v", info)
	}
	found := false
	for _, u := range info.Uses {
		if u.TargetID == chargeID && u.Confidence == confSamePkg {
			found = true
		}
	}
	if !found {
		t.Fatalf("Info.Uses missing call edge: %+v", info.Uses)
	}
	if _, ok := e.Info("no.such.Node"); ok {
		t.Fatal("Info must miss unknown IDs")
	}
}

func TestInspectDanglingAfterRemoval(t *testing.T) {
	e, runID, chargeID := inspectFixture(t)

	if _, totals := e.DanglingEdges(0); totals.Sum() != 0 {
		t.Fatalf("fresh graph dangling = %+v, want 0", totals)
	}

	// Removing the callee leaves the caller's persisted edge pointing at a
	// node that is no longer on the read path.
	e.RemoveNodes([]string{chargeID})
	e.Flush()

	out, totals := e.DanglingEdges(10)
	if totals.Code != 1 || totals.DB != 0 || len(out) != 1 {
		t.Fatalf("dangling = %+v (%d rows), want code 1", totals, len(out))
	}
	d := out[0]
	if d.SourceID != runID || d.Edge.TargetID != chargeID || d.DBDomain {
		t.Fatalf("dangling row: %+v", d)
	}

	// max=0 counts without collecting rows.
	out, totals = e.DanglingEdges(0)
	if totals.Sum() != 1 || out != nil {
		t.Fatalf("count-only scan: rows=%v totals=%+v", out, totals)
	}
}

func TestInspectDanglingDBDomain(t *testing.T) {
	e := newEngine(t)
	applyAll(e, dbFixture(t, "main.graphindb.json"))

	out, totals := e.DanglingEdges(100)
	if totals.DB == 0 {
		t.Fatal("db snapshot must contain the auth.users dangling FK")
	}
	found := false
	for _, d := range out {
		if d.Edge.TargetID == "db.main.auth.users" {
			if !d.DBDomain {
				t.Fatalf("auth.users must be DB-domain: %+v", d)
			}
			found = true
		}
	}
	if !found {
		t.Fatalf("auth.users not in dangling rows: %+v", out)
	}
}

// Sized on purpose, and not with inspectFixture.
//
// Flush → Sync kicks compaction ASYNCHRONOUSLY when tombstones/records rises
// above compactTombstoneFrac, and compaction folds tombstones into a fresh
// base — erasing the very counter this test reads. With a single call edge the
// ratio after one removal is 1/2, so the trigger always fired and the
// assertion only passed when it won the race against the goroutine (observed
// failing roughly 1 run in 35).
//
// Keeping the ratio under the trigger makes the assertion deterministic
// instead. The guard below fails loudly if the constant ever moves, rather
// than letting the test drift back into flaking.
func TestInspectReverseStats(t *testing.T) {
	const callers = 4 // 1 tombstone / (callers+1) records
	if 1.0/float64(callers+1) > compactTombstoneFrac {
		t.Fatalf("fixture too small: 1/%d exceeds compactTombstoneFrac=%v",
			callers+1, compactTombstoneFrac)
	}

	pkg := "com.a"
	files := []*parse.FileResult{
		fileRes(parse.LangJava, "a/Svc.java", pkg, nil,
			methodNode(pkg, "Svc", "charge", []string{"long"}, 1, 1)),
	}
	var runID string
	for i := range callers {
		name := fmt.Sprintf("run%d", i)
		files = append(files, fileRes(parse.LangJava, fmt.Sprintf("a/Flow%d.java", i), pkg, nil,
			methodNode(pkg, "Flow"+strconv.Itoa(i), name, nil, 0, 0,
				parse.Call{Name: "charge", Args: 1, Recv: "svc"})))
		if i == 0 {
			runID = nodeid.Method(pkg, "Flow"+strconv.Itoa(i), name, nil)
		}
	}
	e := newEngine(t)
	applyAll(e, files...)

	st := e.ReverseStats()
	if st.Targets == 0 || st.Edges == 0 || st.LogRecords == 0 {
		t.Fatalf("fresh reverse stats: %+v", st)
	}
	if st.LogTombstones != 0 {
		t.Fatalf("fresh log has %d tombstones", st.LogTombstones)
	}

	// Removing the caller tombstones its outgoing edge.
	e.RemoveNodes([]string{runID})
	e.Flush()
	if st = e.ReverseStats(); st.LogTombstones == 0 {
		t.Fatalf("removal must tombstone: %+v", st)
	}
}
