# 스코프와 커버리지 (2026-08-11)

무엇을 하겠다고 했는지, 실제로 무엇을 덮는지, 남들이 무엇을 덮는지, 그래서
어디로 갈지. 수치는 전부 이 저장소의 구현과 2026-08 시점 조사에서 나왔다.

## 0. 선언한 스코프

README의 한 줄은 좁고 분명하다 — **AI 에이전트를 위한 로컬 코드베이스 탐색 MCP
서버**. 목표는 검색 정확도가 아니라 **토큰 소모 최소화**이고, 수단은 3단계
점진적 공개다(search → explore → read). PRD는 >500k 노드 모노레포를 v1 범위
밖으로 명시한다.

이 문장에서 **"코드"와 "탐색"이 실제로 어디까지인지**가 이 문서의 주제다.

## 1. 실제 커버리지

| 축 | 커버하는 것 | 근거 |
|---|---|---|
| 문법 언어 | Java · Kotlin · Python · JavaScript · TypeScript/TSX · **Go** — 6종 | `internal/parse/parse.go` |
| 문법 없는 색인 | 마크다운(섹션 노드) · plain 12확장자 · `*.graphindb.json` | 파일 = 노드 1개 |
| 노드 종류 | class · interface · method · function · file · section · DB(table/view/function) | `internal/nodeid` |
| 엣지 종류 | import · extends · implements · call · reference · foreign_key · contains — **7종** | `internal/graph/types.go` |
| 검색 | Tier-0 exact → BM25 → RRF 하이브리드(multilingual-e5-small INT8) | `internal/search/router.go` |
| 해석 정밀도 | **타입 해석 없음.** 이름 + 아리티 + 스코프 티어(1.0 / 0.95 / 0.90 / 0.80) | `nodeid.go`: "no type resolution" |
| 규모 | semantic 40,000 노드 게이트(초과 시 lexical 폴백), >500k 범위 밖 | `cmd/graphin/main.go:68` |
| 플랫폼 | linux amd64·arm64 릴리스. darwin/amd64는 의미 검색 불가, windows 범위 밖 | README |

## 2. 가장 큰 간극은 언어였고, 대가를 우리가 치르고 있었다 (해소됨)

이 문서를 처음 쓴 시점에 `.go`는 라우팅 표에 없었고 `internal/scan/walk.go:85`가
아예 걸러냈다. 즉 **graphin은 Go로 쓰였는데 자기 자신을 색인하지 못했다.**

실측이 그것을 그대로 보여 줬다 — 이 저장소에서 Go 구현 세부를 검색하면 결과가
**전부 마크다운 문서**였고 소스는 한 줄도 안 나왔다. 파장은 도구 자랑에
그치지 않았다:

- **도그푸딩이 불가능했다.** 2026-08-08~11에 찾아낸 결함 셋(`contains` 낡음,
  Tier-0 미발화, 가드 슬러그 불일치)을 전부 `grep`으로 찾았다. 우리 도구가
  풀라고 만들어진 문제를 우리 도구 없이 풀고 있었다.
- **채택 측정의 대조군이 반쪽이었다.** 이 저장소의 usage 이벤트에서 graphin이
  답할 수 있는 질문은 문서뿐이라, kinder(Python)와 나란히 놓을 수 없었다.

**2026-08-11에 Go 파서를 넣어 해소했다**(§5-①). 같은 질의가 이제 이렇게 답한다:

```
search_hybrid("ApplyFile")
  1. internal.graph.Engine.ApplyFile        engine.go:186     exact
  2. internal.workspace.Workspace.applyFileResult  indexer.go:82
explore_graph("internal.graph.Engine.ApplyFile")
  uses:    dropRecordLocked 1.00 · resolver.register 0.95 · nodeid.Class 0.90
  used_by: TestContainsRefreshesWhenChildAdded 0.95 · applyAll 0.95
```

신뢰도 사다리가 Go에서 그대로 산다 — 같은 파일 1.00, 같은 패키지 0.95, import된
패키지 0.90. 설계 메모는 `internal/parse/golang.go`에 있다.

## 3. 남들이 덮는 우리 공백 (2026-08 조사)

