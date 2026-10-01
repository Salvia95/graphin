# effort·채택 벤치 — 설계안

상태: **Phase 0 구현(2026-09-30).** §9의 결정 여섯은 소유자가 제안대로
닫았다. 러너는 `scripts/eval-effort-adoption.py`(`selftest`·`probe`·`run`·`score`).
선택 후회·부트스트랩·판정 규칙을 계산하는 `analyze`는 Phase 1 스모크 데이터가
나온 뒤에 붙인다.

이 벤치는 두 가지를 묻는다. graphin-rag를 어떤 모델과 effort로 돌려야 하는가,
그리고 graphin이 붙어 있지만 강제되지 않을 때 에이전트가 그걸 얼마나 쓰고
그 선택이 결과를 바꾸는가. 코퍼스·태스크·채점기는 `eval/rag` + `scripts/eval-rag.py`를
**읽기 전용으로** 재사용한다. 게이트 러너와 골든셋은 건드리지 않는다.

## 0. 왜 새로 재나 — 확인된 사실 셋

**① 지금까지의 rag 벤치는 effort를 고정한 적이 없다.** `eval-rag.py`의 자식
`claude -p`는 `--model`만 받고 `--effort`는 받지 않는다(`scripts/eval-rag.py:867-879`).
meta에도, 트랜스크립트에도 effort 값은 남지 않는다 — init에는
`per_turn_effort_active:true`만 찍히고, 간접 증거는 result 이벤트
`modelUsage.thinkingTokens`뿐이다. 자식은 사용자 설정을 상속하는데 이 머신의
`~/.claude/settings.json`은 전역 `effortLevel: xhigh`에 `modelSettings`로
opus-5-5만 `high`를 따로 두고 **sonnet-5-5 항목이 없다.** 그러니 과거 sonnet
베이스라인과 게이트 대장의 수치는 당시의 전역값으로 돌았고 그 값은 기록되지 않았다.
게이트 임계 0.80도 effort를 모르는 채로 교정된 기준이다.

**② 결과를 배포로 옮길 길은 있다.** Claude Code 2.1.285의 에이전트 정의는
`effort` 필드를 받는다(바이너리의 정의 필드 목록 `name, description, prompt,
tools, disallowedTools, model, effort, …`). `plugin/graphin-guide/agents/graphin-rag.md`
프론트매터에는 지금 `model: sonnet`만 있다.

**③ Grep·Glob은 원래 막혀 있지 않았고, graphin-rag 아래서 채택률은 이미 포화다.**
graphin 팔의 `ALLOWED_TOOLS`에 Read·Grep·Glob이 들어 있다. 1.4.3 재베이스라인
108런(`docs/eval/2026-09-13-isolation-hardening/report.json`)의 호출 수:

| 도구 | 호출 |
|---|--:|
| graphin 내비(search_keyword 334·search_hybrid 196·read_code 164·explore_graph 48) | 742 |
| Grep | 3 |
| Glob | 0 |
| Read | 13 |
| Bash | 46 (allowedTools에 없어 전부 거부) |

채택률이 약 99%로 거의 변하지 않으니 **"채택 비율에 따른 성능"은 graphin-rag로는
나오지 않는다.** 그 질문은 graphin을 권하지 않는 프롬프트에서만 변이를 얻는다.
그리고 모델이 실제로 쓰는 grep 경로는 Bash인 경우가 많다(거부된 46회) —
"막지 않는다"는 Bash까지 여는 것이다.

## 1. 두 실험

두 질문을 한 팔에서 재면 서로를 가린다. 그래서 가른다.

| | 실험 A — 설정 선택 | 실험 B — 자연 채택 |
|---|---|---|
| 질문 | graphin-rag를 어떤 (모델, effort)로 배포하나 | graphin이 선택지일 때 얼마나 쓰이고, 그 선택이 결과를 바꾸나 |
| 에이전트 | graphin-rag 그대로(`compose_prompt`) | Claude Code 기본 시스템 프롬프트 + 중립 답변 계약 |
| 독립변수 | (모델, effort) | 도구 가용성(혼합·graphin 강제·grep 강제) × (모델, effort) |
| 채택률 | 기록만 한다(포화 예상) | 결과 변수 — 주 관심사 |
| 답이 되는 것 | 비열등 판정 뒤 비용·시간이 가장 싼 설정 | 선택 후회(§4.4)와 층별 채택 양상 |

## 2. 공통 골격

**러너는 새 파일 하나다 — `scripts/eval-effort-adoption.py`.** eval-rag를
`importlib`로 불러 쓴다(선례: `eval-wiki-author.py:61`의 `_load`). 가져다 쓰는 것:
`load_set`·`materialize`·`preindex`·`write_containment_hook`·`compose_prompt`·
`parse_transcript`·`behavior_metrics`·`grade`·`escapes_of`·`_hit_limit`·`SELF_PREFIXES`.
eval-rag.py를 고치지 않으니 `rubric.lock`·`RUN_COMPAT`·트레일러 절차가 걸리지
않는다. 대신 **채점기는 eval-rag의 것이 그대로 흐르므로** meta에 eval-rag의
`RUBRIC_VERSION`을 적고, 루브릭이 바뀌면 이 벤치의 옛 런도 비교 불가다.
파일명이 `scripts/eval-`로 시작하므로 스냅샷에서 자동으로 잘린다.

**코퍼스·태스크.** `eval/rag/tasks.jsonl` 36태스크(answered 12·multi-hop 9·db-nav 8·
not-here 3·out-of-reach 2·budget-pressure 2)를 그대로 쓴다. 태스크를 더하거나
고치지 않는다(변경 통제 대상, 스펙 §8). 층 외에 러너 안에서만 쓰는 보조
구분 하나를 둔다: `rag-lit-*` 4개는 **리터럴 계열**이다 — graphin의 값이 산문
질의에만 있다는 결론이 세 모집단에서 반복됐으므로, 채택의 옳고 그름이 이
구분에서 갈릴 것으로 본다.

**셀 식별자.**

| id | 모델 | effort | 역할 |
|---|---|---|---|
| C0 | sonnet | 플래그 없음(모델 기본값) | 역사적 조건의 재현 — 1.4.3 베이스라인(96/108, $13.35)과 닮아야 한다 |
| C1 | opus | low | 후보 |
| C2 | sonnet | medium | 후보 |
| C3 | sonnet | low | 분해용 — 모델 효과와 effort 효과를 가른다 |
| C4 | opus | medium | 분해용 |

C0는 처음에 xhigh로 잡았다가 Phase 0에서 바꿨다(§6.1) — 과거 조건은 xhigh가
아니라 모델 기본값이었다.

C1 대 C2만으로는 모델과 effort가 교란된다. "둘 중 무엇을 배포하나"에는 충분하고,
"왜"에는 C3·C4가 있어야 2×2가 선다.

