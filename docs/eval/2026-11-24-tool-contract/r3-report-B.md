# eval-rag — rubric 1.5.0 · arm bare

corpus 392d066 · graphin 392d066 · model claude-sonnet-5-5 · runs 3 · lexical-only
agent 14c25158c013 · skill - · taskset edd99ac9013e · cli 2.1.331 (Claude Code)
asked claude-sonnet-5-5 · effort high · answered by claude-sonnet-5-5

## answered

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-fk-edges | 3/3 | pass pass pass | 17946 | 9 | within |
| rag-hint-conditions | 3/3 | pass pass pass | 20258 | 7 | within |
| rag-lit-arity | 3/3 | pass pass pass | 10826 | 5 | within |
| rag-lit-debounce | 3/3 | pass pass pass | 6363 | 2 | within |
| rag-lit-stoplist | 3/3 | pass pass pass | 10823 | 5 | within |
| rag-lit-tracks | 3/3 | pass pass pass | 19128 | 9 | within |
| rag-lock-steal | 3/3 | pass pass pass | 19400 | 7 | within |
| rag-md-section-id | 2/3 | pass fail pass | 14332 | 3 | within |
| rag-read-omission | 3/3 | pass pass pass | 10325 | 6 | within |
| rag-semantic-gate | 3/3 | pass pass pass | 15164 | 6 | within |
| rag-stem-rules | 3/3 | pass pass pass | 16166 | 5 | within |
| rag-usage-rotation | 3/3 | pass pass pass | 21070 | 5 | within |

tier pass rate: 35/36

## multi-hop

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-chain-model-mismatch | 3/3 | pass pass pass | 15940 | 12 | within |
| rag-chain-pin-drift | 3/3 | pass pass pass | 19375 | 7 | within |
| rag-chain-redirect | 3/3 | pass pass pass | 25016 | 4 | within |
| rag-chain-tokenizer | 2/3 | fail pass pass | 14117 | 5 | within |
| rag-chain-wiki-token | 3/3 | pass pass pass | 23194 | 5 | within |
| rag-hop-grep-baseline | 3/3 | pass pass pass | 12234 | 8 | within |
| rag-hop-patternshape | 3/3 | pass pass pass | 17332 | 4 | within |
| rag-hop-stem-sides | 3/3 | pass pass pass | 21683 | 6 | within |
| rag-hop-truncate | 3/3 | pass pass pass | 23843 | 7 | within |

tier pass rate: 26/27

## not-here

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-nh-grpc | 3/3 | pass pass pass | 2735 | 7 | within |
| rag-nh-rankdef | 3/3 | pass pass pass | 3000 | 4 | within |
| rag-nh-redis | 0/3 | fail fail fail | 3222 | 4 | within |

tier pass rate: 6/9

## out-of-reach

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-oor-adoption | 3/3 | pass pass pass | 16540 | 8 | within |
| rag-oor-latency | 3/3 | pass pass pass | 2192 | 3 | within |

tier pass rate: 6/6

## budget-pressure

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-bp-change-flow | 0/3 | fail fail fail | 25075 | 12 | over_silent |
| rag-bp-hybrid-path | 2/3 | pass fail pass | 45922 | 12 | over_stated |

tier pass rate: 2/6

## db-nav

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-db-cross-ds | 3/3 | pass pass pass | 9106 | 7 | within |
| rag-db-dangling | 3/3 | pass pass pass | 13079 | 6 | within |
| rag-db-impact | 1/3 | pass fail fail | 9469 | 7 | within |
| rag-db-nh-payments | 3/3 | pass pass pass | 4095 | 7 | within |
| rag-db-oor-rows | 3/3 | pass pass pass | 8382 | 4 | within |
| rag-db-rls-off | 3/3 | pass pass pass | 5845 | 6 | within |
| rag-db-trigger-fn | 3/3 | pass pass pass | 20704 | 5 | within |
| rag-db-unenforced | 3/3 | pass pass pass | 13176 | 8 | within |

tier pass rate: 22/24

## behavior

- left the snapshot (scored `escaped`, never pass): 0 run(s)
- invented a node id the server did not have (scored `invented`, never pass): 0 run(s)
- invented node ids: 0 run(s)
- fake citations: 1 run(s) — [('rag-md-section-id', ['docs/x.md'])]
- keyword hints seen 58, followed by search_keyword next 19
- first retriever: hybrid 47 · keyword 52 · none 9
- search_keyword calls 145: empty 14 · hits without a node id 7
- after a keyword call, first consuming move (≤3 calls): keyword 31 · hybrid 22 · read_code 5 · read_code_other 10 · explore 4 · Read 43 · grep 16 · bash_read 1 · none 13
- cost self-report ratio (reported/actual, median): 0.81 over 108 run(s)
