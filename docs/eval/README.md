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

## 규약

- 디렉터리명: `<YYYY-MM-DD>-<범위 슬러그>`.
- 각 런: `manifest.md`(재현 좌표) + `report.md`(생성 리포트) +
  `findings.md`(해석) + `scores.json`(집계) + `subset.txt`(대상 목록).
- 기본값 변경은 이 디렉터리의 리포트를 근거로만 수행한다(스펙 §3.4).
