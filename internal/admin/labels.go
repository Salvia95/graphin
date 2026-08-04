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

// helpTerm is one ⓘ popover body (DESIGN.md §4.3·P6). Title keeps the English
// source term because that is what appears in logs and flags; Body explains it
// in Korean. More is optional operational advice.
type helpTerm struct {
	Title string
	Body  string
	More  string
}

// helpTerms is the term dictionary behind /help/{term}. Keys are stable URL
// slugs — they are not user-visible, so they stay raw like the enum values.
var helpTerms = map[string]helpTerm{
	"dangling": {
		Title: "dangling edge — 끊어진 엣지",
		Body: "출발 노드는 있는데 대상 ID에 해당하는 노드가 인덱스에 없는 엣지입니다. " +
			"코드 도메인에서는 대개 대상이 삭제됐거나 아직 해석되지 않은 흔적입니다.",
		More: "DB 도메인의 끊어진 엣지는 스냅샷 밖 참조(예: auth.users)를 스텁 없이 남긴 의도적 설계일 수 있습니다.",
	},
	"partial": {
		Title: "partial — 부분 인덱싱",
		Body: "구문 오류가 있는 채로 파싱된 파일의 노드입니다. 파서가 ERROR 노드를 만난 구간은 " +
			"엣지가 누락될 수 있어 탐색 결과가 불완전합니다.",
		More: "소스를 고치면 워처가 자동으로 재인덱싱합니다.",
	},
	"lexical": {
		Title: "lexical — 키워드 검색",
		Body:  "BM25 기반의 문자열 검색입니다. 식별자를 정확히 알 때 가장 빠르고, 인덱싱이 끝나는 즉시 사용할 수 있습니다.",
	},
	"semantic": {
		Title: "semantic — 의미 검색",
		Body: "임베딩 벡터의 코사인 유사도로 찾는 검색입니다. '주문 취소 흐름' 같은 자연어 질의에 쓰이며, " +
			"임베딩 워밍업이 끝나야 동작합니다.",
		More: "준비 전이나 실패 시에는 키워드 검색으로 자동 폴백합니다.",
	},
	"semantic_gate": {
		Title: "semantic gate — 의미 검색 게이트",
		Body: "노드 수가 --semantic-max-nodes 상한을 넘으면 임베딩을 아예 시작하지 않는 안전장치입니다. " +
			"대형 저장소에서 임베딩이 디스크와 시간을 과도하게 쓰는 것을 막습니다.",
		More: "상한을 올리고 재기동하면 게이트 마커가 해제되고 재임베딩이 시작됩니다.",
	},
	"embed_pending": {
		Title: "embed pending — 임베딩 대기",
		Body:  "아직 벡터가 만들어지지 않은 노드 수입니다. 백로그(backlog)와 처리 중(in-flight)을 합한 값입니다.",
	},
	"confidence": {
		Title: "confidence — 신뢰도",
		Body: "엣지가 실제 참조일 가능성입니다. 1.00은 확실한 참조, 0.95는 동일 패키지 추정, " +
			"0.90은 임포트 기반 추정, 그 아래는 전역 이름 추정입니다.",
		More: "최소 신뢰도를 올리면 추정 엣지가 빠지고 정확도가 올라갑니다.",
	},
	"shard": {
		Title: "shard — 샤드",
		Body: "그래프를 패키지 단위로 나눈 저장 단위입니다. 파일이 바뀌면 해당 샤드만 다시 쓰므로 " +
			"증분 인덱싱이 빨라집니다.",
	},
	"reverse_index": {
		Title: "reverse index — 역인덱스",
		Body: "'참조됨(used_by)'을 상수 시간에 답하기 위한 역방향 색인입니다. 압축 베이스와 " +
			"툼스톤 델타 로그로 관리됩니다.",
	},
	"delta_log": {
		Title: "delta log — 델타 로그",
		Body: "역인덱스의 변경분을 append-only로 쌓는 로그입니다. 10MB를 넘거나 툼스톤 비율이 " +
			"30%를 넘으면 자동으로 압축됩니다.",
	},
	"uses": {
		Title: "uses / used_by — 참조함 / 참조됨",
		Body: "참조함은 이 노드가 호출·임포트·상속하는 대상이고, 참조됨은 반대로 이 노드를 가리키는 " +
			"노드들입니다. 영향 범위를 볼 때는 참조됨을 봅니다.",
	},
	"adoption": {
		Title: "adoption — 채택률",
		Body: "graphin 호출 직후 Read/Edit로 이어진 비율입니다. 채택/(채택+폴백)으로 계산하며, " +
			"에이전트가 원하는 것을 실제로 찾았는지를 뜻합니다.",
	},
	"fallback": {
		Title: "fallback — 폴백",
		Body: "graphin 호출 직후 Grep으로 후퇴한 경우입니다. 결과가 불만족스러웠다는 신호이며, " +
			"인덱스와 랭킹을 고칠 실측 테스트케이스가 됩니다.",
	},
	"same_intent": {
		Title: "same-intent — 동일 의도",
		Body: "graphin 질의와 뒤이은 grep 패턴의 토큰이 겹치는 폴백입니다. 같은 것을 찾다가 실패한 " +
			"경우라 search_hybrid가 놓친 리터럴 재현 케이스입니다.",
		More: "토큰이 겹치지 않는 폴백은 그냥 새 검색이지 graphin의 실패가 아닙니다.",
	},
}

// stateBadges maps the 3 FSM states onto the design system's 5 semantic badge
// axes (DECISIONS.md §3.2). Unknown states fall back to neutral rather than
// rendering an unstyled badge.
var stateBadges = map[string]string{
	"not_bootstrapped": "neutral",
	"indexing":         "info",
	"ready":            "ok",
}

func stateBadge(s string) string {
	if v, ok := stateBadges[s]; ok {
		return v
	}
	return "neutral"
}

// matchLabel accepts the named type so templates can pass .Match directly.
func matchLabel(m search.MatchType) string { return label(matchLabels, string(m)) }

// minConfLabel renders one select option text: "0.85 — 기본값".
func minConfLabel(v string) string {
	if l, ok := minConfLabels[v]; ok {
		return v + " — " + l
	}
	return v
}
