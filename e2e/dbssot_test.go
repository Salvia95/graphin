package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const dbssotFixtures = "../testdata/fixtures/dbssot"

// TestDBSSOTRoundtrip: 매니페스트가 SSOT(schema.sql/prisma/tbls/커스텀 JSON)를
// 라우팅하는 워크스페이스의 전체 여정.
func TestDBSSOTRoundtrip(t *testing.T) {
	root := t.TempDir()
	copyTree(t, dbssotFixtures, root)
	c := newClient(t, root)

	text, isErr := c.tool("bootstrap_workspace", map[string]any{})
	if isErr {
		t.Fatalf("bootstrap: %s", text)
	}
	if strings.Contains(text, "<hint>") || strings.Contains(text, "db_manifest_errors") {
		t.Fatalf("valid manifest must not hint/error: %s", text)
	}
	c.bootstrapAndWait(root)

	// SQL 소스: 검색 → FK/트리거/정책 엣지 → 진짜 DDL 반환
	text, _ = c.tool("search_hybrid", map[string]any{"query": "job_posting"})
	if !strings.Contains(text, "db.main.public.job_posting") {
		t.Fatalf("search: %s", text)
	}
	text, _ = c.tool("explore_graph", map[string]any{
		"node_id": "db.main.public.job_posting", "direction": "both"})
	for _, want := range []string{
		`id="db.main.public.company" type="foreign_key"`,
		`id="db.main.public.job_posting.trg_job_posting_updated_at"`,
		`id="db.main.public.job_posting.rls.job_posting_public_read"`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("explore missing %s:\n%s", want, text)
		}
	}
	text, _ = c.tool("read_code", map[string]any{"node_id": "db.main.public.job_posting"})
	if !strings.Contains(text, "CREATE TABLE public.job_posting") ||
		!strings.Contains(text, "공고 제목") {
		t.Fatalf("read_code must return the CREATE TABLE statement: %s", text)
	}

	// prisma 소스: @@schema/@@map 반영 + model 블록 반환
	text, _ = c.tool("read_code", map[string]any{"node_id": "db.app.blog.posts"})
	if !strings.Contains(text, "model Post") {
		t.Fatalf("prisma read_code: %s", text)
	}

	// tbls 프리셋: virtual relation → 0.9 논리 참조
	text, _ = c.tool("explore_graph", map[string]any{
		"node_id": "db.legacy.public.posts", "direction": "uses"})
	if !strings.Contains(text, `id="db.legacy.public.users" type="foreign_key" confidence="1.0"`) &&
		!strings.Contains(text, `id="db.legacy.public.users" type="foreign_key"`) {
		t.Fatalf("tbls FK: %s", text)
	}
	if !strings.Contains(text, `id="db.legacy.public.categories" type="foreign_key"`) {
		t.Fatalf("tbls virtual relation: %s", text)
	}

	// 커스텀 매핑 DSL
	text, _ = c.tool("explore_graph", map[string]any{
		"node_id": "db.custom.public.job_posting", "direction": "uses"})
	if !strings.Contains(text, `id="db.custom.public.company"`) ||
		!strings.Contains(text, `id="db.custom.public.resume"`) {
		t.Fatalf("mapped JSON FKs: %s", text)
	}
}

// TestDBManifestErrorsSurface: 매니페스트 오류는 에이전트가 반복 수정할 수
// 있도록 상태 속성으로 노출된다.
func TestDBManifestErrorsSurface(t *testing.T) {
	root := t.TempDir()
	manifest := `{"version":1,"datasources":{"x":{"sources":[{"path":"a.bin","format":"weird"}]}}}`
	if err := os.WriteFile(filepath.Join(root, "graphindb.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	c := newClient(t, root)
	text, isErr := c.tool("bootstrap_workspace", map[string]any{})
	if isErr {
		t.Fatalf("bootstrap: %s", text)
	}
	if !strings.Contains(text, "db_manifest_errors=") ||
		!strings.Contains(text, "unknown format") {
		t.Fatalf("manifest errors must surface: %s", text)
	}
}

// TestDBManifestLiveReload: 부트스트랩 후 매니페스트가 커밋되면 워처가
// 라우팅을 갱신하고, 내용이 그대로인 SSOT 파일도 DB 노드로 재인덱싱된다.
func TestDBManifestLiveReload(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "db"), 0o755); err != nil {
		t.Fatal(err)
	}
	src, err := os.ReadFile(filepath.Join(dbssotFixtures, "db", "schema.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "db", "schema.sql"), src, 0o644); err != nil {
		t.Fatal(err)
	}
	c := newClient(t, root)
	c.bootstrapAndWait(root)

	// 라우팅 전: plain 파일 노드만 존재
	text, _ := c.tool("explore_graph", map[string]any{"node_id": "db.main.public.job_posting"})
	if !strings.Contains(text, "NODE_NOT_FOUND") && !strings.Contains(text, "not_found") {
		t.Fatalf("table node must not exist before routing: %s", text)
	}

	manifest := `{"version":1,"datasources":{"main":{"engine":"postgresql","sources":[{"path":"db/schema.sql"}]}}}`
	if err := os.WriteFile(filepath.Join(root, "graphindb.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(15 * time.Second)
	for {
		text, isErr := c.tool("read_code", map[string]any{"node_id": "db.main.public.job_posting"})
		if !isErr && strings.Contains(text, "CREATE TABLE public.job_posting") {
			return // 리로드 성공: 동일 바이트 파일이 DB 노드로 승격됨
		}
		if time.Now().After(deadline) {
			t.Fatalf("manifest live reload never took effect: %s", text)
		}
		time.Sleep(100 * time.Millisecond)
	}
}
