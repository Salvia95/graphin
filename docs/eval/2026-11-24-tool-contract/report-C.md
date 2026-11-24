# eval-rag — rubric 1.5.0 · arm bare

corpus 6b750fb · graphin 6b750fb · model claude-sonnet-5-5 · runs 2 · lexical-only
agent 14c25158c013 · skill - · taskset edd99ac9013e · cli 2.1.331 (Claude Code)
asked claude-sonnet-5-5 · effort high · answered by claude-sonnet-5-5

## answered

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-fk-edges | 1/2 | pass fail | 34216 | 8 | within |
| rag-hint-conditions | 2/2 | pass pass | 28530 | 6 | within |
| rag-lit-arity | 2/2 | pass pass | 10257 | 6 | within |
| rag-lit-debounce | 2/2 | pass pass | 6059 | 2 | within |
| rag-lit-stoplist | 1/2 | pass fail | 9648 | 3 | within |
| rag-lit-tracks | 2/2 | pass pass | 23016 | 4 | within |
| rag-lock-steal | 2/2 | pass pass | 29335 | 8 | within |
| rag-md-section-id | 1/2 | fail pass | 14327 | 4 | within |
| rag-read-omission | 2/2 | pass pass | 15624 | 6 | within |
| rag-semantic-gate | 2/2 | pass pass | 14870 | 6 | within |
| rag-stem-rules | 1/2 | fail pass | 26741 | 7 | within |
| rag-usage-rotation | 2/2 | pass pass | 31683 | 5 | within |

tier pass rate: 20/24

## multi-hop

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-chain-model-mismatch | 0/2 | fail fail | 28077 | 8 | within |
| rag-chain-pin-drift | 1/2 | pass fail | 29200 | 6 | within |
| rag-chain-redirect | 2/2 | pass pass | 21260 | 4 | within |
| rag-chain-tokenizer | 2/2 | pass pass | 19689 | 3 | within |
| rag-chain-wiki-token | 2/2 | pass pass | 20803 | 4 | within |
| rag-hop-grep-baseline | 2/2 | pass pass | 19628 | 7 | within |
| rag-hop-patternshape | 2/2 | pass pass | 16122 | 3 | within |
| rag-hop-stem-sides | 1/2 | fail pass | 25961 | 7 | within |
| rag-hop-truncate | 2/2 | pass pass | 22152 | 6 | within |

tier pass rate: 14/18

## not-here

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-nh-grpc | 2/2 | pass pass | 2095 | 8 | within |
| rag-nh-rankdef | 2/2 | pass pass | 7246 | 4 | within |
| rag-nh-redis | 0/2 | fail fail | 21324 | 4 | within |

tier pass rate: 4/6

## out-of-reach

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-oor-adoption | 2/2 | pass pass | 21344 | 6 | within |
| rag-oor-latency | 2/2 | pass pass | 3464 | 2 | within |

tier pass rate: 4/4

## budget-pressure

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-bp-change-flow | 0/2 | fail fail | 30392 | 14 | over_silent |
| rag-bp-hybrid-path | 1/2 | pass fail | 34782 | 9 | over_stated |

tier pass rate: 1/4

## db-nav

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-db-cross-ds | 2/2 | pass pass | 14670 | 5 | within |
| rag-db-dangling | 2/2 | pass pass | 11367 | 6 | within |
| rag-db-impact | 1/2 | fail pass | 15603 | 7 | within |
| rag-db-nh-payments | 2/2 | pass pass | 5714 | 8 | within |
| rag-db-oor-rows | 2/2 | pass pass | 16158 | 3 | within |
| rag-db-rls-off | 2/2 | pass pass | 13326 | 6 | within |
| rag-db-trigger-fn | 2/2 | pass pass | 24082 | 6 | within |
| rag-db-unenforced | 1/2 | fail pass | 25424 | 7 | within |

tier pass rate: 14/16

## behavior

- left the snapshot (scored `escaped`, never pass): 0 run(s)
- invented a node id the server did not have (scored `invented`, never pass): 0 run(s)
- invented node ids: 0 run(s)
- fake citations: 2 run(s) — [('rag-md-section-id', ['docs/a.md']), ('rag-db-unenforced', ['internal/graph/dbedges.go'])]
- keyword hints seen 35, followed by search_keyword next 17
- first retriever: hybrid 27 · keyword 42 · none 3
- search_keyword calls 105: empty 11 · hits without a node id 3
- after a keyword call, first consuming move (≤3 calls): keyword 29 · hybrid 22 · read_code 3 · read_code_other 6 · explore 3 · Read 18 · grep 13 · none 11
- cost self-report ratio (reported/actual, median): 0.89 over 72 run(s)
