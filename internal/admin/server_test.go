package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Salvia95/graphin/internal/obs"
	"github.com/Salvia95/graphin/internal/workspace"
)

func newTestWS(t *testing.T, files map[string]string) *workspace.Workspace {
	t.Helper()
	root := t.TempDir()
	for name, src := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	ws := workspace.New(workspace.Config{
		Root: root, Log: obs.Nop(), OrtLib: "/nonexistent-ort", Workers: 1,
	})
	t.Cleanup(ws.Close)
	return ws
}

func bootstrapWS(t *testing.T, ws *workspace.Workspace) {
	t.Helper()
	if _, err := ws.Bootstrap(context.Background(), "", false); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if ws.Status().LexicalReady && ws.GraphStats().Nodes > 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("workspace never became lexical-ready with a populated graph")
}

func newTestServer(t *testing.T, ws *workspace.Workspace) *Server {
	t.Helper()
	s, err := NewServer(ws, "test", obs.Nop())
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// get issues a loopback-Host request and returns the response.
func get(t *testing.T, s *Server, target string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	req.Host = "127.0.0.1:7466"
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	return rec
}

func wantContains(t *testing.T, rec *httptest.ResponseRecorder, wantCode int, substrs ...string) {
	t.Helper()
	if rec.Code != wantCode {
		t.Fatalf("status = %d, want %d; body: %.300s", rec.Code, wantCode, rec.Body.String())
	}
	for _, sub := range substrs {
		if !strings.Contains(rec.Body.String(), sub) {
			t.Fatalf("body missing %q; body: %.500s", sub, rec.Body.String())
		}
	}
}

const javaFixture = "class A { void run() { help(); } void help() {} }"

func TestHostGuardRejectsNonLoopback(t *testing.T) {
	s := newTestServer(t, newTestWS(t, nil))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "evil.example.com"
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("non-loopback Host got %d, want 403", rec.Code)
	}
	for _, ok := range []string{"127.0.0.1:7466", "localhost:80", "[::1]:9999"} {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Host = ok
		rec := httptest.NewRecorder()
		s.ServeHTTP(rec, req)
		if rec.Code == http.StatusForbidden {
			t.Fatalf("loopback Host %q rejected", ok)
		}
	}
}

func TestDashboardBeforeBootstrap(t *testing.T) {
	s := newTestServer(t, newTestWS(t, nil))
	wantContains(t, get(t, s, "/"), http.StatusOK, "bootstrap_workspace", "대시보드")
}

func TestDashboardAfterBootstrap(t *testing.T) {
	ws := newTestWS(t, map[string]string{"A.java": javaFixture})
	bootstrapWS(t, ws)
	s := newTestServer(t, ws)
	wantContains(t, get(t, s, "/"), http.StatusOK, "그래프", "샤드 상위")
}

func TestStatusPartialStopsPollingWhenTerminal(t *testing.T) {
	ws := newTestWS(t, map[string]string{"A.java": javaFixture})
	bootstrapWS(t, ws)
	s := newTestServer(t, ws)

	// The nonexistent ORT lib fails the async semantic warmup; once that
	// failure lands the poller must receive htmx's 286 stop code.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		rec := get(t, s, "/partial/status")
		if rec.Code == 286 {
			wantContains(t, rec, 286, "lexical")
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("status partial never returned 286")
}

func TestStaticAssets(t *testing.T) {
	s := newTestServer(t, newTestWS(t, nil))
	wantContains(t, get(t, s, "/static/htmx.min.js"), http.StatusOK, "htmx")
	wantContains(t, get(t, s, "/static/style.css"), http.StatusOK, "graphin admin")
}

func TestServeRefusesBadAddresses(t *testing.T) {
	ws := newTestWS(t, nil)
	for _, addr := range []string{"0.0.0.0:0", "192.168.0.10:8080", "noport", ""} {
		if err := Serve(context.Background(), ws, addr, "test", obs.Nop()); err == nil {
			t.Fatalf("Serve(%q) must refuse", addr)
		}
	}
}

func TestServeShutsDownOnCancel(t *testing.T) {
	ws := newTestWS(t, nil)
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- Serve(ctx, ws, "127.0.0.1:0", "test", obs.Nop()) }()
	time.Sleep(150 * time.Millisecond)
	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("graceful shutdown returned %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Serve did not return after cancel")
	}
}
