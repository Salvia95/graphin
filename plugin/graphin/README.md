# graphin (Claude Code 플러그인)

[graphin](https://github.com/Salvia95/graphin) MCP 서버를 **설치 없이** 쓰기 위한
플러그인. 저장소를 클론하거나 바이너리 경로를 손으로 배선할 필요가 없다 — 플러그인이
자기 바이너리를 받아 검증하고 실행한다.

```
/plugin marketplace add Salvia95/graphin
/plugin install graphin@graphin
```

**요구사항: Claude Code 2.1.83 이상**(`userConfig` 지원). 배포 플랫폼은
**linux/amd64 · linux/arm64**. 그 외 플랫폼은 아래 "다른 플랫폼"을 보라.

## 무엇이 딸려 오는가

| 구성 | 내용 |
|---|---|
| MCP 서버 | `search_hybrid` · `explore_graph` · `read_code` · `bootstrap_workspace` · `run_local_benchmark` |
| 명령 | `/graphin:doctor` · `/graphin:setup` · `/graphin:admin` · `/graphin:report` |
| 훅 | SessionStart(설치 예열) · PostToolUse(채택 계측) |

도구 이름은 `mcp__plugin_graphin_graphin__*`이다 — 플러그인이 제공하는 MCP 서버는
Claude Code가 `plugin:<플러그인>:<서버>`로 네임스페이싱하기 때문이다.

## 설정 (`/plugin` → Manage → graphin → Configure)

전부 선택 사항이다. 비워 두면 서버 기본값을 쓴다.

| 옵션 | 용도 |
|---|---|
| `admin_addr` | 관리자 페이지 주소(`127.0.0.1:7466`). 비우면 비활성 |
| `model_type` | `english_optimal` \| `multilingual_cjk` |
| `offline` | 다운로드 금지. `model_dir`과 함께 쓴다 |
| `model_dir` | 로컬 ONNX 모델·토크나이저 디렉터리 |
| `semantic_max_nodes` | 이 노드 수를 넘으면 의미 검색 비활성(lexical 유지) |
| `workspace_subdir` | 프로젝트 루트 대신 하위 디렉터리를 인덱싱 |
| `binary_path` | 내려받는 대신 이 바이너리를 쓴다 |

> **`admin_addr`는 전역 값 하나다.** 플러그인 옵션은 user settings에만 저장되므로
> 프로젝트별로 다르게 줄 수 없다. 프로젝트마다 다른 포트가 필요하면
> `<프로젝트>/.graphin/admin-addr`에 주소를 한 줄 써라 — 런처가 전역 옵션보다 먼저
> 읽는다. `/graphin:admin`이 대신 해 준다.

## 바이너리는 어디서 오는가

`~/.claude/plugins/data/graphin-graphin/`에 설치된다. 플러그인 디렉터리가 아니라
**데이터 디렉터리**인 이유는 플러그인 업데이트가 전자를 통째로 갈아치우기 때문이다 —
그랬다면 업데이트마다 25MB를 다시 받는다.

```
bin/graphin-1.0.0-linux-amd64      # 버전 박힌 실제 파일
bin/graphin -> graphin-1.0.0-…     # 링크만 갈아끼운다
state/{manifest.json, installed.json, last-error.txt}
logs/install.log
```

설치 순서: `binary_path` → 이미 설치된 것(매니페스트 동일) → 이미 받아 둔 버전 재링크
→ 릴리스 다운로드(SHA256 검증, 불일치면 중단) → `go install` 폴백. 어느 단계에서
왔는지는 `state/installed.json`의 `source`에 남는다.

## 첫 실행이 느릴 때

바이너리 25MB를 처음 받는 세션은 MCP 기동 타임아웃을 넘길 수 있다. 그때는
`/mcp`에서 graphin을 재연결하거나, `MCP_TIMEOUT`을 올려 잡는다(예: `60000`).
SessionStart 훅이 보통 먼저 이겨서 다운로드를 기동 밖으로 밀어내지만, 순서는
보장되지 않는다.

## 이미 `claude mcp add graphin`을 해 뒀다면

**지워야 한다.** Claude Code는 커맨드가 다르면 중복으로 보지 않으므로 수동 등록과
플러그인 서버가 **둘 다** 뜨고, 같은 워크스페이스 락을 두고 다투다 뒤에 뜬 쪽이
반쪽으로 죽는다.

```sh
claude mcp remove graphin -s local
claude mcp remove graphin -s user
claude mcp remove graphin -s project
```

`/graphin:doctor`가 이 상태를 감지한다.

## 다른 플랫폼

| 플랫폼 | 상태 |
|---|---|
| linux/amd64 · linux/arm64 | 릴리스 바이너리 제공, 의미 검색 가능 |
| darwin/arm64 | 릴리스 바이너리는 없지만 **`go install` 폴백이 자동으로 빌드**하고, 의미 검색도 동작한다(ORT 핀 있음). Go 툴체인과 clang이 필요하다 |
| darwin/amd64 | onnxruntime 1.26.0 빌드 자체가 없다 — 의미 검색 영구 불가(lexical은 동작) |
| windows | 범위 밖 |

`graphin version --json`의 `semantic_supported`가 이 판정을 한 줄로 알려 준다.

## 문제가 생기면

`/graphin:doctor`가 바이너리 출처·플랫폼 지원·인덱스 상태·중복 등록을 한 번에
점검한다. 설치를 다시 하려면 `/graphin:setup`.

## 계측 (무엇을 기록하나)

PostToolUse 훅이 툴콜 시퀀스를 `<워크스페이스>/.graphin/usage/events.jsonl`에
기록한다 — gitignore된 `.graphin/` 안, **로컬 전용, 외부 전송 없음**, 32MiB 회전.
집계는 `/graphin:report`.

| 기록한다 | 기록하지 않는다 |
|---|---|
| 툴 이름, graphin 쿼리·node_id | **Bash 전체 커맨드라인** |
| Grep/Glob 패턴 | 파일 내용 |
| Read/Edit 파일 경로(루트 상대) | 툴 응답 본문 |
| Bash의 "검색인가" 여부와 그 검색 패턴 | (예외: graphin 검색 결과 id 최대 5개) |

문자열은 300자에서 잘린다. graphin 인덱스(`.graphin/merkle.json`)가 없는
프로젝트에서는 훅이 즉시 종료하고 아무것도 기록하지 않으므로, user 스코프로
설치해도 다른 프로젝트를 건드리지 않는다.

**관찰이 개입과 섞이지 않게 되어 있다.** 이 플러그인은 에이전트 행동을 유도하지
않는다 — 유도는 별도의 [`graphin-guide`](../graphin-guide/README.md)이고, 설치
여부가 곧 "유도 없는 베이스라인"과의 경계다.

이벤트가 안 쌓일 때는 `/graphin:doctor`가 훅 발화 여부까지 점검한다. 워크스페이스가
프로젝트 루트보다 아래에 있으면 `workspace_subdir` 옵션을 설정해야 훅이 찾는다.
설계 전문: [docs/usage-spec.md](../../docs/usage-spec.md).