**effort는 플래그로 고정하고 사용자 설정은 끊는다.** 모든 자식에
`--effort <level>`과 `--setting-sources project,local`을 준다. 사용자 스코프의
`effortLevel`·`modelSettings`가 플래그와 어느 쪽이 이기는지는 가정하지 않고
Phase 0에서 잰다. 끊으면 사용자 플러그인도 로드되지 않는다(2026-09-14 확인
사실) — 그래도 `enabledPlugins`의 명시 차단은 유지한다.

**실행 순서는 섞는다.** 한 Phase의 (셀, 팔, 태스크, 런) 전체를 고정 시드로
섞은 큐 하나에서 워커가 뽑는다. 셀을 차례로 돌리면 시간대별 API 지연이 셀 차이로
둔갑한다. 사용량 한도로 며칠에 걸쳐 돌더라도 하루치 조각이 모든 셀을 고르게
담게 된다. `runs.jsonl`은 append, `--resume`은 끝난 태그를 건너뛰고, 429는
eval-rag와 같이 조기 중단한다.

**산출물은 `/tmp`에 두지 않는다.** `graphin-eval-sandbox/effort-runs/<날짜>/`에
두고(스케일링 벤치에서 스크래치 정리로 결과를 통째로 잃은 적이 있다), 기록은
`docs/eval/<날짜>-effort-adoption/`에 남긴다.

## 3. 실험 A — 설정 선택

**조건은 게이트와 같다 + effort.** 시스템 프롬프트는 `compose_prompt("graphin")`,
로스터는 eval-rag graphin 팔의 `ALLOWED_TOOLS`/`DENIED_TOOLS` 그대로, 격리는
1.4.3(blockReads·containment 훅·플러그인 차단), lexical-only. **Bash는 허용목록에
없다** — 프로덕션 graphin-rag는 Bash를 갖고 있으므로 이것은 알려진 차이다.

**게이트와 갈리는 점 하나(Phase 0에서 발견).** eval-rag의 자식은 사용자 설정을
상속하는데, 이 머신의 `permissions.defaultMode`가 `auto`라서 게이트는 **auto 권한
모드**로 돌았다. 허용목록 밖의 Bash는 거부가 아니라 auto 분류기의 판단에 맡겨졌고,
1.4.3 베이스라인의 Bash 46회 중 일부는 결과가 500~2,000바이트다 — 실제로 실행됐다.
이 러너는 `--setting-sources project,local`로 사용자 설정을 끊으므로 `default`
모드이고, 허용목록 밖 Bash는 항상 거부된다. 그래서 A의 C0는 게이트 수치와
**Bash 몇 번만큼** 다를 수 있다. 게이트의 Bash 가용성이 사용자 설정에 달려 있다는
것 자체가 effort와 같은 종류의 숨은 환경 의존이다(§7).

**셀.** C0·C1·C2를 36태스크 × 3런으로 돈다(324런). C3·C4는 Phase 1 스모크에서
비용·thinking 곡선만 보고, 분해가 결정에 필요할 때만 풀셋으로 올린다.

**판정 규칙 — 런을 보기 전에 고정한다.**

1. **비열등**: 후보의 전체 pass율이 0.80 이상이고, C0 대비 차이의 태스크 단위
   클러스터 부트스트랩 95% 하한이 **−5%p** 위에 있다. 층별 표는 함께 내지만
   층 하나의 하락으로 탈락시키지 않는다 — 층당 표본이 2~12태스크라 못 버틴다.
   예외 하나: `not-here`·`out-of-reach`에서 C0에 없던 **가짜 인용·escaped·빗나간
   지어낸 id**가 나오면 탈락이다(부재 오보는 이 도구가 팔 수 없는 실패).
2. 비열등을 통과한 셀 중 **런당 `costUSD` 중앙값**이 가장 낮은 것.
3. 비용 차이가 부트스트랩 CI상 0을 포함하면 **`duration_ms` 중앙값**으로 가른다.

품질 차이를 이 표본으로 **검출하려 하지 않는다.** 과거 grep 대 graphin의
5~6점 차이가 부호검정 p≈0.2였다 — 10%p 미만의 차이는 가려지지 않는다고
전제하고 비열등 설계를 택한 것이다.

## 4. 실험 B — 자연 채택

### 4.1 표면 — "붙어 있지만 권하지 않는다"

- **시스템 프롬프트는 Claude Code 기본값**(`--system-prompt-file` 없음). 여기에
  `--append-system-prompt`로 **중립 답변 계약** 하나만 붙인다: 역할, 40KB 예산,
  세 종료 상태, 인용 규칙, 보고 모양. `GREP_AGENT_PROMPT`에서 `# Tools`와
  루프의 검색 지시를 뺀 부분과 같은 말로 쓴다. 계약 없이 원 질문만 주면 인용
  모양 때문에 떨어진다 — 그건 검색이 아니라 형식의 실패다. **계약에는 검색기
  이름이 하나도 없어야 한다** — `grep|rg|glob|graphin|search_|explore|read_code`가
  들어가면 러너가 시작을 거부한다.
- **graphin 노출은 프로덕션 메인 세션과 같은 경로로.** `--mcp-config`로
  저장소 빌드 서버(eval-rag와 같은 사전 인덱스 스냅샷), `--plugin-dir
  plugin/graphin-guide`로 가이드 스킬(설명 목록에 오르고 본문은 호출될 때만).
  MCP 도구는 프로덕션처럼 ToolSearch 뒤로 지연 로드된다 — 이 지연이
  채택 장벽의 일부이므로 우회하지 않는다.
- **graphin 플러그인(훅)은 로드하지 않는다.** wiki 게이트는 편집·위임을 위한
  장치인데 읽기 전용 탐색의 Bash를 막는다(1.4.3 이전 graphin 팔에서만 Bash 거부
  17/81) — 이러면 채택률이 게이트 때문에 오른다. usage 훅은 실제
  `~/.graphin` 로그를 오염시킨다. SessionStart 훅은 설치 실패 때만 말하므로
  빠져도 표면이 바뀌지 않는다.
- **스냅샷에서 `.claude/`를 통째로 자른다**(eval-scaling과 같다). 기본 시스템
  프롬프트가 저장소의 훅·에이전트 목록을 읽으면 안 된다. 저장소와 사용자 스코프
  모두에 CLAUDE.md는 없다(확인함).
- **로스터**: Read·Grep·Glob·Bash·ToolSearch·Skill + graphin MCP. 거부: Edit·Write·
  NotebookEdit·Agent·Task·WebFetch·WebSearch·ScheduleWakeup. Agent를 막는 이유는
  eval-rag와 같다 — 위임된 지출과 증거가 원장 밖으로 나간다.
