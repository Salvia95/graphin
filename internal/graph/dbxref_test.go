package graph

// Phase 7a: code→DB cross-domain edge resolution (docs/phase7-spec.md §1).

import (
	"testing"

	"github.com/Salvia95/graphin/internal/merkle"
	"github.com/Salvia95/graphin/internal/nodeid"
	"github.com/Salvia95/graphin/internal/parse"
)

func entityNode(pkg, name string, refs ...parse.DBRef) parse.Node {
	n := classNode(pkg, name, nodeid.KindClass)
	n.DBRefs = refs
	return n
}

func tableNode(ds, schema, name string, aliases ...string) parse.Node {
	id := nodeid.DBNode(ds, schema, name)
	return parse.Node{
		ID: id, DisplayName: nodeid.DBDisplay(ds, schema, name), SimpleName: name,
		Kind: nodeid.KindTable, Container: schema, Aliases: aliases,
		Hash: merkle.Sum([]byte(id)),
	}
}

func TestXrefExplicitMapping(t *testing.T) {
	e := newEngine(t)
	applyAll(e,
		dbFixture(t, "main.graphindb.json"),
		fileRes(parse.LangJava, "src/JobPosting.java", "com.acme", nil,
			entityNode("com.acme", "JobPosting",
				parse.DBRef{Name: "job_posting", Source: parse.DBRefExplicit})))

	uses := usesOf(t, e, "com.acme.JobPosting")
	if !hasEdge(uses, "db.main.public.job_posting", "reference", 1.0) {
		t.Fatalf("explicit xref missing: %+v", uses)
	}
	// 핵심 시나리오: 테이블을 고치면 어떤 코드가 영향받나
	rev := usedByOf(t, e, "db.main.public.job_posting")
	if !hasEdge(rev, "com.acme.JobPosting", "reference", 1.0) {
		t.Fatalf("table used_by must surface entity class: %+v", rev)
	}
}

func TestXrefConventionSnakeCase(t *testing.T) {
	e := newEngine(t)
	applyAll(e,
		dbFixture(t, "main.graphindb.json"),
		fileRes(parse.LangJava, "src/JobPosting.java", "com.acme", nil,
			entityNode("com.acme", "JobPosting",
				parse.DBRef{Name: "JobPosting", Source: parse.DBRefConvention})))

	uses := usesOf(t, e, "com.acme.JobPosting")
	if !hasEdge(uses, "db.main.public.job_posting", "reference", 0.8) {
		t.Fatalf("convention xref missing: %+v", uses)
	}
}

func TestXrefClientAlias(t *testing.T) {
	e := newEngine(t)
	m := methodNode("src.svc", "OrderService", "list", nil, 0, 0)
	m.DBRefs = []parse.DBRef{{Name: "orderItem", Source: parse.DBRefClient}}
	applyAll(e,
		fileRes(parse.LangDBSchema, "db/app.graphindb.json", "db.app", nil,
			tableNode("app", "public", "order_item", "OrderItem", "orderItem")),
		fileRes(parse.LangTypeScript, "src/svc.ts", "src.svc", nil, m))

	uses := usesOf(t, e, m.ID)
	if !hasEdge(uses, "db.app.public.order_item", "reference", 0.9) {
		t.Fatalf("client alias xref missing: %+v", uses)
	}
}

func TestXrefMultiDatasourceDemotion(t *testing.T) {
	e := newEngine(t)
	applyAll(e,
		dbFixture(t, "main.graphindb.json"),
		fileRes(parse.LangDBSchema, "db/other.graphindb.json", "db.other", nil,
			tableNode("other", "public", "job_posting")),
		fileRes(parse.LangJava, "src/JobPosting.java", "com.acme", nil,
			entityNode("com.acme", "JobPosting",
				parse.DBRef{Name: "job_posting", Source: parse.DBRefExplicit})))

	uses := usesOf(t, e, "com.acme.JobPosting")
	if !hasEdge(uses, "db.main.public.job_posting", "reference", 0.8) ||
		!hasEdge(uses, "db.other.public.job_posting", "reference", 0.8) {
		t.Fatalf("ambiguous xref must fan out demoted: %+v", uses)
	}
}

func TestXrefDanglingSoleDatasource(t *testing.T) {
	e := newEngine(t)
	applyAll(e,
		dbFixture(t, "main.graphindb.json"),
		fileRes(parse.LangJava, "src/Legacy.java", "com.acme", nil,
			entityNode("com.acme", "Legacy",
				parse.DBRef{Name: "legacy_orders", Source: parse.DBRefExplicit})))

	uses := usesOf(t, e, "com.acme.Legacy")
	if !hasEdge(uses, "db.main.public.legacy_orders", "reference", 1.0) {
		t.Fatalf("explicit mapping must dangle on sole datasource: %+v", uses)
	}
}

func TestXrefNoDanglingOnMultiDatasource(t *testing.T) {
	e := newEngine(t)
	applyAll(e,
		dbFixture(t, "main.graphindb.json"),
		fileRes(parse.LangDBSchema, "db/other.graphindb.json", "db.other", nil,
			tableNode("other", "public", "unrelated")),
		fileRes(parse.LangJava, "src/Legacy.java", "com.acme", nil,
			entityNode("com.acme", "Legacy",
				parse.DBRef{Name: "legacy_orders", Source: parse.DBRefExplicit})))

	if uses := usesOf(t, e, "com.acme.Legacy"); len(uses) != 0 {
		t.Fatalf("no dangling FQN is synthesizable across datasources: %+v", uses)
	}
}

func TestXrefInactiveDBDomainIsNoop(t *testing.T) {
	e := newEngine(t)
	applyAll(e,
		fileRes(parse.LangJava, "src/JobPosting.java", "com.acme", nil,
			entityNode("com.acme", "JobPosting",
				parse.DBRef{Name: "job_posting", Source: parse.DBRefExplicit})))

	if uses := usesOf(t, e, "com.acme.JobPosting"); len(uses) != 0 {
		t.Fatalf("DB-less workspace must stay noise-free: %+v", uses)
	}
}

func TestXrefConventionNeverDangles(t *testing.T) {
	e := newEngine(t)
	applyAll(e,
		dbFixture(t, "main.graphindb.json"),
		fileRes(parse.LangJava, "src/Ghost.java", "com.acme", nil,
			entityNode("com.acme", "Ghost",
				parse.DBRef{Name: "Ghost", Source: parse.DBRefConvention})))

	if uses := usesOf(t, e, "com.acme.Ghost"); len(uses) != 0 {
		t.Fatalf("convention refs are registry-bound: %+v", uses)
	}
}

func TestSnakeCase(t *testing.T) {
	for in, want := range map[string]string{
		"JobPosting": "job_posting",
		"Order":      "order",
		"HTTPRoute":  "http_route",
		"orderItem":  "order_item",
		"A":          "a",
		"user2Fa":    "user2_fa",
	} {
		if got := snakeCase(in); got != want {
			t.Errorf("snakeCase(%q) = %q, want %q", in, got, want)
		}
	}
}
