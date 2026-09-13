# eval-rag — rubric 1.4.3 · arm graphin

corpus 1816b4e · graphin 1816b4e (dirty) · model sonnet · runs 3 · lexical-only
agent c11b4614ad78 · skill 30a7f105c3bc · taskset edd99ac9013e · cli 2.1.270 (Claude Code)

## answered

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-fk-edges | 2/3 | fail pass pass | 11762 | 10 | within |
| rag-hint-conditions | 3/3 | pass pass pass | 14255 | 10 | within |
| rag-lit-arity | 3/3 | pass pass pass | 13517 | 5 | within |
| rag-lit-debounce | 3/3 | pass pass pass | 2811 | 5 | within |
| rag-lit-stoplist | 3/3 | pass pass pass | 7979 | 6 | within |
| rag-lit-tracks | 3/3 | pass pass pass | 17931 | 10 | within |
| rag-lock-steal | 3/3 | pass pass pass | 9391 | 11 | within |
| rag-md-section-id | 3/3 | pass pass pass | 13358 | 10 | within |
| rag-read-omission | 0/3 | fail fail fail | 0 | 0 | within |
| rag-semantic-gate | 3/3 | pass pass pass | 21055 | 8 | within |
| rag-stem-rules | 3/3 | pass pass pass | 19589 | 16 | within |
| rag-usage-rotation | 3/3 | pass pass pass | 22684 | 14 | over_stated,within |

tier pass rate: 32/36

## multi-hop

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-chain-model-mismatch | 2/3 | pass fail pass | 16765 | 13 | within |
| rag-chain-pin-drift | 1/3 | pass fail fail | 20321 | 12 | within |
| rag-chain-redirect | 3/3 | pass pass pass | 14837 | 6 | within |
| rag-chain-tokenizer | 3/3 | pass pass pass | 10531 | 10 | within |
| rag-chain-wiki-token | 3/3 | pass pass pass | 33088 | 16 | within |
| rag-hop-grep-baseline | 3/3 | pass pass pass | 11831 | 9 | within |
| rag-hop-patternshape | 2/3 | pass pass fail | 15184 | 11 | within |
| rag-hop-stem-sides | 3/3 | pass pass pass | 12616 | 6 | within |
| rag-hop-truncate | 3/3 | pass pass pass | 16492 | 9 | within |

tier pass rate: 23/27

## not-here

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-nh-grpc | 3/3 | pass pass pass | 5388 | 7 | within |
| rag-nh-rankdef | 3/3 | pass pass pass | 5219 | 8 | within |
| rag-nh-redis | 3/3 | pass pass pass | 10012 | 7 | within |

tier pass rate: 9/9

## out-of-reach

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-oor-adoption | 3/3 | pass pass pass | 20807 | 11 | within |
| rag-oor-latency | 3/3 | pass pass pass | 2512 | 8 | within |

tier pass rate: 6/6

## budget-pressure

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-bp-change-flow | 2/3 | pass pass fail | 18966 | 13 | within |
| rag-bp-hybrid-path | 3/3 | pass pass pass | 23688 | 12 | over_stated,within |

tier pass rate: 5/6

## db-nav

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-db-cross-ds | 3/3 | pass pass pass | 9049 | 9 | within |
| rag-db-dangling | 2/3 | pass fail pass | 16763 | 14 | within |
| rag-db-impact | 3/3 | pass pass pass | 15629 | 13 | within |
| rag-db-nh-payments | 3/3 | pass pass pass | 12145 | 9 | within |
| rag-db-oor-rows | 3/3 | pass pass pass | 5818 | 5 | within |
| rag-db-rls-off | 3/3 | pass pass pass | 7445 | 7 | within |
| rag-db-trigger-fn | 1/3 | fail fail pass | 11602 | 7 | within |
| rag-db-unenforced | 3/3 | pass pass pass | 12607 | 10 | within |

tier pass rate: 21/24

## behavior

- left the snapshot (scored `escaped`, never pass): 0 run(s)
- invented a node id the server did not have (scored `invented`, never pass): 0 run(s)
- invented node ids: 3 run(s) — ['rag-stem-rules', 'rag-chain-pin-drift', 'rag-chain-wiki-token']
- fake citations: 1 run(s) — [('rag-bp-change-flow', ['internal/watch/debouncer.go'])]
- keyword hints seen 99, followed by search_keyword next 46
- first retriever: hybrid 73 · keyword 30 · none 5
- search_keyword calls 334: empty 52 · hits without a node id 36
- after a keyword call, first consuming move (≤3 calls): keyword 145 · hybrid 59 · read_code 37 · read_code_other 30 · explore 8 · Read 2 · bash_read 2 · none 51
- cost self-report ratio (reported/actual, median): 0.99 over 100 run(s)
