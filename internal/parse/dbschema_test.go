package parse

import (
	"encoding/json"
	"os"
	"slices"
	"testing"

	"github.com/zeebo/blake3"

	"github.com/Salvia95/graphin/internal/nodeid"
)

const dbFixtureDir = "../../testdata/fixtures/dbschema/db/"

func loadDBFixture(t *testing.T, name string) (*FileResult, []byte) {
	t.Helper()
	src, err := os.ReadFile(dbFixtureDir + name)
	if err != nil {
		t.Fatal(err)
	}
	res, err := File("db/"+name, src)
	if err != nil {
		t.Fatal(err)
	}
	return res, src
}

func dbFind(t *testing.T, res *FileResult, id string) Node {
	t.Helper()
	for _, n := range res.Nodes {
		if n.ID == id {
			return n
		}
	}
	var ids []string
	for _, n := range res.Nodes {
		ids = append(ids, n.ID)
	}
	t.Fatalf("node %s not found, have %v", id, ids)
	return Node{}
}

func TestDBSchemaDetection(t *testing.T) {
	for _, p := range []string{"db/main.graphindb.json", "main.rls.graphindb.json", "x/y/warehouse.graphindb.json"} {
		if DetectLanguage(p) != LangDBSchema {
			t.Errorf("%s should be LangDBSchema", p)
		}
	}
	if DetectLanguage("config/app.json") != LangPlain { // 일반 .json은 plain 유지
		t.Error("plain .json regressed")
	}
}

func TestDBSchemaMainNodes(t *testing.T) {
	res, _ := loadDBFixture(t, "main.graphindb.json")
	if res.Lang != LangDBSchema || res.Partial {
		t.Fatalf("lang=%v partial=%v", res.Lang, res.Partial)
	}
	if res.Package != "db.main" {
		t.Fatalf("package = %q", res.Package)
	}
	if len(res.Nodes) != 7 { // 6 tables + 1 view
		t.Fatalf("node count = %d", len(res.Nodes))
	}

	jp := dbFind(t, res, "db.main.public.job_posting")
	if jp.Kind != nodeid.KindTable || jp.DisplayName != "main.public.job_posting" ||
		jp.SimpleName != "job_posting" || jp.Container != "public" {
		t.Fatalf("job_posting node = %+v", jp)
	}
	if !slices.Contains(jp.Params, "company_id bigint") {
		t.Fatalf("columns not folded into Params: %v", jp.Params)
	}
	if !slices.Equal(jp.Supers, []string{"db.main.public.company"}) {
		t.Fatalf("FK refs = %v", jp.Supers)
	}
	// JSON 블록이 그대로 BM25 토큰이 된다 (식별자는 서브토큰+결합형으로 분해)
	for _, want := range []string{"companyid", "timestamptz", "채용"} {
		if !slices.Contains(jp.BodyTokens, want) {
			t.Fatalf("BodyTokens missing %q: %v", want, jp.BodyTokens)
		}
	}

	// cross-schema dangling ref (auth.users는 스냅샷 밖) 그대로 정규화
	resume := dbFind(t, res, "db.main.public.resume")
	if !slices.Equal(resume.Supers, []string{"db.main.auth.users"}) {
		t.Fatalf("resume refs = %v", resume.Supers)
	}

	// enforced:false → LogicalRefs 분리
	fb := dbFind(t, res, "db.main.public.ai_feedback")
	if !slices.Equal(fb.Supers, []string{"db.main.auth.users"}) ||
		!slices.Equal(fb.LogicalRefs, []string{"db.main.public.resume"}) {
		t.Fatalf("ai_feedback supers=%v logical=%v", fb.Supers, fb.LogicalRefs)
	}

	// 다중 스키마: audit.event_log
	dbFind(t, res, "db.main.audit.event_log")

	// 뷰: 명시 references가 있으면 Calls 휴리스틱은 비활성
	vw := dbFind(t, res, "db.main.public.v_active_job_posting")
	if vw.Kind != nodeid.KindView || len(vw.Calls) != 0 {
		t.Fatalf("view node = %+v", vw)
	}
	if !slices.Equal(vw.Supers, []string{"db.main.public.job_posting", "db.main.public.company"}) {
		t.Fatalf("view refs = %v", vw.Supers)
	}
}

func TestDBSchemaSpanIntegrity(t *testing.T) {
	res, src := loadDBFixture(t, "main.graphindb.json")
	for _, n := range res.Nodes {
		if n.StartByte >= n.EndByte || int(n.EndByte) > len(src) {
			t.Fatalf("%s span %d..%d out of range", n.ID, n.StartByte, n.EndByte)
		}
		slice := src[n.StartByte:n.EndByte]
		if !json.Valid(slice) { // read_code가 반환할 조각은 그 자체로 유효 JSON
			t.Fatalf("%s slice is not standalone JSON: %q", n.ID, slice)
		}
		if n.Hash != blake3.Sum256(slice) {
			t.Fatalf("%s subtree hash mismatch", n.ID)
		}
	}
	jp := dbFind(t, res, "db.main.public.job_posting")
	var block struct {
		Comment string `json:"comment"`
	}
	if err := json.Unmarshal(src[jp.StartByte:jp.EndByte], &block); err != nil || block.Comment != "채용 공고" {
		t.Fatalf("span does not cover the table block: %v %q", err, block.Comment)
	}
}

