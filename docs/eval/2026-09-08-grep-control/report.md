# eval-rag — rubric 1.4.2 · arm grep

corpus 8c0d392 · graphin c8981dd · model sonnet · runs 3 · lexical-only
agent 237ea8f43410 · skill - · taskset 5ca2a37b2b92 · cli 2.1.263 (Claude Code)

## answered

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-fk-edges | 1/3 | fail fail pass | 57934 | 13 | over_silent,over_stated |
| rag-hint-conditions | 3/3 | pass pass pass | 19118 | 9 | within |
| rag-lock-steal | 3/3 | pass pass pass | 12036 | 5 | within |
| rag-md-section-id | 3/3 | pass pass pass | 22341 | 8 | within |
| rag-read-omission | 3/3 | pass pass pass | 18005 | 9 | within |
| rag-semantic-gate | 3/3 | pass pass pass | 14949 | 10 | within |
| rag-stem-rules | 3/3 | pass pass pass | 17017 | 7 | within |
| rag-usage-rotation | 3/3 | pass pass pass | 40644 | 11 | over_stated,within |

tier pass rate: 22/24

## multi-hop

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-hop-grep-baseline | 3/3 | pass pass pass | 15147 | 11 | within |
| rag-hop-patternshape | 2/3 | fail pass pass | 9612 | 7 | within |
| rag-hop-stem-sides | 3/3 | pass pass pass | 22509 | 10 | within |
| rag-hop-truncate | 3/3 | pass pass pass | 14645 | 15 | within |

tier pass rate: 11/12

## not-here

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-nh-grpc | 3/3 | pass pass pass | 7797 | 7 | within |
| rag-nh-rankdef | 3/3 | pass pass pass | 6148 | 13 | within |
| rag-nh-redis | 2/3 | pass fail pass | 5600 | 7 | within |

tier pass rate: 8/9

## out-of-reach

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-oor-adoption | 3/3 | pass pass pass | 18485 | 7 | within |
| rag-oor-latency | 3/3 | pass pass pass | 5966 | 6 | within |

tier pass rate: 6/6

## budget-pressure

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-bp-change-flow | 3/3 | pass pass pass | 42430 | 20 | over_stated |
| rag-bp-hybrid-path | 3/3 | pass pass pass | 41065 | 16 | over_stated |

tier pass rate: 6/6

## db-nav

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-db-cross-ds | 3/3 | pass pass pass | 27875 | 13 | within |
| rag-db-dangling | 3/3 | pass pass pass | 20353 | 13 | within |
| rag-db-impact | 3/3 | pass pass pass | 20885 | 13 | within |
| rag-db-nh-payments | 3/3 | pass pass pass | 5665 | 5 | within |
| rag-db-oor-rows | 3/3 | pass pass pass | 5401 | 4 | within |
| rag-db-rls-off | 3/3 | pass pass pass | 6314 | 6 | within |
| rag-db-trigger-fn | 3/3 | pass pass pass | 6028 | 7 | within |
| rag-db-unenforced | 3/3 | pass pass pass | 28693 | 17 | within |

tier pass rate: 24/24

## behavior

- left the snapshot (scored `escaped`, never pass): 0 run(s)
- invented a node id the server did not have (scored `invented`, never pass): 0 run(s)
- invented node ids: 0 run(s)
- calls the containment hook denied: 12 in 12 run(s)
- fake citations: 0 run(s)
- keyword hints seen 9, followed by search_keyword next 0
- first retriever: hybrid 0 · keyword 0 · none 81
- cost self-report ratio (reported/actual, median): 0.70 over 80 run(s)
