---
description: graphin 설치·연결 진단 — 바이너리 출처, 버전 드리프트, 플랫폼 지원, 인덱스 상태와 그래프 건강, 중복 MCP 등록, 붙어 있는데 안 쓰이는 워크스페이스를 점검한다
allowed-tools: Bash(*), Read, mcp__plugin_graphin_graphin__diagnose_index
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

## 5. 그래프 건강 — 인덱스 내부

§4까지는 셸로 보이는 것(파일 유무, 로그 tail)이다. 인덱스가 **믿을 만한지**는
서버 프로세스 안에서만 읽힌다 — 별도 프로세스가 그래프를 열면 델타 로그를
truncate하므로, 라이브 워크스페이스는 MCP 도구로만 안전하게 들여다본다.

**`diagnose_index`를 인자 없이 한 번 호출하라.**

- 도구가 목록에 없으면(=MCP 미연결) 이 절은 "확인 불가"로 두고 **§3 중복 등록과
  §1 바이너리를 먼저 해결하게 하라.** 여기서 막히면 그게 곧 진단 결과다.
- 부트스트랩 전이면 `state="not_bootstrapped"`와 설정·용량만 돌아온다. 오류가
  아니라 §4와 같은 판정이다.

**`<hint>`가 있으면 그 문장이 곧 조치다. 없으면 인덱스는 건강하다.** 아래는 각
신호를 요약표로 옮길 때 쓰는 판정 기준이다.

| 신호 | 뜻 | 조치 |
|---|---|---|
| `mismatch="true"` | `vectors.bin`이 지금 설정과 **다른 모델**로 쓰였다. 두 벡터 공간이 섞여 검색 품질이 떨어진다 | 넷 중 유일하게 즉시 조치 대상. 서버 정지 → `rm .graphin/search/vectors.bin` → 재시작 |
| `<dangling code="N" …>`, N>0 | 타깃이 인덱스에 없는 코드 엣지 — 삭제된 대상이거나 미해결 참조 | 대개 재인덱싱으로 자가 치유. 큰 저장소에서 작고 안정된 수치는 정상 |
| `<dangling … db="N">`, N>0 | 스냅샷 밖 참조(예: Supabase `auth.users`) | **결함으로 단정하지 말 것.** 무엇을 가리키는지 먼저 확인하라 |
| `<partial count="N">`, N>0 | 구문 오류로 부분 파싱된 파일. 깨진 구간의 엣지가 빠져 있어 그 주변 탐색이 불완전하다 | 해당 파일 구문 수정 → 워처가 재인덱싱 |
| `<semantic gated="true" …>` | 노드 수 상한을 넘겨 의미 검색이 꺼졌다 | `--semantic-max-nodes`를 올려 재기동. 그때까지 lexical은 정상 동작 |
| `<semantic … failure="…">` | 모델/ORT 워밍업 영구 실패 | 그 문장을 그대로 인용하고 §1의 `semantic_supported`와 대조하라 |
| `<config … *_changed="true">` | 그 플래그가 기본값과 다르다 | 의도한 값인지만 확인. **값 비교이지 "무엇을 타이핑했는가"의 기록이 아니다** — 기본값을 명시적으로 넘겨도 `changed`로 안 잡힌다 |

`<graph nodes= edges= shards=>`가 §7 요약표의 노드 수 칸을 채운다. `<storage>`에서
`runtime/`이 큰 것은 정상이고(모델+ORT), `search/`가 끝없이 자라면 재임베딩
churn이므로 `agent-nav.log`와 대조하라.

## 6. 붙어 있는데 안 쓰이는 워크스페이스

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

## 7. 요약

| 항목 | 상태 | 조치 |
|---|---|---|
| 바이너리 | 버전·출처·경로 | |
| 버전 | 설치본 ↔ 마켓플레이스 일치 / 낡음(몇 버전) | |
| 의미 검색 | 가능/불가(사유) | |
| MCP 등록 | 플러그인 단독 / 중복 | |
| 인덱스 | 부트스트랩 여부·노드 수 | |
| 그래프 건강 | 끊어진 엣지(코드/DB)·partial 노드 | |
| 의미 인덱스 | 모델 일치·게이트·임베딩 백로그 | |
| 설정·용량 | 기본값과 다른 플래그·`.graphin` 총 용량 | |
| 계측 | 이벤트 수 | |
| 붙어 있는데 안 쓰임 | N개 중 M개 미부트스트랩 | |

문제가 없으면 그냥 "정상"이라고 짧게 말하라. 없는 문제를 만들어내지 말 것.
설치를 강제로 다시 하려면 `/graphin:setup`이다.
