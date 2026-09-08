# eval-rag — rubric 1.4.2 · arm grep

corpus b564780 · graphin 5e15147 · model sonnet · runs 3 · lexical-only
agent 237ea8f43410 · skill - · taskset edd99ac9013e · cli 2.1.263 (Claude Code)

## answered

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-fk-edges | 0/3 | fail fail fail | 52788 | 14 | over_silent |
| rag-hint-conditions | 2/3 | pass pass fail | 18904 | 5 | within |
| rag-lit-arity | 3/3 | pass pass pass | 5894 | 7 | within |
| rag-lit-debounce | 3/3 | pass pass pass | 3007 | 2 | within |
| rag-lit-stoplist | 3/3 | pass pass pass | 10282 | 7 | within |
| rag-lit-tracks | 3/3 | pass pass pass | 11471 | 10 | within |
| rag-lock-steal | 3/3 | pass pass pass | 19076 | 9 | within |
| rag-md-section-id | 2/3 | pass fail pass | 16736 | 9 | within |
| rag-read-omission | 2/3 | pass fail pass | 20801 | 7 | within |
| rag-semantic-gate | 3/3 | pass pass pass | 18026 | 13 | within |
| rag-stem-rules | 3/3 | pass pass pass | 18256 | 6 | within |
| rag-usage-rotation | 2/3 | pass fail pass | 52319 | 12 | over_silent,over_stated |

tier pass rate: 29/36

## multi-hop

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-chain-model-mismatch | 3/3 | pass pass pass | 25430 | 12 | within |
| rag-chain-pin-drift | 2/3 | fail pass pass | 24299 | 9 | within |
| rag-chain-redirect | 3/3 | pass pass pass | 21329 | 10 | within |
| rag-chain-tokenizer | 3/3 | pass pass pass | 11953 | 7 | within |
| rag-chain-wiki-token | 3/3 | pass pass pass | 22220 | 5 | over_stated,within |
| rag-hop-grep-baseline | 3/3 | pass pass pass | 13219 | 10 | within |
| rag-hop-patternshape | 2/3 | pass fail pass | 10658 | 8 | within |
| rag-hop-stem-sides | 3/3 | pass pass pass | 17920 | 10 | within |
| rag-hop-truncate | 3/3 | pass pass pass | 18410 | 16 | within |

tier pass rate: 25/27

## not-here

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-nh-grpc | 3/3 | pass pass pass | 4364 | 7 | within |
| rag-nh-rankdef | 3/3 | pass pass pass | 10182 | 10 | within |
| rag-nh-redis | 2/3 | pass pass fail | 5256 | 6 | within |

tier pass rate: 8/9

## out-of-reach

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-oor-adoption | 3/3 | pass pass pass | 29262 | 9 | within |
| rag-oor-latency | 3/3 | pass pass pass | 6747 | 6 | within |

tier pass rate: 6/6

## budget-pressure

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-bp-change-flow | 3/3 | pass pass pass | 44202 | 29 | over_stated |
| rag-bp-hybrid-path | 3/3 | pass pass pass | 43673 | 18 | over_stated |

tier pass rate: 6/6

## db-nav

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-db-cross-ds | 3/3 | pass pass pass | 19902 | 13 | within |
| rag-db-dangling | 3/3 | pass pass pass | 16380 | 15 | within |
| rag-db-impact | 3/3 | pass pass pass | 21027 | 12 | within |
| rag-db-nh-payments | 3/3 | pass pass pass | 9745 | 9 | within |
| rag-db-oor-rows | 3/3 | pass pass pass | 16141 | 4 | within |
| rag-db-rls-off | 3/3 | pass pass pass | 4504 | 6 | within |
| rag-db-trigger-fn | 3/3 | pass pass pass | 4508 | 4 | within |
| rag-db-unenforced | 3/3 | pass pass pass | 27847 | 13 | within |

tier pass rate: 24/24

## behavior

- left the snapshot (scored `escaped`, never pass): 0 run(s)
- invented a node id the server did not have (scored `invented`, never pass): 0 run(s)
- invented node ids: 0 run(s)
- calls the containment hook denied: 5 in 5 run(s)
- fake citations: 3 run(s) — [('rag-read-omission', ['internal/mcp/tools/tools_test.go']), ('rag-md-section-id', ['docs/foo.md']), ('rag-hint-conditions', ['internal/mcp/tools/tools_test.go'])]
- keyword hints seen 12, followed by search_keyword next 0
- first retriever: hybrid 0 · keyword 0 · none 108
- cost self-report ratio (reported/actual, median): 0.68 over 108 run(s)
