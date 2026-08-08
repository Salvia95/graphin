package admin

import (
	"fmt"
	"net/http"
	"path/filepath"
	"sort"

	"github.com/Salvia95/graphin/internal/usage"
)

// usage 채택 대시보드: graphin usage report와 같은 Report를 웹으로 렌더한다.
// 산식은 internal/usage/report.go의 Markdown과 동일하게 유지한다.

type usageGroupRow struct {
	Name string
	usage.GroupMetrics
	Adoption         string // 채택/(채택+폴백)
	LateSwitch       string // 분모 = graphin 사용 윈도우
	DiscoveryFailure string // 분모 = 심볼형 search 있는 윈도우 (usage-spec §4.2)
}

// usageTargetRow is the code/db split (usage-spec §4.3). Separate from
// usageGroupRow because the unit is the run, not the window — sharing a row
// type would put a meaningless 윈도우 수 column next to it.
type usageTargetRow struct {
	Name string
	usage.TargetMetrics
	Adoption string
}

// ratio mirrors report.go: "n (p%)" 또는 분모 0이면 "–".
func ratio(n, denom int) string {
	if denom == 0 {
		return "–"
	}
	return fmt.Sprintf("%d (%.0f%%)", n, 100*float64(n)/float64(denom))
}

// usageBar is one positioned rect of the daily chart.
type usageBar struct {
	X, Y, W, H float64
	Kind       string // adoption | fallback
	Title      string
}

type chartLabel struct {
	X    float64
	Text string
}

type usageChart struct {
	W, H     float64
	Baseline float64
	Bars     []usageBar
	Labels   []chartLabel
	MaxV     int
}

const (
	chartDays    = 30
	chartBarW    = 9.0
	chartDayW    = 24.0
	chartPlotH   = 120.0
	chartBottomH = 18.0
)

// buildUsageChart lays out grouped daily bars (마지막 chartDays일).
func buildUsageChart(days []usage.DayTrend) usageChart {
	if len(days) > chartDays {
		days = days[len(days)-chartDays:]
	}
	c := usageChart{
		W:        float64(len(days))*chartDayW + 10,
		H:        chartPlotH + chartBottomH,
		Baseline: chartPlotH,
	}
	for _, d := range days {
		if d.Adoptions > c.MaxV {
			c.MaxV = d.Adoptions
		}
		if d.Fallbacks > c.MaxV {
			c.MaxV = d.Fallbacks
		}
	}
	if c.MaxV == 0 {
		return c
	}
	scale := (chartPlotH - 10) / float64(c.MaxV)
	for i, d := range days {
		x := float64(i)*chartDayW + 5
		ah := float64(d.Adoptions) * scale
		fh := float64(d.Fallbacks) * scale
		c.Bars = append(c.Bars,
			usageBar{X: round1(x), Y: round1(chartPlotH - ah), W: chartBarW, H: round1(ah),
				Kind: "adoption", Title: fmt.Sprintf("%s 채택 %d", d.Date, d.Adoptions)},
			usageBar{X: round1(x + chartBarW + 1), Y: round1(chartPlotH - fh), W: chartBarW, H: round1(fh),
				Kind: "fallback", Title: fmt.Sprintf("%s 폴백 %d", d.Date, d.Fallbacks)},
		)
		// 날짜 라벨은 5일 간격 + 마지막 날.
		if i%5 == 0 || i == len(days)-1 {
			c.Labels = append(c.Labels, chartLabel{X: round1(x), Text: d.Date[5:]}) // MM-DD
		}
	}
	return c
}

// bigramMatrix fixes axes to the spec §4.1 vocabulary (report.go classOrder와
// 동일 순서), appearing classes only.
type bigramMatrix struct {
	Cols []string
	Rows []bigramRow
}

type bigramRow struct {
	From  string
	Cells []int
}

var bigramOrder = []usage.Class{
	usage.ClassGSearch, usage.ClassGExplore, usage.ClassGRead, usage.ClassGBoot, usage.ClassGBench,
	usage.ClassSearch, usage.ClassRead, usage.ClassAction, usage.ClassOther,
}

