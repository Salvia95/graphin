package admin

// UC-8 (USE_CASES.md): "이 테이블을 건드리는 코드"를 테이블 노드의 참조됨
// (used_by) 한 번으로 찾는다. 문서가 약속하는 경로 — 검색 → 테이블 노드 →
// used_by — 를 admin 화면 기준으로 고정한다.

import (
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Salvia95/graphin/internal/workspace"
)

const (
	xrefTableID = "db.main.public.job_posting"
	// 두 경로가 같은 테이블에 닿는다 — 명시 물리명(@Table, 1.00)과 SQL
	// 리터럴(0.90). USE_CASES.md UC-8이 설명하는 신뢰도 티어 그대로다.
	xrefEntityID = "com.acme.JobPosting"
	xrefSQLID    = "src.report.load_active_postings"
)

// copyTree mirrors a fixture directory into the workspace root.
func copyTree(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, b, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
}

// xrefServer boots a workspace holding the dbxref fixture: a JPA entity and a
// raw-SQL reporting function, both pointing at public.job_posting.
func xrefServer(t *testing.T) (*Server, *workspace.Workspace) {
	t.Helper()
	ws := newTestWS(t, nil)
	copyTree(t, filepath.Join("..", "..", "testdata", "fixtures", "dbxref"), ws.Root)
	bootstrapWS(t, ws)

	// 크로스 도메인 엣지는 DB 레지스트리가 선 것을 보고 코드 파일이 해석될 때
	// 생긴다 — lexical ready보다 늦을 수 있으므로 엣지 자체를 기다린다.
	deadline := time.Now().Add(10 * time.Second)
	for {
		page, err := ws.Explore(xrefTableID, "used_by", "", defaultMinConf)
		if err == nil && len(page.UsedBy) > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("크로스 도메인 엣지가 생기지 않았다 (explore %s: %v)", xrefTableID, err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	return newTestServer(t, ws), ws
}

// UC-8 1단계: 테이블 이름으로 검색하면 테이블 노드가 잡혀야 한다.
func TestUC8TableNodeIsSearchable(t *testing.T) {
	s, _ := xrefServer(t)
	rec := get(t, s, "/search?q=job_posting")
	wantContains(t, rec, http.StatusOK, xrefTableID, "테이블")
}

// UC-8 2단계: 테이블 노드의 참조됨 목록이 그 테이블을 만지는 코드를 한 홉에
// 보여줘야 한다. 문서가 "one hop"이라고 약속한 부분이다.
func TestUC8TableUsedByReachesCode(t *testing.T) {
	s, _ := xrefServer(t)
	rec := get(t, s, "/node?id="+url.QueryEscape(xrefTableID))
	if rec.Code != http.StatusOK {
		t.Fatalf("table node page = %d", rec.Code)
	}
	body := rec.Body.String()

	if !strings.Contains(body, "참조됨") {
		t.Fatal("노드 상세에 참조됨(used_by) 섹션이 없다")
	}
	// 두 진입 경로가 모두 한 홉에 보여야 한다 — 하나만 나오면 "이 테이블을
	// 건드리는 코드"라는 UC-8의 약속이 반만 지켜진 것이다.
	for _, want := range []string{xrefEntityID, xrefSQLID} {
		if !strings.Contains(body, want) {
			t.Errorf("used_by에 %s가 없다", want)
		}
	}
}

// UC-8이 신뢰도를 읽는 근거: 명시 물리명(@Table)은 최상위 티어라 기본
// min_conf 0.85에서 살아남아야 한다. 이게 깨지면 문서의 "1.00 explicit
// physical name" 설명이 거짓이 된다.
func TestUC8ExplicitMappingSurvivesDefaultConfidence(t *testing.T) {
	_, ws := xrefServer(t)
	page, err := ws.Explore(xrefTableID, "used_by", "", defaultMinConf)
	if err != nil {
		t.Fatalf("explore: %v", err)
	}
	got := map[string]float32{}
	for _, e := range page.UsedBy {
		got[e.NodeID] = e.Confidence
	}
	// USE_CASES.md UC-8이 독자에게 약속하는 티어. 값이 흔들리면 문서가 거짓이
	// 되므로 여기서 먼저 실패해야 한다.
	for id, want := range map[string]float32{
		xrefEntityID: 1.00, // 명시 물리명 @Table(name="job_posting")
		xrefSQLID:    0.90, // SQL 리터럴 SELECT … FROM job_posting
	} {
		conf, ok := got[id]
		if !ok {
			t.Errorf("기본 신뢰도 %.2f에서 %s 엣지가 사라졌다", defaultMinConf, id)
			continue
		}
		if conf < want-0.001 || conf > want+0.001 {
			t.Errorf("%s 신뢰도 = %.2f, 문서는 %.2f라고 설명한다", id, conf, want)
		}
	}
}
