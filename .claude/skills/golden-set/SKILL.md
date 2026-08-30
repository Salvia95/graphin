---
name: golden-set
description: Rebuild the search recall golden set for this repository — the fixed question/answer pairs that scripts/eval-recall.py scores graphin against. Use ONLY when the user asks for it by name, or right after a large-scale change (a package moved or was split, a subsystem was rewritten, files were renamed en masse) has made the recorded answers stale. Never as routine upkeep, never as part of an unrelated task, and never to "check" search quality — measuring is the script's job, not this skill's.
---

# 골든셋 재구성

이 저장소 자신이 코퍼스다. 골든셋은 "이 질문에는 이 파일이 답이다"의 목록이고,
`scripts/eval-recall.py`가 그것을 상대로 graphin의 recall@k를 잰다.

## 절대 규칙: 이 작업에 graphin을 쓰지 않는다

`search_hybrid`, `explore_graph`, `read_code`, `diagnose_index` — **어느 것도
호출하지 않는다.** `graphin` 바이너리도 실행하지 않는다. Grep, Glob, Read만
쓴다.

이유는 취향이 아니다. graphin으로 정답을 찾아 적으면, 그 정답은 graphin이
이미 찾을 수 있는 것들의 집합이 된다. 그 위에서 잰 재현율은 검색 품질이 아니라
**자기 자신과의 일치도**이고, 언제나 높게 나오며, 어떤 회귀도 잡지 못한다.
골든셋의 가치는 전부 "graphin이 못 찾는 것도 들어 있다"에서 나온다.

같은 이유로 `rank-definition-vs-callers` 세트는 일부러 남겨 둔 실패다
(`docs/eval/2026-08-07-adoption-diagnosis`). 재현율이 100%인 골든셋은 버려도
되는 골든셋이다.

## 산출물

두 파일로 **분리해서** 쓴다. 질문과 정답이 한 파일에 있으면 정답을 보면서
질문을 쓰게 되고, 그러면 질문이 정답의 단어를 베낀다.

세트는 **세 층**으로 나뉘고, 층마다 두 파일을 갖는다. 층을 섞어 하나의 평균을
내면 서로 다른 질문들의 가중 평균이 되어 아무 뜻도 없는 숫자가 나온다 — 변형은
같은 질문을 두 번 세고, 2턴은 난이도가 다르다.

| 층 | 무엇인가 | 헤드라인 지표인가 |
|---|---|---|
| `base` | 이 저장소의 대표 질문들 | **그렇다.** 회귀는 여기서 본다 |
| `variants` | base의 질문에 `target`만 더한 짝 | 아니다. base와의 **짝 비교**로만 읽는다 |
| `hop` | `explore_graph`를 거쳐야 답이 나오는 질문 | 아니다. 따로 읽는다 |

```
eval/golden/base/{queries,expected}.jsonl
eval/golden/variants/{queries,expected}.jsonl
eval/golden/hop/{queries,expected}.jsonl
```

- `<층>/queries.jsonl` — `id`, `shape`, `query`, `top_k`
  - `target` (선택) — `code`/`docs`/`db`로 인구를 좁힌다
  - `hops` (선택) — `1`이면 2턴 세트다. 1턴 결과의 1위를 앵커로 `explore_graph`를
    거쳐, 엣지 **건너편**을 읽고 채점한다. "이게 바뀌면 뭐가 깨지나"류는 정의만
    읽어서는 답이 안 되므로 여기에 속한다.
  - `grep` (선택) — grep 대조군이 칠 법한 패턴. 비우면 질의에서 식별자 모양
    토큰을 뽑아 쓰고, 그런 토큰이 없으면 대조군은 `n/a`가 된다 — 산문 질문은
    grep 에이전트가 시작조차 못 한다는 뜻이고, 그 자체가 결과다.
- `<층>/expected.jsonl` — `id`, `files`(저장소 상대 경로 배열), `evidence`, `why`
  - `evidence` — 답이 실제로 전달됐다면 읽은 내용에 **반드시 들어 있을 문자열**
    배열. 소스에서 그대로 복사한 리터럴이어야 한다.

`id`는 두 파일에서 **같은 순서로 1:1 대응**해야 한다. 스크립트가 시작할 때
그것부터 검사하고, 어긋나면 측정을 아예 시작하지 않는다.

### 형식은 `.jsonl`이어야 한다 — 확장자가 설계다

`.json`, `.yaml`, `.md`는 graphin이 **색인한다**(`internal/parse/parse.go`의
`plainExtensions`). 골든셋이 색인되면 질의문을 그대로 담은 파일이 바로 그
질의의 검색 결과로 올라와 자기 답을 오염시킨다. `.jsonl`은 그 목록에 없어
색인되지 않는다. 옮기거나 이름을 바꾸지 않는다.

**이 문서에는 질의문을 적지 않는다.** 이 파일은 `.md`라서 색인되고 섹션
노드까지 생긴다. 예시가 필요하면 id만 언급한다.

## 절차

