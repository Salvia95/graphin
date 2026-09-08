# 런 스냅샷 매니페스트 — 2026-09-08 36태스크 첫 풀셋

[골든셋 확장](../2026-09-08-golden-set-expansion/findings.md) 뒤의 첫 측정이자
새 `taskset_sha`의 베이스라인.

| 항목 | 값 |
|---|---|
| 수행일 | 2026-09-08 (108런, 오류 0, `--jobs 3`으로 실경과 약 40분) |
| 명령 | `scripts/eval-rag.py run --out <샌드박스>/rag-2026-09-08-36-baseline --runs 3 --jobs 3 --detach` |
| 소유자 지시 | "풀셋 돌려줘" |
| 바이너리 | `bin/graphin` v0.4.12-6-gb564780 — Go 트리는 v0.4.12 릴리스(`2f05f8b`)와 바이트 동일 |
| 에이전트 · 스킬 | `c11b4614ad78` · `1fa366d41424` — 확장 전과 같은 sha |
| 태스크셋 | **`edd99ac9013e`** (36태스크). 이전 `5ca2a37b2b92`(27태스크)와 총점을 직접 비교하지 않는다 |
| 코퍼스 | `b564780`(HEAD, 트리 clean) |
| 검색 | lexical-only(`--ort-lib /nonexistent-ort`) |
| 채점 | 루브릭 **1.4.2** |
| 모델 · CLI | sonnet · Claude Code 2.1.263 |
| 게이트 | `score --gate 0.80` 통과 → 마커 `commit b564780` · mode **full** |
| 산출 | 샌드박스 `~/projects/graphin-eval-sandbox/out/rag-2026-09-08-36-baseline/`, 이 디렉터리의 `report.md` · `meta.json` |
