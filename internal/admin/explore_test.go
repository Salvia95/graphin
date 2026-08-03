package admin

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/Salvia95/graphin/internal/graph"
	"github.com/Salvia95/graphin/internal/workspace"
)

// bootstrappedServer spins up a two-class fixture: Flow.run → Svc.help 호출.
func bootstrappedServer(t *testing.T) (*Server, *workspace.Workspace) {
	t.Helper()
	ws := newTestWS(t, map[string]string{
		"Svc.java":  "class Svc { void help() {} }",
		"Flow.java": "class Flow { void run() { Svc s; s.help(); } }",
	})
	bootstrapWS(t, ws)
	return newTestServer(t, ws), ws
}

func findNode(t *testing.T, ws *workspace.Workspace, simple string) string {
	t.Helper()
	id := ""
	ws.GraphForEach(func(n graph.NodeInfo) bool {
		if strings.HasSuffix(n.ID, simple) {
			id = n.ID
			return false
		}
		return true
	})
	if id == "" {
		t.Fatalf("node %q not found in graph", simple)
	}
	return id
}

func TestSearchPageAndPartial(t *testing.T) {
	s, _ := bootstrappedServer(t)
	wantContains(t, get(t, s, "/search"), http.StatusOK, "검색")
	rec := get(t, s, "/partial/search?q=help")
	wantContains(t, rec, http.StatusOK, "/node?id=", "help")
	// 빈 질의는 안내문만.
	wantContains(t, get(t, s, "/partial/search?q="), http.StatusOK, "질의를 입력")
}

func TestNodeDetailShowsEdgesAndCode(t *testing.T) {
	s, ws := bootstrappedServer(t)
	runID := findNode(t, ws, "Flow.run()")

	rec := get(t, s, "/node?id="+url.QueryEscape(runID))
	wantContains(t, rec, http.StatusOK,
		"uses", "used_by", "help", // 호출 엣지
		"Flow.java", "void run()", // 코드 블록(메서드 슬라이스)
		"min_conf")
	if !strings.Contains(rec.Body.String(), "confbadge") {
		t.Fatal("confidence badge missing")
	}
}

func TestNodeNotFound(t *testing.T) {
	s, _ := bootstrappedServer(t)
	wantContains(t, get(t, s, "/node?id=no.such.Node"), http.StatusOK, "찾을 수 없습니다")
}

func TestEdgesPartialMinConfFilter(t *testing.T) {
	s, ws := bootstrappedServer(t)
	runID := findNode(t, ws, "Flow.run()")
	esc := url.QueryEscape(runID)

	// 같은 파일이 아니므로 call 엣지는 0.95(동일 패키지 티어).
	rec := get(t, s, "/partial/edges?id="+esc+"&dir=uses&min_conf=0.85")
	wantContains(t, rec, http.StatusOK, "help")
	// 1.0 초과 필터에서는 사라진다.
	rec = get(t, s, "/partial/edges?id="+esc+"&dir=uses&min_conf=1.00")
	wantContains(t, rec, http.StatusOK, "없음")
}

func TestCodePartialUnknownNode(t *testing.T) {
	s, _ := bootstrappedServer(t)
	wantContains(t, get(t, s, "/partial/code?id=nope"), http.StatusOK, "메타데이터가 없습니다")
}
