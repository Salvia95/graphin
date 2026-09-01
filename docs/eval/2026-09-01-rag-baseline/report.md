# eval-rag — rubric 1.0.3

corpus worktree · graphin e7f9999 (dirty) · model sonnet · runs 3 · lexical-only
agent b83715c342ad · skill e7b2b89cae31 · taskset 0c1c39bae800 · cli 2.1.252 (Claude Code)

## answered

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-fk-edges | 2/3 | fail pass pass | 19114 | 17 | over_silent,within |
| rag-hint-conditions | 1/3 | pass fail fail | 0 | 0 | within |
| rag-lock-steal | 3/3 | pass pass pass | 7677 | 10 | within |
| rag-md-section-id | 3/3 | pass pass pass | 17574 | 9 | within |
| rag-read-omission | 0/3 | fail fail fail | 0 | 0 | within |
| rag-semantic-gate | 3/3 | pass pass pass | 23238 | 15 | within |
| rag-stem-rules | 3/3 | pass pass pass | 17773 | 20 | within |
| rag-usage-rotation | 3/3 | pass pass pass | 17635 | 18 | within |

tier pass rate: 18/24

## multi-hop

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-hop-grep-baseline | 3/3 | pass pass pass | 25362 | 16 | within |
| rag-hop-patternshape | 2/3 | pass fail pass | 8419 | 9 | within |
| rag-hop-stem-sides | 3/3 | pass pass pass | 19375 | 14 | within |
| rag-hop-truncate | 3/3 | pass pass pass | 15556 | 8 | within |

tier pass rate: 11/12

## not-here

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-nh-grpc | 3/3 | pass pass pass | 3767 | 10 | within |
| rag-nh-rankdef | 3/3 | pass pass pass | 2848 | 7 | within |
| rag-nh-redis | 3/3 | pass pass pass | 7926 | 7 | within |

tier pass rate: 9/9

## out-of-reach

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-oor-adoption | 3/3 | pass pass pass | 1689 | 5 | within |
| rag-oor-latency | 3/3 | pass pass pass | 1775 | 8 | within |

tier pass rate: 6/6

## budget-pressure

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-bp-change-flow | 3/3 | pass pass pass | 18045 | 14 | over_stated,within |
| rag-bp-hybrid-path | 2/3 | pass fail pass | 35462 | 20 | over_silent,over_stated |

tier pass rate: 5/6

## behavior

- invented node ids: 0 run(s)
- fake citations: 0 run(s)
- keyword hints seen 50, followed by search_keyword next 22
- cost self-report ratio (reported/actual, median): 0.79 over 36 run(s)
