---
description: graphin 바이너리 강제 재설치 — 설치가 깨졌거나 업그레이드가 걸렸을 때의 복구 한 방
allowed-tools: Bash(*), Read
---

graphin 서버 바이너리를 **강제로 다시 설치**하라. 정상 경로(런처·SessionStart 훅)는
매니페스트가 동일하면 아무것도 하지 않으므로, 설치가 깨졌을 때는 이 명령이 필요하다.

```sh
R="${CLAUDE_PLUGIN_ROOT:?}"
D="${CLAUDE_PLUGIN_DATA:-$HOME/.claude/plugins/data/graphin-graphin}"
GRAPHIN_FORCE_INSTALL=1 "$R/install/install.sh" 2>&1 | tail -40
```

그다음 결과를 확인한다:

```sh
"$D/bin/graphin" version --json
cat "$D/state/installed.json"
```

보고할 것:

1. 설치된 버전과 **출처**(`release` / `go-install` / `relink`).
2. `semantic_supported`가 `false`면 그 플랫폼에 onnxruntime 빌드가 없다는 뜻임을
   명시하라 — 설치 실패가 아니다. lexical 검색은 동작한다.
3. 실패했다면 `$D/state/last-error.txt`의 사유를 그대로 인용하고, 로그
   (`$D/logs/install.log`)의 마지막 몇 줄을 덧붙여라.

**설치 성공 후 MCP 서버는 자동으로 새 바이너리를 쓰지 않는다** — 이미 떠 있는
프로세스는 옛 것이다. `/mcp`에서 graphin을 재연결하거나 세션을 새로 시작하라고
안내하라.

폐쇄망이거나 자기 빌드를 쓰려면 설치 대신 `/plugin` → Manage → graphin →
Configure에서 `binary_path`를 지정하는 쪽이 맞다.
