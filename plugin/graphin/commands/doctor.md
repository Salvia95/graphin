---
description: graphin 설치·연결 진단 — 바이너리 출처, 버전 드리프트, 플랫폼 지원, 인덱스 상태, 중복 MCP 등록, 붙어 있는데 안 쓰이는 워크스페이스를 점검한다
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

## 2. 버전 드리프트 — 릴리스는 스스로 배달되지 않는다

**바이너리 버전은 플러그인 캐시에 묶여 있다.** 런처가 `$CLAUDE_PLUGIN_ROOT`의
매니페스트와 설치본의 매니페스트를 바이트 비교해서 같으면 아무것도 안 하기
때문에, 플러그인이 안 올라가면 새 릴리스가 나와도 **옛 바이너리가 계속 뜬다.**
실측으로 세 릴리스가 이렇게 미배달로 남은 적이 있다(2026-08-08, 0.2.1~0.2.3).

```sh
claude plugin marketplace update graphin > /dev/null 2>&1   # 캐시만 갱신, 플러그인은 안 건드린다
claude plugin list 2>/dev/null | grep -A1 '@graphin'
M="$HOME/.claude/plugins/marketplaces/graphin/plugin"
grep -m1 '"version"' "$M/graphin/.claude-plugin/plugin.json" "$M/graphin-guide/.claude-plugin/plugin.json"
```

앞(설치본)과 뒤(마켓플레이스가 서빙 중인 `main`)가 다르면 낡은 것이다.

- **첫 줄을 건너뛰지 말 것.** 마켓플레이스 캐시 자체가 낡으면 드리프트가 아예
  안 보인다 — 그게 정확히 위 사고가 눈에 안 띈 이유다.
- 조치: `claude plugin update graphin@graphin` ·
  `claude plugin update graphin-guide@graphin` → **재시작**. 재시작 전까지
  `installed.json`은 옛 버전 그대로다(런처가 아직 안 돌았다).
- **`/graphin:setup`으로는 안 고쳐진다** — 그건 *지금 캐시에 있는* 매니페스트대로
  다시 설치할 뿐이다. 낡은 것이 캐시면 낡은 것을 다시 설치한다.
- 업그레이드가 옛 바이너리 파일을 정리하므로 `.graphin/binpath`가 잠시 없는 곳을
  가리킨다. **정상이다** — 훅은 심링크를 먼저 보고, 다음 서버 기동이 다시 쓴다.
- 위 `$M` 경로가 없으면 마켓플레이스를 다른 이름으로(포크·로컬 경로로) 추가한
  것이다. `claude plugin marketplace list`로 실제 위치를 찾아 같은 비교를 하라.

## 3. 중복 MCP 등록 — 가장 흔한 사고

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

## 4. 인덱스와 계측

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

## 5. 붙어 있는데 안 쓰이는 워크스페이스

**채택 실패의 가장 완전한 형태는 지표가 세지 못한다.** 서버가 떴는데 에이전트가
`bootstrap_workspace`를 한 번도 안 부르면 인덱스가 없고, 인덱스가 없으면 이벤트도
없고, 그러면 그 워크스페이스는 리포트에 **0%가 아니라 아예 없는 것**으로 남는다
(usage-spec §5). `usage report`는 자기가 받은 로그 하나만 보므로 이 사실을
**그 워크스페이스에 대고 물어야만** 말해 준다. 여기서 한 번에 훑는다.

```sh
ps -eo args= 2>/dev/null |
  awk '/graphin .*--workspace/ {for (i=1;i<=NF;i++) if ($i=="--workspace") print $(i+1)}' |
  sort -u |
  while IFS= read -r ws; do
    if [ -f "$ws/.graphin/merkle.json" ]; then
      printf '  부트스트랩됨    %s\n' "$ws"
    else
      printf '  한 번도 안 불림 %s\n' "$ws"
    fi
  done
```

- 열거의 출처는 **실행 중인 우리 서버의 인자**다. 디렉터리를 훑지 않는다 —
  graphin이 지금 붙어 있는 곳만 본다. 서버가 안 떠 있는 워크스페이스는 안 보이고,
  그건 이 점검의 질문("붙어 있는데 안 쓰이나")과 일치한다.
- **"한 번도 안 불림"이 하나라도 있으면 그 경로를 그대로 보고하라.** 도구가
  깨진 것이 아니라 에이전트가 부르지 않은 것이므로, 조치는 재설치가 아니다:
  그 프로젝트에서 `graphin-guide` 플러그인이 설치·활성인지, 프로젝트 지시문이
  graphin을 언급하는지를 보게 하라.
- 전부 부트스트랩돼 있으면 이 절은 한 줄로 끝내라.

## 6. 요약

| 항목 | 상태 | 조치 |
|---|---|---|
| 바이너리 | 버전·출처·경로 | |
| 버전 | 설치본 ↔ 마켓플레이스 일치 / 낡음(몇 버전) | |
| 의미 검색 | 가능/불가(사유) | |
| MCP 등록 | 플러그인 단독 / 중복 | |
| 인덱스 | 부트스트랩 여부·노드 수 | |
| 계측 | 이벤트 수 | |
| 붙어 있는데 안 쓰임 | N개 중 M개 미부트스트랩 | |

문제가 없으면 그냥 "정상"이라고 짧게 말하라. 없는 문제를 만들어내지 말 것.
설치를 강제로 다시 하려면 `/graphin:setup`이다.
