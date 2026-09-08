# 골든셋 확장 — 27 → 36태스크 (2026-09-08)

리터럴형 4개와 연쇄형 5개를 기존 층에 넣었다. 소유자 지시("골든셋 확장을 진행하자",
규모·종류·배치는 같은 세션에서 선택). 절차는 `.claude/skills/rag-golden-set`.

## 0. 왜 이 둘인가

[grep 대조군](../2026-09-08-grep-control/findings.md)이 근거다. 세 채점기에 걸쳐
정답률 차이가 없었고(p=0.227 → 0.727 → 0.688), **값은 비용 쪽에 있는데 그 축이
n=27에서 p=0.052로 경계에 걸렸다.** 검정력을 올리려면 표본이 필요하고, 아무 태스크나
늘리는 것이 아니라 **차이가 실제로 나는 자리**를 늘려야 한다.

- **연쇄형** — `rag-fk-edges`가 두 시대에 걸쳐 재현된 유일한 우세다(3/3·12KB 대
  1/3·58KB). 그 모양이 세트에 하나뿐이었다.
- **리터럴형** — 세트에 **0개**였다. `first_retriever`의 리터럴 부분집합이 통째로
  비어 있어, v0.4.12가 배달한 P1(`context`)과 P3(리터럴 힌트)가 겨냥한 자리를
  재는 태스크가 없었다.

어휘 비유출 산문형은 이번에 넣지 않았다(소유자 선택).

## 1. 무엇을 넣었나

소재는 **기존 27태스크가 한 번도 `must_cite`로 짚지 않은 패키지**에서 골랐다 —
`internal/wiki`(6,211줄로 가장 큰데 통째로 비어 있었다), `graph`, `semantic`,
`tokenizer`, `nodeid`, `merkle`, `watch`.

**리터럴형 4 (answered, `rag-lit-*`)** — 식별자를 질문에 그대로 넣는다.

| id | 재는 사실 | 오답 경로 |
|---|---|---|
| `rag-lit-stoplist` | 억제는 **전역 계층에서만** 걸린다 — 같은 패키지·import된 심볼은 여전히 엣지를 만든다 | "엣지가 아예 안 생긴다" |
| `rag-lit-debounce` | quiet 500ms·maxWait 2s와 **maxWait가 막는 것**(연속 스트림의 기아) | 값만 읽고 이유를 빠뜨림 |
| `rag-lit-tracks` | Track A는 Start/End 바이트 메타만, Track B만 재임베딩+엣지 재추출 | 두 트랙을 뭉뚱그림 |
| `rag-lit-arity` | `UnboundedArity = -1`, Python `*args`·Java varargs가 받는다 | — |

**연쇄형 5 (multi-hop, `rag-chain-*`)** — 한 파일로 답이 닫히지 않는 것만.

| id | 홉 | 핵심 |
|---|---|---|
| `rag-chain-model-mismatch` | `provision/manifest.go` → `semantic/persist.go` → `mcp/tools/diagnose.go` | 비교되는 두 id의 **출처가 다르다** |
| `rag-chain-wiki-token` | `wiki/token.go` → `preflight.go` → `gate.go` | Fingerprint에 HMAC 서명하지만 **세트가 그 일에 맞는지는 증명하지 않는다** |
| `rag-chain-pin-drift` | `parse/markdown.go` → `wiki/pins.go` → `resolve.go` | 핀은 해시를 **둘** 든다 |
| `rag-chain-redirect` | `graph/deltalog.go` → `engine.go` | compaction이 tombstone을 버리므로 redirect는 upsert급이어야 하고, 엣지 맵은 공유하지 않는다 |
| `rag-chain-tokenizer` | `tokenizer/tokenizer.go` → `semantic/engine.go` | 자체 구현, 두 계열, HF 참조 토큰 id로 패리티 증명 |

## 2. 작성 중에 뒤집힌 것 — 핀은 해시를 둘 든다

메모리에 "핀 해시는 리네임 탐지기가 아니다(헤딩이 해시 범위 안)"로 적혀 있었으나
코드가 다르다. `Pin`은 **두 해시**를 기록한다:

