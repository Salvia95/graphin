package graph

import (
	"os"
	"testing"

	"github.com/Salvia95/graphin/internal/parse"
)

// graphindb fixtures are parsed for real (parse.File) so these tests cover
// the extractor → ApplyFile → resolveDBEdges → Explore chain end to end.

func dbFixture(t *testing.T, name string) *parse.FileResult {
	t.Helper()
	src, err := os.ReadFile("../../testdata/fixtures/dbschema/db/" + name)
	if err != nil {
		t.Fatal(err)
	}
	res, err := parse.File("db/"+name, src)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestDBForeignKeyEdges(t *testing.T) {
	e := newEngine(t)
	applyAll(e, dbFixture(t, "main.graphindb.json"), dbFixture(t, "warehouse.graphindb.json"))

	uses := usesOf(t, e, "db.main.public.job_posting")
	if !hasEdge(uses, "db.main.public.company", "foreign_key", 1.0) {
		t.Fatalf("FK edge missing: %+v", uses)
	}

	// dangling(스냅샷 밖 auth.users) FK + enforced:false 다형성 참조
	fb := usesOf(t, e, "db.main.public.ai_feedback")
	if !hasEdge(fb, "db.main.auth.users", "foreign_key", 1.0) {
		t.Fatalf("dangling FK must survive: %+v", fb)
	}
	if !hasEdge(fb, "db.main.public.resume", "foreign_key", 0.9) {
		t.Fatalf("logical ref tier: %+v", fb)
	}

	// 역방향: 테이블을 고치면 어떤 참조가 깨지나
	rev := usedByOf(t, e, "db.main.public.company")
	if !hasEdge(rev, "db.main.public.job_posting", "foreign_key", 1.0) {
		t.Fatalf("reverse FK missing: %+v", rev)
	}

	// 크로스 데이터소스 논리 참조 (warehouse → main, 샤드 경계 통과)
	fact := usesOf(t, e, "db.warehouse.main.fact_job_daily")
	if !hasEdge(fact, "db.main.public.job_posting", "foreign_key", 0.9) {
		t.Fatalf("cross-datasource ref: %+v", fact)
	}
	rev = usedByOf(t, e, "db.main.public.job_posting")
	if !hasEdge(rev, "db.warehouse.main.fact_job_daily", "foreign_key", 0.9) {
		t.Fatalf("cross-datasource reverse: %+v", rev)
	}
}

func TestDBViewExplicitRefs(t *testing.T) {
	e := newEngine(t)
	applyAll(e, dbFixture(t, "main.graphindb.json"))
	uses := usesOf(t, e, "db.main.public.v_active_job_posting")
	if !hasEdge(uses, "db.main.public.job_posting", "reference", 1.0) ||
		!hasEdge(uses, "db.main.public.company", "reference", 1.0) {
		t.Fatalf("explicit view refs: %+v", uses)
	}
}

func TestDBRoutineAndTriggerEdges(t *testing.T) {
	e := newEngine(t)
	applyAll(e,
		dbFixture(t, "main.graphindb.json"),
		dbFixture(t, "main.functions.graphindb.json"),
		dbFixture(t, "main.triggers.graphindb.json"))

	// 명시 references → 1.0
	fn := usesOf(t, e, "db.main.public.fn_company_job_count")
	if !hasEdge(fn, "db.main.public.job_posting", "reference", 1.0) {
		t.Fatalf("explicit routine ref: %+v", fn)
	}

	// definition 토큰 휴리스틱 → 0.8, 같은 데이터소스의 테이블만
	prc := usesOf(t, e, "db.main.public.prc_archive_job_posting")
	if !hasEdge(prc, "db.main.public.job_posting", "reference", 0.8) {
		t.Fatalf("heuristic ref: %+v", prc)
	}

	// 트리거 → 테이블 Reference + 트리거 함수 Call
	trg := usesOf(t, e, "db.main.public.job_posting.trg_job_posting_updated_at")
	if !hasEdge(trg, "db.main.public.job_posting", "reference", 1.0) ||
		!hasEdge(trg, "db.main.public.tg_job_posting_set_updated_at", "call", 1.0) {
		t.Fatalf("trigger edges: %+v", trg)
	}
	rev := usedByOf(t, e, "db.main.public.tg_job_posting_set_updated_at")
	if !hasEdge(rev, "db.main.public.job_posting.trg_job_posting_updated_at", "call", 1.0) {
		t.Fatalf("trigger function reverse: %+v", rev)
	}
}

func TestDBRLSEdges(t *testing.T) {
	e := newEngine(t)
	applyAll(e, dbFixture(t, "main.graphindb.json"), dbFixture(t, "main.rls.graphindb.json"))
	rls := usesOf(t, e, "db.main.public.resume.rls")
	if !hasEdge(rls, "db.main.public.resume", "reference", 1.0) {
		t.Fatalf("rls anchor: %+v", rls)
	}
	rev := usedByOf(t, e, "db.main.public.resume")
	if !hasEdge(rev, "db.main.public.resume.rls", "reference", 1.0) {
		t.Fatalf("table used_by must surface RLS bundle: %+v", rev)
	}
}