- **Bash 격리**: blockReads + `sandbox.network.allowedDomains: []` +
  `failIfUnavailable`(1.4.4 grep 팔과 같다). containment 훅과 `escaped` 판정은 백스톱이다.
- **시맨틱**: `--semantic --semantic-wait 60s`. 프로덕션 기본은 하이브리드고,
  graphin의 값이 산문 질의에 있다면 그 값의 대부분은 시맨틱이 만든다. 런마다
  서버가 새로 떠서 생기는 모델 로드 대기(8~15s)는 하니스 인공물이므로 §5에서 시간을 분해한다.

### 4.2 팔

같은 표면에서 **도구 가용성만** 다르다.

| 팔 | graphin | Grep·Glob·검색 Bash |
|---|---|---|
| `mixed` | 있음(지연 로드 — ToolSearch 뒤) | 있음 |
| `mixed-al` | 있음(`alwaysLoad` — 첫 턴부터 프롬프트에) | 있음 |
| `g-only` | 있음 | Grep·Glob 거부, Bash는 허용하되 검색 명령이면 containment 훅이 거부(§4.3의 분류기) |
| `s-only` | MCP·가이드 플러그인 없음 | 있음 |

앵커 둘이 있어야 `mixed`를 읽을 수 있다. 채택률은 처치가 아니라 **결과**이기
때문이다 — graphin이 실패해서 grep으로 폴백한 런은 grep 비중이 높고 실패도
하므로, `mixed` 안에서 채택률과 pass율을 그냥 상관시키면 역인과를 잰다. 어려운
태스크일수록 도구를 섞는다는 난이도 교란도 같은 방향으로 겹친다.

**셀과 런 (Phase 1 뒤 재설계, §6.2).** 네 팔 모두 3런, **A의 승자 셀 하나** →
36 × 4 × 3 = 432런. `mixed` 대 `mixed-al`은 노출만 조작한 처치라 채택 효과를 인과로
읽는다. 원안은 `mixed` 5런 + 앵커 3런 × C1·C2 = 792런이었는데, 지연 로드 아래 채택이
0/12라 5런으로 늘린 이유(태스크 안 채택 변이)가 사라졌다.

### 4.3 채택 지표 — usage-spec의 분류를 그대로

실사용 지표(`docs/usage-spec.md §4`)와 같은 말을 써야 벤치 수치를 실사용 리포트
옆에 놓을 수 있다.

- **클래스**: 내비 = `search_hybrid`·`search_keyword`·`explore_graph`·`read_code` /
  검색 = Grep·Glob·검색 Bash / 읽기 = Read·읽기 Bash(`cat`·`sed -n`·`head`·`tail`) —
  **읽기는 분모에 넣지 않는다**(두 경로 모두 결국 읽는다) / 기타 = bootstrap·
  diagnose·ToolSearch·Skill.
- **검색 Bash 판정**은 `internal/usage/ingest.go`의 `classifyBash`를 파이썬으로 옮긴다
  (`grep rg egrep fgrep ag ack fd find` + `git grep`, 세그먼트 분할, 패턴 추출).
  Go 쪽 테스트 픽스처로 **패리티 테스트**를 붙인다 — 둘이 갈라지면 벤치와 실사용
  지표가 다른 것을 센다.
- **런 단위 필드**:
  - `nav_calls`·`search_calls`(grep/glob/bash 분리)·`read_calls`
  - `adopt_call_share` = 내비 / (내비 + 검색), `adopt_byte_share`(결과 바이트 기준)
  - `touched` = 내비 ≥ 1, `first_retriever`(내비/검색), `toolsearch_graphin`(graphin
    도구를 ToolSearch로 불러왔는가 — 발견 단계의 채택)
  - `fallbacks`·`same_intent_fallbacks`(usage-spec §4.2의 토큰 겹침 규칙),
    `late_switch`(첫 내비 전 검색 ≥ 2)
  - `skill_invoked`(가이드 스킬을 불렀는가)

### 4.4 분석 — 무엇을 인과로 읽고 무엇을 서술로만 두나

1. **선택 후회(주 지표).** 태스크 t, 셀 c마다 두 앵커 중 pass율이 높은 쪽(동률이면
   비용이 싼 쪽)을 "그 태스크의 더 나은 도구"로 둔다. `mixed`의 pass율·비용을
   그것과 비교한다. 후회 0이면 에이전트가 도구를 잘 고른 것이다. 층별·리터럴 계열로 가른다.
   **예상**: 리터럴 계열에서는 `s-only`가 이기고 `mixed`가 grep 쪽으로 가는 것이
   정답이다 — 여기서 채택률이 낮은 것은 실패가 아니다.
2. **태스크 안 비교.** `mixed` 5런 안에서 `touched`가 갈린 태스크만 골라, 같은
   태스크 안에서 graphin을 쓴 런과 안 쓴 런의 pass·비용·시간을 짝지어 본다.
   난이도 교란은 태스크 고정으로 빠진다. 역인과는 남으므로 **폴백 런은 따로 표시**한다
   (graphin을 먼저 쓰고 실패해서 grep으로 간 런은 "graphin을 쓴 런"이 아니라
   "graphin이 실패한 런"이다).
3. **채택률 구간표 — 서술용.** `adopt_call_share`를 0 / (0, 0.5) / [0.5, 1) / 1 네
   구간으로 나눠 pass·토큰·비용·시간을 낸다. 사용자가 처음 보고 싶어 한 표지만
   표 머리에 **"관찰이지 인과 아님 — 1·2를 보라"**를 박는다.
4. **셀 차이.** C1 대 C2에서 채택률 자체가 다른가(모델·effort가 도구 선택을 바꾸나).

## 5. 측정 필드

**런 행**(eval-rag의 grade·behavior_metrics 필드에 더해):

| 필드 | 출처 | 메모 |
|---|---|---|
| `start_ts` | 러너 | 시간대 공변량 |
| `wall_s` | 러너 | 서버 기동·모델 로드·CLI 기동 포함 |
| `duration_ms`·`duration_api_ms`·`ttft_ms` | result 이벤트 | 모델 시간 = api, 도구 시간 ≈ duration − api |
| `num_turns` | result | |
| `tokens_in` | `modelUsage`의 input + cacheRead + cacheCreation | **input만 읽으면 25만 토큰 런이 21로 찍힌다**(스케일링 벤치 함정) |
| `tokens_out`·`thinking_tokens` | `modelUsage` | effort의 직접 증거 |
| `cost_usd` | `modelUsage[*].costUSD` 합 | `costBasis: list` — OAuth 계정이라 청구액이 아니라 정가 환산. 실제 제약은 사용량 한도다 |
| `canonical_model` | `modelUsage` 키 | 별칭이 의도한 모델로 풀렸는지 |
| `permission_denials` | result | 로스터·훅이 막은 것 |
| `semantic_ready_first` | graphin 응답 | B의 첫 hybrid 콜이 시맨틱을 봤는가 |

