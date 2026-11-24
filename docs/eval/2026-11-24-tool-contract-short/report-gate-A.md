# eval-rag — rubric 1.5.0 · arm bare

corpus 342d29c · graphin 342d29c · model claude-sonnet-5-5 · runs 3 · lexical-only
agent 14c25158c013 · skill - · taskset edd99ac9013e · cli 2.1.331 (Claude Code)
asked claude-sonnet-5-5 · effort high · answered by claude-sonnet-5-5

## answered

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-fk-edges | 3/3 | pass pass pass | 18044 | 8 | within |
| rag-hint-conditions | 3/3 | pass pass pass | 24403 | 6 | within |
| rag-lit-arity | 3/3 | pass pass pass | 6405 | 7 | within |
| rag-lit-debounce | 3/3 | pass pass pass | 4603 | 2 | within |
| rag-lit-stoplist | 3/3 | pass pass pass | 8985 | 5 | within |
| rag-lit-tracks | 3/3 | pass pass pass | 20912 | 5 | within |
| rag-lock-steal | 3/3 | pass pass pass | 20934 | 6 | within |
| rag-md-section-id | 3/3 | pass pass pass | 21558 | 3 | within |
| rag-read-omission | 3/3 | pass pass pass | 8040 | 3 | within |
| rag-semantic-gate | 3/3 | pass pass pass | 16867 | 5 | within |
| rag-stem-rules | 3/3 | pass pass pass | 17406 | 6 | within |
| rag-usage-rotation | 3/3 | pass pass pass | 16788 | 7 | within |

tier pass rate: 36/36

## multi-hop

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-chain-model-mismatch | 3/3 | pass pass pass | 21808 | 12 | within |
| rag-chain-pin-drift | 2/3 | pass fail pass | 27781 | 7 | over_stated,within |
| rag-chain-redirect | 3/3 | pass pass pass | 17219 | 5 | within |
| rag-chain-tokenizer | 3/3 | pass pass pass | 13320 | 5 | within |
| rag-chain-wiki-token | 2/3 | pass fail pass | 24439 | 5 | within |
| rag-hop-grep-baseline | 3/3 | pass pass pass | 11648 | 7 | within |
| rag-hop-patternshape | 3/3 | pass pass pass | 9804 | 5 | within |
| rag-hop-stem-sides | 3/3 | pass pass pass | 7820 | 6 | within |
| rag-hop-truncate | 2/3 | pass fail pass | 16213 | 5 | within |

tier pass rate: 24/27

## not-here

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-nh-grpc | 3/3 | pass pass pass | 3088 | 8 | within |
| rag-nh-rankdef | 3/3 | pass pass pass | 2861 | 5 | within |
| rag-nh-redis | 1/3 | fail fail pass | 2719 | 4 | within |

tier pass rate: 7/9

## out-of-reach

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-oor-adoption | 3/3 | pass pass pass | 14759 | 6 | within |
| rag-oor-latency | 3/3 | pass pass pass | 2859 | 4 | within |

tier pass rate: 6/6

## budget-pressure

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-bp-change-flow | 0/3 | fail fail fail | 28422 | 10 | over_silent |
| rag-bp-hybrid-path | 1/3 | fail pass fail | 49091 | 11 | over_stated |

tier pass rate: 1/6

## db-nav

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-db-cross-ds | 3/3 | pass pass pass | 6475 | 3 | within |
| rag-db-dangling | 3/3 | pass pass pass | 12482 | 9 | within |
| rag-db-impact | 2/3 | pass pass fail | 11767 | 5 | within |
| rag-db-nh-payments | 3/3 | pass pass pass | 3764 | 6 | within |
| rag-db-oor-rows | 3/3 | pass pass pass | 5811 | 3 | within |
| rag-db-rls-off | 3/3 | pass pass pass | 8544 | 5 | within |
| rag-db-trigger-fn | 3/3 | pass pass pass | 6003 | 5 | within |
| rag-db-unenforced | 3/3 | pass pass pass | 11205 | 6 | within |

tier pass rate: 23/24

## behavior

- left the snapshot (scored `escaped`, never pass): 0 run(s)
- invented a node id the server did not have (scored `invented`, never pass): 0 run(s)
- invented node ids: 0 run(s)
- fake citations: 0 run(s)
- keyword hints seen 35, followed by search_keyword next 8
- first retriever: hybrid 20 · keyword 48 · none 40
- search_keyword calls 95: empty 6 · hits without a node id 5
- after a keyword call, first consuming move (≤3 calls): keyword 18 · hybrid 21 · read_code 7 · read_code_other 5 · Read 17 · grep 19 · none 8
- cost self-report ratio (reported/actual, median): 0.92 over 105 run(s)
