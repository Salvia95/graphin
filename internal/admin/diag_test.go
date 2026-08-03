package admin

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

// dbWS bootstraps a workspace containing the dbschema fixture, whose
// auth.users FK dangles by design.
func dbWS(t *testing.T) *Server {
	t.Helper()
	snap, err := os.ReadFile(filepath.Join("..", "..", "testdata", "fixtures", "dbschema", "db", "main.graphindb.json"))
	if err != nil {
		t.Fatal(err)
	}
	ws := newTestWS(t, map[string]string{"Svc.java": "class Svc { void help() {} }"})
	if err := os.MkdirAll(filepath.Join(ws.Root, "db"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws.Root, "db", "main.graphindb.json"), snap, 0o644); err != nil {
		t.Fatal(err)
	}
	bootstrapWS(t, ws)
	return newTestServer(t, ws)
}

func TestDiagnosticsDanglingTab(t *testing.T) {
	s := dbWS(t)
	rec := get(t, s, "/diagnostics?tab=dangling")
	wantContains(t, rec, http.StatusOK, "끊어진 엣지", "auth.users", "foreign_key")

	// 코드 필터에서는 DB dangling이 사라진다.
	rec = get(t, s, "/diagnostics?tab=dangling&filter=code")
	wantContains(t, rec, http.StatusOK, "해당 없음")
}

func TestDiagnosticsOtherTabs(t *testing.T) {
	s := dbWS(t)
	wantContains(t, get(t, s, "/diagnostics?tab=partial"), http.StatusOK, "부분 인덱싱")
	wantContains(t, get(t, s, "/diagnostics?tab=semantic"), http.StatusOK, "임베딩 대기")
	wantContains(t, get(t, s, "/diagnostics?tab=reverse"), http.StatusOK, "대상 수")
}

func TestSettingsReadOnlyView(t *testing.T) {
	s := dbWS(t)
	rec := get(t, s, "/settings")
	wantContains(t, rec, http.StatusOK,
		"읽기 전용", "--model-type", "--semantic-max-nodes", "저장소", "graph/")
}