**모델 간 비교는 토큰이 아니라 `cost_usd`로 한다.** 같은 토큰이라도 모델마다 값이 다르다.

**meta**: eval-rag `RUBRIC_VERSION`, 실험(A/B), 셀 표, `cli_version`, `graphin_commit`,
`agent_sha`·`skill_sha`·`contract_sha`, `taskset_sha`, 셔플 시드, `jobs`,
`setting_sources`, 시맨틱 설정.

## 6. 실행 계획

| Phase | 내용 | 런 | 통과 조건 |
|---|---|--:|---|
| 0 | 러너 뼈대 + 프로브 일곱(아래) | ~15 | 전부 통과해야 1로 |
| 1 | 스모크: A 5셀 × 스모크 6태스크 × 1런, B C1·C2 × 3팔 × 6 × 1 | 66 | effort 단조성, 셀별 런당 비용 확정, C0이 1.4.3과 닮음 |
| 2 | 실험 A 본 실행(C0·C1·C2) | 324 | 판정(§3) |
| 3 | 실험 B 본 실행(C1·C2) | 792 | 분석(§4.4) |
| 4 | 기록·결정 | — | `docs/eval/<날짜>-effort-adoption/` |

**Phase 0 프로브.**

1. **effort 우선순위**: 같은 추론형 프롬프트를 `--effort low|medium|xhigh` × 사용자 설정
   (켬/끔)으로 돌려 `thinking_tokens`를 비교한다 — 플래그가 `modelSettings`를 이기는지.
2. `--setting-sources project,local`에서 OAuth 인증이 사는지.
3. `sonnet`·`opus` 별칭이 `claude-sonnet-5-5`·`claude-opus-5-5`로 풀리는지.
4. B: init 이벤트에 가이드 스킬만 있고 사용자 플러그인·graphin 플러그인은 없는지.
5. B: graphin 도구가 지연 로드(ToolSearch 경유)로 뜨는지.
6. B: Bash에서 `git clone`이 거부되는지(샌드박스 카나리).
7. 중립 계약 린트, `classifyBash` 패리티 테스트.

**Phase 1의 C0 점검**: C0이 스모크에서 과거 스모크 수치와 크게 어긋나면(pass, 런당
비용) 역사 재현이 아니라 셀 하나로만 읽는다. 권한 모드 차이(§3)가 그 후보다.

### 6.1 Phase 0 결과 (2026-09-30)

프로브 여덟 개 모두 통과. 기록은 `graphin-eval-sandbox/effort-runs/2026-09-30-phase0{,-effort2}/`.

- **인증·별칭**: 사용자 설정을 끊어도 OAuth가 살고, `sonnet`·`opus`는
  `claude-sonnet-5-5`·`claude-opus-5-5`로 풀린다.
- **B 표면**: 로드된 플러그인은 `plugin/graphin-guide`(inline)와 내장 둘
  (`cc-plugin-agents-md`·`cc-plugin-telemetry`, `@builtin` — 모든 세션에 있으니 표면의
  일부다). graphin MCP connected, 가이드 스킬 둘이 보이고 사용자 플러그인은 없다.
  권한 모드는 `default`.
- **샌드박스**: `git clone`이 `CONNECT tunnel failed, response 403`로 막혔다.
- **g-only 훅**: `grep -n module go.mod`는 거부, `head -1 go.mod`는 통과.
- **effort** — 계산해야 풀리는 문제(정답 140), 셀당 3회:

  | 조건 | thinking 중앙 | 정답 |
  |---|--:|--:|
  | sonnet low | 643 | 1/3 |
  | sonnet medium | 613 | 3/3 |
  | sonnet high | 738 | 3/3 |
  | sonnet xhigh | 906 | 3/3 |
  | sonnet 기본값(플래그·사용자 설정 없음) | 745 | 3/3 |
  | sonnet 사용자 설정 상속(과거 벤치 조건) | 737 | 3/3 |
  | sonnet low + 사용자 설정 | 600 | 1/3 |
  | opus low | 505 | 3/3 |
  | opus medium | 599 | 3/3 |

  플래그는 먹는다(low만 틀리고, 사용자 설정이 있어도 low가 이긴다). **과거 조건은
  xhigh가 아니다** — 상속 조건은 기본값과 구별되지 않고, 첫 프로브의 쉬운 문제에서는
  xhigh만 두 번 모두 thinking을 냈고 상속 조건은 두 번 모두 0이었다. 전역
  `effortLevel: xhigh`는 sonnet 자식에 닿지 않았다. 그래서 C0는 플래그 없음이다.
  thinking 토큰은 반복 간 편차가 커서 low와 medium의 순서를 못 가른다 —
  effort의 증거로는 정답률과 함께 읽는다.

### 6.2 Phase 1 스모크 결과 (2026-09-30)

스모크 6태스크 × 1런. 기록은 `graphin-eval-sandbox/effort-runs/2026-09-30-p1-{A,B}/`.

**A (30런)** — pass: C0 6/6 · C1(opus low) 5/6 · C2(sonnet medium) 6/6 · C3(sonnet low) 6/6 ·
C4(opus medium) 6/6. 런당 비용 중앙: C3 $0.057 < C2 $0.066 < C1 $0.097 < C0 $0.104 < C4
$0.130. escaped·가짜 인용·지어낸 id 0. **C0의 비용은 부풀어 있다** — 셔플 큐 맨 앞 4런이
C0였고 그 런들이 모델의 프롬프트 캐시를 처음 쓰는 비용(생성 25~29K 토큰)을 떠안았다.
그래서 러너에 **워밍업**을 넣었다: 큐를 시작하기 전(그리고 재개할 때마다) (모델, 팔, 워커)마다
기록하지 않는 호출을 한 번 한다.

**B (36런)** — 세 팔 모두 6/6이라 스모크로는 품질 차이가 없다. 결정적인 것은 채택이다:

| 셀 | mixed touched | g-only touched | ToolSearch로 graphin을 불러온 mixed 런 |
|---|--:|--:|--:|
| C1 opus low | 0/6 | 6/6 | 0/6 |
| C2 sonnet medium | 0/6 | 6/6 | 0/6 |

mixed는 **12런 모두 첫 호출부터 Grep**이었고 graphin을 한 번도 부르지 않았다. g-only는
먼저 셸 grep을 시도했다가 막히고서야(12런 중 거부 13회) ToolSearch로 graphin을 찾았다.
원인은 **지연 로드**다 — 기본값(`ENABLE_TOOL_SEARCH=auto`)에서 모델에게 "지금 직접 부를 수
있는 graphin 도구"를 물으면 NONE이라고 답하고, `false`면 13개를 모두 댄다. mixed의 0%는
graphin을 고르지 않은 게 아니라 **보지 못한 것**이다.