func buildBigramMatrix(m map[string]map[string]int) bigramMatrix {
	used := map[string]bool{}
	for from, row := range m {
		used[from] = true
		for to := range row {
			used[to] = true
		}
	}
	var cols []string
	for _, c := range bigramOrder {
		if used[string(c)] {
			cols = append(cols, string(c))
			delete(used, string(c))
		}
	}
	var extra []string
	for c := range used {
		extra = append(extra, c)
	}
	sort.Strings(extra)
	cols = append(cols, extra...)

	out := bigramMatrix{Cols: cols}
	for _, from := range cols {
		row := bigramRow{From: from}
		for _, to := range cols {
			row.Cells = append(row.Cells, m[from][to])
		}
		out.Rows = append(out.Rows, row)
	}
	return out
}

type usageVM struct {
	pageVM
	Missing bool
	Dir     string

	R       usage.Report
	Groups  []usageGroupRow
	Targets []usageTargetRow // 비어 있으면 이 워크스페이스에 db 질의가 없었다
	Shapes  []usageShapeRow  // 탐색 실패 분모가 왜 그 크기인지 보여 준다
	Funnel  string           // adherence ratio ("" = 표시 안 함)
	Chart   usageChart
	Bigram  bigramMatrix
}

// usageShapeRow is one search-pattern shape and how often it appeared. The
// table exists so the discovery-failure denominator is auditable on the page,
// not only in the CLI report — a rate whose denominator is invisible is the
// thing this metric was fixed for.
type usageShapeRow struct {
	Name    string // 한국어 라벨
	Shape   string // 원어 (symbol/regex/literal/none)
	Count   int
	Example string
	Counted bool // 탐색 실패 분모에 들어가는가
}

var usageShapes = []struct {
	shape   usage.Shape
	name    string
	example string
	counted bool
}{
	{usage.ShapeSymbol, "심볼", "CONTEXT_TYPES · validate_and_seal · class PublishBody", true},
	{usage.ShapeRegex, "정규식·glob", "^ *$ · *.go · ^###", false},
	{usage.ShapeLiteral, "리터럴·산문", "database is locked · 한국어 문장", false},
	{usage.ShapeNone, "패턴 없음", "패턴을 파싱하지 못한 셸 검색", false},
}

func (s *Server) handleUsage(w http.ResponseWriter, r *http.Request) {
	dir := filepath.Join(s.ws.Dir, "usage")
	vm := usageVM{pageVM: s.pageVM("usage"), Dir: dir}

	events, problems, err := usage.Load(dir)
	if err != nil {
		vm.Missing = true
		s.renderPage(w, "usage.html", vm)
		return
	}
	rep := usage.Compute(events, problems, usage.Options{})
	vm.R = rep
	for _, name := range []string{"all", "main", "subagent"} {
		g, ok := rep.Groups[name]
		if !ok || g.Windows == 0 {
			continue
		}
		vm.Groups = append(vm.Groups, usageGroupRow{
			Name:             name,
			GroupMetrics:     g,
			Adoption:         ratio(g.Adoptions, g.Adoptions+g.Fallbacks),
			LateSwitch:       ratio(g.LateSwitches, g.WindowsWithGraphin),
			DiscoveryFailure: ratio(g.DiscoveryFailures, g.WindowsWithSymbolSearch),
		})
	}
	// Only when something other than code was touched: a workspace with no
	// schema snapshot would otherwise carry a permanent "db 0%" row, and "no
	// db queries were made" must not look like "graphin fails on db queries".
	if rep.Targets["db"].Runs > 0 || rep.Targets["docs"].Runs > 0 {
		for _, name := range []string{"code", "db", "docs"} {
			t := rep.Targets[name]
			if t.Runs == 0 {
				continue
			}
			vm.Targets = append(vm.Targets, usageTargetRow{
				Name:          name,
				TargetMetrics: t,
				Adoption:      ratio(t.Adoptions, t.Adoptions+t.Fallbacks),
			})
		}
	}
	for _, s := range usageShapes {
		if n := rep.SearchShapes[string(s.shape)]; n > 0 {
			vm.Shapes = append(vm.Shapes, usageShapeRow{
				Name: s.name, Shape: string(s.shape), Count: n,
				Example: s.example, Counted: s.counted,
			})
		}
	}
	if g, ok := rep.Groups["all"]; ok && g.FunnelSearches > 0 {
		vm.Funnel = fmt.Sprintf("%s (%d/%d)", ratio(g.FunnelAdherent, g.FunnelSearches),
			g.FunnelAdherent, g.FunnelSearches)
	}
	vm.Chart = buildUsageChart(rep.Daily)
	vm.Bigram = buildBigramMatrix(rep.Bigrams)
	s.renderPage(w, "usage.html", vm)
}
