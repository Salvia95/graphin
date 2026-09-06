# 런 스냅샷 매니페스트 — 2026-09-06 p1-fold (D′ 팔: 재검토 개선 넷 반영, lexical)

[keyword-plan §2b](../../keyword-plan.md#2b-2026-09-06-재검토-라운드--개선-넷과-d의-사전-등록-기준)에
**먼저 적어 둔 기준**으로 판정하는 재측정. D 팔([p1-hybrid](../2026-09-06-p1-hybrid/findings.md))의
트랜스크립트가 가리킨 원인 셋을 고친 바이너리·프롬프트로, 게이트와 같은 축(lexical-only)에서
B·C 옆에 놓는다.

| 항목 | 값 |
|---|---|
| 수행일 | 2026-09-06 (81런, 오류 0, `--jobs 3`으로 실경과 약 45분) |
| 명령 | `scripts/eval-rag.py run --out <샌드박스>/rag-p1-fold --runs 3 --jobs 3 --ref eae65b6 --detach` |
| 소유자 지시 | "재검토할 부분을 다시 개선 후 릴리즈를 결정할게 추가 작업 진행해줘" |
| 바이너리 | `bin/graphin` v0.4.11-2-g7bf8826-dirty(06:38 빌드) — P1·P3·P4 + **① `read_code`·`explore_graph` `<cost>` ② 데이터 파일 접기(`folded="data"`, `IsDBSource` 예외) ③ `-semantic-wait`**(이 팔은 lexical이라 미사용) |
| 에이전트 · 스킬 | `agents/graphin-rag.md` **`c11b4614ad78`**(예산 절에 "호스트 도구 결과도 세라" 한 문장) · `SKILL.md` **`1fa366d41424`**(`folded="data"` 계약) — 둘 다 새 sha라 **B·D와 프롬프트가 다르다**. 태스크셋 `5ca2a37b2b92` |
| 코퍼스 | `--ref eae65b6` — A·B·C·D와 동일 |
| 검색 | lexical-only(`--ort-lib /nonexistent-ort`). 하이브리드는 별도 축 |
| 채점 | 루브릭 **1.3.5**(1.3.4 `--semantic-wait` 러너 옵션 + 1.3.5 "초과를 숫자로 말한 것도 stated" — 아래). 다섯 팔을 같은 채점기로 |
| 모델 · CLI | sonnet · Claude Code 2.1.263(D까지는 2.1.261 — CLI 마이너 업데이트가 런 사이에 있었다) |
| 비용 | $10.29 · 벽시계 합 72분 |
| 산출 | 샌드박스 `~/projects/graphin-eval-sandbox/out/rag-2026-09-05/rag-p1-fold/`, 판정 `tools/criteria.py rag-p1-fold`, 이 디렉터리의 `scores.json`(다섯 팔 pass·부호검정·태스크별·행동·실패) |

## 루브릭 1.3.5 — 초과를 숫자로 말한 것은 침묵이 아니다 (소유자 허가, 같은 라운드)

D′의 budget-pressure 3런이 `over_silent`로 채점됐는데 본문은 이랬다: "~25.4 KB of retrieved
content — **about 5 KB over the ~20,000-byte target**", "≈21.7 KB — slightly over the
~20,000-byte target (about 8% over)", "**The overrun came from** the last `read_code`".
`TRUNCATION_STATED`(`budget|truncat|ran out|stopped early|cut off|예산`)는 이 어휘를 몰랐다.
①의 `<cost>`가 붙자 에이전트가 "budget"이라는 낱말 대신 **숫자**를 쓰기 시작한 것이 원인이다.
1.3.5는 `exceed|overr[au]n|overage|ran over|over (the|its|my)? (숫자|target|limit|cap|ceiling)`을
더한다 — "over the call chain"은 안 걸린다. 채점 전용, `RUN_COMPAT` 붙음.

| 팔 | 1.3.3/1.3.4 | **1.3.5** | 바뀐 것 |
|---|--:|--:|---|
| A 변경 전(77) | 67 | **70** | bp 3/6 → 5/6, answered +1 |
| B 프롬프트만 | 73 | **74** | bp 5/6 → 6/6 |
| C grep | 76 | **77** | answered 21 → 22. 남은 `over_silent` 3은 진짜 침묵(fk-edges 45·60KB, oor-adoption 60KB) |
| D P1+부분 하이브리드 | 74 | **76** | bp 4/6 → 6/6 |
| **D′** | 72 | **75** | bp 3/6 → 6/6 |

옛 리포트는 각 팔 디렉터리에 `report-pre-1.3.5.json`으로 보존.

## 접기 예외가 못 본 것

`IsDBSource`는 `*.graphindb.json`과 **루트 매니페스트가 라우팅한** 소스를 예외로 둔다. rag 코퍼스의
`testdata/fixtures/dbssot/`는 자기 매니페스트를 **픽스처 안에** 두는데, 그 매니페스트가 라우팅한
소스는 루트 기준 경로로 조회하는 `routeFor`에 잡히지 않는다(관찰 — 중첩 매니페스트는 e2e에서만
루트로 쓰인다). 그래서 `custom.json`·`tbls.json`(7건)과 매니페스트 자신(1건)이 접혔다 — 101건의 접기 중 8. 런 뒤 매니페스트 파일명(`graphindb.json`)을 예외에 더했고(e2e 통과),
라우팅 소스는 설계대로다. db-nav의 하락(§findings 2)은 이것과 인과가 닿지 않는다 — must_cite는
다른 픽스처(dbschema)의 트리거 스냅샷이고 그 파일은 접히지 않았다.
