# 런 스냅샷 매니페스트 — 2026-08-08 Tier-0 문장 내 심볼

[2026-08-07 채택 진단](../2026-08-07-adoption-diagnosis/findings.md)의 권고 ④에서
나온 랭킹 변경(`b582f6c`, v0.2.5)이 SWE-Explore 지표를 어느 쪽으로 움직이는지
확인한 런. **가설 검증이 아니라 회귀 확인**이므로 H1~H3 판정을 다시 내지 않고,
같은 벤치·같은 config에서 기준선과 페어 비교만 한다.

## 재현 좌표

- graphin 신규: `b582f6c` (릴리스 v0.2.5, `8ba7320`에서 빌드된 것과 동일 코드).
  변경 내용은 `internal/search/router.go` — 전체 질의 Tier-0가 비면 질의의
  식별자 모양 토큰(`identTokens`)으로 Tier-0를 태우고, 토큰 히트는 `topK/2`로
  상한.
- graphin 기준선: 샌드박스 `bin/graphin` (2026-07-25 빌드 = mc축 교체 시점).
  [2026-07-25 H1 재검증](../2026-07-25-h1-reverify/manifest.md)의 `out/sweep-451`을
  그대로 기준선으로 쓴다 — 재실행하지 않았다.
- 데이터셋: `bench.enriched.jsonl` (sha256 `a1e5dd8873b01c15…`) — 2026-07-25 런과
  **동일 파일**. 451 verified 인스턴스 전부.
- 스코어러: SWE-Explore-Bench 공식 `eval.py` @ `3c12dc5`. 채점은 항상 이것.
- 모델: 없음. **lexical-only** 런이다(`--semantic` 미지정) — Tier-0는 심볼
  테이블이므로 변경의 사정거리가 렉시컬 경로 안에 있고, 벡터 인덱스를 끌어들이면
  비교가 흐려진다.

## 설계

- 27점 스윕(top_k {5,10,20} × rrf_k {20,60,100} × min_conf {0.75,0.85,0.95}),
  451 인스턴스, `--sweep`. 소요 **1h14m27s**, caveat·태스크 에러 0.
- grep -C20 베이스라인은 **재실행하지 않았다.** 이 변경이 grep 정책을 건드리지
  않으므로 `out/grep-451`이 그대로 유효하다.
- 판정 단위는 집계 평균이 아니라 **인스턴스 단위 페어 비교**(`tier0_detail.md`).
  같은 벤치·같은 결정론 정책이라 451개가 정확히 짝지어진다. 1차 H1이 집계
  평균만 보고 뒤집혔던 전례가 이 규칙의 이유다.
- per-repo 층화를 함께 낸다. django가 표본의 46%(209/451)라 집계 하나로는
  "django가 끌었나"에 답할 수 없다.

## 산출

- `findings.md` (해석·판정) · `tier0_detail.md` (인스턴스 단위 부호검정 +
  per-repo 층화) · `report.md` (하니스 summary) · `scores.json` (기준선·신규
  27 config씩 54개 집계)
- 원시 제출은 커밋하지 않는다: 샌드박스 `out/sweep-451-tier0/`
  (제출 27 + 채점 27). 기준선은 `out/sweep-451/`.
