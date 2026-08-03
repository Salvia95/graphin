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
