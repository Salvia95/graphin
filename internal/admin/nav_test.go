package admin

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// US-1: 미부트스트랩 상태에서 데이터 화면 어디를 열어도 동일한 안내가 보인다.
func TestEmptyStateBeforeBootstrap(t *testing.T) {
	s := newTestServer(t, newTestWS(t, nil))
	for _, p := range []string{"/browse", "/browse?pkg=x", "/search", "/diagnostics", "/node?id=x"} {
		wantContains(t, get(t, s, p), http.StatusOK, "bootstrap_workspace", "아직 인덱스가 없습니다")
	}
}

// US-3: 어느 화면에나 전역 검색 폼이 있다.
func TestGlobalSearchFormInShell(t *testing.T) {
	s := newTestServer(t, newTestWS(t, nil))
	wantContains(t, get(t, s, "/"), http.StatusOK, `role="search"`, `action="/search"`)
}

// US-2: 문제가 있을 때만 대시보드 경고 스트립이 뜬다.
func TestDashboardWarningStrip(t *testing.T) {
	// 정상 그래프: 스트립 없음 + "문제 없음" 요약.
	s, _ := bootstrappedServer(t)
	rec := get(t, s, "/")
	if strings.Contains(rec.Body.String(), "확인이 필요합니다") {
		t.Fatal("healthy graph must not show the warning strip")
	}
	wantContains(t, rec, http.StatusOK, "문제 없음")

	// 구문 오류 파일 → 부분 인덱싱 노드 → 스트립 표시.
	ws := newTestWS(t, map[string]string{
		"Ok.java":  "class Ok { void fine() {} }",
		"Bad.java": "class Bad { void broken( { }",
	})
	bootstrapWS(t, ws)
	s2 := newTestServer(t, ws)
	body := get(t, s2, "/").Body.String()
	if !strings.Contains(body, "확인이 필요합니다") || !strings.Contains(body, "부분 인덱싱") {
		t.Fatalf("warning strip missing for partial nodes; body: %.400s", body)
	}
}

// US-3: 노드 페이지 breadcrumb이 구조/패키지/파일로 이어진다.
func TestNodeBreadcrumb(t *testing.T) {
	s, ws := bootstrappedServer(t)
	runID := findNode(t, ws, "Flow.run()")
	rec := get(t, s, "/node?id="+url.QueryEscape(runID))
	wantContains(t, rec, http.StatusOK,
		`aria-label="breadcrumb"`, "/browse?pkg=", "#f-Flow-java", "구조")
	if !strings.Contains(rec.Body.String(), "Flow.java") {
		t.Fatal("breadcrumb missing file segment")
	}
}
