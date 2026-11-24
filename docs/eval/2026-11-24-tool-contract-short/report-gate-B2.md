# eval-rag — rubric 1.5.0 · arm bare

corpus 342d29c · graphin 69ea715 · model claude-sonnet-5-5 · runs 3 · lexical-only
agent 14c25158c013 · skill - · taskset edd99ac9013e · cli 2.1.331 (Claude Code)
asked claude-sonnet-5-5 · effort high · answered by claude-sonnet-5-5

## answered

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-fk-edges | 3/3 | pass pass pass | 22169 | 11 | within |
| rag-hint-conditions | 3/3 | pass pass pass | 23237 | 7 | within |
| rag-lit-arity | 3/3 | pass pass pass | 9189 | 3 | within |
| rag-lit-debounce | 2/3 | pass pass fail | 6463 | 2 | within |
| rag-lit-stoplist | 3/3 | pass pass pass | 7736 | 3 | within |
| rag-lit-tracks | 3/3 | pass pass pass | 18701 | 6 | within |
| rag-lock-steal | 3/3 | pass pass pass | 21731 | 7 | within |
| rag-md-section-id | 2/3 | fail pass pass | 16007 | 4 | within |
| rag-read-omission | 3/3 | pass pass pass | 11599 | 5 | within |
| rag-semantic-gate | 3/3 | pass pass pass | 11907 | 7 | within |
| rag-stem-rules | 3/3 | pass pass pass | 11284 | 5 | within |
| rag-usage-rotation | 3/3 | pass pass pass | 38252 | 6 | within |

tier pass rate: 34/36

## multi-hop

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-chain-model-mismatch | 2/3 | pass fail pass | 18383 | 11 | within |
| rag-chain-pin-drift | 3/3 | pass pass pass | 17371 | 9 | within |
| rag-chain-redirect | 3/3 | pass pass pass | 16666 | 6 | within |
| rag-chain-tokenizer | 3/3 | pass pass pass | 14626 | 5 | within |
| rag-chain-wiki-token | 3/3 | pass pass pass | 14545 | 4 | within |
| rag-hop-grep-baseline | 3/3 | pass pass pass | 10514 | 8 | within |
| rag-hop-patternshape | 3/3 | pass pass pass | 17248 | 6 | within |
| rag-hop-stem-sides | 3/3 | pass pass pass | 19771 | 6 | within |
| rag-hop-truncate | 3/3 | pass pass pass | 25994 | 6 | within |

tier pass rate: 26/27

## not-here

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-nh-grpc | 3/3 | pass pass pass | 4690 | 9 | within |
| rag-nh-rankdef | 3/3 | pass pass pass | 7269 | 4 | within |
| rag-nh-redis | 1/3 | pass fail fail | 5481 | 4 | within |

tier pass rate: 7/9

## out-of-reach

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-oor-adoption | 3/3 | pass pass pass | 18078 | 9 | within |
| rag-oor-latency | 3/3 | pass pass pass | 2733 | 3 | within |

tier pass rate: 6/6

## budget-pressure

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-bp-change-flow | 0/3 | fail fail fail | 26466 | 11 | over_silent |
| rag-bp-hybrid-path | 3/3 | pass pass pass | 37195 | 12 | over_stated |

tier pass rate: 3/6

## db-nav

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-db-cross-ds | 3/3 | pass pass pass | 9971 | 4 | within |
| rag-db-dangling | 3/3 | pass pass pass | 12011 | 7 | within |
| rag-db-impact | 2/3 | fail pass pass | 19287 | 9 | within |
| rag-db-nh-payments | 3/3 | pass pass pass | 5074 | 6 | within |
| rag-db-oor-rows | 3/3 | pass pass pass | 5531 | 3 | within |
| rag-db-rls-off | 3/3 | pass pass pass | 9396 | 5 | within |
| rag-db-trigger-fn | 3/3 | pass pass pass | 13159 | 5 | within |
| rag-db-unenforced | 3/3 | pass pass pass | 14452 | 9 | within |

tier pass rate: 23/24

## behavior

- left the snapshot (scored `escaped`, never pass): 0 run(s)
- invented a node id the server did not have (scored `invented`, never pass): 0 run(s)
- invented node ids: 0 run(s)
- fake citations: 1 run(s) — [('rag-md-section-id', ['docs/spec.md'])]
- keyword hints seen 55, followed by search_keyword next 13
- first retriever: hybrid 36 · keyword 53 · none 19
- search_keyword calls 92: empty 8 · hits without a node id 1
- after a keyword call, first consuming move (≤3 calls): keyword 9 · hybrid 20 · read_code 2 · read_code_other 10 · explore 4 · Read 23 · grep 23 · none 1
- cost self-report ratio (reported/actual, median): 0.79 over 107 run(s)
