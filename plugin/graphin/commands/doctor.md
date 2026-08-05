---
description: graphin 설치·연결 진단 — 바이너리 출처, 플랫폼 지원, 인덱스 상태, 중복 MCP 등록을 점검한다
allowed-tools: Bash(*), Read
---

graphin 플러그인이 정상 동작하는지 진단하고, 문제가 있으면 **조치까지** 제시하라.
아래를 순서대로 실행하고, 마지막에 짧은 요약 표 하나로 정리한다.

## 1. 바이너리

```sh
D="${CLAUDE_PLUGIN_DATA:-$HOME/.claude/plugins/data/graphin-graphin}"
ls -l "$D/bin/" 2>/dev/null
cat "$D/state/installed.json" 2>/dev/null
cat "$D/state/last-error.txt" 2>/dev/null
"$D/bin/graphin" version --json 2>/dev/null
```

- `last-error.txt`가 있으면 그 내용이 곧 진단 결과다. 먼저 보고하라.
- `version --json`의 **`semantic_supported`가 `false`면 의미 검색은 이 플랫폼에서
  영구히 불가능하다** — 모델 다운로드 실패와 혼동하지 말 것. lexical 검색은 정상이다.
- `installed.json`의 `source`가 `go-install`이면 릴리스 에셋이 아니라 소스 빌드다.

## 2. 중복 MCP 등록 — 가장 흔한 사고

```sh
claude mcp list 2>/dev/null | grep -i graphin
```

플러그인 서버는 `plugin:graphin:graphin`으로 뜬다. 그 **외에** `graphin` 항목이
따로 보이면 수동 등록이 남아 있는 것이다. Claude Code는 커맨드가 다르면 중복으로
보지 않으므로 둘 다 뜨고, **뒤에 뜬 쪽이 워크스페이스 락을 못 잡아 반쪽으로 죽는다.**

조치를 그대로 제시하라:

```sh
claude mcp remove graphin -s local
claude mcp remove graphin -s user
claude mcp remove graphin -s project
```

(등록된 스코프에서만 성공한다. 나머지는 실패해도 무시)

## 3. 인덱스와 계측

프로젝트 루트에서:

```sh
ls -la .graphin/ 2>/dev/null
wc -l .graphin/usage/events.jsonl 2>/dev/null
tail -5 .graphin/agent-nav.log 2>/dev/null
```

- `.graphin/merkle.json`이 없으면 아직 부트스트랩되지 않았다 —
  `bootstrap_workspace`를 한 번 호출하면 된다.
- `merkle.json`은 있는데 `usage/events.jsonl`이 없으면 PostToolUse 훅이 발화하지
  않는 것이다. `/plugin`에서 graphin이 enabled인지 확인하게 하라.
- `agent-nav.log`에 `semantic_unavailable`이 있으면 그 `error` 필드를 그대로 인용하라.

## 4. 요약

| 항목 | 상태 | 조치 |
|---|---|---|
| 바이너리 | 버전·출처·경로 | |
| 의미 검색 | 가능/불가(사유) | |
| MCP 등록 | 플러그인 단독 / 중복 | |
| 인덱스 | 부트스트랩 여부·노드 수 | |
| 계측 | 이벤트 수 | |

문제가 없으면 그냥 "정상"이라고 짧게 말하라. 없는 문제를 만들어내지 말 것.
설치를 강제로 다시 하려면 `/graphin:setup`이다.
