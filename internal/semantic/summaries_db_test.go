package semantic

import (
	"os"
	"strings"
	"testing"

	"github.com/Salvia95/graphin/internal/parse"
)

func dbFixtureNodes(t *testing.T, name string) *parse.FileResult {
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

func dbSummaryOf(t *testing.T, res *parse.FileResult, id string) string {
	t.Helper()
	for _, n := range res.Nodes {
		if n.ID == id {
			return Summarize(res.Package, n)
		}
	}
	t.Fatalf("node %s not in fixture", id)
	return ""
}

func TestSummarizeDBTable(t *testing.T) {
	res := dbFixtureNodes(t, "main.graphindb.json")
	s := dbSummaryOf(t, res, "db.main.public.job_posting")
	for _, want := range []string{
		"table main.public.job_posting in db.main",
		"job posting",                        // 식별자 분해
		"; columns ", "company_id bigint",    // 접힌 컬럼
		"; references db.main.public.company", // FK
		"; content ",                          // 스냅샷 블록 토큰(주석 포함)
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("summary missing %q:\n%s", want, s)
		}
	}
	// enforced:false 참조도 요약에 포함된다
	fb := dbSummaryOf(t, res, "db.main.public.ai_feedback")
	if !strings.Contains(fb, "db.main.public.resume") {
		t.Fatalf("logical ref missing:\n%s", fb)
	}
}

func TestSummarizeDBRoutineAndRLS(t *testing.T) {
	fn := dbSummaryOf(t, dbFixtureNodes(t, "main.functions.graphindb.json"),
		"db.main.public.fn_company_job_count")
	if !strings.Contains(fn, "db_function main.public.fn_company_job_count") ||
		!strings.Contains(fn, "; args p_company_id bigint") {
		t.Fatalf("routine summary:\n%s", fn)
	}
	rls := dbSummaryOf(t, dbFixtureNodes(t, "main.rls.graphindb.json"),
		"db.main.public.resume.rls")
	if !strings.Contains(rls, "; policies ") ||
		!strings.Contains(rls, "resume_self_read SELECT") {
		t.Fatalf("rls summary:\n%s", rls)
	}
}
