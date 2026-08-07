package admin

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Salvia95/graphin/internal/usage"
)

func TestUsagePageMissingData(t *testing.T) {
	s := newTestServer(t, newTestWS(t, nil))
	// The empty state has to name the way out, not just the absence.
	wantContains(t, get(t, s, "/usage"), http.StatusOK,
		"계측 데이터가 없습니다", "/plugin install graphin@graphin", "/graphin:doctor")
}

func TestUsagePageRendersReport(t *testing.T) {
	ws := newTestWS(t, nil)
	sample, err := os.ReadFile(filepath.Join("..", "..", "testdata", "fixtures", "usage", "events-sample.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(ws.Dir, "usage")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "events.jsonl"), sample, 0o644); err != nil {
		t.Fatal(err)
	}
	s := newTestServer(t, ws)

	rec := get(t, s, "/usage")
	wantContains(t, rec, http.StatusOK,
		"핵심 지표", "채택률", "폴백 페어", "바이그램", "이벤트")
	// 픽스처에는 채택·폴백이 모두 있으므로 all 그룹 행과 차트가 나온다.
	body := rec.Body.String()
	if !strings.Contains(body, "<svg") || !strings.Contains(body, `class="bar adoption"`) {
		t.Fatal("daily chart missing")
	}
	if !strings.Contains(body, "cancelOrder") {
		t.Fatal("fallback pair pattern missing")
	}
}

func TestBuildUsageChartLayout(t *testing.T) {
	days := []usage.DayTrend{
		{Date: "2026-08-01", Adoptions: 4, Fallbacks: 2},
		{Date: "2026-08-02", Adoptions: 0, Fallbacks: 1},
	}
	c := buildUsageChart(days)
	if c.MaxV != 4 || len(c.Bars) != 4 {
		t.Fatalf("chart shape: max=%d bars=%d", c.MaxV, len(c.Bars))
	}
	// 채택 4 = 최대값 → 막대 높이는 plot 최대(110), 바닥은 baseline.
	a := c.Bars[0]
	if a.Kind != "adoption" || a.H != 110 || a.Y+a.H != c.Baseline {
		t.Fatalf("adoption bar: %+v (baseline %v)", a, c.Baseline)
	}
	// 폴백 2 = 절반.
	f := c.Bars[1]
	if f.Kind != "fallback" || f.H != 55 {
		t.Fatalf("fallback bar: %+v", f)
	}
	// 0 값은 높이 0.
	if z := c.Bars[2]; z.H != 0 {
		t.Fatalf("zero bar: %+v", z)
	}
	if len(c.Labels) == 0 || c.Labels[0].Text != "08-01" {
		t.Fatalf("labels: %+v", c.Labels)
	}

	if empty := buildUsageChart(nil); len(empty.Bars) != 0 || empty.MaxV != 0 {
		t.Fatalf("empty chart: %+v", empty)
	}
}

func TestBuildBigramMatrixOrder(t *testing.T) {
	m := map[string]map[string]int{
		"search":   {"g_search": 2},
		"g_search": {"read": 3},
	}
	b := buildBigramMatrix(m)
	// spec 순서: g_search가 search·read보다 앞.
	if len(b.Cols) != 3 || b.Cols[0] != "g_search" {
		t.Fatalf("cols: %+v", b.Cols)
	}
	if b.Rows[0].From != "g_search" {
		t.Fatalf("rows: %+v", b.Rows)
	}
}

// The code/db table is conditional on purpose: a workspace with no schema
// snapshot has no db queries, and a permanent "db 0%" row would read as
// "graphin fails on db queries" — a different claim entirely.
func TestUsagePageHidesTargetTableWithoutDBRuns(t *testing.T) {
	ws := newTestWS(t, nil)
	writeUsageEvents(t, ws.Dir, []string{
		`{"v":1,"ts":"2026-08-06T10:00:00Z","session_id":"s1","prompt_id":"p1","tool_use_id":"u1","tool":"mcp__k__search_hybrid","p":{"query":"order cancel","result_ids":["src.order.OrderService.cancel"]}}`,
		`{"v":1,"ts":"2026-08-06T10:00:01Z","session_id":"s1","prompt_id":"p1","tool_use_id":"u2","tool":"Read","p":{"file_path":"a.go"}}`,
	})
	body := get(t, newTestServer(t, ws), "/usage").Body.String()
	if strings.Contains(body, "코드 vs DB 스키마 vs 문서 질의") {
		t.Fatal("target table rendered for a workspace with no db runs")
	}
}

func TestUsagePageRendersTargetTable(t *testing.T) {
	ws := newTestWS(t, nil)
	writeUsageEvents(t, ws.Dir, []string{
		// db run abandoned for a grep, code run adopted.
		`{"v":1,"ts":"2026-08-06T10:00:00Z","session_id":"s1","prompt_id":"p1","tool_use_id":"u1","tool":"mcp__k__search_hybrid","p":{"query":"job posting table","result_ids":["db.main.public.job_posting"]}}`,
		`{"v":1,"ts":"2026-08-06T10:00:01Z","session_id":"s1","prompt_id":"p1","tool_use_id":"u2","tool":"Grep","p":{"pattern":"job_posting","search":true}}`,
		`{"v":1,"ts":"2026-08-06T10:01:00Z","session_id":"s1","prompt_id":"p2","tool_use_id":"u3","tool":"mcp__k__search_hybrid","p":{"query":"order cancel","result_ids":["src.order.OrderService.cancel"]}}`,
		`{"v":1,"ts":"2026-08-06T10:01:01Z","session_id":"s1","prompt_id":"p2","tool_use_id":"u4","tool":"Read","p":{"file_path":"a.go"}}`,
	})
	rec := get(t, newTestServer(t, ws), "/usage")
	wantContains(t, rec, http.StatusOK, "코드 vs DB 스키마 vs 문서 질의", "런 수")

	// Assert inside the target table only. The headline table renders the same
	// "n (p%)" strings, so a body-wide Contains would pass on wrong numbers.
	body := rec.Body.String()
	i := strings.Index(body, "코드 vs DB 스키마 vs 문서 질의")
	section := body[i:]
	if j := strings.Index(section, "</article>"); j > 0 {
		section = section[:j]
	}
	// code adopted 1 of 1, db adopted 0 of 1 — the split is the whole point.
	code := strings.Index(section, "<td>code</td>")
	db := strings.Index(section, "<td>db</td>")
	if code < 0 || db < 0 || code > db {
		t.Fatalf("rows missing or out of order in:\n%s", section)
	}
	if !strings.Contains(section[code:db], "1 (100%)") {
		t.Fatalf("code row should be 100%% adoption:\n%s", section[code:db])
	}
	if !strings.Contains(section[db:], "0 (0%)") {
		t.Fatalf("db row should be 0%% adoption:\n%s", section[db:])
	}
}

func writeUsageEvents(t *testing.T, wsDir string, lines []string) {
	t.Helper()
	dir := filepath.Join(wsDir, "usage")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "events.jsonl"),
		[]byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// A markdown lookup must show up as its own row, not inside code — the reason
// the docs target exists (docs/markdown-spec.md §6).
func TestUsagePageRendersDocsTarget(t *testing.T) {
	ws := newTestWS(t, nil)
	writeUsageEvents(t, ws.Dir, []string{
		`{"v":1,"ts":"2026-08-07T10:00:00Z","session_id":"s1","prompt_id":"p1","tool_use_id":"u1","tool":"mcp__k__search_hybrid","p":{"query":"버저닝","result_ids":["docs/plugin-distribution.md#13-버저닝"]}}`,
		`{"v":1,"ts":"2026-08-07T10:00:01Z","session_id":"s1","prompt_id":"p1","tool_use_id":"u2","tool":"Read","p":{"file_path":"a.go"}}`,
	})
	rec := get(t, newTestServer(t, ws), "/usage")
	wantContains(t, rec, http.StatusOK, "코드 vs DB 스키마 vs 문서 질의")

	body := rec.Body.String()
	i := strings.Index(body, "코드 vs DB 스키마 vs 문서 질의")
	section := body[i:]
	if j := strings.Index(section, "</article>"); j > 0 {
		section = section[:j]
	}
	if !strings.Contains(section, "<td>docs</td>") {
		t.Fatalf("docs row missing:\n%s", section)
	}
	if strings.Contains(section, "<td>code</td>") {
		t.Fatalf("a docs-only run must not appear as code:\n%s", section)
	}
}
