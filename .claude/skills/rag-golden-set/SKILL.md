---
name: rag-golden-set
description: Rebuild the graphin-rag behavior golden set — the fixed tasks that scripts/eval-rag.py runs the real agent against. Use ONLY when the user asks for it by name, when the graphin-rag agent contract (its end states, budget, or report shape) has changed, or when a large-scale repo change has made recorded answers stale. Never as routine upkeep, and never to "check" agent quality — measuring is the script's job, not this skill's.
---

# rag 골든셋 재구성

이 저장소 자신이 코퍼스다. `eval/rag/tasks.jsonl`(질문)과
`eval/rag/expected.jsonl`(정답)을 상대로 `scripts/eval-rag.py`가 실제
graphin-rag 서브에이전트를 태워 행동을 잰다. 축의 뜻은
`docs/rag-bench-spec.md`, 판정 규칙의 정본은 채점기 코드다.

## 시작 전에: 소유자 허가 없이는 아무것도 바꾸지 않는다

루브릭과 골든셋은 변경 통제 대상이다(스펙 §8). 이 스킬이 호출됐다는 것
자체가 허가는 아니다 — **이 세션에서 소유자가 이 변경을 명시적으로
허가했는지 먼저 확인**하고, 허가가 없으면 무엇을 왜 바꾸려는지 설명하고
멈춘다. 허가를 받았으면:

1. 저장소 루트에 `.rag-bench-unlock`을 만든다 — 소유자가 직접, 또는 이
   변경이 **소유자의 이 세션 메시지에서 명시적으로 지시된 경우에 한해**
   Claude가 만들고 작업 직후 삭제한다(스펙 §8). 지시 없이 스스로 만드는
   것은 위반이다.
2. 작업 후 `scripts/eval-rag.py lock --approved-by <소유자>`로 락을 갱신한다.
3. 커밋에는 `Rag-Bench-Approved-By:` 트레일러를 단다 — 없으면 CI가 막는다.
4. `.rag-bench-unlock`을 지운다.

## 절대 규칙: 이 작업에 graphin을 쓰지 않는다

`.claude/skills/golden-set`의 규칙이 그대로 적용된다. 정답을 graphin으로
찾아 적으면 세트는 graphin이 이미 잘하는 것들의 집합이 되고, 그 위의 측정은
자기 자신과의 일치도가 된다. Grep·Glob·Read만 쓴다.

## 층은 에이전트 계약이 정한다

층(`answered` · `multi-hop` · `not-here` · `out-of-reach` ·
`budget-pressure`)은 graphin-rag.md가 선언한 종료 상태에서 온다. **에이전트
계약이 바뀌면 이 세트도 따라 바뀌어야 한다** — 층이 계약과 어긋난 채 남는
것이 이 세트가 늙는 방식이다.

층을 늘리거나 규모를 바꾸는 것은 사용자 결정이다. 먼저 물어라.

## 절차

1. **질문을 먼저 쓴다.** 정답을 찾기 전에, 에이전트에게 실제로 위임할 법한
   문장으로. 순수한 위치 질문("어디 있나")은 이 세트의 것이 아니다 — 검색
   호출 한 번의 문제라 `eval/golden`이 결정론으로 재고 있고, 여기는 **내용**
   질문(어떻게 동작하나, 값이 뭔가, 바꾸면 뭐가 닿나)이다. 통합된 에이전트가
   위치 질문도 맡지만, 그 축은 엔진 층이 대변한다.
2. **정답을 grep으로 찾고, 읽어서 확인한다.** 경로만 맞는 정답은 측정을
   조용히 망가뜨린다. 코퍼스에 같은 사실의 출처가 여럿일 수 있다(픽스처가
   셋이다) — 하나만 정답으로 박으면 다른 출처를 든 정답이 fail 난다.
   `must_cite`와 `evidence`의 `|` 대안이 그 자리다.
3. **evidence는 "보고서 전달" 기준으로 고른다.** eval/golden의 evidence가
   "읽은 노드에 있어야 할 문자열"이라면, 여기서는 **에이전트의 최종 보고서에
   있어야 할 문자열**이다. 그래서 두 조건을 모두 만족해야 한다: 답을 아는
   사람만 쓸 수 있는 리터럴이면서, **정답을 아는 사람이라면 반드시 쓰게 되는**
   리터럴. 후자를 어기면 완답이 fail로 채점된다 — 실측 사례: 마커 파일의
   동작을 다 설명하고 파일명 철자만 안 쓴 답. 철자가 여럿이면 `a|b|c` 대안
   문법을 쓴다.
4. **not-here는 subject와 forbidden으로 짠다.** forbidden은 날조된 답에만
   나올 문자열이어야 한다 — 질문을 되받아 말하기만 해도 걸리는 단어를 넣으면
   정답이 실격된다. 텍스트로는 있는데 심볼이 아닌 함정(문서 속 유사 문구)을
   최소 하나 유지한다.
5. **프롬프트 오염을 확인한다.** graphin 스킬 본문이 에이전트의 시스템
   프롬프트에 들어간다. 스킬이 이미 서술하는 사실을 묻는 질문은 검색 없이
   답할 수 있으므로, 그 질문의 측정은 인용 요구(must_cite)가 진다 —
   must_cite 없는 answered 태스크를 만들지 않는다.
6. **검증하고 재 본다.**

   ```
   scripts/eval-rag.py validate          # 경로·evidence 실재, LLM 없음
   make build
   scripts/eval-rag.py run --out <fresh> --worktree --max-tasks 2
   scripts/eval-rag.py score --out <fresh>
   ```

   전부 pass면 질문이 너무 쉽거나 evidence가 너무 느슨한 것이고, 전부 fail이면
   세트가 아니라 기대값의 결함부터 의심한다. **런을 본 뒤 evidence를 고치는
   것은 보정이지 오염이 아니다** — 단, 고칠 수 있는 것은 표현(철자·대안)뿐이고
   질문과 사실 자체를 에이전트에 맞춰 바꾸면 그때부터 오염이다.

## 이 스킬이 하지 않는 것

- **측정하지 않는다.** 점수는 `scripts/eval-rag.py`만 낸다.
- **임계값·게이트 승격을 정하지 않는다.** 그건 데이터가 쌓인 뒤 사용자가
  정한다.
- **루브릭을 바꾸지 않는다.** 판정 규칙 변경은 채점기의 `RUBRIC_VERSION`을
  올리는 일이고, 세트 재구성과 같은 커밋에 섞지 않는다.
- **기존 세트를 말없이 지우지 않는다.** 실패를 담은 세트는 고장이 아니라
  기록이다.
- **이 파일에 질문 본문을 적지 않는다.** 이 파일은 색인된다. 예시가 필요하면
  id만 언급한다.