**제품이 쥔 레버가 있다.** MCP 서버 설정의 `alwaysLoad: true`는 그 서버의 도구를 지연 로드에서
뺀다(도구별 예외는 `_meta anthropic/alwaysLoad: false`). `--mcp-config`로 확인했다 — graphin
도구만 처음부터 보이고 나머지는 그대로 ToolSearch 뒤에 남는다. graphin 플러그인의 MCP 설정 한
줄로 바꿀 수 있는 것이다.

이 레버를 팔로 만들어 스모크를 돌렸다 — `mixed-al`(C2, 6런): graphin 사용 **5/6**(mixed
0/6), 첫 검색기 graphin 5/6, pass 6/6, tokens_in 중앙 104.9K(mixed 79.6K), 런당 비용
$0.089($0.077). 쓴 런 중 셋은 graphin 결과 뒤에 같은 의도로 grep을 다시 했다
(same-intent 폴백 3). 토큰 증가가 항상 로드되는 스키마 값인지 탐색 방식의 차이인지는
본 실행이 가른다.

부수 수정: 거부된 호출(훅·권한)은 아무것도 가져오지 않았으므로 채택 분모에서 빼고
`search_denied`로 따로 센다. 이 수정 전에는 g-only가 "text 먼저 6/6"으로 잘못 찍혔다.

**B 설계에 미치는 영향.** 채택률이 0에 붙어 있으면 §4.4의 태스크 안 비교와 채택률 구간표는
변이가 없어 계산할 것이 없다. mixed를 5런으로 늘린 이유가 사라졌다. 대신 **노출 방식을 처치로
조작하는 팔**을 두면 채택률에 설계된 변이가 생기고, 그 변이는 인과로 읽을 수 있다.
재설계안은 소유자 결정 대기다(§9 7번).

**규모와 한도.** 1.4.3은 sonnet 108런에 $13.35(런당 $0.12), 벽시계 중앙값 42.5s였다.
opus 셀과 B의 기본 시스템 프롬프트는 런당 비용을 올리므로 **Phase 1에서 잰 단가로
다시 계산한다.** 문제는 돈보다 한도다 — 이 계정은 108런 세트를 하루 세 번 못
돌린다(429). A+B 약 1,100런은 **여러 날**에 걸친다. §2의 셔플 큐가 그것을 견디게 하는 장치다.

### 6.3 Phase 2 — 실험 A 결과와 환경 드리프트 (2026-09-30)

432런(C0·C1·C2·C3 × 36 × 3), 오류 0, 워밍업 후 시작. 기록
`graphin-eval-sandbox/effort-runs/2026-09-30-p2-A/`(report.md·analysis.md).

| 셀 | pass | C0 대비 (95% CI) | 런당 비용 중앙 | duration 중앙 | graphin 사용 |
|---|--:|---|--:|--:|--:|
| C0 sonnet 기본값 | 79/108 (73.1%) | — | $0.057 | 13.1s | 104/108 |
| C1 opus low | 91/108 (84.3%) | **+11.1%p [+2.8, +20.4]** | $0.084 | 17.4s | 55/108 |
| C2 sonnet medium | 79/108 (73.1%) | +0.0%p [−8.3, +8.3] | $0.053 | 13.2s | 102/108 |
| C3 sonnet low | 81/108 (75.0%) | +1.9%p [−4.6, +8.3] | $0.050 | 12.0s | 96/108 |

사전 규칙(§3)대로면 비열등 통과는 **C1 하나**다. sonnet 셋은 모두 절대 하한 0.80에서
떨어진다 — C0 자신도. C1의 우위는 graphin에서 오지 않았다: graphin을 쓴 런 84%, 안 쓴
런 85%이고, multi-hop에서는 27런 중 8런만 graphin을 썼다(관찰이지 인과 아님). sonnet의
effort 세 단계(기본값·medium·low)는 pass·비용 모두 서로 구별되지 않는다.

**C0가 게이트 이력보다 16%p 낮다 — 원인은 러너가 아니라 환경이다.** 1.4.3 베이스라인
(2026-09-13)은 96/108, v0.4.16 풀셋(09-22)은 97/108이었다. 게이트 러너
`eval-rag.py`를 **고치지 않고** 오늘 multi-hop 층으로 다시 돌리자 13/27 — 이 러너의
C0(14/27)과 같고 1.4.3(23/27)과 다르다. 탐색 깊이도 같이 반토막이다(런당 콜 9→5,
바이트 15.5K→8.4K, 벽시계 45s→20s). 실패의 대부분은 인용 누락이다: 예컨대
`rag-hop-truncate`에서 에이전트는 `explore_graph`로 호출자 `Server.handleToolCall`까지
정확히 찾고도 그 파일(`internal/mcp/server.go`)을 열거나 경로로 적지 않고 노드 이름만
쓰고 끝냈다. 09-22 이후 바뀐 것은 CLI(modern 프로토콜 시대, 2.1.282+ — 지금 2.1.285),
그에 맞춘 graphin 수정(94a901d·46f1638), 서버 쪽 모델 동작이다. 이 머신에는 2.1.283~
2.1.285만 남아 있고 v0.4.16 바이너리는 modern CLI에서 도구가 0개가 되므로(09-25 사고)
**깨끗한 분리 실험이 없다.** 근본 원인은 이 벤치의 범위 밖이다. **(정정: §6.5 — 원인은 모델 별칭이었고, 모델만 되돌리는 분리 실험은 가능했다.)**

**이것이 결과 해석에 미치는 것.** A 안의 셀 비교는 유효하다 — 모든 셀이 같은 날 같은
환경에서 무작위 순서로 섞여 돌았다. 절대 수준은 과거 게이트 수치와 비교할 수 없다.
그리고 **릴리스 게이트가 지금 풀셋을 돌리면 0.80을 못 넘을 가능성이 크다**(오늘
multi-hop 13/27 대 이력 23/27).

**풀셋 게이트로 확인했다 (소유자 결정, 같은 날).** 수정하지 않은 `eval-rag.py` 풀셋 108런을
`--gate` 없이 돌렸다(통과하면 릴리스 마커가 써지므로): **85/108 = 78.7% — 하한 0.80에 2런
모자란다.** 층별 answered 29/36 · multi-hop 18/27 · not-here 7/9 · out-of-reach 6/6 ·
budget-pressure 5/6 · db-nav 20/24, escaped·invented 0, 가짜 인용 1(`rag-md-section-id`가
예시 경로 `docs/a.md`를 인용). 풀셋 비용 **$6.27**(1.4.3 $13.35, v0.4.16 $14.27) — 지출이
반으로 준 것이 얕아진 탐색과 맞는다. 같은 multi-hop 9태스크가 층 단독 실행에선 13/27,
풀셋에선 18/27이라 런 간 변동도 크다. 오늘 세 번 잰 multi-hop을 합치면 45/81(56%), 이력은
23/27(85%). 기록 `graphin-eval-sandbox/effort-runs/2026-09-30-gate-full/`.

