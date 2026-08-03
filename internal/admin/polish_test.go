package admin

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestSearchResultsShowFilePath(t *testing.T) {
	s, _ := bootstrappedServer(t)
	wantContains(t, get(t, s, "/partial/search?q=help"), http.StatusOK, "Svc.java")
}

func TestExplorePartialReplacesURL(t *testing.T) {
	s, ws := bootstrappedServer(t)
	runID := findNode(t, ws, "Flow.run()")

	req := httptest.NewRequest(http.MethodGet,
		"/partial/explore?id="+url.QueryEscape(runID)+"&min_conf=0.75", nil)
	req.Host = "127.0.0.1:7466"
	req.Header.Set("HX-Request", "true")
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	got := rec.Header().Get("HX-Replace-Url")
	if !strings.HasPrefix(got, "/node?id=") || !strings.Contains(got, "min_conf=0.75") {
		t.Fatalf("HX-Replace-Url = %q", got)
	}

	// htmx 요청이 아니면(직접 URL 접근) 치환하지 않는다.
	rec2 := get(t, s, "/partial/explore?id="+url.QueryEscape(runID))
	if rec2.Header().Get("HX-Replace-Url") != "" {
		t.Fatal("non-htmx request must not set HX-Replace-Url")
	}
}

func TestDashboardKindDistribution(t *testing.T) {
	s, _ := bootstrappedServer(t)
	wantContains(t, get(t, s, "/"), http.StatusOK, "구성 (kind)", "method")
}

func TestStrictCSPAndNoInlineStyles(t *testing.T) {
	s, ws := bootstrappedServer(t)

	rec := get(t, s, "/")
	if csp := rec.Header().Get("Content-Security-Policy"); csp != "default-src 'self'" {
		t.Fatalf("CSP = %q", csp)
	}
	// 진행 바는 <progress>, ego SVG는 표현 속성 — 인라인 style이 없어야 CSP가 성립.
	// (상태 파셜은 종료 상태에서 286을 반환하므로 코드는 단언하지 않는다.)
	if body := get(t, s, "/partial/status").Body.String(); !strings.Contains(body, "<progress") {
		t.Fatalf("progress element missing: %.300s", body)
	}
	runID := findNode(t, ws, "Flow.run()")
	body := get(t, s, "/node?id="+url.QueryEscape(runID)).Body.String()
	if strings.Contains(body, "style=") {
		t.Fatal("inline style attribute survived")
	}
	if !strings.Contains(body, `stroke-width=`) || !strings.Contains(body, `opacity=`) {
		t.Fatal("SVG presentational attributes missing")
	}
}
