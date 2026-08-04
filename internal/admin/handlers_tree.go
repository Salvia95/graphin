package admin

import (
	"net/http"
	"net/url"
	"sort"

	"github.com/Salvia95/graphin/internal/graph"
	"github.com/Salvia95/graphin/internal/nodeid"
)

// 구조 트리(DESIGN.md §4.4·§7): 패키지 → 파일 → 노드를 깊이 3으로 펼친다.
// 자식은 htmx가 필요할 때만 가져오므로 첫 응답에는 패키지 줄만 들어간다(P4).

// treeRow is one rendered line. Kind/Metric/State fill the three fixed
// right-hand columns; Children marks a row that can be expanded.
type treeRow struct {
	Depth int
	Label string
	Kind  string
	// Metric은 깊이마다 세는 대상이 다르다 — 패키지·파일은 품고 있는 노드 수,
	// 노드는 나가는 참조 수. 헤더 한 칸으로는 둘 다 정확히 못 부르므로 각 칸이
	// MetricTitle로 무엇을 센 값인지 밝힌다.
	Metric      int
	MetricTitle string
	Partial     bool
	Children    bool
	ChildURL    string // 자식 조회 URL (Children일 때만)
	LinkURL     string // 라벨이 가리키는 곳 — 패키지/파일은 표 뷰(§7 보조 A), 노드는 상세
	Title       string // 툴팁 — 전체 ID/경로
}

// treePackages lists shards as depth-0 rows.
func (s *Server) treePackages() []treeRow {
	st := s.cachedStats()
	rows := make([]treeRow, 0, len(st.Shards))
	for _, sh := range st.Shards {
		rows = append(rows, treeRow{
			Depth: 0, Label: sh.Pkg, Kind: "패키지", Metric: sh.Nodes,
			MetricTitle: "노드 " + comma(sh.Nodes) + "개",
			Children:    sh.Nodes > 0, LinkURL: "/browse?pkg=" + url.QueryEscape(sh.Pkg),
			ChildURL: "/partial/tree?pkg=" + url.QueryEscape(sh.Pkg),
			Title:    sh.Pkg,
		})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Label < rows[j].Label })
	return rows
}

// treeFiles lists one package's files as depth-1 rows.
func (s *Server) treeFiles(pkg string) []treeRow {
	counts := map[string]int{}
	partial := map[string]bool{}
	s.ws.GraphForEach(func(n graph.NodeInfo) bool {
		if n.Pkg != pkg {
			return true
		}
		counts[n.FilePath]++
		if n.Partial {
			partial[n.FilePath] = true
		}
		return true
	})
	rows := make([]treeRow, 0, len(counts))
	for path, n := range counts {
		rows = append(rows, treeRow{
			Depth: 1, Label: path, Kind: "파일", Metric: n, Partial: partial[path],
			MetricTitle: "노드 " + comma(n) + "개",
			Children:    n > 0, LinkURL: "/browse?pkg=" + url.QueryEscape(pkg) + "#" + fileAnchor(path),
			ChildURL: "/partial/tree?pkg=" + url.QueryEscape(pkg) + "&file=" + url.QueryEscape(path),
			Title:    path,
		})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Label < rows[j].Label })
	return rows
}

// treeNodes lists one file's nodes as depth-2 leaves, in source order.
func (s *Server) treeNodes(pkg, file string) []treeRow {
	type entry struct {
		row   treeRow
		start uint32
	}
	var found []entry
	s.ws.GraphForEach(func(n graph.NodeInfo) bool {
		if n.Pkg != pkg || n.FilePath != file {
			return true
		}
		label := n.DisplayName
		if label == "" {
			label = nodeid.Simple(n.ID)
		}
		found = append(found, entry{
			row: treeRow{
				Depth: 2, Label: label, Kind: kindLabel(n.Kind), Metric: len(n.Uses),
				MetricTitle: "참조 " + comma(len(n.Uses)) + "개",
				Partial:     n.Partial, LinkURL: "/node?id=" + url.QueryEscape(n.ID), Title: n.ID,
			},
			start: n.Start,
		})
		return true
	})
	sort.Slice(found, func(i, j int) bool {
		if found[i].start != found[j].start {
			return found[i].start < found[j].start
		}
		return found[i].row.Label < found[j].row.Label
	})
	rows := make([]treeRow, len(found))
	for i, e := range found {
		rows[i] = e.row
	}
	return rows
}

// handleTreePartial answers one expand click with the child <li> fragments.
func (s *Server) handleTreePartial(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	pkg, file := q.Get("pkg"), q.Get("file")
	if pkg == "" {
		http.Error(w, "pkg 파라미터가 필요합니다", http.StatusBadRequest)
		return
	}
	rows := s.treeFiles(pkg)
	if file != "" {
		rows = s.treeNodes(pkg, file)
	}
	s.renderPartial(w, "tree_rows.html", http.StatusOK, rows)
}
