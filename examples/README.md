# examples/ → `plugin/graphin-guide/`

여기 있던 SKILL과 서브에이전트는 **배포되는 플러그인**이 됐다. 복사해 쓰는
템플릿이 아니라 설치하는 물건이다.

```
/plugin marketplace add Salvia95/graphin
/plugin install graphin-guide@graphin
```

| 예전 위치 | 지금 위치 |
|---|---|
| `examples/skills/graphin/SKILL.md` | [`plugin/graphin-guide/skills/graphin/SKILL.md`](../plugin/graphin-guide/skills/graphin/SKILL.md) |
| `examples/agents/graphin-explorer.md` | [`plugin/graphin-guide/agents/graphin-explorer.md`](../plugin/graphin-guide/agents/graphin-explorer.md) |

두 벌로 유지하지 않는다 — 사본이 갈라지면 어느 쪽이 진짜인지 알 수 없게 된다.
직접 손봐 쓰고 싶으면 위 파일을 프로젝트의 `.claude/skills/`·`.claude/agents/`로
복사하면 되고, 그때는 `graphin-guide`를 설치하지 않는 편이 낫다(같은 스킬이 두
벌 로드된다).

`graphin-guide`가 `graphin`과 **별도 플러그인인 것은 의도**다. 계측(`graphin`)과
유도(`graphin-guide`)가 한 플러그인에 섞이면 "유도 없이 얼마나 쓰이는가"라는
베이스라인이 오염되고, 그건 한번 섞이면 복구되지 않는다.
