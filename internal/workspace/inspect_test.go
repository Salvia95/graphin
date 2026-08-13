package workspace

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Salvia95/graphin/internal/graph"
	"github.com/Salvia95/graphin/internal/obs"
)

// 부트스트랩 전에는 모든 그래프 프록시가 zero 값으로 응답해야 한다 —
// diagnose_index는 부트스트랩 전에도 답하며, 그때가 바로 진단이 필요한
// 상황이다.
func TestInspectFacadeBeforeBootstrap(t *testing.T) {
	ws := New(Config{Root: t.TempDir(), Log: obs.Nop(), OrtLib: "/nonexistent-ort"})
	defer ws.Close()

	if st := ws.GraphStats(); st.Nodes != 0 || len(st.Shards) != 0 {
		t.Fatalf("stats before bootstrap: %+v", st)
	}
	if rows, totals := ws.GraphDangling(10); rows != nil || totals.Sum() != 0 {
		t.Fatalf("dangling before bootstrap: %v %+v", rows, totals)
	}
	visited := false
	ws.GraphForEach(func(graph.NodeInfo) bool { visited = true; return true })
	if visited {
		t.Fatal("GraphForEach must be a no-op before bootstrap")
	}
	if h := ws.SemanticHeader(); h != nil {
		t.Fatalf("semantic header before bootstrap: %+v", h)
	}
	if _, _, drained := ws.SemanticQueue(); !drained {
		t.Fatal("absent semantic must report drained")
	}
	if _, _, gated := ws.GateInfo(); gated {
		t.Fatal("no gate marker expected")
	}
}

func TestInspectFacadeAfterBootstrap(t *testing.T) {
	root := t.TempDir()
	src := "class A { void run() { help(); } void help() {} }"
	if err := os.WriteFile(filepath.Join(root, "A.java"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	ws := New(Config{Root: root, Log: obs.Nop(), OrtLib: "/nonexistent-ort", Workers: 1})
	defer ws.Close()
	if _, err := ws.Bootstrap(context.Background(), "", false); err != nil {
		t.Fatal(err)
	}
	waitLexical(t, ws)

	// 초기 스캔의 Flush가 샤드를 내려놓을 때까지 통계를 폴링한다.
	deadline := time.Now().Add(3 * time.Second)
	var nodes int
	for time.Now().Before(deadline) {
		if nodes = ws.GraphStats().Nodes; nodes > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if nodes == 0 {
		t.Fatal("graph stats never populated")
	}

	found := ""
	ws.GraphForEach(func(n graph.NodeInfo) bool {
		if n.Kind == "method" && n.FilePath == "A.java" {
			found = n.ID
			return false
		}
		return true
	})
	if found == "" {
		t.Fatal("no method node visible via GraphForEach")
	}
	if cfg := ws.EffectiveConfig(); cfg.Root != root || cfg.Workers != 1 {
		t.Fatalf("effective config: %+v", cfg)
	}
}