1. **규모와 층을 정한다.** `base` 기본 10세트. 늘릴 거면, 또는 `variants`·`hop`을
   건드릴 거면 한 번에 몇 개인지 사용자에게 먼저 확인한다 — 세트 구성은 사용자의
   책임이지 이 스킬의 재량이 아니다. `variants`에 새 항목을 넣을 때는 짝이 되는
   base 세트가 반드시 있어야 하고, 질의는 `target` 하나만 달라야 한다. 그 외의
   차이가 섞이면 짝 비교가 무너진다.

2. **질문을 먼저 쓴다.** 정답을 찾기 전에. 에이전트가 실제로 물을 법한
   문장이어야 하고, 아래 형태가 골고루 섞여야 한다. 한 형태에 몰리면 그
   형태의 회귀만 잡힌다.

   | shape | 무엇을 재나 |
   |---|---|
   | `bare-symbol` | 심볼 이름 한 개 — 가장 쉬운 축, 여기서 실패하면 심각하다 |
   | `sentence-with-symbol` | 문장 안에 심볼이 섞인 질문 — Tier-0 승격이 걸리는 축 |
   | `prose` | 심볼 없는 산문 질문 — 문서가 코드를 밀어내는 축 |
   | `docs` | 답이 실제로 문서인 질문 |
   | `shell` | 답이 코드가 아닌 파일(스크립트·설정)인 질문 |
   | `prose-hard` | 알려진 실패를 재현하는 질문 — 최소 하나는 있어야 한다 |

3. **정답을 grep으로 찾는다.** 정의가 어디 있는지, 그 파일이 정말 그 질문에
   답하는지 **읽어서** 확인한다. 경로만 맞고 내용이 다른 정답은 측정을 조용히
   망가뜨린다.

4. **여러 파일이 답이면 다 적는다.** 한쪽만 알면 답이 반쪽인 질문
   (정의 + 유일한 호출부 같은)은 두 개를 적는다. recall은 기대 파일 전부에
   대해 계산되므로, 이것이 난이도를 정직하게 만든다.

5. **증거 문자열을 고른다.** 파일 경로만으로는 부족하다. 경로는 맞고 답은 없는
   노드를 히트로 세게 되고, 반대로 답이 든 이웃 파일을 미스로 센다. `evidence`는
   **그 답을 아는 사람만 쓸 수 있는 리터럴**이어야 한다 — 함수 시그니처 한 줄,
   표의 한 칸, 스크립트의 조건문 같은 것. grep으로 찾은 그 줄을 그대로 복사한다.

   범위도 중요하다. 채점은 에이전트가 **실제로 읽은 노드**의 내용만 본다
   (기본 상위 3개). 파일 어딘가에 있지만 그 노드 밖에 있는 문자열은 영원히
   전달되지 않는다.

6. **경로 존재를 검증한다.** 모든 기대 경로가 `HEAD`에 실재해야 하고, 모든
   `evidence`가 그 경로 안에 실재해야 한다. 없는 경로·없는 문자열은 영원히 못
   맞히는 세트가 된다.

7. **커밋하기 전에 재 본다.** 이 단계에서는 graphin을 쓴다 — 그게 측정의
   정의다.

   ```
   make build
   scripts/eval-recall.py --worktree              # base 만
   scripts/eval-recall.py --worktree --tier all   # 세 층 (색인은 한 번만 한다)
   ```

   네 축이 나온다: **recall**(찾았나) · **delivery**(읽은 내용에 답이 있었나) ·
   **bytes**(그 대가) · 그리고 같은 질문을 grep으로 풀었을 때의 같은 세 값.
   recall만 보면 grep이 언제나 100%다 — 파일을 다 읽으니까. 비용과 함께 보지
   않으면 어떤 결론도 나오지 않는다.

   전부 100%가 나오면 질문이 너무 쉽다. 전부 0%면 기대 경로나 증거 문자열을
   잘못 적었을 가능성부터 의심한다. `recall 0% · delivery 100%`처럼 모순돼
   보이는 값은 대개 세트가 아니라 측정의 결함이니 그때는 스크립트를 의심한다.

## 이 스킬이 하지 않는 것

- **측정하지 않는다.** 점수는 `scripts/eval-recall.py`만 낸다.
- **임계값을 정하지 않는다.** 바닥값은 사용자가 정한다.
- **앵커를 고르지 않는다.** 2턴 세트에서 스크립트는 1턴 1위를 그대로 앵커로
   쓴다. 사람이라면 테스트 헬퍼를 건너뛰고 진짜 정의를 골랐을 자리에서도
   그렇게 한다. 이건 하한을 재는 것이지 에이전트를 흉내 내는 것이 아니며,
   실제로 그 자리에서 실패가 나온다면 그건 세트의 결함이 아니라 1턴 순위의
   결함이다.
- **기존 세트를 말없이 지우지 않는다.** 재구성이 기존 id를 없애면, 무엇을 왜
   버리는지 먼저 말한다. 실패를 담은 세트는 특히 — 그건 고장이 아니라 기록이다.
