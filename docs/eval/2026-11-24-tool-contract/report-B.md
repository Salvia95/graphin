# eval-rag — rubric 1.5.0 · arm bare

corpus 6b750fb · graphin 6b750fb · model claude-sonnet-5-5 · runs 2 · lexical-only
agent 14c25158c013 · skill - · taskset edd99ac9013e · cli 2.1.331 (Claude Code)
asked claude-sonnet-5-5 · effort high · answered by claude-sonnet-5-5

## answered

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-fk-edges | 2/2 | pass pass | 16168 | 8 | within |
| rag-hint-conditions | 2/2 | pass pass | 25362 | 6 | within |
| rag-lit-arity | 2/2 | pass pass | 11912 | 6 | within |
| rag-lit-debounce | 2/2 | pass pass | 6059 | 2 | within |
| rag-lit-stoplist | 2/2 | pass pass | 10836 | 4 | within |
| rag-lit-tracks | 2/2 | pass pass | 17853 | 6 | within |
| rag-lock-steal | 2/2 | pass pass | 18214 | 6 | within |
| rag-md-section-id | 1/2 | fail pass | 13731 | 3 | within |
| rag-read-omission | 2/2 | pass pass | 15046 | 5 | within |
| rag-semantic-gate | 2/2 | pass pass | 11572 | 6 | within |
| rag-stem-rules | 2/2 | pass pass | 17872 | 5 | within |
| rag-usage-rotation | 2/2 | pass pass | 44460 | 8 | over_stated |

tier pass rate: 23/24

## multi-hop

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-chain-model-mismatch | 2/2 | pass pass | 22371 | 12 | within |
| rag-chain-pin-drift | 2/2 | pass pass | 23595 | 6 | within |
| rag-chain-redirect | 2/2 | pass pass | 22962 | 5 | within |
| rag-chain-tokenizer | 2/2 | pass pass | 14540 | 5 | within |
| rag-chain-wiki-token | 2/2 | pass pass | 23194 | 5 | within |
| rag-hop-grep-baseline | 2/2 | pass pass | 13466 | 8 | within |
| rag-hop-patternshape | 2/2 | pass pass | 11596 | 5 | within |
| rag-hop-stem-sides | 2/2 | pass pass | 20627 | 6 | within |
| rag-hop-truncate | 2/2 | pass pass | 26304 | 7 | within |

tier pass rate: 18/18

## not-here

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-nh-grpc | 2/2 | pass pass | 3074 | 8 | within |
| rag-nh-rankdef | 2/2 | pass pass | 3066 | 4 | within |
| rag-nh-redis | 0/2 | fail fail | 8508 | 4 | within |

tier pass rate: 4/6

## out-of-reach

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-oor-adoption | 2/2 | pass pass | 15178 | 8 | within |
| rag-oor-latency | 2/2 | pass pass | 2192 | 4 | within |

tier pass rate: 4/4

## budget-pressure

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-bp-change-flow | 1/2 | fail pass | 28511 | 13 | over_silent,over_stated |
| rag-bp-hybrid-path | 1/2 | fail pass | 39332 | 11 | over_silent,over_stated |

tier pass rate: 2/4

## db-nav

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-db-cross-ds | 2/2 | pass pass | 11967 | 7 | within |
| rag-db-dangling | 1/2 | pass fail | 10148 | 8 | within |
| rag-db-impact | 1/2 | pass fail | 11262 | 7 | within |
| rag-db-nh-payments | 2/2 | pass pass | 6540 | 6 | within |
| rag-db-oor-rows | 2/2 | pass pass | 9895 | 4 | within |
| rag-db-rls-off | 2/2 | pass pass | 9137 | 5 | within |
| rag-db-trigger-fn | 2/2 | pass pass | 17519 | 5 | within |
| rag-db-unenforced | 2/2 | pass pass | 16744 | 9 | within |

tier pass rate: 14/16

## behavior

- left the snapshot (scored `escaped`, never pass): 0 run(s)
- invented a node id the server did not have (scored `invented`, never pass): 0 run(s)
- invented node ids: 0 run(s)
- fake citations: 1 run(s) — [('rag-md-section-id', ['docs/a.md'])]
- keyword hints seen 42, followed by search_keyword next 15
- first retriever: hybrid 28 · keyword 40 · none 4
- search_keyword calls 86: empty 10 · hits without a node id 2
- after a keyword call, first consuming move (≤3 calls): keyword 15 · hybrid 19 · read_code 4 · read_code_other 6 · explore 1 · Read 22 · grep 14 · none 5
- cost self-report ratio (reported/actual, median): 0.75 over 71 run(s)
