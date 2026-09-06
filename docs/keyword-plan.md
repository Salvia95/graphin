# 키워드 검색 개선 계획 — 노드 id 계약은 두고, 비는 자리를 메운다 (2026-09-05)

`search_keyword`를 grep 래퍼로 바꾸지 않는다. 히트가 소유 노드 id를 달고 나와
`explore_graph`·`read_code`로 이어지는 계약은 검색 품질(정확한 슬라이스만
읽는다)과 모니터링(퍼널·지어낸 id 탐지) 양쪽의 근거다. 대신 그 계약이 **비는
자리 넷**을 메우고, 메운 것을 **결정론 축과 행동 축 둘 다에서** 잰다.

## 진행 상태 (2026-09-06 갱신)

**이어받을 때는 [docs/handoff-2026-09-06.md](handoff-2026-09-06.md)를 먼저 읽는다** —
이 표 뒤에 grep 대조군(v2)과 그에 대한 비판, 다음 한 수가 거기 있다.

| 단계 | 상태 | 기록 |
|---|---|---|
| 1 P4 계측 승격 | **완료** — 분류·same-intent·퍼널·검색기별 표, 테스트, spec | `internal/usage`, `docs/usage-spec.md` |
| 2 M2 채점기 + 기준선 | **완료(단서 있음)** — 루브릭 1.3.1(관찰 3종), 스모크 18런 재채점, 변경 전 77/81런(429 중단, 관찰 지표만) + 프롬프트만 변경 81런(스킬 편집 뒤 재실행한 순서 실수, 대조 팔로 전용) **→ 1.3.3(2026-09-06) 재채점: 표기 규칙 둘 수정, B 73/81 · grep 76/81 · A 67/77, [기록](eval/2026-09-06-rescore-1.3.3/findings.md)** | [2026-09-05-keyword-context](eval/2026-09-05-keyword-context/findings.md) |
| 3 P1 context · P3 힌트 · M1 · M3 | **완료** — 테스트 통과, 리터럴 힌트 산문 오발화 0/23, 키워드 팔 3/3, 대조군 451/451 동일(md5 불변) | 같은 디렉터리 |
| 4 릴리스 | **소유자 결정 대기(2026-09-06 2차)** — 재검토 개선 넷 뒤 D′가 사전 등록 기준 10 중 9 충족, 미충족은 db-nav 한 태스크. 찬성 시 patch 0.4.12 + 가이드 0.6.3 | [§2b](#2b-2026-09-06-재검토-라운드--개선-넷과-d의-사전-등록-기준) · [p1-fold §4·§5](eval/2026-09-06-p1-fold/findings.md) |
| 5 rag 재측정 | **완료(릴리스 앞으로 당김)** — D 74/81 · B 73 · C 76, p=1.0 둘. P1 기제 실측(히트 뒤 `read_code` 후속 47→22), 재검색·통째 Read 불변, 바이트 +15%, `--semantic`은 부분 하이브리드 | [p1-hybrid](eval/2026-09-06-p1-hybrid/findings.md) |
| 6 P2 판정 | **구현 안 함(확정)** — D′에서 키워드 뒤 통째 Read 3/81런, id 있는 히트 뒤 0%. 창이 그 자리를 대신했다 | [p1-fold §3](eval/2026-09-06-p1-fold/findings.md) |

09-01 재베이스라인 81런의 트랜스크립트가 남아 있지 않아 "공짜 재채점"은 불가했고,
변경 전 바이너리·스킬로 기준선을 한 번 더 돌렸다(2단계). 그 런이 80/81에서
사용량 한도로 끊겼고, 재실행은 스킬 본문이 이미 P1 편집된 뒤라 "프롬프트만
변경" 팔이 됐다 — 깨끗한 81런 재실행은 소유자 결정. 리터럴 태스크가 rag
골든셋에 없어 `first_retriever`의 리터럴 부분집합은 비어 있다.

## 0. 근거 — 무엇이 비어 있나

벤치 트랜스크립트(스크래치에 남은 163런 중 `search_keyword`를 쓴 27런, 97콜,
sonnet, 이 저장소 코퍼스)와 실사용 로그에서:

| 관찰 | 수치 | 뜻 |
|---|--:|---|
| 결과가 있는데 노드 id가 하나도 없는 응답 | 29 / 77 (38%) | 패키지 상수·노드 밖 라인. 히트에 "다음 수"가 없다 |
| 결과 0건 응답 | 20 / 97 (21%) | 하이브리드는 비면 힌트를 주는데 키워드는 침묵한다 |
| 키워드 뒤 첫 소비 행동 = 재검색 | 42 / 97 (새 패턴 35) | grep 루프를 도구 안에서 돌린다 |
| 키워드 뒤 = 파일 통째 `Read` | 6 / 97 (히트 파일 4) | 주변 컨텍스트를 두 번째 콜로 산다 |
| 키워드 뒤 = 셸 grep | 3 / 97 | 드물다 |
| 돌려준 id를 `read_code`·`explore_graph`에 실제로 넘김 | 13 / 97 | 퍼널이 닫히는 비율 |
| 실사용 로그의 `search_keyword` 이벤트 | 0 / 4,233 | 관측 자체가 없다 |
| usage 분류 | `other` | 있었어도 지표에 안 들어간다 |

그리고 `search_hybrid`의 리다이렉트 힌트는 식별자 모양(snake·camel·CAPS)
토큰에만 반응한다(`internal/search/router.go` `isIdentShaped`). grep이 가장 강한
형태인 산문형 리터럴("database is locked")에서는 사후 유도가 침묵한다.

## 1. 제약 — 바꾸지 않는 것

1. **노드 id 계약.** 키워드 히트는 소유 노드 id를 단다. 텍스트만 돌려주는 모드는
   만들지 않는다.
2. **`internal/keyword`의 매처와 정렬은 불변.** 이 패키지는 SWE-Explore grep
   대조군(`eval/sweexplore.GrepRegions`)을 정의한다. 매칭·랭킹을 건드리면
   대조군이 움직여 벤치 재실행이 필요하다(패키지 헤더 주석). 변경은 **추가
   필드**와 **MCP 계층**(`internal/mcp/tools/tools.go`)에 한정하고, §3 M3로
   불변을 확인한다.
3. **새 MCP 도구 없음**(`docs/phase7-spec.md` §0, 내비게이션 역설). 파라미터와
   응답 속성만 더한다.
4. **기본값은 `docs/eval/` 리포트를 근거로만 바꾼다.** 새 파라미터의 기본값은
   현행 동작과 바이트 동일해야 한다.
5. **응답 12KB 캡**(`mcp.MaxResponseBytes`) 안에서 핸들러가 스스로 예산을 지킨다.
   서버의 `Truncate`에 기대지 않는다.
6. **rag 벤치 채점기·골든셋은 변경 통제 대상.** `scripts/eval-rag.py`와
   `eval/rag/*`는 소유자가 `.rag-bench-unlock`을 두어야 열리고, `lock
   --approved-by`와 `Rag-Bench-Approved-By:` 트레일러가 따른다.
7. 에이전트 프롬프트·스킬 본문이 바뀌면 `agent_sha`가 바뀌어 rag 벤치는 **새
   베이스라인**이다(`docs/rag-bench-spec.md` §3). 도구 설명(서버 쪽)만 바뀌는
   것은 해당 없음.

## 2. 제품 변경

### P1. `search_keyword`에 `context` 파라미터 — "위치 + 주변"을 한 콜로

grep -C가 이기는 단발 질문("이 문자열이 어디 있고 주변은 어떻게 생겼나")을
두 콜에서 한 콜로 만든다. 데이터의 "파일 통째 Read 6건"과 "재검색 42건" 중
일부가 겨냥하는 자리다.

**스키마.** `context` integer, 기본 `0`, 상한 `5`. 상한을 넘기면 거절한다
(모르는 `target`을 거절하는 것과 같은 규칙 — 조용히 줄이면 호출자가 다 받았다고
믿는다).

**엔진(`internal/keyword`).** `searchFile`이 이미 `ContextLines>0`일 때 include
마스크를 만든다. 여기에 **추가 필드**만 더한다: 보존된 `Line`마다
`Window Region`(lo..hi)과 `Context []string`(그 구간의 원문 라인). `Matches`
카운트·`Lines` 선택·정렬·`Regions`(대조군이 쓰는 것)는 한 줄도 바꾸지 않는다.
같은 파일 안에서 앞 창과 겹치는 뒤 창은 앞 창 끝 다음 줄부터 시작해 중복을
없앤다.

**응답(`tools.go` `keywordHandler`).** `context>0`일 때만 형태가 바뀐다.

```
<results retriever="keyword" mode="literal" files="2" node_ids="true" context="2">
  <file path="internal/lock/lockfile.go" matches="3" rank="1">
    <node id="internal.lock.Acquire" line="41" match_type="keyword" window="39-43">
39    if err := flock(fd); err != nil {
40        return nil, err
41>   log.Printf("lock steal: pid %d", pid)
42    }
43    return &amp;Lock{fd: fd}, nil
    </node>
  </file>
  <file path="cmd/graphin/main.go" matches="1" rank="2" omitted="budget" />
</results>
```

- 줄 번호를 앞에 붙여 `file:line` 인용이 바로 되게 한다. 매치 라인은 `>`로
  표시한다.
- `context=0`이면 `<results>`에 `context` 속성을 붙이지 않고 오늘과 **바이트
  동일**하다. e2e로 고정한다.
- **예산.** `keywordBudget = MaxResponseBytes − 512`. 파일 순서대로 채우고,
  다음 파일이 예산을 넘기면 그 파일부터는 `<file path matches rank
  omitted="budget"/>` 한 줄로 접는다. 접힌 파일은 `path` 필터로 다시 부를 수
  있다. 최악 케이스 5파일 × 3라인 × 11줄 × 80B ≈ 13KB가 캡을 넘기므로 예산이
  실제로 작동하는 경로다.
- 라인 텍스트 절단(`keywordMaxText` 160)은 컨텍스트 라인에도 같이 적용한다.

**테스트.**
- 단위(`internal/keyword`): 파일 첫·끝 줄 경계, 겹치는 창의 중복 제거,
  `context=0`에서 `Context`가 nil, **`Regions`와 `Matches`가 이전과 동일**.
- e2e(`e2e/search_keyword_test.go` 패턴): `context=2`에서 인접 줄이 보인다,
  `context=0`은 기존 응답과 바이트 동일, `context=6`은 거절, 큰 픽스처에서
  `omitted="budget"`가 나오고 총 바이트가 캡 아래다.

**문서.** 도구 설명(`tools.go`), `plugin/graphin-guide/skills/graphin/SKILL.md`의
네 검색기 표와 "Where is this exact string?" 레시피에 한 줄. 스킬 본문이 바뀌므로
rag 벤치는 새 베이스라인이다(§1-7). 그래서 P1은 **M2 기준선을 먼저 찍은 뒤**
넣는다(§4).

### P2. id 없는 히트의 다음 수 — `read_code` 라인 범위 (조건부)

38%의 id 없는 히트는 P1으로 대부분 자기 완결된다(주변 몇 줄이면 상수 선언은
보인다). 그래도 남는 것은 "노드 밖 텍스트를 더 넓게 읽고 싶다"이고, 지금 그
출구는 파일 통째 `Read`뿐이다. 그 읽기를 graphin 안으로 들여 예산과 계측 아래
두는 것이 P2다.

**스키마.** `read_code`에 `file`(상대 경로)과 `lines`(`"start-end"`, 상한
200줄)를 더한다. `node_id`/`node_ids`와 배타. 응답은 기존 `<code_block>`에
`kind="range"`를 붙여 노드 읽기와 구별한다.

**조건.** P1 배포 후 M2 재측정에서 "키워드 뒤 파일 통째 Read" 비율이 지금(6/97)
아래로 내려가지 않으면 구현한다. 내려가면 보류한다. 노드가 아닌 것을 노드처럼
읽는 첫 경로라 **데이터 없이는 넣지 않는다.**

**대안(더 싸다).** id 없는 `<node line>`에 같은 파일에서 바이트 거리가 가장
가까운 노드를 `nearest="<id>"`로 달아 `explore_graph` 진입점만 주는 것. 상수의
이웃 함수가 그 상수의 문맥은 아니므로 값이 약하다. P2 판정 때 함께 본다.

### P3. 사후 유도 두 곳 — 리터럴 힌트와 0건 힌트

**P3a. `search_hybrid`가 리터럴형 질의를 `search_keyword`로 보낸다.**
`searchHint`(`tools.go`)에 조건 둘을 더한다. 둘 다 산문 질의(하이브리드의 본업)를
밀어내지 않도록 좁게 잡는다.

1. **따옴표 규칙.** 질의 전체가 `"…"`, `'…'`, `` `…` ``로 감싸여 있으면 인용한
   텍스트다. 결과와 무관하게 힌트: *quoted text is a search_keyword query; the
   ranking matched its words, not the string*.
2. **어휘 부재 규칙.** 질의의 내용어(3자 이상, 불용어 제외, 어간 정규화 후)
   중 **절반 이상이 BM25 어휘에 df=0**이면 인덱스가 답할 수 없는 질의다. 힌트:
   *most of this query's words are not in the index; search_keyword reads the
   files the index skipped*. `search.Stats`에 `AbsentTerms []string`과
   `ContentTerms int`를 더한다. df 조회는 기존 `Index.HasTerm`(어간 정규화 후
   posting 유무)으로 충분해 새 API는 넣지 않았다. 내용어는 BM25 토큰이 아니라
   **낱말** 단위로 센다 — `Tokenize`는 복합 식별자의 결합형을 한 번 더 내놓아
   한 낱말을 두 번 세게 된다.

우선순위는 기존과 같이 식별자 규칙(Unnamed → Absent) 다음, 0건 힌트 앞이다.
**오발화 고정**: `eval/golden/base`·`variants`의 산문 질의 전부에서 이 두 힌트가
뜨지 않아야 한다(단위테스트로 질의 목록을 박는다). 뜨면 규칙이 너무 넓은 것이다.

**P3b. `search_keyword`가 0건일 때 다음 수를 말한다.** `files="0"`이면
`<hint>`: 이름이면 `search_hybrid`, 메시지면 더 짧고 특징적인 조각으로,
가변부가 있으면 `regex=true`. `node_ids="false"`(미부트스트랩)면 id는
부트스트랩 뒤에 붙는다는 한 줄. 20/97이 이 자리였다.

**테스트.** `internal/mcp/tools/signals_test.go` 패턴으로 조건별 발화·비발화,
우선순위, 오발화 고정 목록.

### P4. usage 계측 — `search_keyword`를 graphin 내비 콜로

분류는 리포트 시점에 일어나므로(`internal/usage/event.go` 주석) **소급
적용**된다. 데이터 이관 없음.

1. `Classify`: `search_keyword` → `ClassGSearch`.
2. `judgeRun`(`metrics.go`): `lastQuery`를 `query`가 없으면 `pattern`에서
   읽는다. same-intent 폴백 쌍이 키워드 런에도 붙는다.
3. 퍼널: 키워드 이벤트는 이미 `result_ids`(≤5)를 기록한다. `funnel`이 그것을
   집계하는지 확인하고 테스트로 고정한다.
4. **검색기별 분리.** 헤드라인 옆에 `retriever` 표를 낸다: hybrid / keyword 각각
   콜 수, 런 수, 채택 / 폴백 / same-intent, 퍼널 준수. "키워드가 채택되는가"는
   합산 채택률로는 안 보인다.
5. 문서: `docs/usage-spec.md` §4.1 클래스 정의와 §4.3 부가 지표에 반영.

**테스트.** `event_test`(분류), `metrics_test`(키워드만 있는 런의 채택·폴백 판정,
`pattern` 기반 same-intent, 검색기별 표).

## 2b. 2026-09-06 재검토 라운드 — 개선 넷과 D′의 사전 등록 기준

D 팔([기록](eval/2026-09-06-p1-hybrid/findings.md))이 §7-3 기준을 문면으로 못 넘긴 뒤
소유자가 "재검토할 부분을 다시 개선 후 릴리스를 결정"하기로 했다. 트랜스크립트가
가리킨 원인 셋을 고치고, 기준을 **D′를 돌리기 전에** 다시 쓴다. 데이터를 본 뒤의
기준 수정이므로 여기 그 사실을 적는다: 아래 기준은 D를 통과시키려고 고른 것이
아니라 D가 드러낸 기제를 재는 것이고, D′는 이 기준으로만 판정한다.

**원인과 수정**

| 원인 (D에서 실측) | 수정 | 어디 |
|---|---|---|
| `read_code`·`explore_graph` 응답에 `<cost>`가 없다 — 에이전트는 두 검색기의 cost만 합산해 가장 큰 지출을 못 셌다(자기 보고 중앙값 0.84, budget-pressure 침묵 초과 2) | 두 도구 응답 끝에 `<cost bytes>` 추가. 프롬프트 "Every response tells you what it cost"가 참이 된다. 호스트 도구(Read·Grep·셸) 결과도 세라는 한 문장 | `internal/mcp/tools/tools.go`, `agents/graphin-rag.md`, e2e `TestReadCodeAndExploreReportCost` |
| 키워드 응답의 rank-1이 데이터 파일(json·lock 등)인 비율 20%, 바이트의 21% — 반복으로 매치 수 순위를 이긴다. 에이전트의 다음 수는 `path=`로 다시 검색(24%) | 핸들러 층에서 데이터 확장자 파일을 소스·산문 뒤로 보내고 `folded="data"`(경로·개수만)로 접는다. `path=`를 주면 연다. `*.graphindb.json`과 매니페스트가 라우팅한 스키마 소스는 예외. `internal/keyword`는 불변(M3 대조군 보존) | `tools.go`(`isDataPath`, `IsDBSource`), e2e `TestSearchKeywordFoldsDataFilesAfterCode`, SKILL.md |
| rag 벤치 `--semantic`이 런당 첫 8~15초는 어휘 검색(hybrid 응답의 62%만 시맨틱) | 서버 플래그 `-semantic-wait`(기본 0): `search_hybrid`가 모델 로드를 최대 그만큼 기다린다. 러너는 `--semantic` 팔에만 `60s`를 넘긴다(루브릭 1.3.4, 기본 팔 불변) | `cmd/graphin`, `workspace.Config.SemanticWait`, `scripts/eval-rag.py` |
| 판정 기준 (b)의 "재검색 감소"가 P3b(0건 → 재검색 힌트)와 충돌하고, P1의 실제 효과는 기준에 없었다 | 아래 기준으로 교체 | 이 절 |

재검색은 건드리지 않는다: D의 히트 뒤 재검색 96건 중 64건은 **다른 용어로 넘어가는
연쇄**(같은 패턴 3, 좁힘 13, 넓힘 7)라 탐색의 정상 진행이고, 남은 것은 위 둘째 행이
줄이는 잡음 뒤의 재검색이다.

**D′ 실행 조건.** 코퍼스 `eae65b6`, 27태스크 × 3런, **lexical-only, `--jobs 3`**(게이트와
같은 축이고 B·C와 같은 조건; 하이브리드는 별도 축으로 `-semantic-wait`가 실측된 뒤),
새 바이너리(위 수정 포함), 새 에이전트·스킬 sha, 루브릭 1.3.4로 A·B·C·D와 함께 채점.

**D′ 판정 기준 (사전 등록, 2026-09-06)**

| | 기준 | 참조값 |
|---|---|---|
| (a) 정답률 | D′ 총 pass ≥ 73(B) **그리고** `rag-read-omission`을 뺀 태스크 짝 비교에서 C에 진 태스크 수 ≤ 이긴 태스크 수 | D 74, C 76(read-omission 제외 73/78) |
| (b) 기제 | id 있는 키워드 히트 뒤 `read_code`(돌려준 id) 후속 ≤ 15% · 키워드 응답의 rank-1이 **펼쳐진** 데이터 파일인 비율 ≤ 5% · 키워드 응답 바이트 중 데이터 파일 비중 ≤ 5% (데이터 = json·jsonl·lock·sum·csv, `*.graphindb.json`과 매니페스트 라우팅 스키마 소스 제외 — 핸들러의 `isDataPath`/`IsDBSource`와 같은 정의) | D 10.6% / 12.6% / 9.9%, B 21.6% / 10.8% / 10.6% (`tools/criteria.py`) |
| (c) 정직·회귀 | `over_silent` 0 · 자기 보고 비율 중앙값 ≥ 0.9 · 가짜 인용 ≤ 1런 · `escaped` 0 · 어느 층도 D보다 2 이상 낮지 않음 | D: 2 / 0.84 / 1 / 0 |

셋 다 충족이면 릴리스 권고(patch 0.4.12 + 가이드 0.6.3, [plugin-distribution §13.3](plugin-distribution.md)),
하나라도 미충족이면 원인을 기록하고 보류. 판정은 샌드박스 `out/rag-2026-09-05/tools/criteria.py <팔>`
한 번으로 열 항목을 낸다(D: 4 미충족, B: 7 미충족 — 기준이 D를 통과시키지 않음을 D′ 전에 확인했다).

**결과 (2026-09-06, [기록](eval/2026-09-06-p1-fold/findings.md)).** D′ 75/81(루브릭 1.3.5; D 76 · C 77 ·
B 74, 부호검정 전부 p=1.0). **열 항목 중 아홉 충족** — 자기 보고 0.99, 침묵 초과 0, 데이터 rank-1
2.1%, 데이터 바이트 2.5%, read_code 후속 14.5%, C 대비 2승 1패. **미충족 하나**: db-nav 22 대
D의 24(`rag-db-trigger-fn` 1/3, 두 런이 잘못된 픽스처로 감; 같은 lexical 팔 B도 22). 규칙대로
**보류로 기록**, 릴리스는 소유자 결정. 덤으로 나온 것: 채점기 1.3.5(초과를 "5 KB over the target"처럼
숫자로 말한 것을 stated로 — `<cost>`가 붙자 에이전트가 낱말 대신 숫자를 쓰기 시작했다).

## 3. 측정

### M1. 결정론 축 — `eval-recall.py`에 키워드 검색기 팔

`scripts/eval-recall.py`는 락 밖이다(락은 `eval-rag.py`·`eval/rag`만). 골든셋
파일은 건드리지 않는다.

- `--retriever keyword` (또는 `--keyword-arm`): `shape="literal"`이고 `grep`
  필드가 있는 질의에 대해 `search_keyword(pattern=q["grep"], context=N)`을
  부르고, `<file path>`로 recall, 응답 본문 안의 `evidence`로 delivery, 바이트를
  잰다. 기존 하이브리드 팔·grep 팔 옆에 세 번째 열로 나온다.
- N ∈ {0, 2, 4} 세 점. **판정 기준(방향)**: `tests` 층 리터럴 3문항에서 recall
  100% 유지, `context=2`에서 delivery가 하이브리드 팔 이상, 바이트가 grep -C20의
  절반 이하.
- 이 축이 P1의 값을 **LLM 변동 없이** 처음 재는 자리다. 3문항이라 판정이 아니라
  방향이다. 문항을 늘리려면 리터럴형 층 신설이 필요하고 그것은 골든셋 변경이므로
  소유자가 `golden-set` 스킬로 결정한다(이 계획의 범위 밖).

### M2. 행동 축 — rag 벤치 관찰 지표 2종 (허가 필요)

`behavior_metrics`(`scripts/eval-rag.py`)에 게이트가 아닌 관찰 지표를 더한다.
루브릭 1.3.x — 판정 규칙이 아니라 관찰만 바뀌므로 `RUN_COMPAT`에 붙어 **09-01
재베이스라인 81런을 재채점**할 수 있다. 즉 "P1 이전" 기준선이 공짜로 나온다.

1. `keyword_next`: 각 `search_keyword` 뒤 3콜 안의 첫 소비 행동 분포
   (재검색 / hybrid / read_code[돌려준 id 사용 여부] / explore / Read /
   grep·rg / 종료). §0의 표가 그대로 지표가 된다.
2. `keyword_idless`·`keyword_empty`: 결과는 있는데 id 없는 응답 수, 0건 응답 수.
3. `first_retriever`: 태스크별 첫 검색 콜의 이름. 리터럴 태스크 집합은 채점기
   상수로 둔다(태스크 파일에 `shape`를 더하는 것은 골든셋 변경이라 피한다).

절차: 소유자 허가 → `.rag-bench-unlock` → 수정 → `lock --approved-by` →
트레일러 → unlock 삭제. `docs/rag-bench-spec.md` §3 루브릭 버전 규칙대로
`docs/eval/`에 재채점 기록을 남긴다.

### M3. SWE-Explore 대조군 불변 확인

P1이 `internal/keyword`에 필드를 더한 뒤 `graphin eval swe-explore --policy grep`
451을 다시 돌려 제출 JSONL의 md5를 `out/grep-451`과 비교한다. 인덱싱이 없어
수 분이다. **하나라도 다르면 매처를 건드린 것**이고 그때는 패키지 헤더의 규칙대로
전체 벤치를 돌린다. 같으면 §2.1의 수치는 그대로 인용할 수 있다.

### M4. 실사용

P4가 배포되면 `usage report`가 검색기별 표를 낸다. 다만 데이터원이 문제다 —
이 저장소는 8월 31일 이후 graphin 콜이 거의 wiki뿐이고 kinder는 멈췄다.
실사용 워크스페이스 확보는 이 계획 밖의 선결 조건이고, 그 전까지 M4는 "표가
나온다"까지다.

## 4. 순서와 게이트

| 단계 | 내용 | 게이트 |
|---|---|---|
| 1 | **P4** 계측 승격 + spec 갱신 | 단위테스트. 소급 적용이라 릴리스 없이도 리포트가 바뀐다 |
| 2 | **M2** 허가 요청 → 채점기 수정 → 81런 재채점 | "P1 이전" 기준선 확보. 여기까지 제품 변경 없음 |
| 3 | **P1** context + **P3** 힌트 | 단위·e2e, **M3** md5 불변, **M1** 세 점 측정 |
| 4 | 릴리스 | 스킬 본문·도구 스키마 변경이므로 가이드 버전 범프(`docs/plugin-distribution.md` §13). rag 게이트 통과 |
| 5 | rag 벤치 재측정(27태스크 × 3런) | M2 지표를 2단계 기준선과 비교. 스킬이 바뀌었으니 새 베이스라인으로 기록 |
| 6 | **P2** 판정 | "키워드 뒤 파일 통째 Read"가 안 내려갔으면 구현, 내려갔으면 보류 |

2단계를 3단계 앞에 두는 이유 하나: 스킬 본문이 바뀌면 옛 트랜스크립트는 새
에이전트의 것이 아니므로, 기준선은 **바꾸기 전에** 찍어야 한다.

## 5. 규모

| 항목 | 변경 | 크기 |
|---|---|---|
| P1 | `keyword.go` 필드·컨텍스트 채우기, `tools.go` 응답·예산, 테스트, 문서 3곳 | 반나절 |
| P3 | `tools.go` 힌트 2조건, `router.go` `AbsentTerms`, `lexical` df 조회, 오발화 고정 테스트 | 반나절 |
| P4 | `event.go`·`metrics.go`·리포트 표, 테스트, spec | 반나절 |
| M1 | `eval-recall.py` 키워드 팔 | 2시간 |
| M2 | `eval-rag.py` 지표 3종 + 허가 절차 + 재채점 | 2시간 + 절차 |
| M3 | grep-451 재실행·md5 | 수 분 |
| 벤치 실행 | rag 27×3 (~35분, ~$10) × 1회 | |

합쳐 2~3일. P2는 별도.

## 6. 리스크와 대응

- **리터럴 힌트의 오발화.** 산문 질의를 키워드로 밀면 하이브리드의 본업을
  해친다. 따옴표·어휘 부재 두 규칙만 쓰고, base·variants 질의 전부를 비발화
  고정 테스트에 박는다. 규칙을 넓히고 싶으면 그 테스트가 먼저 말한다.
- **컨텍스트가 예산을 먹는다.** 기본 0이고 핸들러가 파일 단위로 접는다. 접힌
  파일은 `path` 필터로 되부를 수 있어 정보가 사라지지 않는다.
- **대조군 이동.** M3가 잡는다. 필드 추가만으로는 움직일 수 없지만 확인은 실행으로
  한다.
- **채점기 변경의 통제.** 게이트 지표가 아니라 관찰 지표라도 락 대상이다.
  허가 없이는 2단계를 건너뛰고 3단계로 가지 않는다(기준선 없는 개선은 재지
  못한다).
- **자기 코퍼스.** M1·M2 모두 이 저장소가 코퍼스다. §0의 38%·21%도 그렇다.
  다른 저장소에서 같은 분포가 나온다는 근거는 없고, 이 계획은 그 한계를 물려받는다.
