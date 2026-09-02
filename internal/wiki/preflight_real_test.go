package wiki

import (
	"sort"
	"strings"
	"testing"
)

// selectCase is one task with the sets it must pull in and how many others
// it is currently allowed to pull in alongside them.
type selectCase struct {
	name     string
	task     string
	want     []string
	maxExtra int
}

// runSelectCases runs every case against the real wiki and returns the total
// number of extra sets, so a caller can hold the sum as well as each case.
func runSelectCases(t *testing.T, st *Store, cases []selectCase) (extras int) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := map[string]bool{}
			for _, n := range st.Select("", tc.task).Matched {
				got[n] = true
			}
			var missing, extra []string
			for _, w := range tc.want {
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
			if len(extra) > tc.maxExtra {
				t.Errorf("여분 %d개 %v, 허용 %d — 과매칭 회귀다",
					len(extra), extra, tc.maxExtra)
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
	cases := []selectCase{
		{"release", "read_code가 한 번에 돌려주는 응답 상한을 늘리려 한다, 버전은 어느 자리를 올리고 릴리스 게이트는 어느 계층으로 도나",
			[]string{"release"}, 1}, // 여분: delegation-gate — 질의의 "게이트"가 이름을 직격한다
		{"delegation", "새 서브에이전트를 추가했는데 위키 에이전트 표에 줄을 안 적었다, 위임 게이트는 어떻게 반응하나",
			[]string{"delegation-gate"}, 1}, // 여분: wiki-work — 질의의 "위키"가 이름을 직격한다
		{"adoption", "새 검색 도구를 냈는데 에이전트가 산문 질의로만 쓴다면 발견 실패 지표에 어떻게 잡히나",
			[]string{"adoption"}, 1}, // 여분: delegation-gate — "지표" 그룹을 갖고 있다
		{"console+design", "콘솔에 핀이 드리프트한 항목 목록 화면을 추가한다, API는 어떤 모양이어야 하고 드리프트 행에는 어떤 색 토큰을 쓰나",
			[]string{"console", "design"}, 0},
		{"none-python", "Python 100만 줄 저장소를 인덱싱하면 시맨틱 검색이 켜지나", nil, 0},
		{"none-go", "Go 저장소의 노드 수가 얼마면 시맨틱이 꺼지나", nil, 0},
	}
	extras := runSelectCases(t, st, cases)
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
	cases := []selectCase{
		{"cb-response-cap", "We want read_code to return more per response than it does now. Which file and constant sets that limit today, what is its current value, which version digit does this change move, and which release gate tier has to run?",
			[]string{"release"}, 2}, // 여분: delegation-gate — "gate"가 이름을 직격한다
		{"cb-unlisted-agent", "A new subagent type is added to the plugin but nobody adds a line for it to the wiki's agent table. What does the delegation gate do when something tries to spawn it, and which function decides that?",
			[]string{"delegation-gate"}, 2}, // 여분: wiki-work — "wiki"가 이름을 직격한다
		{"cb-prose-only-tool", "Suppose we ship a new search tool and agents only ever call it with prose questions, never symbol names. How would that show up in the discovery-failure metric, and which function makes that call?",
			[]string{"adoption"}, 1},
		{"cb-console-drift-screen", "We want a console screen listing entries whose pins have drifted. What shape must the API take and which existing function should it serve unchanged, and which colour token marks a drifted row rather than yellow?",
			[]string{"console", "design"}, 1},
		{"cb-python-semantic", "A Python repository of about one million lines is indexed. Does semantic search come up? Name the constant that decides it, its default, and the number it is compared against for a corpus that size.",
			nil, 1},
		{"cb-go-semantic", "A Go repository of about 1.7 million lines is indexed. Does semantic search come up for it? Give the node count that decides it and say what the real unit of that limit is.",
			nil, 1},
		{"wiki-release-digit", "We are releasing a change that renames one MCP tool. Which version digit moves, and which tier of the release gate has to run before dispatch?",
			[]string{"release"}, 2}, // 여분: delegation-gate
		{"wiki-forged-token", "An agent hands over a token it invented without running preflight. Separately, someone edits a set summary after a token was already issued. What happens in each case, and why?",
			[]string{"delegation-gate"}, 1},
		{"wiki-rescore-compat", "If we change only the rubric's grading rules, versus changing the runner's behaviour, how does each affect whether runs recorded earlier can still be re-scored?",
			[]string{"rag-bench"}, 1},
		{"wiki-console-index", "Why can the console not open the index directly, and where had that same constraint already forced a design decision before the console existed?",
			[]string{"console"}, 2}, // 여분: design — "design decision"이 이름을 직격한다
		{"wiki-severity-colour", "In the console's decision queue, which colour token marks a drifted entry, and why is yellow not used to signal severity?",
			[]string{"design"}, 2}, // 여분: console — 질의가 콘솔의 큐를 말한다
		{"wiki-failure-denominator", "Why does the discovery-failure metric count only symbol-shaped searches, and why was the dot deliberately left out of the characters that mark a pattern as a regex?",
			[]string{"adoption"}, 1},
	}
	extras := runSelectCases(t, st, cases)
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
