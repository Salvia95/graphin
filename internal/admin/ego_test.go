package admin

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestFanAngleAlternatesAroundHorizontal(t *testing.T) {
	if a := fanAngle(0, 7); a != 0 {
		t.Fatalf("first item must sit on the horizontal axis, got %v", a)
	}
	// 교대로 +s, -s, +2s, -2s …
	s1, s2 := fanAngle(1, 7), fanAngle(2, 7)
	if s1 <= 0 || s2 >= 0 || s1 != -s2 {
		t.Fatalf("alternation broken: %v %v", s1, s2)
	}
	if a3, a1 := fanAngle(3, 7), fanAngle(1, 7); a3 <= a1 {
		t.Fatalf("later items must fan wider: %v vs %v", a3, a1)
	}
	// 부채꼴 상한 80°.
	if a := fanAngle(29, 30); a < -80.01 || a > 80.01 {
		t.Fatalf("angle out of fan: %v", a)
	}
}

func TestBuildEgoLayoutDeterministic(t *testing.T) {
	uses := edgeListVM{
		Rows:       []edgeRowVM{{NodeID: "a.B.c()", Display: "B.c", Type: "call", Confidence: 0.95}},
		NextCursor: "cur",
	}
	usedBy := edgeListVM{
		Rows: []edgeRowVM{{NodeID: "db.m.s.t", Type: "reference", Confidence: 0.8, Dangling: true}},
	}
	d := buildEgo("pkg.Ctr.run()", "Ctr.run", uses, usedBy)

	if d.W != 920 || d.CX != 460 || d.CY != 160 || d.H != 320 {
		t.Fatalf("canvas: %+v", d)
	}
	u := d.Uses[0]
	if u.X != 760 || u.Y != 160 || u.Anchor != "start" || u.LabelX != 772 {
		t.Fatalf("uses layout: %+v", u)
	}
	if u.Dashed || u.StrokeW != 2.7 || u.Opacity != 0.92 {
		t.Fatalf("uses stroke: %+v", u)
	}
	b := d.UsedBy[0]
	if b.X != 160 || b.Y != 160 || b.Anchor != "end" || b.LabelX != 148 {
		t.Fatalf("used_by layout: %+v", b)
	}
	if !b.Dangling || !b.Dashed {
		t.Fatalf("0.80 dangling row flags: %+v", b)
	}
	if b.Label != "db.m.s.t" {
		t.Fatalf("dangling label fallback: %q", b.Label)
	}
	if !d.MoreUses || d.MoreUsedBy {
		t.Fatalf("more flags: %+v", d)
	}

	if e := buildEgo("x", "x", edgeListVM{}, edgeListVM{}); !e.Empty {
		t.Fatal("empty ego must flag Empty")
	}
}

func TestNodePageRendersEgoSVG(t *testing.T) {
	s, ws := bootstrappedServer(t)
	runID := findNode(t, ws, "Flow.run()")
	rec := get(t, s, "/node?id="+url.QueryEscape(runID))
	body := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.Contains(body, "<svg") {
		t.Fatalf("no svg in node page (code %d)", rec.Code)
	}
	for _, want := range []string{"ncenter", "el call", "/node?id="} {
		if !strings.Contains(body, want) {
			t.Fatalf("svg missing %q", want)
		}
	}
}

// 엣지 유형이 색으로만 구분되면 안 된다(DECISIONS.md E2). 범례가 색에
// 이름을 붙이고, 선 자체도 <title>로 유형을 말한다.
func TestEgoEdgeTypeIsNotColourOnly(t *testing.T) {
	s, ws := bootstrappedServer(t)
	runID := findNode(t, ws, "Flow.run()")
	body := get(t, s, "/node?id="+url.QueryEscape(runID)).Body.String()

	if !strings.Contains(body, `class="egolegend"`) {
		t.Fatal("ego legend missing — edge colour would be unlabelled")
	}
	if !strings.Contains(body, `<i class="sw call"`) || !strings.Contains(body, "호출") {
		t.Fatal("legend does not name the call edge type")
	}
	if !strings.Contains(body, "<title>호출 · 신뢰도") {
		t.Fatal("edge lines carry no textual type/confidence title")
	}
}

func TestBuildLegendOrderAndDedup(t *testing.T) {
	uses := []egoNode{{Type: "reference"}, {Type: "call"}, {Type: "call"}}
	usedBy := []egoNode{{Type: "import"}}
	got := buildLegend(uses, usedBy)
	want := []string{"call", "import", "reference"} // legendOrder 고정 순서
	if len(got) != len(want) {
		t.Fatalf("legend = %+v", got)
	}
	for i, w := range want {
		if got[i].Type != w {
			t.Fatalf("legend[%d] = %q, want %q", i, got[i].Type, w)
		}
	}
	if got[0].Label != "호출" {
		t.Fatalf("legend label not localised: %q", got[0].Label)
	}
}
