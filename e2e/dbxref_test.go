package e2e

import (
	"strings"
	"testing"
)

const xrefFixtures = "../testdata/fixtures/dbxref"

// TestDBXrefRoundtrip (Phase 7a): JPA 엔티티와 graphindb 스냅샷이 공존하는
// 워크스페이스에서 코드↔DB 크로스 도메인 엣지의 전체 여정 — 테이블 used_by가
// 엔티티 클래스를 보여주고, 엔티티 uses가 테이블로 이어지며, 양쪽 모두
// read_code로 원문에 도달한다.
func TestDBXrefRoundtrip(t *testing.T) {
	root := t.TempDir()
	copyTree(t, xrefFixtures, root)
	c := newClient(t, root)

	text, isErr := c.tool("bootstrap_workspace", map[string]any{})
	if isErr {
		t.Fatalf("bootstrap: %s", text)
	}
	c.bootstrapAndWait(root)

	// 코드 → DB: 엔티티 클래스의 uses에 테이블 Reference(명시 매핑 1.0)
	text, isErr = c.tool("explore_graph", map[string]any{
		"node_id": "com.acme.JobPosting", "direction": "uses"})
	if isErr || !strings.Contains(text, `id="db.main.public.job_posting" type="reference"`) {
		t.Fatalf("entity uses must reach the table: %s", text)
	}

	// DB → 코드: "이 테이블 고치면 어떤 코드가 영향받나" — JPA 엔티티(1.0)와
	// raw SQL 리포트 함수(0.9, Phase 7b)가 함께 나온다
	text, isErr = c.tool("explore_graph", map[string]any{
		"node_id": "db.main.public.job_posting", "direction": "used_by"})
	if isErr || !strings.Contains(text, `id="com.acme.JobPosting" type="reference"`) {
		t.Fatalf("table used_by must surface the entity: %s", text)
	}
	if !strings.Contains(text, `id="src.report.load_active_postings" type="reference"`) {
		t.Fatalf("table used_by must surface the SQL-literal caller: %s", text)
	}

	// 종단 read_code: 테이블 노드는 스냅샷 블록, 엔티티는 자바 원문
	text, isErr = c.tool("read_code", map[string]any{"node_id": "db.main.public.job_posting"})
	if isErr || !strings.Contains(text, "채용 공고") {
		t.Fatalf("table read_code: %s", text)
	}
	text, isErr = c.tool("read_code", map[string]any{"node_id": "com.acme.JobPosting"})
	if isErr || !strings.Contains(text, `@Table(name = "job_posting")`) {
		t.Fatalf("entity read_code: %s", text)
	}
}

// TestDBTraceBenchmark (Phase 7b §2): db-trace 시나리오가 제네릭 3-step
// 벤치마크 경로로 측정된다 — 테이블 노드가 기대 노드일 때 explore가 크로스
// 엣지를 동봉하므로 grep 대비 바이트 절감이 리포트에 잡힌다.
func TestDBTraceBenchmark(t *testing.T) {
	root := t.TempDir()
	copyTree(t, xrefFixtures, root)
	c := newClient(t, root)
	if text, isErr := c.tool("bootstrap_workspace", map[string]any{}); isErr {
		t.Fatalf("bootstrap: %s", text)
	}
	c.bootstrapAndWait(root)

	text, isErr := c.tool("run_local_benchmark", map[string]any{
		"target_query":  "job_posting",
		"expected_node": "db.main.public.job_posting",
	})
	if isErr {
		t.Fatalf("benchmark: %s", text)
	}
	for _, want := range []string{`hit="true"`, "graphin (search→explore→read)", "Grep Full"} {
		if !strings.Contains(text, want) {
			t.Fatalf("benchmark report missing %q:\n%s", want, text)
		}
	}
}
