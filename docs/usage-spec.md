# Usage 스펙 — 인접 툴콜 시퀀스 채택/폴백 계측 (v1)

Claude Code **플러그인 훅**(PostToolUse)으로 툴콜 스트림을 로깅하고,
`graphin usage report`가 인접 시퀀스 통계로 graphin 채택/폴백 신호를 뽑는다.
계측은 **관찰 전용**이다 — 에이전트 행동에 개입하는 순간 지표가 오염된다(§8).

## 0. 목표와 지표 철학

에이전트 세션에서 graphin이 실제로 쓰이는지, 어디서 버려지는지를 실측한다.
헤드라인 신호 4종:

| 시퀀스 | 해석 | 의미 |
|---|---|---|
| `graphin → Read/Edit` | 채택 | 원하는 걸 찾았다 |
| `graphin → Grep` | 폴백 | 결과 불만족 — **금맥**: 인덱스/랭킹 개선의 실측 테스트케이스 |
| `Grep → … → graphin` | 늦은 전환 | 유도 실패 (스킬/문구가 graphin을 먼저 태우지 못함) |
| `Grep→Grep→Grep`, graphin 없음 | 발견 실패 | 인덱스가 있는데도 고려조차 안 됨 |

폴백 중에서도 **같은 의도의 폴백**(graphin 쿼리와 후속 grep 패턴의 토큰이
겹침)이 진짜 신호다 — 그 (query, pattern) 쌍이 곧 search_hybrid가 놓친
리터럴 재현 케이스가 된다. 겹침이 없는 폴백은 새 의도의 검색일 뿐,
graphin의 실패가 아니다.

## 1. 플러그인 구조와 설치

계측은 서버와 **같은 플러그인**에 있다. 별도 플러그인이던 `graphin-usage`는
제거됐다([plugin-distribution](plugin-distribution.md) §8.2) — 서버와
훅이 한 플러그인에 있어야 워크스페이스 설정을 공유하고, 훅이 플러그인이 설치한
바이너리를 곧장 찾을 수 있다.

```
plugin/graphin/
├── .claude-plugin/plugin.json   # name "graphin", userConfig 7종
├── hooks/
│   ├── hooks.json               # PostToolUse matcher "*" + SessionStart
│   └── usage.sh                 # sh 가드 + graphin 바이너리 해석
├── commands/report.md           # /graphin:report
└── …                            # 서버·런처·설치기는 plugin-distribution §6
```

- 로컬 개발: `claude --plugin-dir /path/to/graphin/plugin/graphin`
  (해당 세션 한정). 정식 설치는 `/plugin install graphin@graphin`.
- **핫리로드 함정**: `hooks.json`/`plugin.json` 변경은 자동 반영되지 않는다 —
  `/reload-plugins` 또는 재시작 필요. 반면 `usage.sh`와 graphin 바이너리는
  **매 툴콜마다 fresh 실행**되므로 수정이 즉시 먹는다. 개발 루프에서 리로드가
  필요한 건 훅 *설정* 변경뿐이다.
- 커맨드는 플러그인 이름으로 네임스페이스된다: `/report`가 아니라
  **`/graphin:report`**. 모든 문서·유도 문구는 이 형태로 쓴다.

## 2. 훅 핸들러와 가드

핸들러는 **어떤 경로로도 세션을 막지 않는다**: 모든 실패는 exit 0,
stdout은 절대 비운다(출력이 있으면 Claude Code가 해석하려 든다).

### 2.1 가드 — 스코프 오염 차단

유저 스코프로 설치되면 훅은 graphin을 쓰지 않는 프로젝트에서도 발화한다.
가드 없이는 인덱스도 없는 프로젝트의 Grep이 폴백률 분모에 섞인다.

`usage.sh` 1단계 (fork 없는 순수 sh builtins, 비대상 프로젝트 ~수 ms):

1. 시작점 `d` = `$GRAPHIN_USAGE_ROOT` → 없으면 `$CLAUDE_PROJECT_DIR` → 없으면 exit 0
2. `d`에서 최대 8단계 walk-up 하며 `$d/.graphin/merkle.json` 존재 확인, 없으면 exit 0

