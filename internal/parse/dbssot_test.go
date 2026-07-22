package parse

import (
	"bytes"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/Salvia95/graphin/internal/nodeid"
)

const ssotDir = "../../testdata/fixtures/dbssot/"

func ssotRoutes(t *testing.T) map[string]*DBRoute {
	t.Helper()
	src, err := os.ReadFile(ssotDir + "graphindb.json")
	if err != nil {
		t.Fatal(err)
	}
	m, errs := LoadDBManifest(src)
	if len(errs) > 0 {
		t.Fatalf("manifest errors: %v", errs)
	}
	routes, errs := m.Routes()
	if len(errs) > 0 {
		t.Fatalf("route errors: %v", errs)
	}
	return routes
}

func parseSSOT(t *testing.T, rel string) *FileResult {
	t.Helper()
	routes := ssotRoutes(t)
	src, err := os.ReadFile(ssotDir + rel)
	if err != nil {
		t.Fatal(err)
	}
	res, err := FileWithRoute(rel, src, routes[rel])
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestManifestRoutes(t *testing.T) {
	routes := ssotRoutes(t)
	if len(routes) != 4 {
		t.Fatalf("routes = %v", SortedRoutePaths(routes))
	}
	r := routes["db/schema.sql"]
	if r == nil || r.Format != "sql" || r.Datasource != "main" || r.DefaultSchema != "public" {
		t.Fatalf("sql route = %+v", r)
	}
	if r := routes["prisma/schema.prisma"]; r == nil || r.Format != "schema" {
		t.Fatalf("prisma route = %+v", r)
	}
}

func TestManifestValidation(t *testing.T) {
	src := []byte(`{"version":1,"datasources":{
		"a":{"sources":[{"path":"x.sql"},{"path":"y.bin","format":"weird"}]},
		"b":{"sources":[{"path":"x.sql"},{"path":"z.json"}]},
		"c":{"sources":[]}}}`)
	m, errs := LoadDBManifest(src)
	if m == nil || len(errs) != 0 {
		t.Fatalf("load: %v", errs)
	}
	routes, errs := m.Routes()
	if _, ok := routes["x.sql"]; !ok {
		t.Fatal("first claim must win")
	}
	joined := strings.Join(errs, "\n")
	for _, want := range []string{"unknown format \"weird\"", "claimed by datasources", "format json needs", "no sources"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing error %q in:\n%s", want, joined)
		}
	}
}

func TestSQLExtract(t *testing.T) {
	res := parseSSOT(t, "db/schema.sql")
	if res.Package != "db.main" || res.Partial {
		t.Fatalf("pkg=%q partial=%v", res.Package, res.Partial)
	}
	// 4 tables + 1 view + 1 function + 1 trigger + 1 policy
	if len(res.Nodes) != 8 {
		var ids []string
		for _, n := range res.Nodes {
			ids = append(ids, n.ID)
		}
		t.Fatalf("nodes = %v", ids)
	}
	jp := dbFind(t, res, "db.main.public.job_posting")
	if jp.Kind != nodeid.KindTable ||
		!slices.Equal(jp.Supers, []string{"db.main.public.company"}) {
		t.Fatalf("job_posting = %+v", jp)
	}
	for _, p := range []string{"company_id bigint", "title varchar(200)", "updated_at timestamptz"} {
		if !slices.Contains(jp.Params, p) {
			t.Fatalf("columns = %v", jp.Params)
		}
	}
	// 인라인 REFERENCES
	comp := dbFind(t, res, "db.main.public.company")
	if !slices.Equal(comp.Supers, []string{"db.main.public.company_group"}) {
		t.Fatalf("inline FK = %v", comp.Supers)
	}
	// ALTER TABLE ADD CONSTRAINT 병합 (크로스 스키마 dangling 대상 포함)
	resume := dbFind(t, res, "db.main.public.resume")
	if !slices.Equal(resume.Supers, []string{"db.main.auth.users"}) {
		t.Fatalf("ALTER FK = %v", resume.Supers)
	}
	// 뷰 정의 토큰 → 휴리스틱 재료
	vw := dbFind(t, res, "db.main.public.v_active_job_posting")
	var toks []string
	for _, c := range vw.Calls {
		toks = append(toks, c.Name)
	}
	if !slices.Contains(toks, "job_posting") {
		t.Fatalf("view tokens = %v", toks)
	}
	// 트리거: 테이블 + 함수
	trg := dbFind(t, res, "db.main.public.job_posting.trg_job_posting_updated_at")
	want := []string{"db.main.public.job_posting", "db.main.public.tg_job_posting_set_updated_at"}
	if !slices.Equal(trg.Supers, want) {
		t.Fatalf("trigger = %v", trg.Supers)
	}
	// 정책문당 RLS 노드
	rls := dbFind(t, res, "db.main.public.job_posting.rls.job_posting_public_read")
	if rls.Kind != nodeid.KindRLSPolicy ||
		!slices.Equal(rls.Params, []string{"job_posting_public_read SELECT"}) {
		t.Fatalf("policy = %+v", rls)
	}
	// 스팬은 문장 그 자체 (read_code가 진짜 DDL을 반환)
	src, _ := os.ReadFile(ssotDir + "db/schema.sql")
	for _, n := range res.Nodes {
		slice := src[n.StartByte:n.EndByte]
		if !bytes.HasPrefix(bytes.ToUpper(slice), []byte("CREATE")) {
			t.Fatalf("%s span does not start at CREATE: %q", n.ID, slice[:min(40, len(slice))])
		}
	}
}

