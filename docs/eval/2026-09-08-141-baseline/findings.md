# 1.4.1 새 베이스라인, 그리고 지어낸 노드 id의 승격 (2026-09-08)

루브릭 1.4.0·1.4.1이 `RUN_COMPAT`을 두 번 리셋한 뒤 **비교 가능한 런이 하나도
없는 상태**였다. 이 문서는 그 자리를 메운 풀셋 한 셋과, 그 셋이 예고된 조건을
충족시켜 발동시킨 판정 승격의 기록이다.

- 산출물: `~/projects/graphin-eval-sandbox/out/rag-2026-09-08-141-baseline/`
- 이 디렉터리: [report.md](report.md) (1.4.2 채점) · [report-1.4.1.md](report-1.4.1.md)
  (승격 전) · [meta.json](meta.json)

## 0. 요약

| | 1.4.1 채점 | 1.4.2 채점(승격 후) |
|---|---|---|
| 총점 | 76/81 = 93.8% | **74/81 = 91.4%** |
| answered | 21/24 | 20/24 |
| multi-hop | 10/12 | 9/12 |
| not-here · out-of-reach · budget-pressure | 9/9 · 6/6 · 6/6 | 변화 없음 |
| db-nav | 24/24 | 24/24 |

게이트는 두 채점 모두 통과했고(플로어 80%), 마커는 `commit 8c0d392` · mode
`full`로 발행됐다. 81런 0에러, $10.15, 실경과 23분(워커 3, lexical-only).

## 1. 무엇을 재려던 런인가

1.4.1까지의 변경이 답을 요구한 질문 넷이 있었다. 넷 다 이 한 셋으로 답했다.

### ① 봉쇄 대칭 이후 graphin 팔 — 이탈 0, 다만 이번엔 *측정된* 0이다

`escaped` 0, `contained` 2. 그런데 그 두 건의 정체는 둘 다 이것이다:

```
Read {"file_path": "/dev/null"}
→ Path outside the workspace under test: /dev/null
```

`rag-hop-patternshape` r2는 `Bash {"command":"true","description":"noop"}`도
함께 찍었다. **진짜 이탈이 아니라 도구를 한 번 찔러 본 노옵**이고, 훅이
`/dev/null`을 경로 정책으로 걸러낸 것이다. 수치를 그대로 이탈로 읽으면 틀린다.

의미는 여전히 크다. 1.4.1 이전에는 `Read`·`Grep`·`Glob`의 절대 경로가 graphin
팔에서 **막히지도 잡히지도 않았고**, 그래서 "graphin은 81런에 이탈 0"은 측정
없이 나온 0이었다. 이제 같은 훅과 같은 네 필드 아래에서 0이다. grep 팔의
10/81(1.3.2)과 처음으로 같은 자로 비교된다.

### ② 끊긴 `docs/eval` 참조 244건 → 재구성 인용은 없었다

1.4.1이 "남은 위험"으로 적어 둔 시나리오 — 끊긴 참조를 따라간 에이전트가 경로를
재구성해 인용하면 가짜 인용 판정이 된다 — 는 실현되지 않았다. `fake_citations`
**0**. `elided_citations`도 리포트에 뜨지 않았다.

### ③ 지어낸 노드 id → 승격 조건이 충족됐다

5런에서 8건. 결과까지 따라가면 **2건이 빗나갔다**:

| 지어낸 id | 런 | 서버 응답 |
|---|---|---|
| `internal.keyword.keyword.go` | `rag-hop-grep-baseline` r0 | `<error code="NODE_NOT_FOUND">` |
| `internal.lexical.particles` | `rag-stem-rules` r2 | `<omitted reason="not_found" />` |

나머지 6건은 맞았다: `db.main.public.company_group` ×2(`rag-db-impact` r0·r1),
`internal.lexical.undouble`·`asciiLetters`·`endsHangul`(`rag-stem-rules` r2의
같은 배치), `internal.workspace.Workspace.applyFileResult`(`rag-bp-change-flow` r2).

`company_group`은 실존 노드다 — `testdata/fixtures/dbschema/db/main.graphindb.json`에
있고, 빈 `<graph_context>`로 돌아온 것은 **엣지가 없어서**지 노드가 없어서가
아니다(`internal/workspace/indexer_test.go:245`가 그 구분을 못박는다). 이
구분이 검출기의 핵심이라 §2에 다시 적는다.

**두 빗나간 런 모두 pass로 채점되고 있었다.** 스펙 §7이 2026-09-01에 미리
적어 둔 조건이 그대로 충족됐다.

