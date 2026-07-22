package sweexplore

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const javaFixtures = "../../../testdata/fixtures/java"

const demoIssue = "# cancelPayment leaves orders stuck\n\n" +
	"When `OrderService.cancelPayment` runs after a partial refund the order\n" +
	"stays in PROCESSING. Likely related to `PaymentGateway` retries in\n" +
	"cancel_payment handling.\n"

func copyFixtureRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	err := filepath.WalkDir(javaFixtures, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(javaFixtures, p)
		dst := filepath.Join(root, rel)
		if d.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(dst, b, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func TestDeriveQueriesDeterministicAndPrioritized(t *testing.T) {
	q1 := DeriveQueries(demoIssue, 3)
	q2 := DeriveQueries(demoIssue, 3)
	if !reflect.DeepEqual(q1, q2) {
		t.Fatalf("derivation must be deterministic: %v vs %v", q1, q2)
	}
	if len(q1) != 3 {
		t.Fatalf("queries = %v", q1)
	}
	if q1[0] != "cancelPayment leaves orders stuck" {
		t.Fatalf("title first, got %q", q1[0])
	}
	if q1[1] != "OrderService.cancelPayment" {
		t.Fatalf("backtick span second, got %q", q1[1])
	}
}

func TestLoadBenchMetaFallbackAndProblems(t *testing.T) {
	dir := t.TempDir()
	repo := filepath.Join(dir, "repos", "demo__1")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	lines := []string{
		// meta.problem_statement 폴백 + repos/<instance_id> 레이아웃 폴백
		`{"instance_id":"demo__1","meta":{"problem_statement":"issue body"}}`,
		`{"instance_id":"gone__2","repo_dir":"missing","problem_statement":"x"}`,
		`{"repo_dir":"whatever"}`,
	}
	bench := filepath.Join(dir, "bench.jsonl")
	if err := os.WriteFile(bench, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tasks, problems, err := LoadBench(bench, filepath.Join(dir, "repos"))
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 || tasks[0].InstanceID != "demo__1" || tasks[0].Issue != "issue body" {
		t.Fatalf("tasks = %+v", tasks)
	}
	if len(problems) != 2 {
		t.Fatalf("problems = %v", problems)
	}
}

func TestExploreRepoDeterministicRegions(t *testing.T) {
	root := copyFixtureRepo(t)
	o := Defaults()
	r1, st1, err := ExploreRepo(context.Background(), root, demoIssue, o)
	if err != nil {
		t.Fatal(err)
	}
	r2, _, err := ExploreRepo(context.Background(), root, demoIssue, o)
	if err != nil {
		t.Fatal(err)
	}
	if st1.EmbedDropped != 0 {
		t.Fatalf("lexical-only run must not report embed drops: %+v", st1)
	}
	if !reflect.DeepEqual(r1, r2) {
		t.Fatalf("policy must be deterministic:\n%v\n%v", r1, r2)
	}
	if len(r1) == 0 {
		t.Fatal("no regions emitted")
	}
	found := false
	for _, r := range r1 {
		if strings.HasSuffix(r.Path, "OrderService.java") && r.Start >= 1 && r.End >= r.Start {
			found = true
		}
	}
	if !found {
		t.Fatalf("OrderService.java region missing: %+v", r1)
	}
	if _, err := os.Stat(filepath.Join(root, ".graphin")); !os.IsNotExist(err) {
		t.Fatal("index must be cleaned up without --keep-index")
	}
}

func TestGrepRegionsBaseline(t *testing.T) {
	root := copyFixtureRepo(t)
	regions, err := GrepRegions(root, demoIssue, Defaults(), 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(regions) == 0 {
		t.Fatal("grep baseline found nothing")
	}
	if !strings.HasSuffix(regions[0].Path, "OrderService.java") {
		t.Fatalf("top-ranked file should be OrderService.java: %+v", regions[:min(3, len(regions))])
	}
}

func TestRunWritesSubmissionsAndSummary(t *testing.T) {
	root := copyFixtureRepo(t)
	dir := t.TempDir()
	bench := filepath.Join(dir, "bench.jsonl")
	line := fmt.Sprintf(`{"instance_id":"demo__1","repo_dir":%q,"problem_statement":%q}`, root, demoIssue)
	if err := os.WriteFile(bench, []byte(line+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out := filepath.Join(dir, "out")
	ro := RunOptions{
		BenchPath: bench, ReposRoot: dir, OutDir: out, Policy: "graphin",
		Base: Defaults(),
		Configs: []SweepConfig{
			{Name: "k5-rrf60-mc05", TopK: 5, RRFK: 60, MinConf: 0.5},
			{Name: "k10-rrf60-mc00", TopK: 10, RRFK: 60, MinConf: 0},
		},
	}
	if err := Run(context.Background(), ro, io.Discard); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"submission-graphin-k5-rrf60-mc05.jsonl", "submission-graphin-k10-rrf60-mc00.jsonl", "summary.md"} {
		b, err := os.ReadFile(filepath.Join(out, name))
		if err != nil {
			t.Fatal(err)
		}
		if len(b) == 0 {
			t.Fatalf("%s is empty", name)
		}
		if strings.HasSuffix(name, ".jsonl") {
			var sub submissionLine
			if err := json.Unmarshal([]byte(strings.SplitN(string(b), "\n", 2)[0]), &sub); err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			if sub.InstanceID != "demo__1" || len(sub.Regions) == 0 {
				t.Fatalf("%s: %+v", name, sub)
			}
		}
	}
}