func TestDBSchemaFunctions(t *testing.T) {
	res, _ := loadDBFixture(t, "main.functions.graphindb.json")
	if res.Package != "db.main" || len(res.Nodes) != 3 {
		t.Fatalf("pkg=%q nodes=%d", res.Package, len(res.Nodes))
	}
	fn := dbFind(t, res, "db.main.public.fn_company_job_count")
	if fn.Kind != nodeid.KindDBFunction ||
		!slices.Equal(fn.Supers, []string{"db.main.public.job_posting"}) ||
		!slices.Equal(fn.Params, []string{"p_company_id bigint"}) {
		t.Fatalf("fn node = %+v", fn)
	}
	if len(fn.Calls) != 0 { // 명시 references 우선
		t.Fatalf("explicit refs must suppress def tokens: %v", fn.Calls)
	}
	prc := dbFind(t, res, "db.main.public.prc_archive_job_posting")
	if prc.Kind != nodeid.KindProcedure {
		t.Fatalf("kind = %s", prc.Kind)
	}
	var callNames []string
	for _, c := range prc.Calls {
		callNames = append(callNames, c.Name)
	}
	if !slices.Contains(callNames, "job_posting") { // definition 휴리스틱
		t.Fatalf("def tokens = %v", callNames)
	}
}

func TestDBSchemaRLS(t *testing.T) {
	res, _ := loadDBFixture(t, "main.rls.graphindb.json")
	if len(res.Nodes) != 2 { // company: enabled=false + 정책 0 → 노드 없음
		t.Fatalf("nodes = %d", len(res.Nodes))
	}
	rls := dbFind(t, res, "db.main.public.resume.rls")
	if rls.Kind != nodeid.KindRLSPolicy || rls.SimpleName != "resume" ||
		!slices.Equal(rls.Supers, []string{"db.main.public.resume"}) {
		t.Fatalf("rls node = %+v", rls)
	}
	if len(rls.Params) != 4 || !slices.Contains(rls.Params, "resume_self_read SELECT") {
		t.Fatalf("policies = %v", rls.Params)
	}
}

func TestDBSchemaTriggers(t *testing.T) {
	res, _ := loadDBFixture(t, "main.triggers.graphindb.json")
	if len(res.Nodes) != 1 {
		t.Fatalf("nodes = %d", len(res.Nodes))
	}
	trg := res.Nodes[0]
	if trg.ID != "db.main.public.job_posting.trg_job_posting_updated_at" ||
		trg.Kind != nodeid.KindTrigger {
		t.Fatalf("trigger node = %+v", trg)
	}
	want := []string{"db.main.public.job_posting", "db.main.public.tg_job_posting_set_updated_at"}
	if !slices.Equal(trg.Supers, want) { // 테이블 + 트리거 함수
		t.Fatalf("trigger refs = %v", trg.Supers)
	}
}

func TestDBSchemaCrossDatasource(t *testing.T) {
	res, _ := loadDBFixture(t, "warehouse.graphindb.json")
	if res.Package != "db.warehouse" {
		t.Fatalf("package = %q", res.Package)
	}
	fact := dbFind(t, res, "db.warehouse.main.fact_job_daily")
	if !slices.Equal(fact.LogicalRefs, []string{"db.main.public.job_posting"}) {
		t.Fatalf("cross-DS logical ref = %v", fact.LogicalRefs)
	}
	if len(fact.Supers) != 0 {
		t.Fatalf("unexpected enforced refs: %v", fact.Supers)
	}
}

func TestDBSchemaMalformedFallsBackToPlain(t *testing.T) {
	res, err := File("db/broken.graphindb.json", []byte("{ this is not json"))
	if err != nil {
		t.Fatal(err)
	}
	if !res.Partial || len(res.Nodes) != 1 || res.Nodes[0].Kind != nodeid.KindFile {
		t.Fatalf("broken snapshot must degrade to partial plain node: %+v", res.Nodes)
	}
}

func TestNormalizeDBRef(t *testing.T) {
	cases := []struct {
		ref  string
		want string
		ok   bool
	}{
		{"public.company.id", "db.main.public.company", true},
		{"public.company", "db.main.public.company", true},
		{"auth.users.id", "db.main.auth.users", true},
		{"db.main.public.job_posting.id", "db.main.public.job_posting", true},
		{"db.wh.main.fact", "db.wh.main.fact", true},
		{"db.short", "", false},
		{"junk", "", false},
		{"", "", false},
	}
	for _, c := range cases {
		got, ok := normalizeDBRef("main", c.ref)
		if got != c.want || ok != c.ok {
			t.Errorf("normalizeDBRef(%q) = %q,%v want %q,%v", c.ref, got, ok, c.want, c.ok)
		}
	}
}
