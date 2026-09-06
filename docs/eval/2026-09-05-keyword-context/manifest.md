# 런 스냅샷 매니페스트 — 2026-09-05 keyword-context

[`docs/keyword-plan.md`](../../keyword-plan.md)의 1~3단계(P4 · M2 · P1 · P3 ·
M1 · M3)와 그 측정. 하나의 벤치 런이 아니라 **네 종류의 측정을 한 디렉터리에**
둔다 — 전부 같은 변경(키워드 검색기의 빈자리 넷)을 다른 축에서 본 것이라서다.

| 항목 | 값 |
|---|---|
| 수행일 | 2026-09-05 |
| graphin 소스 | `eae65b6`(v0.4.11) + 미커밋 변경: `internal/usage`(P4), `internal/keyword`·`internal/search`·`internal/mcp/tools`(P1·P3), `scripts/eval-rag.py`(M2, 루브릭 1.3.1), `scripts/eval-recall.py`(M1) |
| 소유자 허가 | 채점기 변경은 세션에서 명시 허가 → `.rag-bench-unlock` → `lock --approved-by salvia95` → unlock 삭제 |

## 측정 좌표

| 측정 | 장치 | 코퍼스 · 바이너리 | 산출 |
|---|---|---|---|
| **M2 재채점** (18런) | `scripts/eval-rag.py score`, 루브릭 1.3.1 | v0.4.10·v0.4.11 게이트 스모크 트랜스크립트 3세트(각 6런, 루브릭 1.3.0으로 생성) — 스크래치 `gate-smoke2/3/4` | `scores.json` → `smoke_rescore` |
| **M2 변경 전** (77/81런, 429 중단) | `scripts/eval-rag.py run --runs 3 --jobs 3`, sonnet, lexical-only | 코퍼스 `--ref HEAD`(`eae65b6`), 바이너리 = **P1·P3 이전** 트리 빌드 사본(`graphin-pre-p1`, usage 변경만), 스킬 sha `e7b2b8…`(HEAD) | `scores.json` → `rag_baseline_partial_attempt` (관찰 지표만, 채점기 거부 대상) |
| **M2 프롬프트만** (81런) | 같은 명령, 한도 해제 후 재실행 | 같은 바이너리, **스킬 = P1 편집본**(sha `5d8a9a…`, `context` 레시피 포함) — 순서 실수로 생긴 팔 | `scores.json` → `rag_rerun_skilltext`, 리포트 `report-rag-skilltext.md` |
| **M1 결정론** | `scripts/eval-recall.py --tier all`(lexical-only) + 신설 키워드 팔·힌트 집계 | 코퍼스 `--ref HEAD`, 바이너리 = **P1·P3 포함** 빌드 | `scores.json` → `recall`, 리포트는 `report-recall.md` |
| **M3 대조군 불변** | `graphin eval swe-explore --policy grep` 451 | 샌드박스 `data/bench.enriched.jsonl` + `repos/`, 바이너리 = P1 포함 빌드 | md5 비교 결과는 `findings.md` |

## 규약 주석

- rag 기준선은 **변경 전 상태를 재는 것이 목적**이다. 09-01 재베이스라인
  81런의 트랜스크립트가 스크래치에 남아 있지 않아 재채점이 불가했고, 대신
  변경 전 바이너리·스킬로 한 번 더 돌렸다 — 그것이 429로 77런에서 끊겼고,
  재실행은 스킬이 이미 편집된 뒤라 다른 팔이 됐다(findings §2.2). 그래서 이
  디렉터리에는 "개선"이 없다 — 개선 여부는 4·5단계(릴리스 후 재측정)에서 이
  두 팔과 비교해 판단한다.
- 루브릭 1.3.1은 관찰 지표만 더했으므로 `RUN_COMPAT`에 붙어 1.3.0 트랜스크립트를
  재채점한다. 판정 규칙은 바뀌지 않았다.