마커가 **`merkle.json`인 이유**: `.graphin/` 디렉토리는 서버가 뜨기만 해도
생긴다(`obs.New`가 `agent-nav.log`를 만든다 — internal/obs/log.go). 반면
`merkle.json`은 초기 스캔이 완료된 뒤 `persistIndexesLocked`에서만 기록된다
(internal/workspace/indexer.go) — "인덱싱이 실제로 끝난 워크스페이스"의 프록시.

한계 둘:

- 워크스페이스가 프로젝트 루트보다 **아래**에 있으면 walk-up이 못 찾는다 →
  `GRAPHIN_USAGE_ROOT` 또는 플러그인의 `workspace_subdir` 옵션으로 지정한다.
- **워크스페이스의 최초 `bootstrap_workspace` 호출은 구조적으로 기록되지
  않는다.** 마커는 초기 스캔이 끝나야 생기는데 그 호출의 PostToolUse는 그보다
  먼저 발화하기 때문이다(2026-08-06 실측: 3콜 중 2건 기록). 의도된 동작이지만
  `g_boot` 카운트는 워크스페이스당 1회씩 적게 잡힌다 — 헤드라인 4종은
  `g_boot`을 쓰지 않으므로 영향이 없다.

### 2.2 바이너리 해석 — binpath 사이드카

graphin은 PATH에 없는 경우가 흔하다. 서버가 기동 시
`<ws>/.graphin/binpath`에 `os.Executable()`을 best-effort 기록하는 것은 그
때문이고, 플러그인 배포 이후에도 **유지한다** — 갱신하지 않은 옛 설치와
직접 등록한 서버가 그것에 의존한다.

해석 순서(전부 실패 시 exit 0, 침묵 — §5 진단으로 가시화):

1. `$GRAPHIN_BIN`
2. `$CLAUDE_PLUGIN_OPTION_BINARY_PATH` — 사용자가 지정한 자기 빌드
3. `${CLAUDE_PLUGIN_DATA}/bin/graphin` — 플러그인이 설치한 **심볼릭 링크**
4. `<root>/.graphin/binpath` — 레거시
5. `command -v graphin`

**3이 4보다 위인 것이 핵심이다.** `os.Executable()`은 리눅스에서
`/proc/self/exe`를 읽어 **심볼릭 링크를 해석**하므로, 링크로 기동된 서버는
binpath에 버전 박힌 실제 파일(`graphin-0.1.0-linux-amd64`)을 쓴다. 다음
업그레이드가 그 파일을 정리하면 binpath는 없는 곳을 가리키고, 계측이 조용히
죽으며 `usage report`는 "인덱스는 있는데 이벤트가 없다"만 출력한다. 링크를
위에 두면 자가 치유된다 — `e2e/plugin_test.go`가 이 순서를 고정한다.

### 2.3 인제스트 — `graphin usage ingest`

- stdin(≤8MB)의 PostToolUse JSON을 파싱, `hook_event_name`이 다르면 무시.
- 루트 라우팅: stdin `cwd`에서 ≤8 walk-up으로 가장 가까운 `merkle.json` 루트
  → 실패 시 핸들러가 넘긴 `$GRAPHIN_WS_ROOT` → 둘 다 없으면 exit 0.
  (한 세션이 여러 워크스페이스를 오가도 이벤트가 올바른 루트에 쌓인다)
- `<root>/.graphin/usage/events.jsonl`에 O_APPEND **단일 write**로 1줄 기록
  (로컬 FS에서 라인 원자성). 라인이 8KB를 넘으면 페이로드를 드롭하고 재직렬화.
- 회전: 기존 파일이 32MiB를 넘으면 `events-<UTC스탬프>.jsonl`로 rename
  (동시 세션과의 경합 패배는 무시).
- 프라이버시: Bash **전체 커맨드라인**, 파일 내용, tool_response 본문은
  저장하지 않는다. 문자열 필드는 300자 절단.

## 3. 이벤트 스키마 (`"v": 1`)

