# eval-rag — rubric 1.5.0 · arm bare

corpus 392d066 · graphin 392d066 · model claude-sonnet-5-5 · runs 3 · lexical-only
agent 14c25158c013 · skill - · taskset edd99ac9013e · cli 2.1.331 (Claude Code)
asked claude-sonnet-5-5 · effort high · answered by claude-sonnet-5-5

## answered

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-fk-edges | 3/3 | pass pass pass | 17522 | 8 | within |
| rag-hint-conditions | 3/3 | pass pass pass | 27298 | 7 | within |
| rag-lit-arity | 3/3 | pass pass pass | 5953 | 7 | within |
| rag-lit-debounce | 3/3 | pass pass pass | 3551 | 2 | within |
| rag-lit-stoplist | 2/3 | pass fail pass | 9732 | 6 | within |
| rag-lit-tracks | 3/3 | pass pass pass | 17354 | 6 | within |
| rag-lock-steal | 3/3 | pass pass pass | 20657 | 6 | within |
| rag-md-section-id | 3/3 | pass pass pass | 21494 | 3 | within |
| rag-read-omission | 3/3 | pass pass pass | 10858 | 5 | within |
| rag-semantic-gate | 3/3 | pass pass pass | 14577 | 7 | within |
| rag-stem-rules | 3/3 | pass pass pass | 13215 | 5 | within |
| rag-usage-rotation | 3/3 | pass pass pass | 30104 | 6 | within |

tier pass rate: 35/36

## multi-hop

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-chain-model-mismatch | 3/3 | pass pass pass | 25363 | 10 | within |
| rag-chain-pin-drift | 3/3 | pass pass pass | 27072 | 8 | within |
| rag-chain-redirect | 3/3 | pass pass pass | 21523 | 4 | within |
| rag-chain-tokenizer | 3/3 | pass pass pass | 13638 | 5 | within |
| rag-chain-wiki-token | 3/3 | pass pass pass | 15790 | 4 | within |
| rag-hop-grep-baseline | 3/3 | pass pass pass | 10676 | 6 | within |
| rag-hop-patternshape | 3/3 | pass pass pass | 10875 | 5 | within |
| rag-hop-stem-sides | 3/3 | pass pass pass | 10434 | 6 | within |
| rag-hop-truncate | 3/3 | pass pass pass | 16923 | 10 | within |

tier pass rate: 27/27

## not-here

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-nh-grpc | 3/3 | pass pass pass | 4461 | 8 | within |
| rag-nh-rankdef | 3/3 | pass pass pass | 3668 | 5 | within |
| rag-nh-redis | 0/3 | fail fail fail | 3068 | 5 | within |

tier pass rate: 6/9

## out-of-reach

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-oor-adoption | 3/3 | pass pass pass | 15279 | 7 | within |
| rag-oor-latency | 3/3 | pass pass pass | 2985 | 4 | within |

tier pass rate: 6/6

## budget-pressure

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-bp-change-flow | 1/3 | pass fail fail | 22978 | 11 | over_silent,over_stated |
| rag-bp-hybrid-path | 2/3 | pass pass fail | 51630 | 15 | over_stated |

tier pass rate: 3/6

## db-nav

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-db-cross-ds | 3/3 | pass pass pass | 9087 | 8 | within |
| rag-db-dangling | 3/3 | pass pass pass | 13233 | 7 | within |
| rag-db-impact | 0/3 | fail fail fail | 12809 | 6 | within |
| rag-db-nh-payments | 3/3 | pass pass pass | 5907 | 7 | within |
| rag-db-oor-rows | 3/3 | pass pass pass | 5914 | 3 | within |
| rag-db-rls-off | 3/3 | pass pass pass | 9070 | 4 | within |
| rag-db-trigger-fn | 3/3 | pass pass pass | 8276 | 4 | within |
| rag-db-unenforced | 3/3 | pass pass pass | 19713 | 8 | within |

tier pass rate: 21/24

## behavior

- left the snapshot (scored `escaped`, never pass): 0 run(s)
- invented a node id the server did not have (scored `invented`, never pass): 0 run(s)
- invented node ids: 0 run(s)
- fake citations: 0 run(s)
- keyword hints seen 43, followed by search_keyword next 8
- first retriever: hybrid 19 · keyword 57 · none 32
- search_keyword calls 106: empty 5 · hits without a node id 3
- after a keyword call, first consuming move (≤3 calls): keyword 16 · hybrid 30 · read_code 6 · read_code_other 2 · Read 27 · grep 15 · none 10
- cost self-report ratio (reported/actual, median): 0.88 over 105 run(s)