### ④ 새 실패 — `rag-hop-patternshape` 1/3

`must_cite` 두 개 중 `internal/usage/stream.go`를 놓쳤다.

| 런 | 판정 | 인용 | 콜 |
|---|---|---|---|
| r0 | pass | `event.go`, `stream.go` | 13 |
| r1 | fail | `event.go`만 | 7 |
| r2 | fail | `event.go`만 | 8 |

탐색을 덜 하고 답한 조기 종료다. r2의 `contained` 1은 위의 `/dev/null` 노옵이라
이 실패와 무관하다. 이 태스크는 1.3.x 시대에는 이 모양으로 실패한 적이 없으나,
**코퍼스가 다르므로 회귀라고 부르지 않는다** — 1.4.x의 첫 값이다.

## 2. 승격 — 루브릭 1.4.2 (소유자 허가)

소유자 지시: "기준에 도달했으니 승격하자."

**바꾼 것.** 지어낸 id 중 **서버가 갖고 있지 않은** 것을 넘긴 런은 `escaped`처럼
비-pass다 — 새 판정값 `invented`. 지어낸 id 자체(`invented_ids`)는 관찰 지표로
그대로 남는다. 두 지표가 리포트에 나란히 찍힌다.

**빗나감의 정의 — 좁게 잡았다.** 서버가 그 id를 모른다고 답한 것만 센다:

- `read_code`의 `<omitted id="…" reason="not_found" />`
- `explore_graph`의 `<error code="NODE_NOT_FOUND">unknown node: …</error>`

두 가지를 일부러 제외했다. **엣지 없는 노드는 빗나감이 아니다** — 실존 노드는
빈 `<graph_context>`로 돌아온다. **`reason="gone"`도 아니다** — 검색과 읽기
사이의 리파스 경합이라 인덱스가 움직인 것이지 에이전트가 지어낸 것이 아니다.
이 둘을 안 걸렀다면 `rag-db-impact`의 맞은 추측 둘이 실패로 뒤집혔을 것이다.

**맞은 추측은 그대로 통과한다.** 벌하는 것은 추론 자체가 아니라 **부재로 보이는
실패**다. 계약이 "이전 호출이 돌려준 id만"이라고 못박은 이유가 그것이다.

**채점 전용이다.** 러너는 바이트 불변이라 `RUN_COMPAT = ("1.4.1", "1.4.2")`로
붙였고, 이 셋의 트랜스크립트가 그대로 재채점됐다. 그래서 위 표의 두 열이 같은
81런에서 나온다.

## 3. 부가 관찰

- **키워드 힌트 77건 중 46건(60%)**이 `search_keyword`로 이어졌다 — P3 힌트가
  실제로 유도하고 있다.
- **첫 검색기**: hybrid 63 · keyword 13 · none 5.
- **`search_keyword` 250콜 중 빈 결과 54(22%), 노드 id 없는 히트 35(14%)** —
  보류 중인 "패키지 레벨 선언을 노드로 볼 것인가"(키워드 히트 27% 미해결)가
  기다리던 값이다. 27%보다 낮게 나왔다.
- **비용 자기보고 비율 중앙값 0.98**(75런) — `<cost>` 도입 후 거의 정확하다.
- **`rag-oor-adoption`이 0바이트·1콜로 즉답**했다. v0.4.10에서 249~385초로
  게이트의 임계 경로를 정하던 태스크다. gate-log의 "임계 경로가 옮겨 다니는가"
  질문에 두 번째 데이터.
- `rag-read-omission` 0/3, 0콜·0바이트 — 스펙 §8의 유지 결정 그대로.

## 4. 남은 것

- **원인은 아직 안 갈렸다.** 지어낸 id가 왜 나오는가 — 통합으로 길어진
  프롬프트에서 규율이 밀린 것인지, "값싼 경로로 빨리 멈춰라"가 추측을 부추긴
  것인지. 승격은 그 답이 아니라 **오보가 통과하지 못하게 막은 것**이다.
  비율이 다음 베이스라인에서도 5/81 근처면 프롬프트 조항의 위치를 손본다.
- **grep 대조군은 이 코퍼스에서 아직 안 돌았다.** 1.4.1 대칭 이후 두 팔을 같은
  조건에서 나란히 놓으려면 `--arm grep` 한 셋이 더 필요하다(약 $10, 한 시간).
- `rag-hop-patternshape`의 조기 종료가 이 코퍼스의 성질인지 편차인지는 다음
  셋에서 갈린다.