| 필드 | 타입 | 비고 |
|---|---|---|
| `v` | int | 스키마 버전, 항상 1. 리더는 `v>1`을 problems로 스킵(전방 호환) |
| `ts` | string | RFC3339Nano UTC, 인제스트 시점 |
| `session_id` | string | 스트림 분할 키 |
| `prompt_id` | string | 지표 윈도우(유저 턴 1개) |
| `tool_use_id` | string | dedupe 키 |
| `parallel` | bool | 병렬 배치 소속 여부 |
| `agent_id`, `agent_type` | string, 선택 | 서브에이전트 콜에만 존재. 부재 = 메인 루프 |
| `cwd` | string | 루트 상대(루트 밖이면 절대) |
| `tool` | string | 원본 `tool_name` |
| `p` | object, 선택 | 툴별 추출 페이로드(아래) |

페이로드 추출 — MCP 툴은 **접미사 매칭**(`mcp__*__search_hybrid`):
`mcp__<server>__`의 server 세그먼트는 사용자가 `claude mcp add <key>`로 정한
config key라 신뢰할 수 없다.

| 툴 | p |
|---|---|
| `search_hybrid` | query, top_k + 응답에서 result_count, result_ids(≤5) |
| `explore_graph` | node_id, direction |
| `read_code` | node_id |
| `Grep` | pattern, path(상대), glob |
| `Glob` | pattern |
| `Read`/`Edit`/`Write` | file_path(상대) |
| `Bash` | search(bool) + search일 때만 pattern. 파이프/`&&`/`;` 세그먼트별 argv[0] ∈ {grep, rg, egrep, fgrep, ag, ack, fd, find} 또는 `git grep` |
| 기타 | p 생략 |

## 4. 지표 조작적 정의

### 4.1 스트림·윈도우 구성

1. `(session_id, agent_id||"main")`별 스트림. **파일 append 순서 유지** —
   `ts` 정렬 금지(클럭 스큐 면역; append는 writer별 시간순이고 분할이
   인터리브를 제거한다).
2. 스트림 내 `tool_use_id` 중복 제거.
3. **병렬 배치 collapse**: 같은 `prompt_id`의 연속 `parallel:true` 런을
   무순서 배치 노드(클래스 합집합) 하나로 접는다. 배치 내부 bigram은 만들지
   않는다. PostToolUse에 배치 id가 없어 연속성 휴리스틱이다 — 한 프롬프트의
   연속 두 배치는 병합될 수 있으나 무순서 집합 의미론에선 수용.
4. `prompt_id`로 **프롬프트 윈도우** 분할 — 헤드라인 지표의 단위.

클래스: `g_search`(search_hybrid) / `g_explore` / `g_read` / `g_boot` /
`g_bench` / `search`(Grep·Glob·search-Bash) / `read`(Read) /
`action`(Edit·Write) / `other`. **graphin 내비 콜** = g_search|g_explore|g_read.

### 4.2 헤드라인 4종

윈도우 안 graphin 내비 콜의 **최대 연속 런**마다, 같은 윈도우의 첫
비-`other` 후속 원소로 판정한다:

1. **채택**: 후속이 `read`/`action`이거나, 런이 `g_read`로 끝나고 후속이
   `action`/윈도우 종료. (`read_code`는 퍼널이 graphin 안에서 완결된 것 —
   채택으로 센다.) 후속이 배치면: `search` 포함 → 폴백(비관적 타이브레이크 —
   read 옆의 병렬 grep도 미충족 수요 신호다), 아니면 `read`/`action` 포함 → 채택.
2. **폴백**: 후속이 `search`. **same-intent 판별**: 런의 마지막 `g_search`
   쿼리와 폴백 pattern을 토크나이즈(소문자, 비영숫자 분리, 3자 미만·불용어
   제거)해 겹침 ≥1 → same-intent 폴백. 리포트에 (query, pattern) 실쌍
   top-N 출력. 겹침 0 → 신규 의도.
3. **늦은 전환**: 첫 graphin 내비 콜 **이전에** `search` ≥2인 윈도우.
   분모 = graphin 내비 콜이 있는 윈도우.
