package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const dbFixtures = "../testdata/fixtures/dbschema"

// TestDBSchemaRoundtrip: graphindb 스냅샷이 커밋된 워크스페이스의 전체 여정 —
// bootstrap 속성 → search_hybrid → explore_graph(FK/RLS/크로스 DS) → read_code.
func TestDBSchemaRoundtrip(t *testing.T) {
	root := t.TempDir()
	copyTree(t, dbFixtures, root)
	c := newClient(t, root)

	text, isErr := c.tool("bootstrap_workspace", map[string]any{})
	if isErr {
		t.Fatalf("bootstrap: %s", text)
	}
	if !strings.Contains(text, `db_snapshots="5"`) {
		t.Fatalf("bootstrap must count snapshots: %s", text)
	}
	if strings.Contains(text, "<hint>") {
		t.Fatalf("hint must not fire when snapshots exist: %s", text)
	}
	c.bootstrapAndWait(root)

	// Tier-0: 테이블명 검색이 테이블 노드를 찾는다
	text, isErr = c.tool("search_hybrid", map[string]any{"query": "job_posting"})
	if isErr || !strings.Contains(text, "db.main.public.job_posting") {
		t.Fatalf("search: %s", text)
	}

	// FK·RLS·트리거·크로스 데이터소스 참조가 한 번의 explore로 나온다
	text, isErr = c.tool("explore_graph", map[string]any{
		"node_id": "db.main.public.job_posting", "direction": "both"})
	if isErr {
		t.Fatalf("explore: %s", text)
	}
	for _, want := range []string{
		`id="db.main.public.company" type="foreign_key"`,
		`id="db.main.public.job_posting.rls" type="reference"`,
		`id="db.main.public.job_posting.trg_job_posting_updated_at"`,
		`id="db.warehouse.main.fact_job_daily" type="foreign_key"`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("explore missing %s:\n%s", want, text)
		}
	}

	// read_code는 스냅샷 JSON에서 해당 테이블 블록만 슬라이싱한다
	text, isErr = c.tool("read_code", map[string]any{"node_id": "db.main.public.job_posting"})
	if isErr || !strings.Contains(text, "채용 공고") || strings.Contains(text, "company_group") {
		t.Fatalf("read_code slice: %s", text)
	}
}

// TestDBBootstrapHint: DB 흔적(마이그레이션)만 있고 스냅샷이 없으면
// bootstrap 응답이 생성 절차 힌트를 동봉한다.
func TestDBBootstrapHint(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "supabase", "migrations")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	sql := "CREATE TABLE job_posting (id bigint PRIMARY KEY);\n"
	if err := os.WriteFile(filepath.Join(dir, "20260101000000_init.sql"), []byte(sql), 0o644); err != nil {
		t.Fatal(err)
	}
	c := newClient(t, root)
	text, isErr := c.tool("bootstrap_workspace", map[string]any{})
	if isErr {
		t.Fatalf("bootstrap: %s", text)
	}
	for _, want := range []string{
		`db_sources_detected="supabase/migrations"`, `db_snapshots="0"`, "<hint>", "dbimport",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("bootstrap hint missing %s:\n%s", want, text)
		}
	}
}
