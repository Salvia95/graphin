---
description: graphin 설치·연결 진단 — 바이너리 출처, 버전 드리프트, 플랫폼 지원, 인덱스 상태와 그래프 건강, MCP 등록(중복·빈 도구 목록), 붙어 있는데 안 쓰이는 워크스페이스를 점검한다
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
M="$HOME/.claude/plugins/marketplaces/graphin"
git -C "$M" log -1 --format='캐시 갱신 전: %h %cd' --date=short 2>/dev/null
claude plugin marketplace update graphin                     # 캐시만 갱신, 플러그인은 안 건드린다
git -C "$M" log -1 --format='캐시 갱신 후: %h %cd' --date=short 2>/dev/null
claude plugin list 2>/dev/null | grep -A1 '@graphin'
grep -m1 '"version"' "$M/plugin/graphin/.claude-plugin/plugin.json" \
                     "$M/plugin/graphin-guide/.claude-plugin/plugin.json"
```

앞(설치본)과 뒤(마켓플레이스가 서빙 중인 `main`)가 다르면 낡은 것이다.

- **첫 줄을 건너뛰지 말 것.** 마켓플레이스 캐시 자체가 낡으면 드리프트가 아예
  안 보인다 — 그게 정확히 위 사고가 눈에 안 띈 이유다. 2026-08-21에 다시 걸렸다:
  캐시가 일주일간 v0.3.0에 묶여 있는 동안 `main`은 v0.4.0까지 갔고, 갱신 전에는
  설치본 0.3.0 ↔ 캐시 0.3.0으로 **일치해 보였다.** 릴리스 둘이 통째로 안 보였다.
- 그래서 갱신 명령의 출력을 **버리지 말고 보여라.** 실패해도 이 진단은 계속
  굴러가고, 낡은 캐시끼리 비교해 "일치"라는 거짓 정상을 만들어낸다. `%h`가 안
  움직였다면 이미 최신이거나 갱신이 실패한 것이고, **둘은 해시만으로 구분되지
  않는다** — 갱신 명령이 성공했다고 말했는지를 함께 봐야 한다.
- **"일치"는 그냥 쓰지 말고 비교한 두 버전과 캐시 날짜를 같이 적어라.** 무엇과
  무엇을 대조했는지 안 적힌 "정상"은 위 사고를 한 번 더 통과시킨다.
- 조치: `claude plugin update graphin@graphin` ·
  `claude plugin update graphin-guide@graphin` → **재시작**. 재시작 전까지
  `installed.json`은 옛 버전 그대로다(런처가 아직 안 돌았다).
- **`/graphin:setup`으로는 안 고쳐진다** — 그건 *지금 캐시에 있는* 매니페스트대로
  다시 설치할 뿐이다. 낡은 것이 캐시면 낡은 것을 다시 설치한다.
- 업그레이드가 옛 바이너리 파일을 정리하므로 `.graphin/binpath`가 잠시 없는 곳을
  가리킨다. **정상이다** — 훅은 심링크를 먼저 보고, 다음 서버 기동이 다시 쓴다.
- 위 `$M` 경로가 없으면 마켓플레이스를 다른 이름으로(포크·로컬 경로로) 추가한
  것이다. `claude plugin marketplace list`로 실제 위치를 찾아 같은 비교를 하라.
  git 클론이 아닌 경로라면 `%h` 대조는 건너뛰고 버전 비교만 하되, **캐시가
  최신이라는 근거가 없다는 사실을 보고에 적어라.**

## 3. MCP 서버 — 중복 등록과 빈 도구 목록

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

### 연결됐는데 도구가 하나도 없다

같은 명령이 이렇게 답하는 경우가 있다:

```
plugin:graphin:graphin: … - ! Connected · tools fetch failed —
  Invalid result for tools/list: tools.4.inputSchema.properties:
  Invalid input: expected record, received null
```

**중복 등록이 아니다.** 서버는 정상 기동해서 응답까지 했고, 클라이언트가 그 응답의
스키마를 거부한 것이다. 거부 단위가 도구 하나가 아니라 `tools/list` **응답 전체**라
한 도구의 스키마가 틀리면 도구가 **전부** 사라진다.

이게 특히 고약한 이유는 어디에서도 고장으로 보이지 않기 때문이다. 서버 로그는
깨끗하고(제가 의도한 응답을 보냈다), `agent-nav.log`에는 `server_start`만 쌓이고,
`/mcp`는 "연결됨"이라고 한다. 채택 지표에서는 **도구를 안 쓴 것과 구분되지 않는다.**
`usage report`의 하락이 진짜 하락인지 도구 부재인지 알 방법이 없다.

- **0.3.0 ~ 0.4.0이면 알려진 버그다.** 인자를 받지 않는 `diagnose_index`의 입력
  스키마가 `"properties": null`로 나갔다(nil Go 맵은 `{}`가 아니다). **0.4.1에서
  고쳤다** — §2대로 업그레이드하는 것이 유일한 조치다. 재설치·재등록·`/graphin:setup`
  으로는 안 고쳐진다. 스키마는 바이너리 안에 있다.
- 그 외 버전이면 오류의 `tools.N`이 등록 순서상 N번째(0부터) 도구를 가리킨다. 그
  이름과 메시지 전문을 그대로 보고하라.
- **이 상태면 §5를 건너뛴다.** `diagnose_index`도 함께 사라졌으므로 호출할 수 없다.

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

- 도구가 목록에 없으면 이 절은 "확인 불가"로 두고 원인을 **§3에서** 찾아라.
  **도구 부재는 미연결과 같은 말이 아니다** — 연결된 서버가 `tools/list` 거부로
  도구를 전부 잃는 경우가 실제로 있었고, 그걸 "미연결"로 읽고 중복 등록과 바이너리를
  뒤지면 둘 다 멀쩡해서 아무것도 안 나온다. 여기서 막히면 그게 곧 진단 결과다.
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
| 버전 | 설치본 ↔ 마켓플레이스 — 비교한 두 버전과 캐시 갱신 날짜를 적는다 | |
| 의미 검색 | 가능/불가(사유) | |
| MCP 등록 | 플러그인 단독 / 중복 / **연결됐으나 도구 0개** | |
| 인덱스 | 부트스트랩 여부·노드 수 | |
| 그래프 건강 | 끊어진 엣지(코드/DB)·partial 노드 | |
| 의미 인덱스 | 모델 일치·게이트·임베딩 백로그 | |
| 설정·용량 | 기본값과 다른 플래그·`.graphin` 총 용량 | |
| 계측 | 이벤트 수 | |
| 붙어 있는데 안 쓰임 | N개 중 M개 미부트스트랩 | |

문제가 없으면 그냥 "정상"이라고 짧게 말하라. 없는 문제를 만들어내지 말 것.
설치를 강제로 다시 하려면 `/graphin:setup`이다.
