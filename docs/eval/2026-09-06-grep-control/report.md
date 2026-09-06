# eval-rag — rubric 1.3.2 · arm grep

corpus eae65b6 · graphin eae65b6 (dirty) · model sonnet · runs 3 · lexical-only
agent 237ea8f43410 · skill - · taskset 5ca2a37b2b92 · cli 2.1.261 (Claude Code)

## answered

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-fk-edges | 1/3 | pass fail fail | 45602 | 14 | over_silent,over_stated |
| rag-hint-conditions | 3/3 | pass pass pass | 13919 | 12 | within |
| rag-lock-steal | 3/3 | pass pass pass | 25417 | 8 | within |
| rag-md-section-id | 3/3 | pass pass pass | 21929 | 8 | within |
| rag-read-omission | 3/3 | pass pass pass | 19876 | 8 | within |
| rag-semantic-gate | 3/3 | pass pass pass | 15810 | 7 | within |
| rag-stem-rules | 3/3 | pass pass pass | 14201 | 6 | within |
| rag-usage-rotation | 2/3 | fail pass pass | 51876 | 11 | over_silent,over_stated,within |

tier pass rate: 21/24

## multi-hop

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-hop-grep-baseline | 3/3 | pass pass pass | 15675 | 9 | within |
| rag-hop-patternshape | 1/3 | fail fail pass | 14480 | 9 | within |
| rag-hop-stem-sides | 3/3 | pass pass pass | 22982 | 8 | within |
| rag-hop-truncate | 3/3 | pass pass pass | 16818 | 14 | within |

tier pass rate: 10/12

## not-here

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-nh-grpc | 3/3 | pass pass pass | 7465 | 6 | within |
| rag-nh-rankdef | 3/3 | pass pass pass | 3899 | 9 | within |
| rag-nh-redis | 3/3 | pass pass pass | 7873 | 7 | within |

tier pass rate: 9/9

## out-of-reach

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-oor-adoption | 3/3 | pass pass pass | 49545 | 7 | over_silent,over_stated,within |
| rag-oor-latency | 3/3 | pass pass pass | 6076 | 6 | within |

tier pass rate: 6/6

## budget-pressure

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-bp-change-flow | 3/3 | pass pass pass | 48873 | 33 | over_stated |
| rag-bp-hybrid-path | 3/3 | pass pass pass | 43012 | 21 | over_stated |

tier pass rate: 6/6

## db-nav

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-db-cross-ds | 3/3 | pass pass pass | 20629 | 11 | within |
| rag-db-dangling | 3/3 | pass pass pass | 23090 | 10 | within |
| rag-db-impact | 3/3 | pass pass pass | 26992 | 15 | within |
| rag-db-nh-payments | 2/3 | fail pass pass | 9440 | 8 | within |
| rag-db-oor-rows | 3/3 | pass pass pass | 8435 | 3 | within |
| rag-db-rls-off | 3/3 | pass pass pass | 5046 | 6 | within |
| rag-db-trigger-fn | 3/3 | pass pass pass | 4886 | 5 | within |
| rag-db-unenforced | 3/3 | pass pass pass | 29826 | 17 | within |

tier pass rate: 23/24

## behavior

- left the snapshot (scored `escaped`, never pass): 0 run(s)
- invented node ids: 0 run(s)
- calls the containment hook denied: 11 in 10 run(s)
- fake citations: 2 run(s) — [('rag-db-nh-payments', ['docs/eval/...report.md']), ('rag-hop-patternshape', ['internal/usage/router.go'])]
- keyword hints seen 14, followed by search_keyword next 0
- first retriever: hybrid 0 · keyword 0 · none 81
- cost self-report ratio (reported/actual, median): 0.65 over 81 run(s)
