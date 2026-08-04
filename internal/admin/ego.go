package admin

import (
	"math"
	"sort"
)

// ego-graph: 중심 노드 + 1홉 이웃의 좌우 분할 방사형 SVG. 레이아웃은 Go가
// 계산하고 템플릿은 좌표만 찍는다 — 결정적이라 테스트 가능하고 JS가 필요
// 없다. 노드 클릭은 /node 전체 이동(엣지 목록·코드와 항상 동기).

const (
	egoWidth   = 920.0
	egoRadiusX = 300.0
	labelPad   = 12.0
	maxLabel   = 18 // 라벨 rune 상한
)

// egoNode is one positioned neighbor.
type egoNode struct {
	ID       string
	Label    string
	Type     string
	Conf     float32
	Dangling bool
	X, Y     float64
	LabelX   float64
	Anchor   string // text-anchor: start(오른쪽 부채꼴) | end(왼쪽)
	StrokeW  float64
	Opacity  float64
	Dashed   bool
}

// egoLegendItem names one edge colour present in the drawing. Without it the
// stroke colour would be the only carrier of edge type, which the design
// system forbids (DESIGN.md §1·§9, DECISIONS.md E2). Confidence already owns
// the dash/width/opacity channels, so type gets text instead of a pattern.
type egoLegendItem struct {
	Type  string // raw — CSS 클래스용
	Label string // 한국어 라벨
}

// egoData feeds partials/ego.html.
type egoData struct {
	ID         string
	Label      string
	CX, CY     float64
	W, H       float64
	Uses       []egoNode
	UsedBy     []egoNode
	Legend     []egoLegendItem
	MoreUses   bool
	MoreUsedBy bool
	Empty      bool
}

// buildLegend lists the edge types actually drawn, in the canonical order of
// typeLabels rather than discovery order, so the legend does not reshuffle
// when min_conf changes.
var legendOrder = []string{"call", "import", "extends", "implements", "reference", "foreign_key"}

func buildLegend(groups ...[]egoNode) []egoLegendItem {
	present := map[string]bool{}
	for _, g := range groups {
		for _, n := range g {
			present[n.Type] = true
		}
	}
	var out []egoLegendItem
	for _, t := range legendOrder {
		if present[t] {
			out = append(out, egoLegendItem{Type: t, Label: typeLabel(t)})
			delete(present, t)
		}
	}
	// 미등록 유형이 생겨도 범례에서 빠지지 않게 남은 것을 뒤에 붙인다.
	rest := make([]string, 0, len(present))
	for t := range present {
		rest = append(rest, t)
	}
	sort.Strings(rest)
	for _, t := range rest {
		out = append(out, egoLegendItem{Type: t, Label: typeLabel(t)})
	}
	return out
}

func truncLabel(s string) string {
	r := []rune(s)
	if len(r) <= maxLabel {
		return s
	}
	return string(r[:maxLabel-1]) + "…"
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }
func round2(v float64) float64 { return math.Round(v*100) / 100 }

// fanAngle returns the i-th offset (deg) alternating around the horizontal
// axis so the confidence-descending input lands nearest to horizontal first:
// 0, +s, -s, +2s, -2s, …
func fanAngle(i, n int) float64 {
	if n <= 1 {
		return 0
	}
	half := (n - 1 + 1) / 2
	step := math.Min(16, 80/float64(half))
	k := float64((i + 1) / 2)
	if i%2 == 1 {
		return step * k
	}
	return -step * k
}

// buildEgo lays out the first explore page of each direction (confidence
// descending, PageSize rows per side).
func buildEgo(id, display string, uses, usedBy edgeListVM) egoData {
	n := len(uses.Rows)
	if len(usedBy.Rows) > n {
		n = len(usedBy.Rows)
	}
	ry := math.Max(120, math.Min(420, float64(n)*15))
	d := egoData{
		ID:         id,
		Label:      truncLabel(display),
		CX:         egoWidth / 2,
		CY:         ry + 40,
		W:          egoWidth,
		H:          2*ry + 80,
		MoreUses:   uses.NextCursor != "",
		MoreUsedBy: usedBy.NextCursor != "",
		Empty:      len(uses.Rows) == 0 && len(usedBy.Rows) == 0,
	}
	place := func(rows []edgeRowVM, left bool) []egoNode {
		out := make([]egoNode, 0, len(rows))
		for i, r := range rows {
			a := fanAngle(i, len(rows))
			if left {
				a = 180 - a
			}
			rad := a * math.Pi / 180
			x := d.CX + egoRadiusX*math.Cos(rad)
			y := d.CY + ry*math.Sin(rad)
			node := egoNode{
				ID:       r.NodeID,
				Type:     r.Type,
				Conf:     r.Confidence,
				Dangling: r.Dangling,
				X:        round1(x),
				Y:        round1(y),
				StrokeW:  round1(0.8 + 2*float64(r.Confidence)),
				Opacity:  round2(0.35 + 0.6*float64(r.Confidence)),
				Dashed:   r.Confidence <= 0.80,
			}
			label := r.Display
			if label == "" {
				label = r.NodeID
			}
			node.Label = truncLabel(label)
			if left {
				node.Anchor = "end"
				node.LabelX = round1(x - labelPad)
			} else {
				node.Anchor = "start"
				node.LabelX = round1(x + labelPad)
			}
			out = append(out, node)
		}
		return out
	}
	d.Uses = place(uses.Rows, false)
	d.UsedBy = place(usedBy.Rows, true)
	d.Legend = buildLegend(d.Uses, d.UsedBy)
	return d
}