4. **발견 실패**: `search` ≥3이고 graphin 내비 콜 0인 윈도우. 분모 =
   `search`가 있는 윈도우. 가드 덕에 전 이벤트가 인덱스 존재 프로젝트이므로
   모든 발견 실패는 진짜 미스다.

판정 불가 런(윈도우 종료까지 후속 없음, `g_read` 미종결)은 **inconclusive**로
별도 카운트. 채택률 = 채택 / (채택 + 폴백).

### 4.3 부가 지표

- **퍼널 준수율**: `g_search`의 result_ids ∈ 후속 explore/read node_id —
  progressive disclosure의 ID 핸드오프가 실제로 일어나는지.
- **세션 레벨 채택**: graphin 내비 콜 ≥1 세션 / 전체 세션. 0회 세션 수.
- **최초 graphin까지 이벤트 수** (중앙값).
- 클래스 **bigram 전이 행렬**, **일자별 추이**, **main vs 서브에이전트 분리**.

## 5. `graphin usage report`

```
graphin usage report [--log <dir|file>] [--since <YYYY-MM-DD|72h>] [--json] [--top N]
```

- `--log` 기본값: cwd에서 walk-up으로 `.graphin/usage/` 자동 발견.
  `events*.jsonl` glob(회전 파일 포함).
- 리더는 관대하다(sweexplore `LoadBench` 패턴): 깨진 라인·`v>1`은
  `problems`로 수집하고 리포트에 카운트만 노출한다.
- 출력: 마크다운 파이프 테이블(기본) / `--json`(Report 구조체 그대로).
- 진단: `merkle.json`은 있는데 이벤트 파일이 없으면 "플러그인이 설치되어
  발화 중인지 확인하라"고 안내한다 — 바이너리 미해석 등 침묵 실패의 가시화.

## 6. 운영 노트

- **상시 컨텍스트 비용**: 훅은 출력이 없는 한 harness-only(상시 0)이고,
  커맨드 1개가 ~100–200 tok을 더한다. **릴리스 게이트**:
  `claude plugin details graphin`으로 상시 비용을 확인한 뒤 배포한다. 다만
  `graphin`은 커맨드가 4개(report·setup·doctor·admin)이므로 기준선은 그만큼이다.
- **네임스페이싱**: 호출형은 항상 `/graphin:report`.
- **기업 환경**: 관리자가 `allowManagedHooksOnly`를 켜면 사용자·프로젝트·
  플러그인 훅이 차단된다. 예외는 관리형 설정 `enabledPlugins`로 강제 활성화된
  플러그인의 훅뿐 — 사내 배포는 관리형 마켓플레이스 등재 경로를 탄다.

## 7. 수용 기준

1. 비-graphin 프로젝트: 훅이 아무 파일도 만들지 않고 체감 지연이 없다.
2. graphin 프로젝트: Grep/Read/graphin 콜이 각각 스키마대로 1줄씩 쌓인다.
3. `graphin usage report`가 헤드라인 4종 + same-intent 폴백 쌍을 출력한다.
4. `make vet test` 그린 (유닛 + e2e 훅 블랙박스).
5. `claude plugin details graphin`의 상시 비용이 커맨드 4개분 이내(훅은 0).

## 8. 열린 질문

- **유도 스킬은 별도 플러그인으로 분리했다** — 해결됨
  ([plugin-distribution](plugin-distribution.md) D3). 스킬과 에이전트는
  `plugin/graphin-guide/`에 있고, 계측은 `plugin/graphin/`에 있다. 한 플러그인에
  섞으면 측정과 개입이 섞여 베이스라인이 오염되는데, 그건 **한번 섞이면 복구가
  안 된다** — 설치 여부로 갈리게 두면 무개입 베이스라인이 계속 관측 가능하다.
  전후 비교는 guide 설치 시점을 경계로 `usage report --since`로 낸다.
- 로그 프루닝(`--prune-before`) — v1은 32MiB 회전만.
- `PostToolBatch` 활용(배치 경계 정밀화) — v1은 `parallel` 휴리스틱으로 충분.
- DB 스키마 질의(`db.` node_id) 채택률 분리 집계.