// TestSQLAlterHashMixing: ALTER 문만 바뀌어도 대상 테이블이 Changed로 분류돼야
// 한다 — 테이블 스팬은 그대로지만 해시에 ALTER 원문이 섞이기 때문.
func TestSQLAlterHashMixing(t *testing.T) {
	routes := ssotRoutes(t)
	src, err := os.ReadFile(ssotDir + "db/schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	before, _ := FileWithRoute("db/schema.sql", src, routes["db/schema.sql"])
	edited := bytes.Replace(src, []byte("REFERENCES auth.users (id)"), []byte("REFERENCES public.profiles (id)"), 1)
	after, _ := FileWithRoute("db/schema.sql", edited, routes["db/schema.sql"])
	b := dbFind(t, before, "db.main.public.resume")
	a := dbFind(t, after, "db.main.public.resume")
	if b.StartByte != a.StartByte || b.EndByte != a.EndByte {
		t.Fatal("resume statement span must be unaffected by the ALTER edit")
	}
	if b.Hash == a.Hash {
		t.Fatal("ALTER edit must change the merged table hash")
	}
	if !slices.Equal(a.Supers, []string{"db.main.public.profiles"}) {
		t.Fatalf("edited FK = %v", a.Supers)
	}
}

func TestPrismaExtract(t *testing.T) {
	res := parseSSOT(t, "prisma/schema.prisma")
	if res.Package != "db.app" || len(res.Nodes) != 3 { // enum/datasource/generator 제외
		t.Fatalf("pkg=%q nodes=%d", res.Package, len(res.Nodes))
	}
	users := dbFind(t, res, "db.app.public.users") // @@map
	if users.Kind != nodeid.KindTable || !slices.Contains(users.Params, "created_at DateTime") {
		t.Fatalf("users = %+v", users)
	}
	posts := dbFind(t, res, "db.app.blog.posts") // @@schema + @@map
	if !slices.Equal(posts.Supers, []string{"db.app.public.users"}) { // @relation
		t.Fatalf("relation FK = %v", posts.Supers)
	}
	if !slices.Contains(posts.Params, "author_id BigInt") { // 필드 @map
		t.Fatalf("posts columns = %v", posts.Params)
	}
	dbFind(t, res, "db.app.public.Tag") // @@map 없으면 모델명 그대로
	src, _ := os.ReadFile(ssotDir + "prisma/schema.prisma")
	if !bytes.HasPrefix(src[posts.StartByte:posts.EndByte], []byte("model Post")) {
		t.Fatal("span must cover the model block")
	}
}

func TestTblsPresetExtract(t *testing.T) {
	res := parseSSOT(t, "db/tbls.json")
	if res.Package != "db.legacy" {
		t.Fatalf("pkg = %q", res.Package)
	}
	posts := dbFind(t, res, "db.legacy.public.posts")
	if !slices.Equal(posts.Supers, []string{"db.legacy.public.users"}) ||
		!slices.Equal(posts.LogicalRefs, []string{"db.legacy.public.categories"}) {
		t.Fatalf("posts refs = %v / %v", posts.Supers, posts.LogicalRefs)
	}
	if v := dbFind(t, res, "db.legacy.public.v_posts"); v.Kind != nodeid.KindView {
		t.Fatalf("view kind = %s", v.Kind)
	}
	trg := dbFind(t, res, "db.legacy.public.users.trg_users_updated")
	if !slices.Contains(trg.Supers, "db.legacy.public.set_updated_at") {
		t.Fatalf("trigger fn = %v", trg.Supers)
	}
	if p := dbFind(t, res, "db.legacy.public.archive_posts"); p.Kind != nodeid.KindProcedure {
		t.Fatalf("procedure = %+v", p)
	}
}

func TestJSONMappedExtract(t *testing.T) {
	res := parseSSOT(t, "db/custom.json")
	if res.Package != "db.custom" || len(res.Nodes) != 2 {
		t.Fatalf("pkg=%q nodes=%d", res.Package, len(res.Nodes))
	}
	jp := dbFind(t, res, "db.custom.public.job_posting")
	if !slices.Contains(jp.Params, "company_id bigint") {
		t.Fatalf("columns = %v", jp.Params)
	}
	if !slices.Equal(jp.Supers, []string{"db.custom.public.company"}) ||
		!slices.Equal(jp.LogicalRefs, []string{"db.custom.public.resume"}) {
		t.Fatalf("fks = %v / %v", jp.Supers, jp.LogicalRefs)
	}
	// read_code 스팬 = 해당 테이블 JSON 블록
	src, _ := os.ReadFile(ssotDir + "db/custom.json")
	if !bytes.Contains(src[jp.StartByte:jp.EndByte], []byte("company_id")) ||
		bytes.Contains(src[jp.StartByte:jp.EndByte], []byte("display_name")) {
		t.Fatal("span must cover exactly the job_posting block")
	}
}

func TestRoutedGarbageFallsBackToPlain(t *testing.T) {
	route := &DBRoute{Datasource: "x", Format: "json", DefaultSchema: "public",
		JSON: &DBJSONRules{Preset: "tbls"}}
	res, err := FileWithRoute("db/broken.json", []byte("not json at all"), route)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Partial || len(res.Nodes) != 1 || res.Nodes[0].Kind != nodeid.KindFile {
		t.Fatalf("fallback = %+v", res.Nodes)
	}
}
