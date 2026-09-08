# eval-rag — rubric 1.4.2 · arm graphin

corpus b564780 · graphin b564780 · model sonnet · runs 3 · lexical-only
agent c11b4614ad78 · skill 1fa366d41424 · taskset edd99ac9013e · cli 2.1.263 (Claude Code)

## answered

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-fk-edges | 3/3 | pass pass pass | 13004 | 14 | within |
| rag-hint-conditions | 3/3 | pass pass pass | 21775 | 12 | within |
| rag-lit-arity | 3/3 | pass pass pass | 8220 | 5 | within |
| rag-lit-debounce | 3/3 | pass pass pass | 4309 | 7 | within |
| rag-lit-stoplist | 3/3 | pass pass pass | 11338 | 6 | within |
| rag-lit-tracks | 3/3 | pass pass pass | 19360 | 8 | within |
| rag-lock-steal | 3/3 | pass pass pass | 6961 | 8 | within |
| rag-md-section-id | 3/3 | pass pass pass | 9518 | 9 | within |
| rag-read-omission | 0/3 | fail fail fail | 0 | 0 | within |
| rag-semantic-gate | 3/3 | pass pass pass | 17931 | 7 | within |
| rag-stem-rules | 3/3 | pass pass pass | 15235 | 20 | within |
| rag-usage-rotation | 2/3 | invented pass pass | 24105 | 12 | within |

tier pass rate: 32/36

## multi-hop

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-chain-model-mismatch | 2/3 | pass pass fail | 15963 | 15 | within |
| rag-chain-pin-drift | 1/3 | fail fail pass | 11015 | 8 | within |
| rag-chain-redirect | 3/3 | pass pass pass | 11347 | 6 | within |
| rag-chain-tokenizer | 3/3 | pass pass pass | 13721 | 12 | within |
| rag-chain-wiki-token | 3/3 | pass pass pass | 30629 | 14 | within |
| rag-hop-grep-baseline | 3/3 | pass pass pass | 13591 | 10 | within |
| rag-hop-patternshape | 1/3 | fail pass fail | 7627 | 6 | within |
| rag-hop-stem-sides | 3/3 | pass pass pass | 13947 | 9 | within |
| rag-hop-truncate | 3/3 | pass pass pass | 18065 | 9 | within |

tier pass rate: 22/27

## not-here

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-nh-grpc | 3/3 | pass pass pass | 6120 | 7 | within |
| rag-nh-rankdef | 3/3 | pass pass pass | 5176 | 7 | within |
| rag-nh-redis | 2/3 | pass pass fail | 9720 | 8 | within |

tier pass rate: 8/9

## out-of-reach

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-oor-adoption | 3/3 | pass pass pass | 14324 | 7 | within |
| rag-oor-latency | 3/3 | pass pass pass | 3538 | 12 | within |

tier pass rate: 6/6

## budget-pressure

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-bp-change-flow | 3/3 | pass pass pass | 18084 | 16 | over_stated,within |
| rag-bp-hybrid-path | 3/3 | pass pass pass | 16175 | 12 | over_stated,within |

tier pass rate: 6/6

## db-nav

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-db-cross-ds | 3/3 | pass pass pass | 8827 | 9 | within |
| rag-db-dangling | 3/3 | pass pass pass | 14231 | 15 | within |
| rag-db-impact | 3/3 | pass pass pass | 11973 | 16 | within |
| rag-db-nh-payments | 3/3 | pass pass pass | 7871 | 12 | within |
| rag-db-oor-rows | 3/3 | pass pass pass | 3416 | 4 | within |
| rag-db-rls-off | 3/3 | pass pass pass | 18643 | 13 | within |
| rag-db-trigger-fn | 3/3 | pass pass pass | 13577 | 9 | within |
| rag-db-unenforced | 3/3 | pass pass pass | 22717 | 12 | within |

tier pass rate: 24/24

## behavior

- left the snapshot (scored `escaped`, never pass): 0 run(s)
- invented a node id the server did not have (scored `invented`, never pass): 1 run(s) — [('rag-usage-rotation', ['internal.usage.ingest.go'])]
- invented node ids: 4 run(s) — ['rag-stem-rules', 'rag-usage-rotation', 'rag-chain-tokenizer', 'rag-chain-wiki-token']
- fake citations: 0 run(s)
- keyword hints seen 95, followed by search_keyword next 48
- first retriever: hybrid 78 · keyword 26 · none 4
- search_keyword calls 353: empty 60 · hits without a node id 38
- after a keyword call, first consuming move (≤3 calls): keyword 165 · hybrid 64 · read_code 41 · read_code_other 26 · explore 9 · Read 1 · bash_read 1 · none 46
- cost self-report ratio (reported/actual, median): 0.99 over 101 run(s)
