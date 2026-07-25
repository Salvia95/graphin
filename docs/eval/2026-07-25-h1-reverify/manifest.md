# 런 스냅샷 매니페스트 — 2026-07-25 H1 재검증 (drop 0)

[2026-07-24-verified40-hybrid](../2026-07-24-verified40-hybrid/manifest.md)의
H1 "불충분" 판정을 두 결함(①임베딩 드랍으로 벡터 인덱스 불완전 ②40개 소표본)을
제거해 재검증한 런. 결과는 **역전** — findings.md 참조.

## 재현 좌표
- graphin 소스: base `e62cc00` + `internal/eval/sweexplore/run.go`의 min_confidence
  스윕 축 교체 `{0, 0.5, 0.8}`→`{0.75, 0.85, 0.95}`(이 변경과 함께 커밋). 이전 축은
  최저 confidence 티어 0.80 이하라 전부 no-op였다(H2 vacuous). 파일명 인코딩도
  `mc*10`→`mc*100`(0.75/0.85가 `%.0f`로 충돌하던 것 수정).
- 데이터셋: HF SWE-Explore-Bench 451 verified + SWE-bench_Verified problem_statement/
  base_commit 조인 = `bench.enriched.jsonl` (sha256 `a1e5dd8873b01c15…`).
- 스코어러: SWE-Explore-Bench 공식 `eval.py` @ `3c12dc5`. 채점은 항상 이것.
- 모델(H1): `english_optimal` = multilingual-e5-small-v2 INT8, ORT 1.26.0.
  SHA256는 `internal/provision/pins.go`.

## 설계 (하이브리드 범위)
- **H2/H3**: 전체 451 인스턴스, lexical 27점 스윕 + grep -C20 베이스라인.
- **H1**: repo당 상한 25 = **balanced 203** (`subset-h1-203.txt`,
  sha256 `aea6cfe4ef4776d9…`). django 209→25(12%)로 캡해 단일 repo 지배 제거.
  hybrid(`--semantic`, 임베딩 큐 드레인 대기) vs 같은 203의 lexical을 matched-N
  페어 비교. 페어 유의성은 `h1_detail.md`(인스턴스 단위 부호검정)가 권위.
- **임베딩 드랍 0**: semantic 203/203 태스크에 coverage caveat 없음. `e62cc00`의
  무계 백로그가 1차의 드랍(django 큐 상한 근접)을 제거했다.

## 산출
- `report-h1-203.md` (H1, matched N=203) · `report-h23-451.md` (H2/H3, N=451)
- `findings.md` (해석·권고) · `h1_detail.md` (per-instance 부호검정)
- `h1_perrepo.md` (per-repo 층화) · `scores.json` (전 런 집계)

## 규약 주석
1차와 달리 H1은 balanced 203, H2/H3는 451이라 리포트가 둘로 갈린다. 집계 평균은
방향만 보고, 판정은 `h1_detail.md`의 부호검정으로 한다.
