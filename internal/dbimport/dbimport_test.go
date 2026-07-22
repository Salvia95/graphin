package dbimport

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Salvia95/graphin/internal/nodeid"
	"github.com/Salvia95/graphin/internal/parse"
)

const tblsSample = `{
  "name": "appdb",
  "driver": { "name": "postgres" },
  "tables": [
    {
      "name": "public.users", "type": "BASE TABLE", "comment": "회원",
      "columns": [
        { "name": "id", "type": "bigint", "nullable": false },
        { "name": "email", "type": "varchar(255)", "nullable": false }
      ],
      "constraints": [
        { "name": "users_pkey", "type": "PRIMARY KEY", "columns": ["id"] }
      ],
      "triggers": [
        { "name": "trg_users_updated",
          "def": "CREATE TRIGGER trg_users_updated BEFORE UPDATE ON public.users FOR EACH ROW EXECUTE FUNCTION set_updated_at()" }
      ]
    },
    {
      "name": "public.posts", "type": "BASE TABLE",
      "columns": [
        { "name": "id", "type": "bigint", "nullable": false },
        { "name": "user_id", "type": "bigint", "nullable": false },
        { "name": "category_id", "type": "bigint", "nullable": true }
      ],
      "indexes": [
        { "name": "idx_posts_user", "def": "CREATE INDEX idx_posts_user ON posts(user_id)" }
      ]
    },
    { "name": "public.categories", "type": "BASE TABLE",
      "columns": [ { "name": "id", "type": "bigint", "nullable": false } ] },
    { "name": "public.v_posts", "type": "VIEW",
      "def": "SELECT p.id, u.email FROM posts p JOIN users u ON u.id = p.user_id" }
  ],
  "relations": [
    { "table": "public.posts", "columns": ["user_id"],
      "parent_table": "public.users", "parent_columns": ["id"],
      "def": "FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE" },
    { "table": "public.posts", "columns": ["category_id"],
      "parent_table": "public.categories", "parent_columns": ["id"],
      "def": "posts.category_id -> categories.id", "virtual": true }
  ],
  "functions": [
    { "name": "set_updated_at", "return_type": "trigger", "arguments": "", "type": "FUNCTION" },
    { "name": "archive_posts", "return_type": "void", "arguments": "before date", "type": "PROCEDURE" }
  ]
}`

func runConvert(t *testing.T, out string) []string {
	t.Helper()
	in := filepath.Join(t.TempDir(), "schema.json")
	if err := os.WriteFile(in, []byte(tblsSample), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--from", "tbls", "--datasource", "app", "--out", out,
		"--synced-at", "2026-07-22T00:00:00Z", in}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr.String())
	}
	return strings.Fields(stdout.String())
}

func parseOut(t *testing.T, path string) *parse.FileResult {
	t.Helper()
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	res, err := parse.File("db/"+filepath.Base(path), src)
	if err != nil {
		t.Fatal(err)
	}
	if res.Partial {
		t.Fatalf("converter output failed strict parse: %s", path)
	}
	return res
}

func nodeByID(t *testing.T, res *parse.FileResult, id string) parse.Node {
	t.Helper()
	for _, n := range res.Nodes {
		if n.ID == id {
			return n
		}
	}
	t.Fatalf("node %s missing", id)
	return parse.Node{}
}

// TestTblsRoundTrip proves converter output is directly indexable by the
// graphindb extractor with the expected nodes and edge material.
func TestTblsRoundTrip(t *testing.T) {
	out := t.TempDir()
	written := runConvert(t, out)
	if len(written) != 3 { // main + functions + triggers
		t.Fatalf("written = %v", written)
	}

	main := parseOut(t, filepath.Join(out, "app.graphindb.json"))
	if main.Package != "db.app" {
		t.Fatalf("package = %q", main.Package)
	}
	users := nodeByID(t, main, "db.app.public.users")
	if users.Kind != nodeid.KindTable || !slices.Contains(users.Params, "email varchar(255)") {
		t.Fatalf("users = %+v", users)
	}
	posts := nodeByID(t, main, "db.app.public.posts")
	if !slices.Equal(posts.Supers, []string{"db.app.public.users"}) {
		t.Fatalf("enforced FK = %v", posts.Supers)
	}
	if !slices.Equal(posts.LogicalRefs, []string{"db.app.public.categories"}) {
		t.Fatalf("virtual relation must become enforced:false: %v", posts.LogicalRefs)
	}
	view := nodeByID(t, main, "db.app.public.v_posts")
	var toks []string
	for _, c := range view.Calls {
		toks = append(toks, c.Name)
	}
	if !slices.Contains(toks, "posts") { // 정의 토큰 휴리스틱 재료
		t.Fatalf("view def tokens = %v", toks)
	}

	fns := parseOut(t, filepath.Join(out, "app.functions.graphindb.json"))
	if n := nodeByID(t, fns, "db.app.public.set_updated_at"); n.Kind != nodeid.KindDBFunction {
		t.Fatalf("kind = %s", n.Kind)
	}
	if n := nodeByID(t, fns, "db.app.public.archive_posts"); n.Kind != nodeid.KindProcedure ||
		!slices.Equal(n.Params, []string{"before date"}) {
		t.Fatalf("procedure = %+v", n)
	}

	trgs := parseOut(t, filepath.Join(out, "app.triggers.graphindb.json"))
	trg := nodeByID(t, trgs, "db.app.public.users.trg_users_updated")
	want := []string{"db.app.public.users", "db.app.public.set_updated_at"}
	if !slices.Equal(trg.Supers, want) { // 함수명이 스키마로 정규화되어 Call 재료가 된다
		t.Fatalf("trigger refs = %v", trg.Supers)
	}
}

func TestTblsDeterministicOutput(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	runConvert(t, a)
	runConvert(t, b)
	for _, name := range []string{"app.graphindb.json", "app.functions.graphindb.json", "app.triggers.graphindb.json"} {
		x, err := os.ReadFile(filepath.Join(a, name))
		if err != nil {
			t.Fatal(err)
		}
		y, err := os.ReadFile(filepath.Join(b, name))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(x, y) {
			t.Fatalf("%s is not byte-deterministic", name)
		}
	}
}

func TestInitScaffold(t *testing.T) {
	out := t.TempDir()
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"--init", "main", "--out", out}, nil, &stdout, &stderr); code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr.String())
	}
	src, err := os.ReadFile(filepath.Join(out, "main.graphindb.json"))
	if err != nil {
		t.Fatal(err)
	}
	// 스캐폴드는 그 자체로 유효한(비어 있는) 스냅샷이어야 한다
	res, err := parse.File("db/main.graphindb.json", src)
	if err != nil || res.Partial || len(res.Nodes) != 0 || res.Package != "db.main" {
		t.Fatalf("scaffold parse: err=%v partial=%v nodes=%d pkg=%q",
			err, res.Partial, len(res.Nodes), res.Package)
	}
	// 이미 있으면 덮어쓰지 않는다
	if code := Run([]string{"--init", "main", "--out", out}, nil, &stdout, &stderr); code == 0 {
		t.Fatal("scaffold must refuse to overwrite")
	}
}
