# docs/eval — 평가 런 기록

SWE-Explore 하니스(`graphin eval swe-explore`, docs/phase7-spec.md §3) 실행
결과를 **일자별 스냅샷 디렉터리**로 보관한다. 실행 자체는 레포 밖 샌드박스
(`~/projects/graphin-eval-sandbox` — 데이터셋·저장소 스냅샷·스코어러 clone·
제출 JSONL 보관)에서 하고, 여기에는 각 런의 ①매니페스트(수행일·소스 커밋·
데이터 해시·스코어러 커밋), ②리포트, ③집계 점수만 커밋한다. 원시 제출과
데이터셋은 용량·라이선스상 커밋하지 않는다 — 매니페스트 좌표로 재현한다.

## 런 목록

| 런 | 범위 | 핵심 결과 |
|---|---|---|
| [2026-07-23-verified40](2026-07-23-verified40/manifest.md) | Verified 40, lexical-only 27점 스윕 + grep | H3 PASS (예산 내 리콜 4.1×, 라인 3.1%) · H2 vacuous · H1 보류 |
| [2026-07-24-verified40-hybrid](2026-07-24-verified40-hybrid/manifest.md) | 같은 서브셋, `--semantic` 27점 스윕 | H1 불충분(평균 +1.7%p지만 p≥0.24, ndcg 역행) · H3 5.4×로 강화 |
| [2026-07-25-h1-reverify](2026-07-25-h1-reverify/manifest.md) | H1: balanced 203 `--semantic`; H2/H3: 451 (drop 0) | **H1 역전 — hybrid ndcg@300 +7~9%p(p≤0.002)·recall@300 유의, 1차 ndcg 역행은 드랍 아티팩트** · H2 non-vacuous(축 교체) · H3 PASS |
| [2026-08-07-adoption-diagnosis](2026-08-07-adoption-diagnosis/findings.md) | 실사용 워크스페이스(kinder) 661 이벤트의 채택 0% 원인 | **원인 셋 — 단건 조회는 grep이 더 싸고(실패 아님), 다단계 탐색 7/15는 진짜 놓침, 부른 뒤 답이 안 착지** · `discovery_failure` 분모에 graphin 영역 밖 작업이 섞여 있음 |
| [2026-08-08-tier0-ranking](2026-08-08-tier0-ranking/manifest.md) | 위 진단의 권고 ④(문장 내 심볼 Tier-0) 회귀 확인, 451 lexical 27점 스윕 | **회귀 없음·개선 미증명** — 출하 설정 ndcg@300 +3.8%p(상대 +9.8%)지만 부호검정 p=0.25 · 경고: k20 precision은 크기 0에 방향 유의(p=0.022) |
| [2026-08-11-adoption-remeasure](2026-08-11-adoption-remeasure/findings.md) | 권고 ③ 첫 재측정 — 배달 구간 A/B/C를 갈라 kinder·graphin 1,615 이벤트 | **판정 불가**(표본·kinder 중단·Go 미지원 대조군) · **새 결함: 부트스트랩하지 않은 워크스페이스는 지표에 아예 안 잡힌다** — 지금 채택률은 전부 생존 편향 위 |
| [2026-08-12-target-filter](2026-08-12-target-filter/findings.md) | 이 저장소 대상 산문 질의 8개 × top_k 5 = 40슬롯, 필터 전후 | **산문 질의 슬롯의 70%가 코드가 아니었다**(docs 21 · text 7) · `target=code`로 구현 히트 7 → 22 · **다음 층은 테스트다** — 문서를 걷어내자 40슬롯 중 18을 테스트가 가져갔다 |
| [2026-08-31-stem-normalization](2026-08-31-stem-normalization/findings.md) | 어간 정규화(영어 접미사 + 한국어 조사) 회귀·개선 판정, 451 lexical 27점 스윕 기준선까지 새로 빌드 | **개선 증명** — 출하 설정 ndcg@300 +1.91%p(p=0.047)·first_useful_hit +1.13%p(p=0.024), 135개 검정 중 유의 회귀 0건, 제출 라인 불변 · 한국어 규칙은 이 벤치가 재지 못한다(영어 저장소뿐) |
| [2026-09-01-rag-baseline](2026-09-01-rag-baseline/findings.md) | graphin-rag 행동 골든셋(`eval/rag`, 이 저장소가 코퍼스) 첫 베이스라인 — 19태스크 × 3런, sonnet, lexical-only | **49/57** — not-here 9/9·out-of-reach 6/6, 가짜 인용·지어낸 id 0 · 잔여 실패는 프롬프트-에코 무인용(5)과 침묵 예산 초과(3) · 루브릭 1.0.0→1.0.3 교정 3클래스(문맥 무시 채점·미전달 예산·자기 코퍼스가 search_keyword에 잡힘) 기록 |
| [2026-09-01-rag-merged-agent](2026-09-01-rag-merged-agent/findings.md) | explorer를 rag로 통합(가이드 0.6.0)하고 러너 격리를 고친 뒤 재베이스라인 — 27태스크(db-nav 신설) × 3런, 루브릭 1.3.0 | **76/81** — multi-hop·not-here·budget-pressure·**db-nav 만점**, 위임 호출 0(직전 9) · 프롬프트-에코는 절반만 해소(`rag-read-omission` 0/3은 스킬이 답을 서술해 구조상 함정) · **새 신호: 노드 id를 작명 규칙에서 추론해 넘긴 런 6/81**(직전 0) · `escaped` 판정이 스냅샷 이탈 2건을 pass에서 걷어냈다 |
| [2026-09-02-scaling](2026-09-02-scaling/findings.md) | 코퍼스 크기(50k→1.74M LOC, 34×)에 따른 질문 하나의 비용 — graphin vs grep 에이전트, 72런 × 2설계 | **검출 실패, 그러나 이유가 다르다** — 1차(Python 밸러스트)는 분리 가능성 때문에 난이도가 안 올랐고, 2차(Go 밸러스트)는 **잡음(셀당 28%)이 효과(6~7%)의 4배**라 검정력이 없었다 · 부수 확립: 인덱싱은 LOC가 아니라 노드 수를 따르고(174만 LOC 9초), 언어별 심볼 밀도가 시맨틱 게이트 지점을 바꾼다 |

**릴리스 게이트는 런이 아니라 대장이다.** 릴리스마다 돌리는 rag 벤치 게이트의
수치는 [gate-log.md](gate-log.md)에 한 줄씩 쌓는다 — 산출물은 스크래치에
나오고 마커는 git-ignore라, 적지 않으면 통과한 순간 사라진다. 지금 그 대장이
지켜보는 것은 게이트가 5분을 넘기는 이유다.

## 규약

- 디렉터리명: `<YYYY-MM-DD>-<범위 슬러그>`.
- **벤치마크 런**: `manifest.md`(재현 좌표) + `report.md`(생성 리포트) +
  `findings.md`(해석) + `scores.json`(집계) + `subset.txt`(대상 목록).
- **실사용 계측 분석**은 종류가 다르다. 데이터셋도 스코어러도 없고 좌표는
  워크스페이스의 `.graphin/usage/events.jsonl` 하나이므로 `findings.md`만 둔다.
  대신 findings에 어느 워크스페이스·기간·이벤트 수인지를 반드시 적는다.
- 기본값 변경은 이 디렉터리의 리포트를 근거로만 수행한다(스펙 §3.4).
