# SWE-Explore harness run

- bench: `/home/tipa/projects/graphin-eval-sandbox/data/bench.enriched.jsonl`
- policy: `graphin`
- tasks: 451
- configs: 27
- elapsed: 1h14m27s
- base: top_k=5 rrf_k=60 min_conf=0.5 queries=3 semantic=false

| config | submission |
|---|---|
| k10-rrf100-mc75 | submission-graphin-k10-rrf100-mc75.jsonl |
| k10-rrf100-mc85 | submission-graphin-k10-rrf100-mc85.jsonl |
| k10-rrf100-mc95 | submission-graphin-k10-rrf100-mc95.jsonl |
| k10-rrf20-mc75 | submission-graphin-k10-rrf20-mc75.jsonl |
| k10-rrf20-mc85 | submission-graphin-k10-rrf20-mc85.jsonl |
| k10-rrf20-mc95 | submission-graphin-k10-rrf20-mc95.jsonl |
| k10-rrf60-mc75 | submission-graphin-k10-rrf60-mc75.jsonl |
| k10-rrf60-mc85 | submission-graphin-k10-rrf60-mc85.jsonl |
| k10-rrf60-mc95 | submission-graphin-k10-rrf60-mc95.jsonl |
| k20-rrf100-mc75 | submission-graphin-k20-rrf100-mc75.jsonl |
| k20-rrf100-mc85 | submission-graphin-k20-rrf100-mc85.jsonl |
| k20-rrf100-mc95 | submission-graphin-k20-rrf100-mc95.jsonl |
| k20-rrf20-mc75 | submission-graphin-k20-rrf20-mc75.jsonl |
| k20-rrf20-mc85 | submission-graphin-k20-rrf20-mc85.jsonl |
| k20-rrf20-mc95 | submission-graphin-k20-rrf20-mc95.jsonl |
| k20-rrf60-mc75 | submission-graphin-k20-rrf60-mc75.jsonl |
| k20-rrf60-mc85 | submission-graphin-k20-rrf60-mc85.jsonl |
| k20-rrf60-mc95 | submission-graphin-k20-rrf60-mc95.jsonl |
| k5-rrf100-mc75 | submission-graphin-k5-rrf100-mc75.jsonl |
| k5-rrf100-mc85 | submission-graphin-k5-rrf100-mc85.jsonl |
| k5-rrf100-mc95 | submission-graphin-k5-rrf100-mc95.jsonl |
| k5-rrf20-mc75 | submission-graphin-k5-rrf20-mc75.jsonl |
| k5-rrf20-mc85 | submission-graphin-k5-rrf20-mc85.jsonl |
| k5-rrf20-mc95 | submission-graphin-k5-rrf20-mc95.jsonl |
| k5-rrf60-mc75 | submission-graphin-k5-rrf60-mc75.jsonl |
| k5-rrf60-mc85 | submission-graphin-k5-rrf60-mc85.jsonl |
| k5-rrf60-mc95 | submission-graphin-k5-rrf60-mc95.jsonl |

채점: SWE-Explore-Bench의 `eval.py`/`ExploreEvaluator`에 위 JSONL을 투입한다 — 하니스는 제출만 생성하고 점수는 공식 스코어러가 낸다.
