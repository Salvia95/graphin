package wiki

import (
	"encoding/json"
	"os"
	"sort"
	"strings"
	"testing"
)

// selectCase is one task with the sets it must pull in and how many others it
// is currently allowed to pull in alongside them. Fields are exported and
// tagged because the cases are loaded from testdata/select_cases.json, not
// embedded here: the "en" group is the eval/combined and eval/wiki runners'
// questions verbatim plus their expected sets, so it is measurement apparatus
// and must live where the bench corpora cut it — a source file would ship the
// questions and the answer-shaped `want` inside the corpus under test.
type selectCase struct {
	Name     string   `json:"name"`
	Task     string   `json:"task"`
	Want     []string `json:"want"`
	MaxExtra int      `json:"maxExtra"`
}

// loadSelectCases reads one named group from testdata/select_cases.json. The
// test runs with its package directory as the working directory, so the
// conventional testdata/ path resolves without a repo-root anchor.
func loadSelectCases(t *testing.T, group string) []selectCase {
	t.Helper()
	b, err := os.ReadFile("testdata/select_cases.json")
	if err != nil {
		t.Fatalf("read select cases: %v", err)
	}
	// Parse group-by-group so sibling keys that are not case arrays (e.g. the
	// file's "_comment") are left untouched.
	var all map[string]json.RawMessage
	if err := json.Unmarshal(b, &all); err != nil {
		t.Fatalf("parse select cases: %v", err)
	}
	raw, ok := all[group]
	if !ok {
		t.Fatalf("no select-case group %q", group)
	}
	var cases []selectCase
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatalf("parse select-case group %q: %v", group, err)
	}
	if len(cases) == 0 {
		t.Fatalf("select-case group %q is empty", group)
	}
	return cases
}

// runSelectCases runs every case against the real wiki and returns the total
// number of extra sets, so a caller can hold the sum as well as each case.
func runSelectCases(t *testing.T, st *Store, cases []selectCase) (extras int) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			got := map[string]bool{}
			for _, n := range st.Select("", tc.Task).Matched {
				got[n] = true
			}
			var missing, extra []string
			for _, w := range tc.Want {
				if !got[w] {
					missing = append(missing, w)
				}
				delete(got, w)
			}
			for n := range got {
				extra = append(extra, n)
			}
			sort.Strings(extra)
			extras += len(extra)
			if len(extra) > 0 {
				t.Logf("여분 %v", extra)
			}
			if len(missing) > 0 {
				t.Errorf("놓친 세트 %v — 재현율 회귀다", missing)
			}
			if len(extra) > tc.MaxExtra {
				t.Errorf("여분 %d개 %v, 허용 %d — 과매칭 회귀다",
					len(extra), extra, tc.MaxExtra)
			}
		})
	}
	return extras
}

// 이 저장소의 실제 위키를 코퍼스로 preflight의 매칭 정확도를 고정한다.
//
// 픽스처가 아니라 실물을 쓰는 이유는 2026-09-02 통합 벤치가 실물에서만 드러나는
// 실패를 잡았기 때문이다 — 세트가 셋뿐인 픽스처에서는 큰 세트가 아무 작업에나
// 붙는 과매칭이 재현되지 않는다.
//
// **재현율은 강제하고 여분은 센다.** 정답 세트를 놓치는 것은 회귀이지만, 여분은
// 지금 0이 아니고 그것을 숨기면 개선 여지가 테스트에서 사라진다. maxExtra는 목표가
// 아니라 현재 상태의 기록이며, 줄어들면 함께 줄인다.
func TestSelectOnRealWiki(t *testing.T) {
	st, err := Load("../..")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(st.Sets) < minSetsForCommonStop+1 {
		t.Skipf("위키에 세트가 %d개뿐 — 이 테스트는 실물 규모를 전제한다", len(st.Sets))
	}
	extras := runSelectCases(t, st, loadSelectCases(t, "ko"))
	// 총계를 남긴다. 개별 상한을 지켜도 합이 늘면 매칭이 느슨해진 것이다.
	if extras > 2 {
		t.Errorf("여분 총계 %d — 기록된 2를 넘었다", extras)
	}
	t.Logf("여분 총계 %d (2026-09-02 수정 전: 8)", extras)
}

// TestSelectOnRealWikiEnglish는 같은 위키에 영어 질의를 던진다.
//
// 라벨은 전부 한국어라서, 별칭이 없던 2026-09-02에는 영어 질의가 세트 이름의 영어
// 슬러그로만 닿았다 — 열두 질의 중 정답 세트 재현 5/11, 위키가 답을 가진 문항
// 넷이 빈 카탈로그로 끝났다. 빈 카탈로그가 가장 비싼 실패이므로(wiki-plan §1.4)
// 여분 상한은 한국어 표보다 느슨하게 둔다. 질의는 eval/combined와 eval/wiki
// 러너의 문항 그대로다.
func TestSelectOnRealWikiEnglish(t *testing.T) {
	st, err := Load("../..")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(st.Sets) < minSetsForCommonStop+1 {
		t.Skipf("위키에 세트가 %d개뿐 — 이 테스트는 실물 규모를 전제한다", len(st.Sets))
	}
	extras := runSelectCases(t, st, loadSelectCases(t, "en"))
	// 기록은 5이고 상한은 그보다 느슨하다. 여기서 여분은 전부 "정답 + 하나"이고,
	// 에이전트가 걸러내는 값이 빈 카탈로그를 받는 값보다 싸다.
	if extras > 7 {
		t.Errorf("여분 총계 %d — 기록된 5에 여유 2를 더한 상한을 넘었다", extras)
	}
	t.Logf("영어 여분 총계 %d (별칭 전: 5, 그때 재현 5/11 · 지금 11/11)", extras)
}

// TestCommonKeysAreStopped는 여러 세트가 공유하는 라벨 키가 실제로 걸러지는지
// 본다. 이 규칙이 조용히 꺼지면 위 테스트는 여분 상한으로만 알아채는데, 그때는
// 원인이 어디인지 알 수 없다.
func TestCommonKeysAreStopped(t *testing.T) {
	st, err := Load("../..")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	sets := st.SetList()
	if len(sets) < minSetsForCommonStop {
		t.Skip("세트가 적어 규칙이 적용되지 않는다")
	}
	stop := st.stopKeys()
	count := map[string]int{}
	for _, s := range sets {
		for k := range keySet(setText(s)) {
			count[k]++
		}
	}
	// stopKeys와 같은 문턱(내림)이다. 올림으로 두면 여기서는 통과하는데 실제
	// 규칙은 더 많이 거르는 상태가 되어, 이 테스트가 규칙의 절반만 본다.
	limit := len(sets) / 2
	if limit < 2 {
		limit = 2
	}
	var leaked []string
	for k, n := range count {
		if n >= limit && !stop[k] {
			leaked = append(leaked, k)
		}
	}
	if len(leaked) > 0 {
		sort.Strings(leaked)
		t.Errorf("세트 %d개 중 %d개 이상이 공유하는데 걸러지지 않은 키: %s",
			len(sets), limit, strings.Join(leaked, " "))
	}
}
