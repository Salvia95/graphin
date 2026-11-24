# eval-rag — rubric 1.5.0 · arm bare

corpus 6b750fb · graphin 6b750fb · model claude-sonnet-5-5 · runs 2 · lexical-only
agent 14c25158c013 · skill - · taskset edd99ac9013e · cli 2.1.331 (Claude Code)
asked claude-sonnet-5-5 · effort high · answered by claude-sonnet-5-5

## answered

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-fk-edges | 2/2 | pass pass | 17503 | 8 | within |
| rag-hint-conditions | 2/2 | pass pass | 22827 | 6 | within |
| rag-lit-arity | 2/2 | pass pass | 6653 | 7 | within |
| rag-lit-debounce | 2/2 | pass pass | 4077 | 2 | within |
| rag-lit-stoplist | 2/2 | pass pass | 8685 | 4 | within |
| rag-lit-tracks | 2/2 | pass pass | 21306 | 8 | within |
| rag-lock-steal | 2/2 | pass pass | 19016 | 6 | within |
| rag-md-section-id | 2/2 | pass pass | 18866 | 4 | within |
| rag-read-omission | 2/2 | pass pass | 10248 | 5 | within |
| rag-semantic-gate | 2/2 | pass pass | 16865 | 7 | within |
| rag-stem-rules | 1/2 | pass fail | 15130 | 5 | within |
| rag-usage-rotation | 2/2 | pass pass | 25546 | 5 | within |

tier pass rate: 23/24

## multi-hop

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-chain-model-mismatch | 1/2 | pass fail | 32446 | 14 | within |
| rag-chain-pin-drift | 1/2 | pass fail | 21281 | 6 | within |
| rag-chain-redirect | 2/2 | pass pass | 21070 | 4 | within |
| rag-chain-tokenizer | 2/2 | pass pass | 13982 | 5 | within |
| rag-chain-wiki-token | 2/2 | pass pass | 16742 | 4 | within |
| rag-hop-grep-baseline | 2/2 | pass pass | 13726 | 7 | within |
| rag-hop-patternshape | 1/2 | pass fail | 11014 | 6 | within |
| rag-hop-stem-sides | 2/2 | pass pass | 13502 | 6 | within |
| rag-hop-truncate | 1/2 | pass fail | 16532 | 10 | within |

tier pass rate: 14/18

## not-here

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-nh-grpc | 2/2 | pass pass | 5370 | 9 | within |
| rag-nh-rankdef | 2/2 | pass pass | 2736 | 5 | within |
| rag-nh-redis | 0/2 | fail fail | 6048 | 4 | within |

tier pass rate: 4/6

## out-of-reach

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-oor-adoption | 2/2 | pass pass | 11895 | 8 | within |
| rag-oor-latency | 2/2 | pass pass | 1264 | 3 | within |

tier pass rate: 4/4

## budget-pressure

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-bp-change-flow | 1/2 | pass fail | 25046 | 12 | over_silent,over_stated |
| rag-bp-hybrid-path | 2/2 | pass pass | 42456 | 13 | over_stated |

tier pass rate: 3/4

## db-nav

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-db-cross-ds | 2/2 | pass pass | 7125 | 4 | within |
| rag-db-dangling | 1/2 | pass fail | 12622 | 6 | within |
| rag-db-impact | 0/2 | fail fail | 9572 | 6 | within |
| rag-db-nh-payments | 2/2 | pass pass | 3798 | 6 | within |
| rag-db-oor-rows | 2/2 | pass pass | 3670 | 3 | within |
| rag-db-rls-off | 2/2 | pass pass | 13138 | 7 | within |
| rag-db-trigger-fn | 2/2 | pass pass | 5694 | 5 | within |
| rag-db-unenforced | 2/2 | pass pass | 20390 | 10 | within |

tier pass rate: 13/16

## behavior

- left the snapshot (scored `escaped`, never pass): 0 run(s)
- invented a node id the server did not have (scored `invented`, never pass): 0 run(s)
- invented node ids: 0 run(s)
- fake citations: 0 run(s)
- keyword hints seen 26, followed by search_keyword next 9
- first retriever: hybrid 11 · keyword 34 · none 27
- search_keyword calls 63: empty 1 · hits without a node id 5
- after a keyword call, first consuming move (≤3 calls): keyword 8 · hybrid 17 · read_code_other 8 · Read 12 · grep 9 · bash_read 3 · none 6
- cost self-report ratio (reported/actual, median): 0.93 over 71 run(s)
