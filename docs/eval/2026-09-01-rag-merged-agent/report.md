# eval-rag — rubric 1.3.0

corpus worktree · graphin e7f9999 (dirty) · model sonnet · runs 3 · lexical-only
agent f67281fc2db3 · skill e7b2b89cae31 · taskset 5ca2a37b2b92 · cli 2.1.252 (Claude Code)

## answered

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-fk-edges | 3/3 | pass pass pass | 18961 | 14 | within |
| rag-hint-conditions | 3/3 | pass pass pass | 11031 | 11 | within |
| rag-lock-steal | 3/3 | pass pass pass | 10756 | 14 | within |
| rag-md-section-id | 3/3 | pass pass pass | 11555 | 9 | within |
| rag-read-omission | 0/3 | fail fail fail | 0 | 1 | within |
| rag-semantic-gate | 3/3 | pass pass pass | 16715 | 10 | within |
| rag-stem-rules | 3/3 | pass pass pass | 23518 | 27 | within |
| rag-usage-rotation | 3/3 | pass pass pass | 19319 | 15 | within |

tier pass rate: 21/24

## multi-hop

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-hop-grep-baseline | 3/3 | pass pass pass | 13590 | 10 | within |
| rag-hop-patternshape | 3/3 | pass pass pass | 13934 | 13 | within |
| rag-hop-stem-sides | 3/3 | pass pass pass | 13349 | 8 | within |
| rag-hop-truncate | 3/3 | pass pass pass | 17024 | 11 | within |

tier pass rate: 12/12

## not-here

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-nh-grpc | 3/3 | pass pass pass | 4508 | 7 | within |
| rag-nh-rankdef | 3/3 | pass pass pass | 2403 | 6 | within |
| rag-nh-redis | 3/3 | pass pass pass | 8886 | 8 | within |

tier pass rate: 9/9

## out-of-reach

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-oor-adoption | 1/3 | escaped pass escaped | 38443 | 12 | over_stated,within |
| rag-oor-latency | 3/3 | pass pass pass | 4252 | 11 | within |

tier pass rate: 4/6

## budget-pressure

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-bp-change-flow | 3/3 | pass pass pass | 26553 | 18 | over_stated |
| rag-bp-hybrid-path | 3/3 | pass pass pass | 23584 | 14 | over_stated |

tier pass rate: 6/6

## db-nav

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-db-cross-ds | 3/3 | pass pass pass | 7787 | 9 | within |
| rag-db-dangling | 3/3 | pass pass pass | 9921 | 12 | within |
| rag-db-impact | 3/3 | pass pass pass | 13241 | 18 | within |
| rag-db-nh-payments | 3/3 | pass pass pass | 9284 | 8 | within |
| rag-db-oor-rows | 3/3 | pass pass pass | 3415 | 4 | within |
| rag-db-rls-off | 3/3 | pass pass pass | 5297 | 8 | within |
| rag-db-trigger-fn | 3/3 | pass pass pass | 14968 | 8 | within |
| rag-db-unenforced | 3/3 | pass pass pass | 12862 | 15 | within |

tier pass rate: 24/24

## behavior

- left the snapshot (scored `escaped`, never pass): 2 run(s) — [('rag-oor-adoption', ['~/.claude/projects']), ('rag-oor-adoption', ['/home/tipa/.claude/plugins/cache/graphin', '/home/tipa/.claude/plugins/cache/graphin/graphin/0.4.8/bin/graphin-launch.sh', '/home/tipa/.claude/projects/', '/home/tipa/projects/graphin', '/home/tipa/projects/graphin/.graphin'])]
- invented node ids: 6 run(s) — ['rag-md-section-id', 'rag-stem-rules', 'rag-fk-edges', 'rag-db-impact', 'rag-md-section-id', 'rag-lock-steal']
- fake citations: 0 run(s)
- keyword hints seen 89, followed by search_keyword next 58
- cost self-report ratio (reported/actual, median): 0.77 over 67 run(s)