### 6.4 Phase 3 — 실험 B 결과 (2026-09-30~10-01)

C1(opus low) × 네 팔 × 36 × 3 = 432런, 오류 0. 22:52에 사용량 한도로 270/432에서 멈췄고
다음 날 `--resume`으로 마쳤다(재개 비교가 튜플·리스트 차이로 한 번 거부된 러너 버그를
고쳤다). **그 사이 CLI가 2.1.285 → 2.1.286으로 자동 업데이트돼 268런·164런으로 섞였다** —
셔플 큐라 네 팔 모두 약 60:40으로 고르게 갈렸고, 버전별 pass·채택에 차이가 보이지 않는다.
기록 `graphin-eval-sandbox/effort-runs/2026-09-30-p3-B/`(report.md·analysis.md).

**노출 처치 (mixed 대 mixed-al, 인과)** — 태스크 단위 부트스트랩 95% CI:

| 지표 | mixed(지연 로드) | mixed-al(alwaysLoad) | 차이 |
|---|--:|--:|---|
| graphin 사용 런 | 0/108 | 60/108 | **+56%p [+42, +69]** |
| pass | 84% | 82% | −2%p [−9, +6] |
| 런당 비용 | $0.126 | $0.125 | −$0.001 [−0.007, +0.005] |
| tokens_in | 85.7K | 108.3K | **+22.6K [+15.2K, +30.0K]** |
| duration | 18.5s | 17.7s | −0.9s [−1.9, +0.2] |

지연 로드 아래서 graphin은 **한 번도 쓰이지 않는다**(opus low 108런, 스모크의 sonnet medium
12런). `alwaysLoad`는 채택을 절반 넘게 끌어올리지만 품질·비용·시간은 움직이지 않는다.
토큰은 늘지만 비용이 같은 것은 항상 로드되는 스키마가 캐시 읽기로 들어가서다.

**강제 앵커 (g-only 대 s-only, 인과)** — "graphin만 쓰게 하면"의 답:

| 지표 | g-only | s-only | 차이 |
|---|--:|--:|---|
| pass | 81.5% | 82.4% | −0.9%p [−7.4, +4.6] |
| 런당 비용 | $0.142 | $0.120 | **+$0.022 [+0.015, +0.029]** (+18%) |
| tokens_in | 112.0K | 81.8K | **+30.2K [+23.5K, +37.1K]** (+37%) |
| duration | 21.5s | 18.2s | **+3.3s [+2.1, +4.5]** (+18%) |
| 콜 수 | 5.2 | 3.7 | +1.4 [+1.1, +1.8] |