| 공백 | 덮는 쪽 | 방식 |
|---|---|---|
| 언어 60+ · 정확한 심볼 해석 | [Serena](https://github.com/oraios/serena) (25.2k★) | LSP 브리지 — 남의 언어서버를 빌린다 |
| 정밀 인덱스 · 크로스 레포 | [Sourcegraph MCP](https://sourcegraph.com/mcp) | SCIP 표준 |
| 로컬 그래프 (우리와 같은 축) | CodeGraph(47.4k★) · GitNexus(42k★) | 온디바이스 사전 계산 |
| 벡터 검색 | [claude-context](https://github.com/zilliztech/claude-context) | 하이브리드 BM25+dense |
| 데이터플로우 | [Code Pathfinder](https://codepathfinder.dev/mcp) | 정적 분석 |
| 컨텍스트 패킹 | Repomix(26.2k★) | tree-sitter 압축, 인덱스 없음 |

시장은 **로컬 우선 그래프**로 수렴했다([비교 분석](https://rywalker.com/research/code-intelligence-tools)).
아키텍처 선택은 옳았지만 그 축에 이미 우리보다 큰 것들이 있고, **언어 커버리지는
LSP 브리지가 구조적으로 이긴다** — 우리가 파서를 하나씩 쓰는 동안 저쪽은 이미
있는 언어서버를 빌린다.

## 4. 조사한 어느 툴도 하지 않는 것

- **코드 ↔ DB 크로스 엣지.** 스키마 스냅샷을 1급 노드로 넣고 SQL 리터럴에서
  테이블 참조를 잇는다(Phase 7). 위 어느 카테고리에도 없었다.
- **마크다운을 1급 노드로** + knowledge set. 문서 섹션이 노드이고 에이전트가
  요약만 훑은 뒤 정확히 로드한다.
- **채택 계측.** *에이전트가 실제로 이 도구를 쓰는지*를 재는 장치. 다른 툴들은
  자기가 유용하다고 주장하지만 **버려지는지를 재지 않는다.**
- **바이트 예산을 e2e가 단언한다.** "grep -C20 대비 몇 바이트"가 통과 조건이다.

넷의 공통점: **코드 자체가 아니라 코드의 바깥**이다. 에이전트가 실제로 막히는
곳도 거기다 — 코드와 스키마·문서 사이.

## 5. 결정 (2026-08-11, 사용자 승인)

**① Go 파서를 추가한다. ✅ 완료 (2026-08-11)** 커버리지가 아니라 **도그푸딩**이
이유다. 언어 하나가 "우리가 우리를 측정할 수 있는가"를 가른다. 의존성도 새
종류가 아니었다 — 문법마다 별도 모듈이라는 기존 패턴(`tree-sitter-java` 외
3종)에 `tree-sitter-go` 하나가 붙었다.

구현하며 정한 것 셋:
- **패키지는 디렉터리다**(`internal/graph/engine.go` → `internal.graph`).
  Go의 패키지가 실제로 디렉터리이므로 `FileScoped()`는 false이고, 같은 폴더의
  파일들이 하나의 같은-패키지 티어를 공유한다.
- **메서드는 리시버 타입에 붙는다** — `func (e *Engine) ApplyFile` →
  `internal.graph.Engine.ApplyFile`. 포인터·바인더 이름·타입 파라미터는 뗀다.
- **import는 경로와 두 segment 꼬리를 함께 기록한다.** 파싱은 파일 단위 순수
  함수라 `go.mod`를 읽어 모듈 접두사가 어디서 끝나는지 알 수 없다. 꼬리
  (`internal.merkle.*`)가 저장소 안 패키지 ID와 맞물리는 결정론적 근사이고,
  이것이 없으면 저장소 내부 크로스 패키지 호출이 전부 전역 티어(0.80)로 떨어진다.
- **구조체 임베딩은 supertype이 아니다.** 인터페이스 임베딩만 `extends`의
  의미를 갖는다.

**② LSP 브리지는 하지 않는다.** 60+ 언어를 얻는 대신 단일 바이너리·무의존·
결정론이라는 전제 셋이 동시에 깨진다. Serena가 이미 잘하는 게임이고 뒤늦게
들어가 이길 이유가 없다. **정밀도 경쟁도 같다** — 신뢰도 4티어는 타입 해석을
안 한다는 사실의 정직한 표현이고, 그 정직함이 자산이다.

**③ 이긴 축을 굳힌다.** DB·문서·계측은 남이 안 하는 지점이고 셋 다 "코드 밖"
이다. 언어 수를 좇는 것보다 방어 가능하다.

**④ 방향은 데이터가 정한다.** 채택 재측정(진단 권고 ③)이 ④의 랭킹 변경이
실제로 채택을 올렸는지 답하기 전까지, 다음 큰 투자는 정하지 않는다. 이
저장소는 H1에서 집계 평균만 보고 결론을 뒤집힌 적이 있다.

## 6. 하지 않기로 한 것

- LSP·SCIP 채택 (§5-②)
- 크로스 레포 — 로컬 단일 워크스페이스가 전제다
- 데이터플로우 — 정적 분석은 타입 해석을 요구하고, 그건 §5-②와 같은 이유로
  범위 밖이다
- 클라우드 인덱싱 — 로컬·무네트워크가 제품의 전제다