- `Hash` — 섹션 소스 슬라이스 전체(헤딩 포함)
- `Rename` — `RenameKey`, `src[bodyStart:end]`로 **본문만**(`parse/markdown.go:106`)

`resolve.go:142`의 `driftOf`가 리다이렉트를 따라간 뒤 `Rename`이 같으면
`DriftChanged`가 아니라 `DriftNone`을 낸다 — 제목만 움직인 경우다. 해시 하나로는
이 둘이 갈리지 않는다. `rag-chain-pin-drift`는 **코드에서 확인한 이 사실로** 썼다.

## 3. 검증 스모크 — 전부 pass는 신호다

`--tier answered,multi-hop --runs 1 --worktree`로 21런(새 9 + 기존 12), 오류 0.
[report.md](report.md).

**새 9태스크 전부 pass.** 스킬이 "전부 pass면 질문이 너무 쉽거나 evidence가 너무
느슨한 것"이라고 경고한 상태라 하나씩 되짚었고, **하나를 찾았다**:

- `rag-chain-redirect`의 evidence에 `tombstone`이 들어 있었는데 **그 낱말이 질문
  본문에 있다.** 되받아 쓰기만 해도 걸리므로 "답을 아는 사람만 쓸 수 있는 리터럴"
  조건을 어긴다(스킬 절차 3). 빼고 `compact|compaction`만 남긴 뒤 같은 트랜스크립트로
  재채점 → 여전히 pass. 답변이 실제로 compaction과 `used_by`/`separate`를 썼기
  때문이고, 이제 그 pass는 질문 되받기가 아니다.

나머지 여덟의 evidence는 질문 본문과 겹치지 않는다(`fingerprint`·`HMAC`·`RenameKey`·
`WordPiece`·`Unigram`·`500`·`-1`·`embed`). 답변 품질도 실제로 높았다 —
`rag-lit-stoplist`는 `confidence.go:326`의 `tier == tierGlobal &&` 조건을 짚고
단위 테스트까지 인용했다.

**목적은 달성됐다.** `first_retriever = keyword`가 3건 나왔다(`rag-lit-stoplist`,
`rag-chain-model-mismatch`, `rag-chain-tokenizer`) — 비어 있던 리터럴 부분집합이
채워졌다. 다만 **리터럴형 4개 중 keyword로 시작한 것은 1개뿐**이고 나머지 셋은
식별자를 그대로 줬는데도 hybrid로 시작했다. 결함이 아니라 다음 풀셋이 비율로
답할 관찰 항목이다.

## 4. 비교 규칙

`taskset_sha`가 `5ca2a37b2b92` → **`edd99ac9013e`**로 바뀐다. **27태스크 시대의
총점(74/81, 77/81)과 새 세트의 총점을 직접 비교하지 않는다.** 층별 비율과 행동
지표(`first_retriever`, 바이트, `invented`, `contained`)는 이어진다.

루브릭은 건드리지 않았다 — 1.4.2 그대로다. 스킬이 "세트 재구성과 루브릭 변경을
같은 커밋에 섞지 않는다"고 못박은 규칙이고, 이번 변경은 판정 규칙이 아니라 표본이다.
`SMOKE_TASKS` 6개도 그대로 두었다(스모크 규모 변경은 별개 결정).

## 5. 남은 것

- **새 세트의 첫 풀셋이 아직 없다.** 다음 릴리스 게이트가 그것을 겸하게 되고,
  36태스크 × 3런 = 108런(약 $13.5, `--jobs 3`으로 ~35분)이다.
- 어휘 비유출 산문형은 이번에 안 넣었다 — 의미 검색이 값을 내는지 재려면 그 축이 필요하다.
- 리터럴을 주고도 hybrid로 시작하는 비율(이번 4개 중 3개)이 다음 풀셋에서 유지되면,
  P3 힌트가 아니라 **에이전트 프롬프트의 검색기 선택 조항**을 봐야 한다.
