# graphin-guide (Claude Code 플러그인)

[graphin](https://github.com/Salvia95/graphin) MCP 서버를 **잘 쓰는 법**만 담은
플러그인. 서버 자체는 [`graphin`](../graphin/README.md) 플러그인이 제공한다.

```
/plugin marketplace add Salvia95/graphin
/plugin install graphin-guide@graphin
```

| 구성 | 내용 |
|---|---|
| 스킬 `graphin` | 점진적 정보 공개(search → explore → read) 사용법. 언제 멈춰야 하는지, `min_confidence`를 언제 내리는지, 하지 말아야 할 것들 |
| 스킬 `knowledge` | 문서 섹션을 모은 **knowledge set**을 만들고 쓰는 법. 한 줄 요약으로 훑고 필요한 섹션만 정확히 로드한다 |
| 서브에이전트 `graphin-explorer` | "어디에 있나 / 뭐가 호출하나 / 뭐가 깨지나"를 위임받아 **인용이 붙은 요약**을 돌려준다. 읽기 전용 |

## 왜 서버와 분리되어 있나

계측과 유도가 한 플러그인에 있으면 **"유도 없이 얼마나 쓰이는가"라는 베이스라인이
사라진다.** 그리고 그건 한번 섞이면 복구되지 않는다 — 이미 유도된 사용자에게서
무개입 데이터를 다시 얻을 수 없기 때문이다.

설치 여부로 갈리게 두면 두 모집단이 계속 관측 가능하고, 설치 시점을 경계로
`graphin usage report --since <날짜>`가 전후 비교를 낸다.

## 서브에이전트에 대해

`disallowedTools: Edit, Write, NotebookEdit`로 쓰기만 막고 나머지는 상속한다.
`tools` 화이트리스트를 쓰지 않는 이유는 graphin MCP 도구 이름이 등록 방식에 따라
달라지기 때문이다 — 플러그인 제공 서버는 `mcp__plugin_graphin_graphin__*`이고,
직접 등록한 서버는 사용자가 정한 키를 따른다. 이름에 의존하지 않는 쪽이 안전하다.

에이전트 프론트매터의 `skills: [graphin]`은 맨 이름으로 적는다. Claude Code가
①정확 일치 → ②`<에이전트 네임스페이스>:<이름>` → ③접미사 순으로 찾으므로,
같은 플러그인 안의 스킬이 ②에서 먼저 잡힌다.

## 직접 손봐 쓰려면

`skills/*/SKILL.md`와 `agents/graphin-explorer.md`를 프로젝트의
`.claude/skills/`·`.claude/agents/`로 복사하면 된다. 그때는 이 플러그인을 설치하지
않는 편이 낫다 — 같은 스킬이 두 벌 로드된다.
