package admin

import "github.com/Salvia95/graphin/internal/search"

// 용어 사전(단일 소스): raw enum → 한국어 라벨. UI 정책은 "한국어 우선 +
// 원어 보조" — 사용자에게 보이는 텍스트만 여기를 거치고, CSS 클래스·URL
// 파라미터·노드 ID 같은 데이터는 raw 값을 유지한다. 미등록 값은 원문
// 그대로 반환한다(안전 폴백 — 새 enum이 추가돼도 UI가 깨지지 않는다).

// stateLabels: internal/workspace/fsm.go의 mcp.Status.State 3상태.
var stateLabels = map[string]string{
	"not_bootstrapped": "부트스트랩 전",
	"indexing":         "인덱싱 중",
	"ready":            "준비됨",
}

// kindLabels: internal/nodeid/nodeid.go의 노드 종류 전수.
var kindLabels = map[string]string{
	"class":       "클래스",
	"interface":   "인터페이스",
	"method":      "메서드",
	"function":    "함수",
	"file":        "파일",
	"table":       "테이블",
	"view":        "뷰",
	"db_function": "DB 함수",
	"procedure":   "프로시저",
	"rls_policy":  "RLS 정책",
	"trigger":     "트리거",
}

// typeLabels: internal/graph/types.go EdgeTypeName의 엣지 유형 전수.
var typeLabels = map[string]string{
	"call":        "호출",
	"import":      "임포트",
	"extends":     "상속",
	"implements":  "구현",
	"reference":   "참조",
	"foreign_key": "외래 키",
}

// matchLabels: internal/search/router.go MatchType 4종.
var matchLabels = map[string]string{
	"exact":    "정확 일치",
	"lexical":  "키워드",
	"semantic": "의미",
	"both":     "키워드+의미",
}

// minConfLabels: 최소 신뢰도 셀렉트 옵션 — confidence 티어(1.0/0.95/0.90/
// 0.80)의 의미를 사람 말로 붙인다. value(숫자 원문)는 그대로 전송된다.
var minConfLabels = map[string]string{
	"0.00": "전체 표시",
	"0.75": "전역 추정 이상",
	"0.80": "전역 추정 이상",
	"0.85": "기본값",
	"0.90": "임포트 티어 이상",
	"0.95": "동일 패키지 이상",
	"1.00": "확실한 참조만",
}

func label(m map[string]string, key string) string {
	if v, ok := m[key]; ok {
		return v
	}
	return key
}

func stateLabel(s string) string { return label(stateLabels, s) }
func kindLabel(k string) string  { return label(kindLabels, k) }
func typeLabel(t string) string  { return label(typeLabels, t) }

// matchLabel accepts the named type so templates can pass .Match directly.
func matchLabel(m search.MatchType) string { return label(matchLabels, string(m)) }

// minConfLabel renders one select option text: "0.85 — 기본값".
func minConfLabel(v string) string {
	if l, ok := minConfLabels[v]; ok {
		return v + " — " + l
	}
	return v
}
