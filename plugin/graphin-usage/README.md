# graphin-usage — 채택/폴백 계측 플러그인

graphin이 인덱싱한 프로젝트에서 툴콜 시퀀스를 로깅하고(`PostToolUse` 훅),
`graphin usage report`로 채택/폴백 통계를 낸다. 설계 전문: `docs/usage-spec.md`.

**관찰 전용이다.** 이 플러그인은 에이전트 행동에 개입하지 않는다 — 유도
스킬을 같이 넣으면 측정 베이스라인이 오염된다(스펙 §8).

## 설치

로컬 개발/시험 (해당 세션 한정):

```sh
claude --plugin-dir /path/to/graphin/plugin/graphin-usage
```

정식 설치는 마켓플레이스 경유. 설치 스코프와 무관하게 **가드가 있어**
graphin 인덱스(`.graphin/merkle.json`)가 없는 프로젝트에서는 훅이 즉시
종료하고 아무것도 기록하지 않는다.

전제: graphin MCP 서버가 해당 워크스페이스에서 한 번은 떠서 인덱싱을
마쳤어야 한다(마커·binpath가 그때 생긴다).

## 무엇을 기록하나 (프라이버시)

`<workspace>/.graphin/usage/events.jsonl` (gitignore된 `.graphin/` 안, 로컬
전용, 32MiB 회전):

- 기록: 툴 이름, graphin 쿼리/node_id, Grep/Glob 패턴, Read/Edit 파일 경로
  (루트 상대), Bash의 검색 여부 + 검색 패턴. 문자열은 300자 절단.
- 기록하지 않음: **Bash 전체 커맨드라인**, 파일 내용, 툴 응답 본문
  (graphin 검색 결과 id 최대 5개 제외).

## 리포트

```sh
graphin usage report [--since 72h] [--json] [--top 30]
```

세션 안에서는 **`/graphin-usage:report`** 로 실행한다 (플러그인 커맨드는
플러그인 이름으로 네임스페이스된다 — `/report` 아님).

## 개발 루프

- `hooks.json`/`plugin.json` 변경은 **핫리로드되지 않는다** —
  `/reload-plugins` 또는 재시작.
- `handler.sh`와 graphin 바이너리는 매 툴콜마다 fresh 실행이라 수정이 즉시
  반영된다.
- 배포 전 `claude plugin details graphin-usage`로 상시 컨텍스트 비용을
  확인하라 — 훅은 harness-only(0), 커맨드 1개분(~100–200 tok)만 나와야 정상.

## 트러블슈팅

인덱스는 있는데 이벤트가 안 쌓일 때 (`graphin usage report`가 "index present
but no usage events"를 낼 때), 순서대로:

1. **플러그인이 로드됐나** — `claude plugin details graphin-usage`.
   훅 변경 후에는 `/reload-plugins`.
2. **바이너리 해석 실패** — 훅은 `$GRAPHIN_BIN` → `<root>/.graphin/binpath` →
   PATH 순으로 graphin을 찾고, 전부 실패하면 조용히 포기한다.
   `binpath`는 서버가 기동할 때 기록되므로 구버전 바이너리로 인덱싱한
   워크스페이스에는 없을 수 있다 → 서버를 한 번 재기동하거나
   `GRAPHIN_BIN=/path/to/graphin`을 세션 환경에 설정.
3. **가드 미매칭** — 워크스페이스가 Claude 프로젝트 루트보다 **아래**에
   있으면 walk-up이 못 찾는다 → `GRAPHIN_USAGE_ROOT=/path/to/workspace` 설정.
4. 수동 재현: `echo '<PostToolUse JSON>' | CLAUDE_PROJECT_DIR=$PWD sh
   plugin/graphin-usage/hooks/handler.sh; echo $?` — 항상 0이어야 하고,
   성공 시 `.graphin/usage/events.jsonl`에 줄이 는다.

## 기업 환경

관리자가 `allowManagedHooksOnly`를 켠 조직에서는 사용자/프로젝트/플러그인
훅이 차단된다. 예외는 관리형 설정 `enabledPlugins`로 강제 활성화된 플러그인
— 사내 배포는 관리형 마켓플레이스 등재 경로를 타야 한다.
