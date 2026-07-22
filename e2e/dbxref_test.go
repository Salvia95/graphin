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

	// DB → 코드: "이 테이블 고치면 어떤 코드가 영향받나"
	text, isErr = c.tool("explore_graph", map[string]any{
		"node_id": "db.main.public.job_posting", "direction": "used_by"})
	if isErr || !strings.Contains(text, `id="com.acme.JobPosting" type="reference"`) {
		t.Fatalf("table used_by must surface the entity: %s", text)
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