리터럴 계열은 둘 다 0.92, multi-hop은 0.63 대 0.67. **이 골든셋에서 graphin은 품질을 사지
못하고 비용·토큰·시간을 더 쓴다** — 2026-09-06 grep 대조군("질문이 어휘를 흘리고 코퍼스가
작다")과 같은 결론이다. g-only 108런에서 막힌 셸 grep 시도가 111회 — 모델은 grep 쪽으로
강하게 끌린다.

**선택 후회** — mixed +0.019 [−0.046, +0.083], mixed-al +0.037 [−0.019, +0.093]. 둘 다 0과
구별되지 않는다. 앵커 둘이 같은 품질이라 어느 쪽을 골라도 잃을 것이 거의 없는 판이다.
**태스크 안 비교**(mixed-al, 채택이 갈린 12태스크): 쓴 런 13/15·폴백 런 11/13·안 쓴 런
11/15 — 표본이 작아 방향 이상을 말할 수 없다. **채택률 구간표**(관찰)에서는 콜 비율 1인 런이
28/38(74%), 0인 런이 40/47(85%)이지만, 앵커 비교가 인과로 "차이 없음"이라고 말하므로 이
간격은 태스크 난이도와 폴백의 교란으로 읽는다 — 설계 §4.4가 경고한 바로 그 함정이다.

### 6.5 게이트 드리프트의 근본 원인 — 모델 별칭 (2026-10-01, 소유자 요청)

**원인은 CLI도 graphin도 아니고, 별칭 `sonnet`이 Sonnet 5.5로 넘어간 것이다.** §6.3의
"09-22 이후 CLI modern 전환과 겹친다"는 추정은 틀렸다.

- **증거 1 — 트랜스크립트의 모델 식별자.** 1.4.3 베이스라인(09-13)의 응답 1,804개는 전부
  `claude-sonnet-5`, 오늘 풀셋 게이트의 응답 703개는 전부 `claude-sonnet-5-5`. 에이전트·
  스킬·프롬프트·태스크셋 해시는 같고, 첫 턴 입력 토큰(23.2K)과 도구 목록도 사실상 같다.
- **증거 2 — 모델만 되돌린 실험.** 오늘의 CLI(2.1.286)와 graphin(cc32660)을 그대로 두고
  게이트 러너에 `--model claude-sonnet-5`만 주자 multi-hop **25/27**, 콜 9, 13.5K, 41s —
  이력(23/27, 콜 9, 15.5K, 45s)으로 돌아왔다. 같은 날 5.5는 13/27·18/27, 콜 5, 21s.
- **증거 3 — 전환 시점.** 이 머신의 평가 세션 로그(`~/.claude/projects/-tmp-graphin-rag-*`)는
  09-26(v0.4.17 스모크)까지 전부 `claude-sonnet-5`, 09-30부터 `claude-sonnet-5-5`다.
  **게이트 대장의 모든 과거 행은 Sonnet 5로 일관되고**, 드리프트는 09-26~09-30 사이에 생겼다.

**5.5가 떨어지는 방식 — 틀린 게 아니라 검증을 덜 한다.** 오늘 5.5 multi-hop 실패 36건 중
33건은 답에 증거 문자열이 있고 두 번째 홉 파일의 인용만 빠졌다. `rag-chain-pin-drift`에서
두 모델의 답은 내용이 같다(Hash는 헤딩 포함, Rename은 본문만, 제목만 바뀐 경우를 가르는
`driftOf`). 5.5는 4콜로 `Pin`과 `driftOf`만 읽고 "Rename은 본문만 해시한다"를 이름과
주석에서 추론해 적었고, 5는 9콜로 그 계산식(`internal/parse/markdown.go:105-106`)을 찾아
인용했다. 계약("주장마다 근거를 인용한다")에 비추면 진짜 실패지만, 사용자가 받는 답의
내용은 대체로 맞다.

| multi-hop 27런 | pass | 런당 비용 중앙 | 벽시계 중앙 |
|---|--:|--:|--:|
| Sonnet 5 (오늘, 고정) | 25/27 | $0.137 | 41s |
| Sonnet 5.5 (풀셋 게이트) | 18/27 | $0.065 | 21s |
| Opus 5.5 low (실험 A C1) | 17/27 | $0.079 | 21s |

Opus 5.5 low도 multi-hop에서는 같은 방식으로 떨어진다 — A에서의 우위(84%)는 다른 층에서 왔다.

**effort로 되돌릴 수 있나 (소유자 질문, 같은 날).** Sonnet 5.5를 `claude-sonnet-5-5`로 고정한
네 셀을 multi-hop 9태스크 × 3런으로 섞어 돌렸다(`2026-10-01-s55-effort-multihop/`):

| Sonnet 5.5 | pass | 콜 중앙 | 바이트 | thinking | 런당 비용 | 벽시계 |
|---|--:|--:|--:|--:|--:|--:|
| 기본값 | 16/27 | 4 | 8.6K | 0 | $0.057 | 15s |
| medium | 19/27 | 5 | 8.0K | 0 | $0.059 | 17s |
| high | 17/27 | 7 | 11.3K | 38 | $0.080 | 20s |
| xhigh | 21/27 | 11 | 14.5K | 422 | $0.117 | 29s |
| (Sonnet 5 기본값, 같은 날) | 25/27 | 9 | 13.5K | — | $0.137 | 41s |

탐색 깊이는 effort를 따라 단조롭게 는다 — xhigh에서야 Sonnet 5만큼 파고든다. pass는 xhigh가
격차를 가장 많이 메우지만(21/27) Sonnet 5에 못 미치고, 비용은 Sonnet 5와 비슷해진다.
**medium은 기본값과 구별되지 않는다** — 오늘 19 대 16, 실험 A에서는 12 대 14로 방향이
뒤집혔고, 이틀치를 합치면 medium 31/54(57%), 기본값 61/108(56%). 셀당 27런이라 xhigh 대
기본값도 CI [−0.04, +0.44]로 0을 포함한다 — 깊이 지표는 분명하지만 pass 차이는 미확정.

**구조적 결함 하나.** 게이트 러너는 별칭만 기록한다(meta `"model": "sonnet"`). 그래서 모델이
바뀐 것이 기록 어디에도 안 보였고, 이 벤치가 우연히 발견하기 전까지 드러나지 않았다.
프로덕션도 같은 영향 아래 있다 — graphin-rag 프론트매터가 `model: sonnet`이라 사용자
세션의 explorer도 이미 5.5로 돈다.

### 6.6 프롬프트 보강 — Sonnet 5.5 medium (2026-10-01, 소유자 선택 1번)

변형 사본은 플러그인 밖 `graphin-eval-sandbox/effort-runs/variants/`에 두고, 러너의 `rag-x`
팔(`--agent-variant`)로 원본(`rag`)과 같은 큐에 섞어 돌렸다. multi-hop 9태스크 × 팔당 5런,
`claude-sonnet-5-5` medium.

**v2 — 주장별 근거 감사 (효과 없음).** "보고 전 점검" 맨 앞에 조항 하나: 주장마다 받은 도구
결과의 `path:line`을 짚어라, 노드 id·심볼 이름·필드 이름·주석은 단서이지 근거가 아니다,
짚을 것이 없는 주장은 지금 읽거나 "검증하지 못한 것"으로 옮겨라.
pass 0.53 → 0.58(+4%p [−9, +18]), 콜 4.8 → 5.0, 바이트 +1.3K, 비용 +$0.003. 어려운 셋
(`model-mismatch`·`pin-drift`·`patternshape`)은 두 팔 모두 0/5. **조항은 행동을 바꿨지만
의도와 다른 갈래로 갔다** — 5.5는 읽는 대신 매번 옮겼다. `pin-drift`에서 "body alone이라는
주장은 해시 코드가 아니라 주석에 근거한다"를, `patternshape`에서 "지표를 계산하는 코드는
읽지 않았고 스펙에서 가져왔다"를 스스로 적었다. 보고는 정직해졌지만 한 번 더 읽는 대신
미검증을 선언한다 — 5.5는 추가 탐색보다 면책을 고른다.

**v3 — 답의 핵심은 미룰 수 없다 (일부 효과, 미확정).** v2의 탈출구만 막았다: 짚을 것이 없는
주장은 지금 읽는다, 주변 주장만 "검증하지 못한 것"으로 옮길 수 있고 **질문에 답하는 주장은
예산이 남은 한 옮길 수 없다**. 같은 날 대조군과 다시 섞어 팔당 45런:
pass 0.51 → 0.62(+11%p [−4, +27]), 콜 4.6 → 5.1(+0.5 [+0.2, +0.9]), 바이트 +1.9K, 비용
+$0.006(+10%). `rag-hop-truncate` 2/5 → 5/5, 어려운 셋은 각각 0/5 → 1/5. 남은 실패는 금지한
그 행동이다 — `pin-drift`에서 4콜로 멈추고 "두 해시를 계산하는 코드는 읽지 않았다, 무엇을
덮는지는 주석에서 왔다"를 다시 미검증 칸에 적었다.

**결론: medium에서는 프롬프트로 격차가 닫히지 않는다.** 두 번 시도해 +4%p, +11%p — 방향은
맞지만 어느 쪽도 0과 구별되지 않고, 수준(약 60%)은 Sonnet 5(25/27)와 5.5 xhigh(21/27)에
한참 못 미친다. 5.5 medium은 명시적 금지보다 자기 정지 판단을 앞세운다. 깊이를 실제로 움직인
것은 effort였다(§6.5: 콜 4 → 11). 변형 사본 둘은 그대로 남겨 둔다 — v3 조항은 effort를 올린
조합에서 다시 쓸 후보다.

**v3 + high (첫 유의한 효과, multi-hop 한정).** 같은 큐에 `claude-sonnet-5-5` high 원본과 v3를
섞어 팔당 45런(`2026-10-01-v3-high-multihop/`): pass 0.71 → **0.80 (+9%p [+2, +16])**, 콜
6.9 → 7.2(n.s.), 바이트 +1.7K, thinking 중앙 80 → 108, 비용 $0.081 → $0.088(+9%), 벽시계 차이
없음. 지어낸 id 미스 1(v3 팔). high로 탐색을 이어 갈 여지가 생기자 v3 조항이 따라졌다 —
콜 수가 아니라 같은 콜로 맞는 곳을 읽는 쪽으로 움직였다.

| multi-hop | pass | 런당 비용 |
|---|--:|--:|
| 5.5 medium 원본 / v3 | 51% / 62% | $0.059 / $0.065 |
| 5.5 high 원본 / **v3** | 71% / **80%** | $0.081 / **$0.088** |
| 5.5 xhigh 원본 | 78% (21/27) | $0.117 |
| Sonnet 5 원본 | 93% (25/27) | $0.137 |

v3 + high는 xhigh 수준의 품질을 비용 25% 낮게 낸다. **미확인 셋**: 9태스크만이라 태스크
단위 부트스트랩이 불안정하다, v3가 다른 층(위치 질문의 절약, budget-pressure, not-here)을
해치는지 안 쟀다, high 원본도 날마다 흔들린다(오늘 71%, 앞선 실행 17/27=63%) — 같은 큐 대조군
비교만 믿는다. 다음은 풀셋 검증이다.

**풀셋 검증 — v3 탈락, high 원본은 하한 통과 (2026-10-01, 소유자 승인).** 수용 기준은 런 전에
고정했다: rag-x 전체 ≥ 0.80, 원본 대비 −5%p 넘게 떨어진 층 없음, not-here·out-of-reach 신규
정직성 위반 없음. `claude-sonnet-5-5` high, 원본과 v3를 섞어 36태스크 × 팔당 3런 = 216런,
오류 0(`2026-10-01-v3-high-full/`).

| 층 | high 원본 | high + v3 | 차이 |
|---|--:|--:|--:|
| answered | 32/36 | 32/36 | 0 |
| multi-hop | 18/27 | 19/27 | +3.7%p |
| not-here | 7/9 | 9/9 | +22.2%p |
| out-of-reach | 6/6 | 6/6 | 0 |
| budget-pressure | 6/6 | 4/6 | **−33.3%p** |
| db-nav | 23/24 | 21/24 | **−8.3%p** |
| **전체** | **92/108 (85.2%)** | 91/108 (84.3%) | −1%p |
| 런당 비용 | $0.078 | $0.081 | +$0.003 |

**v3는 둘째 기준에서 탈락한다.** budget-pressure 실패 하나는 `over_silent`(21.9KB, 20KB
예산) — "반드시 읽어라"가 좁은 예산과 부딪힌다는 기제와 맞는다. 층당 표본이 6·24라 두 런
차이지만 기준은 런 전에 고정한 것이라 그대로 적용한다. multi-hop의 +9%p(§ 위)도 풀셋에서는
+1런으로 줄어 재현되지 않았다.

**더 쓸모 있는 결과는 대조군이다 — 프롬프트를 고치지 않은 Sonnet 5.5 high가 85.2%로 하한
0.80을 넘는다.** 같은 날 별칭 기본값의 풀셋 게이트는 78.7%였다. 런당 $0.078은 Sonnet 5
시절 1.4.3 베이스라인($0.124)보다 약 37% 싸다. multi-hop은 여전히 약하다(오늘 high 원본
세 번: 18/27·32/45·17/27 = 67/99, 68%; Sonnet 5는 25/27) — 다른 층이 그것을 메운다.
풀셋 한 번의 측정이므로 반영 뒤 게이트 재베이스라인이 두 번째 측정이 된다.

## 7. 결과가 제품에 닿는 길

A가 설정 하나를 고르면 `graphin-rag.md`에 `model:`·`effort:`를 적는다(graphin-guide
버전 올림). **그 순간 게이트가 다른 에이전트를 재게 된다** — eval-rag의 자식은
`--system-prompt-file`로 프롬프트를 받고 프론트매터는 버리므로(`compose_prompt`),
`effort:`는 게이트에 닿지 않고 게이트는 계속 사용자 설정의 effort로 돈다. 그래서
eval-rag가 프론트매터의 `model`·`effort`를 읽어 플래그로 넘기게 고쳐야 한다. 이건
**러너 변경이라 변경 통제 대상**(unlock·`lock --approved-by`·트레일러)이고
`RUN_COMPAT` 리셋 → 게이트 재베이스라인이 따라온다. 동시에 대장의 과거 행에
"effort 미상" 주석이 필요하다. 같은 변경에서 **권한 모드도 고정**해야 한다 —
게이트가 사용자 설정의 `defaultMode: auto`를 상속해 Bash 가용성이 분류기에 달려
있었다(§3). `--setting-sources project,local`을 쓰면 둘 다 한 번에 닫힌다.

B의 결과는 제품 변경을 직접 요구하지 않는다. 쓰임은 두 가지다. 가이드 스킬의
description·MCP 서버 instructions가 채택을 유도하는 힘의 기준선이 되고, 실사용
리포트(`graphin usage report`)의 채택률을 읽을 때 "그 수치가 좋은 편인가"의 참조점이 된다.

## 8. 알려진 한계

- **코퍼스가 작고 질문이 어휘를 흘린다.** grep 대조군에서 grep 75/81 대 graphin
  69/81, p=0.227. B에서 `mixed`가 grep으로 기우는 것이 이 코퍼스에선 합리적일 수
  있다 — 결론을 "graphin 일반"으로 넓히지 않는다. 산문형·대형 저장소 축은
  SWE-Explore(외부)의 몫이다.
- **B는 메인 세션이 탐색을 직접 하는 경우만 본다.** 프로덕션의 또 다른 채택
  층 — 메인 세션이 graphin-rag에 **위임하느냐** — 은 재지 않는다.
- **A의 Bash 차이**(§3), **B의 wiki 게이트 부재**(§4.1)는 의도한 차이다.
- 서버가 런마다 새로 뜨는 비용은 프로덕션(세션당 한 번)과 다르다. 시간은 §5의 분해로만 읽는다.
- 채점기가 인용 모양에 민감하다(말줄임 경로·검색 로그의 금지 리터럴). effort가
  낮을수록 답이 짧아져 모양 실패가 늘 수 있으므로 **모양 실패(`elided_citations` 등)를
  답 실패와 따로 센다.**

## 9. 소유자 결정 (2026-09-30, 전부 제안대로)

1. **B의 규모** — 표준안 792런(C1·C2).
2. **비열등 마진** — −5%p(부트스트랩 95% 하한).
3. **C3·C4 풀셋** — Phase 1 결과를 보고 결정.
4. **B의 시맨틱** — 하이브리드 + `--semantic-wait`.
5. **B에서 graphin 플러그인 훅** — 뺀다(§4.1).
6. **§7의 eval-rag 변경** — A 결과가 나온 뒤, 변경 통제 절차에 따라 별도 허가로.
7. **(Phase 1 이후)** A 본 실행에 C3를 넣는다(C0·C1·C2·C3, 432런). B는 노출 처치 팔
   `mixed-al`을 더해 네 팔 × 3런, A 승자 셀 하나로 재설계(432런) — §4.2.
