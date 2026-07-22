# 런 스냅샷 매니페스트 — 2026-07-23 verified40

Phase 7c 가설 검증 1차 런의 재현 정보. 실행은 레포 밖 샌드박스
(`~/projects/graphin-eval-sandbox`)에서 수행했고, 이 디렉터리에는 결과와
재현 좌표만 커밋한다(제출 JSONL·스냅샷·데이터셋 원본은 미커밋).

## 수행 정보

| 항목 | 값 |
|---|---|
| 수행일 | 2026-07-23 |
| graphin 소스 | `77efcc4` (Phase 7c 하니스 커밋, clean tree 빌드) |
| 정책 | `graphin eval swe-explore` 결정론 정책 (lexical-only) + `--policy grep` 베이스라인 |
| 스윕 | top_k {5,10,20} × RRF k {20,60,100} × min_confidence {0,0.5,0.8} = 27점 |
| 하니스 파라미터 | queries=3, max-regions=100, max-region-lines=400, grep-context=20 |
| 소요 | 스윕+베이스라인 40태스크 약 40분 (config마다 재파싱 — 개선 후보) |

## 데이터·채점 좌표

| 항목 | 값 |
|---|---|
| 벤치 | HF `SWE-Explore-Bench/SWE-Explore-Bench` `bench.final.public.jsonl` (848행, tree `bdb0ae45d7c3…`) |
| 벤치 sha256 | `dc4f114ececd0bfb987361c26ae5e2440456e2cccb36adfccb09ea5385aec202` |
| 이슈 조인 | `princeton-nlp/SWE-bench_Verified` test split의 problem_statement/base_commit을 instance_id로 조인 → enriched sha256 `a1e5dd8873b0…62ab393e` |
| 서브셋 | verified 451 중 40개 — instance_id 사전순, repo당 최대 6 (결정론; 목록: `subset.txt`) |
| 저장소 스냅샷 | GitHub archive tarball @ base_commit, 인스턴스별 디렉터리 |
| 채점 | 공식 스코어러 [SWE-Explore-Bench](https://github.com/Qiushao-E/SWE-Explore-Bench) `eval.py` @ `3c12dc5` — 하니스는 제출만 생성 |

## 파일

- `report.md` — 스윕 표 + H1~H3 판정 (스크립트 생성물)
- `findings.md` — 해석·발견·후속 제안
- `scores.json` — 28개 제출(27 스윕 + grep)의 공식 스코어러 집계 지표
- `subset.txt` — 평가 대상 instance_id 40개

## 결과 요약 (상세: report.md / findings.md)

- **H3 PASS**: 300라인 예산에서 recall 0.107 vs grep -C20 0.026 (4.1×), 제출 라인 3.1%
- **H2 vacuous PASS**: confidence 최저 티어 0.8 → mc<0.8 스윕은 no-op (축 교체 필요)
- **H1 보류**: SemanticReady가 임베딩 큐 드레인 미보장 — 드레인 신호 추가 후 재실행
- top_k만 실질 손잡이 (5→10: recall@300 +1.6%p, 10→20 포화)
