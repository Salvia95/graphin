# 2026-11-25 — SWE-Explore, 세 언어(Java · TypeScript · Python)

지금까지의 SWE-Explore 측정은 SWE-bench Verified(전부 Python)뿐이었다. README에 언어별 근거를 싣기 위해,
벤치에서 graphin이 지원하는 대표 언어 셋의 큰 저장소를 골라 같은 하니스·같은 스코어러로 다시 쟀다.

## 설정

| 항목 | 값 |
|---|---|
| 벤치 | HF `SWE-Explore-Bench/SWE-Explore-Bench` `bench.final.public.jsonl` (848행, sha256 `dc4f114e…`, 이전 런과 같은 판) |
| 서브셋 | **apache/lucene 7** (Java, 벤치의 lucene 전부) · **element-hq/element-web 12** (TypeScript, 전부) · **django/django 21** (Python, 209개를 instance_id 정렬 후 10개마다 1개) — 40태스크, 목록 [`subset.txt`](subset.txt) |
| 선정 이유 | 언어별로 벤치에 들어 있는 가장 큰 단일 언어 저장소 중 태스크가 여럿인 것. 테스트 제외 소스 파일 수(현재 HEAD 기준) lucene 3,934 · element-web 2,118 · django 921 |
| 이슈 본문 | `instance_id`로 조인 — Verified(`princeton-nlp/SWE-bench_Verified`), Multilingual(`SWE-bench/SWE-bench_Multilingual`), Pro(`ScaleAI/SWE-bench_Pro` default + v1 1건) |
| 스냅샷 | 태스크별 `base_commit`을 depth 1로 fetch해 `git archive`로 풀었다 |
| graphin | `v0.4.23-3-ga8872ee`, `graphin eval swe-explore --policy graphin`, 하니스 기본값(top_k 5 · rrf_k 60 · min_confidence 0.5 · queries 3 · max-regions 100), **lexical 전용** |
| 대조 | `--policy grep` — 같은 질의 유도에 grep ±20줄 |
| 채점 | 공식 `ExploreEvaluator`(`eval.py` @ `3c12dc5`, 이전 런과 같은 커밋), `scripts/swe-explore-score.py` |
| 소요 | graphin 6분 25초(40태스크, 태스크마다 새로 색인) · grep 18초 |

두 정책 모두 LLM이 없는 **결정론 탐색기**다. 에이전트가 아니라, 같은 이슈에서 같은 규칙으로 뽑은 질의를 각 검색기에
넣었을 때 정답 영역을 얼마나 앞쪽에, 얼마나 적은 줄로 내놓는지를 잰다.

## 결과

| | recall | precision | 파일 적중 | **recall@300** | ndcg@300 | 첫 유효 적중 | 제출 줄 |
|---|--:|--:|--:|--:|--:|--:|--:|
| **graphin** 전체 | 0.243 | 0.093 | 0.597 | **0.118** | 0.389 | 0.821 | 2,283 |
| grep ±20 전체 | 0.177 | 0.003 | 0.123 | 0.000 | 0.025 | 0.182 | 107,468 |
| graphin · Java (lucene) | 0.302 | 0.049 | 0.750 | 0.129 | 0.368 | 0.801 | 4,483 |
| graphin · TypeScript (element-web) | 0.154 | 0.045 | 0.260 | 0.091 | 0.272 | 0.567 | 1,908 |
| graphin · Python (django) | 0.275 | 0.135 | 0.739 | 0.130 | 0.462 | 0.973 | 1,765 |
| grep · Java | 0.001 | 0.000 | 0.071 | 0.000 | 0.000 | 0.023 | 105,050 |
| grep · TypeScript | 0.076 | 0.002 | 0.041 | 0.000 | 0.000 | 0.169 | 94,644 |
| grep · Python | 0.294 | 0.004 | 0.187 | 0.000 | 0.048 | 0.242 | 115,602 |

짝 비교(태스크별): **recall@300 graphin 우세 33 · 열세 0 · 동률 7 (부호검정 p < 1e-9)**. 예산 없는 recall도 26:9로 graphin.
원자료: [`report.md`](report.md), 태스크별 점수 [`scores-graphin.json`](scores-graphin.json) · [`scores-grep.json`](scores-grep.json).

## 판정

**1. 451태스크 Python의 결론이 세 언어로 넓어진다.** 300줄 예산 안에서 grep ±20은 세 언어 모두 정답 영역을 하나도 앞에
올리지 못했고(recall@300 0.000), graphin은 0.09~0.13을 냈다. 제출 줄은 grep의 2.1%다. 이전 451태스크 측정(recall@300
0.109 대 0.004, 2.6%)과 같은 자리다.

**2. TypeScript가 가장 약하다.** element-web은 파일 적중 0.26, 첫 유효 적중 0.57로 Java·Python(0.74~0.75, 0.80~0.97)보다
크게 낮다. 정답 파일이 태스크당 평균 9개로 넓게 흩어진 SWE-bench Pro 태스크이고, 이슈 본문이 UI 동작을 서술해 코드
식별자가 적다. 원인 분해는 하지 않았다 — 다음 측정 후보.

**3. grep 대조군은 약한 기준선이다.** 같은 질의를 grep에 넣고 ±20줄을 붙이는 결정론 정책이라, 에이전트가 grep을 쓰는
방식(패턴을 바꿔 가며 좁히기)을 재현하지 않는다. README에는 "같은 질의, 같은 줄 예산에서"라는 범위를 붙여 인용한다.
에이전트 단위의 비교는 rag 벤치 측정(정답률 동률, 이 저장소 크기에서는 토큰 비용도 동률)이 맡는다.

## 재현

```sh
graphin eval swe-explore --bench bench-3lang.jsonl --repos <dir> --out out-graphin --policy graphin
graphin eval swe-explore --bench bench-3lang.jsonl --repos <dir> --out out-grep --policy grep
scripts/swe-explore-score.py --scorer <SWE-Explore-Bench @3c12dc5> --bench bench-3lang.jsonl \
  out-graphin/submission-graphin-default.jsonl out-grep/submission-grep-default.jsonl
```

`bench-3lang.jsonl`은 위 벤치 행에 `problem_statement`·`base_commit`·`repo`를 조인하고 `repo_dir`을 `repos/<instance_id>`로
둔 것이다. 데이터셋과 스냅샷은 용량·라이선스 때문에 커밋하지 않는다.
