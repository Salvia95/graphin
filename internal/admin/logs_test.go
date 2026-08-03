package admin

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeLog(t *testing.T, dir string, lines ...string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(filepath.Join(dir, "agent-nav.log"),
		os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	for _, l := range lines {
		if _, err := f.WriteString(l + "\n"); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLogsPageMissingFile(t *testing.T) {
	s := newTestServer(t, newTestWS(t, nil))
	wantContains(t, get(t, s, "/logs"), http.StatusOK, "아직 없습니다")
}

func TestLogsTailFilterAndErrorHighlight(t *testing.T) {
	ws := newTestWS(t, nil)
	writeLog(t, ws.Dir,
		`{"ts":"2026-08-03T10:00:00Z","event":"watch_batch","files":3}`,
		`{"ts":"2026-08-03T10:00:01Z","event":"embed_error","error":"boom"}`,
		`{"ts":"2026-08-03T10:00:02Z","event":"vectors_export","bytes":1234}`,
		`이건 JSON이 아님 — 건너뛰어야 함`,
	)
	s := newTestServer(t, ws)

	rec := get(t, s, "/logs")
	wantContains(t, rec, http.StatusOK, "watch_batch", "embed_error", "vectors_export", "logerr")
	// 최신 우선: vectors_export가 watch_batch보다 먼저 나와야 한다.
	body := rec.Body.String()
	if strings.Index(body, "vectors_export") > strings.LastIndex(body, "watch_batch") {
		t.Fatal("rows not newest-first")
	}

	// 이벤트 필터.
	rec = get(t, s, "/partial/logs?event=watch_batch")
	wantContains(t, rec, http.StatusOK, "watch_batch", "files=3")
	if strings.Contains(rec.Body.String(), "vectors_export") {
		t.Fatal("filter leaked other events")
	}
}

func TestLogsTailTruncatesPartialFirstLine(t *testing.T) {
	ws := newTestWS(t, nil)
	// tail 윈도(256KB)보다 큰 로그: 앞부분이 잘려도 파싱이 무너지지 않아야 한다.
	pad := strings.Repeat("x", 1000)
	lines := make([]string, 0, 400)
	for i := 0; i < 400; i++ {
		lines = append(lines, `{"ts":"2026-08-03T10:00:00Z","event":"pad","p":"`+pad+`"}`)
	}
	lines = append(lines, `{"ts":"2026-08-03T10:07:00Z","event":"tail_marker"}`)
	writeLog(t, ws.Dir, lines...)
	s := newTestServer(t, ws)
	wantContains(t, get(t, s, "/logs"), http.StatusOK, "tail_marker")
}
