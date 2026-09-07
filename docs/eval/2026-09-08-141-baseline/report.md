# eval-rag — rubric 1.4.2 · arm graphin

corpus 8c0d392 · graphin 8c0d392 · model sonnet · runs 3 · lexical-only
agent c11b4614ad78 · skill 1fa366d41424 · taskset 5ca2a37b2b92 · cli 2.1.263 (Claude Code)

## answered

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-fk-edges | 3/3 | pass pass pass | 12135 | 9 | within |
| rag-hint-conditions | 3/3 | pass pass pass | 9938 | 8 | within |
| rag-lock-steal | 3/3 | pass pass pass | 10213 | 12 | within |
| rag-md-section-id | 3/3 | pass pass pass | 11413 | 8 | within |
| rag-read-omission | 0/3 | fail fail fail | 0 | 0 | within |
| rag-semantic-gate | 3/3 | pass pass pass | 16686 | 8 | within |
| rag-stem-rules | 2/3 | pass pass invented | 15401 | 16 | within |
| rag-usage-rotation | 3/3 | pass pass pass | 29324 | 13 | within |

tier pass rate: 20/24

## multi-hop

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-hop-grep-baseline | 2/3 | invented pass pass | 20470 | 13 | within |
| rag-hop-patternshape | 1/3 | pass fail fail | 9129 | 8 | within |
| rag-hop-stem-sides | 3/3 | pass pass pass | 12138 | 9 | within |
| rag-hop-truncate | 3/3 | pass pass pass | 17455 | 10 | within |

tier pass rate: 9/12

## not-here

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-nh-grpc | 3/3 | pass pass pass | 5387 | 7 | within |
| rag-nh-rankdef | 3/3 | pass pass pass | 7065 | 7 | within |
| rag-nh-redis | 3/3 | pass pass pass | 9426 | 6 | within |

tier pass rate: 9/9

## out-of-reach

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-oor-adoption | 3/3 | pass pass pass | 0 | 1 | within |
| rag-oor-latency | 3/3 | pass pass pass | 3979 | 13 | within |

tier pass rate: 6/6

## budget-pressure

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-bp-change-flow | 3/3 | pass pass pass | 19039 | 16 | over_stated,within |
| rag-bp-hybrid-path | 3/3 | pass pass pass | 25253 | 11 | over_stated |

tier pass rate: 6/6

## db-nav

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-db-cross-ds | 3/3 | pass pass pass | 6094 | 8 | within |
| rag-db-dangling | 3/3 | pass pass pass | 20508 | 18 | within |
| rag-db-impact | 3/3 | pass pass pass | 12515 | 12 | within |
| rag-db-nh-payments | 3/3 | pass pass pass | 19032 | 12 | within |
| rag-db-oor-rows | 3/3 | pass pass pass | 1443 | 3 | within |
| rag-db-rls-off | 3/3 | pass pass pass | 4291 | 7 | within |
| rag-db-trigger-fn | 3/3 | pass pass pass | 11972 | 8 | within |
| rag-db-unenforced | 3/3 | pass pass pass | 11561 | 11 | within |

tier pass rate: 24/24

## behavior

- left the snapshot (scored `escaped`, never pass): 0 run(s)
- invented a node id the server did not have (scored `invented`, never pass): 2 run(s) — [('rag-hop-grep-baseline', ['internal.keyword.keyword.go']), ('rag-stem-rules', ['internal.lexical.particles'])]
- invented node ids: 5 run(s) — ['rag-hop-grep-baseline', 'rag-db-impact', 'rag-db-impact', 'rag-stem-rules', 'rag-bp-change-flow']
- fake citations: 0 run(s)
- keyword hints seen 77, followed by search_keyword next 46
- first retriever: hybrid 63 · keyword 13 · none 5
- search_keyword calls 250: empty 54 · hits without a node id 35
- after a keyword call, first consuming move (≤3 calls): keyword 109 · hybrid 38 · read_code 23 · read_code_other 26 · explore 11 · Read 2 · grep 1 · none 40
- cost self-report ratio (reported/actual, median): 0.98 over 75 run(s)
