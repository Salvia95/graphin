package e2e

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Salvia95/graphin/internal/admin"
	"github.com/Salvia95/graphin/internal/obs"
	"github.com/Salvia95/graphin/internal/workspace"
)

// TestAdminPageOverHTTP: java 픽스처를 부트스트랩한 라이브 워크스페이스를
// 실제 TCP(httptest, 127.0.0.1:0)로 서빙해 admin 페이지의 주요 화면을
// 블랙박스로 검증한다.
func TestAdminPageOverHTTP(t *testing.T) {
	root := t.TempDir()
	copyTree(t, javaFixtures, root)

	ws := workspace.New(workspace.Config{Root: root, Log: obs.Nop(), OrtLib: "/nonexistent-ort"})
	t.Cleanup(ws.Close)
	if _, err := ws.Bootstrap(context.Background(), "", false); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if ws.Status().LexicalReady && ws.GraphStats().Nodes > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	srv, err := admin.NewServer(ws, "e2e", obs.Nop())
	if err != nil {
		t.Fatal(err)
	}
	hs := httptest.NewServer(srv)
	t.Cleanup(hs.Close)

	get := func(path string, wants ...string) {
		t.Helper()
		resp, err := http.Get(hs.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET %s = %d; body: %.300s", path, resp.StatusCode, b)
		}
		for _, want := range wants {
			if !strings.Contains(string(b), want) {
				t.Fatalf("GET %s missing %q; body: %.500s", path, want, b)
			}
		}
	}

	get("/", "대시보드", "그래프")
	get("/search", "검색")
	get("/partial/search?q=cancelPayment", "cancelPayment", "/node?id=")
	get("/node?id="+url.QueryEscape(cancelID),
		"cancelPayment", "<svg", "uses", "used_by", "OrderService.java")
	get("/diagnostics?tab=dangling", "끊어진 엣지")
	get("/diagnostics?tab=semantic", "임베딩 대기")
	get("/settings", "읽기 전용", "--model-type")
}
