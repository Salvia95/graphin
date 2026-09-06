# eval-rag — rubric 1.3.1

corpus eae65b6 · graphin eae65b6 (dirty) · model sonnet · runs 3 · lexical-only
agent f67281fc2db3 · skill 5d8a9a59fe08 · taskset 5ca2a37b2b92 · cli 2.1.261 (Claude Code)

## answered

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-fk-edges | 3/3 | pass pass pass | 18411 | 20 | within |
| rag-hint-conditions | 3/3 | pass pass pass | 13083 | 12 | within |
| rag-lock-steal | 3/3 | pass pass pass | 11026 | 9 | within |
| rag-md-section-id | 3/3 | pass pass pass | 9180 | 9 | within |
| rag-read-omission | 0/3 | fail fail fail | 0 | 0 | within |
| rag-semantic-gate | 3/3 | pass pass pass | 16306 | 13 | within |
| rag-stem-rules | 3/3 | pass pass pass | 24558 | 23 | within |
| rag-usage-rotation | 3/3 | pass pass pass | 31768 | 19 | over_stated,within |

tier pass rate: 21/24

## multi-hop

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-hop-grep-baseline | 3/3 | pass pass pass | 20775 | 13 | within |
| rag-hop-patternshape | 2/3 | fail pass pass | 11192 | 11 | within |
| rag-hop-stem-sides | 3/3 | pass pass pass | 17277 | 9 | within |
| rag-hop-truncate | 3/3 | pass pass pass | 18569 | 10 | within |

tier pass rate: 11/12

## not-here

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-nh-grpc | 2/3 | pass fail pass | 8122 | 9 | within |
| rag-nh-rankdef | 2/3 | pass pass fail | 6971 | 6 | within |
| rag-nh-redis | 2/3 | pass pass fail | 11259 | 8 | within |

tier pass rate: 6/9

## out-of-reach

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-oor-adoption | 3/3 | pass pass pass | 22086 | 11 | within |
| rag-oor-latency | 3/3 | pass pass pass | 3000 | 8 | within |

tier pass rate: 6/6

## budget-pressure

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-bp-change-flow | 3/3 | pass pass pass | 22707 | 20 | over_stated,within |
| rag-bp-hybrid-path | 2/3 | pass fail pass | 30002 | 17 | over_silent,over_stated |

tier pass rate: 5/6

## db-nav

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-db-cross-ds | 3/3 | pass pass pass | 8307 | 10 | within |
| rag-db-dangling | 3/3 | pass pass pass | 22175 | 14 | within |
| rag-db-impact | 2/3 | fail pass pass | 13685 | 16 | within |
| rag-db-nh-payments | 1/3 | fail fail pass | 11350 | 9 | within |
| rag-db-oor-rows | 3/3 | pass pass pass | 3415 | 4 | within |
| rag-db-rls-off | 3/3 | pass pass pass | 18519 | 20 | within |
| rag-db-trigger-fn | 2/3 | pass fail pass | 9752 | 8 | within |
| rag-db-unenforced | 3/3 | pass pass pass | 13834 | 16 | within |

tier pass rate: 20/24

## behavior

- left the snapshot (scored `escaped`, never pass): 0 run(s)
- invented node ids: 2 run(s) — ['rag-hop-grep-baseline', 'rag-stem-rules']
- fake citations: 5 run(s) — [('rag-db-nh-payments', ['docs/eval/.../scores.json']), ('rag-nh-grpc', ['docs/eval/.../scores.json']), ('rag-db-nh-payments', ['docs/eval/.../scores.json']), ('rag-nh-redis', ['docs/eval/.../scores.json']), ('rag-nh-rankdef', ['docs/eval/.../scores.json'])]
- keyword hints seen 98, followed by search_keyword next 56
- first retriever: hybrid 65 · keyword 11 · none 5
- search_keyword calls 317: empty 67 · hits without a node id 32
- after a keyword call, first consuming move (≤3 calls): keyword 138 · hybrid 55 · read_code 47 · read_code_other 26 · explore 10 · Read 8 · grep 4 · bash_read 4 · none 25
- cost self-report ratio (reported/actual, median): 0.75 over 67 run(s)
