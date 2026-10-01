# eval-rag — rubric 1.5.0 · arm graphin

corpus c98cca8 · graphin c98cca8 · model claude-sonnet-5-5 · runs 3 · lexical-only
agent 79c69464daff · skill 30a7f105c3bc · taskset edd99ac9013e · cli 2.1.286 (Claude Code)
asked claude-sonnet-5-5 · effort high · answered by claude-sonnet-5-5

## answered

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-fk-edges | 3/3 | pass pass pass | 9472 | 7 | within |
| rag-hint-conditions | 3/3 | pass pass pass | 12700 | 6 | within |
| rag-lit-arity | 3/3 | pass pass pass | 11297 | 4 | within |
| rag-lit-debounce | 3/3 | pass pass pass | 3653 | 5 | within |
| rag-lit-stoplist | 3/3 | pass pass pass | 7520 | 3 | within |
| rag-lit-tracks | 3/3 | pass pass pass | 13950 | 8 | within |
| rag-lock-steal | 3/3 | pass pass pass | 13029 | 7 | within |
| rag-md-section-id | 2/3 | pass fail pass | 8286 | 5 | within |
| rag-read-omission | 3/3 | pass pass pass | 8722 | 5 | within |
| rag-semantic-gate | 3/3 | pass pass pass | 17872 | 7 | within |
| rag-stem-rules | 2/3 | pass invented pass | 11887 | 6 | within |
| rag-usage-rotation | 3/3 | pass pass pass | 16614 | 7 | within |

tier pass rate: 34/36

## multi-hop

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-chain-model-mismatch | 1/3 | pass fail fail | 14259 | 7 | within |
| rag-chain-pin-drift | 1/3 | fail pass fail | 7391 | 6 | within |
| rag-chain-redirect | 3/3 | pass pass pass | 14340 | 5 | within |
| rag-chain-tokenizer | 3/3 | pass pass pass | 11321 | 7 | within |
| rag-chain-wiki-token | 2/3 | fail pass pass | 15501 | 10 | within |
| rag-hop-grep-baseline | 1/3 | pass fail fail | 7386 | 7 | within |
| rag-hop-patternshape | 2/3 | pass pass fail | 12574 | 6 | within |
| rag-hop-stem-sides | 3/3 | pass pass pass | 12337 | 5 | within |
| rag-hop-truncate | 2/3 | fail pass pass | 10794 | 6 | within |

tier pass rate: 18/27

## not-here

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-nh-grpc | 3/3 | pass pass pass | 4044 | 6 | within |
| rag-nh-rankdef | 3/3 | pass pass pass | 2073 | 4 | within |
| rag-nh-redis | 1/3 | pass fail fail | 4642 | 5 | within |

tier pass rate: 7/9

## out-of-reach

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-oor-adoption | 3/3 | pass pass pass | 11382 | 9 | within |
| rag-oor-latency | 2/3 | inconclusive pass pass | 1078 | 4 | within |

tier pass rate: 5/6

## budget-pressure

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-bp-change-flow | 3/3 | pass pass pass | 22270 | 13 | over_stated,within |
| rag-bp-hybrid-path | 3/3 | pass pass pass | 19852 | 9 | over_stated,within |

tier pass rate: 6/6

## db-nav

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-db-cross-ds | 3/3 | pass pass pass | 4361 | 5 | within |
| rag-db-dangling | 2/3 | pass fail pass | 10090 | 9 | within |
| rag-db-impact | 1/3 | fail fail pass | 7884 | 7 | within |
| rag-db-nh-payments | 3/3 | pass pass pass | 10816 | 9 | within |
| rag-db-oor-rows | 3/3 | pass pass pass | 1819 | 4 | within |
| rag-db-rls-off | 3/3 | pass pass pass | 10973 | 6 | within |
| rag-db-trigger-fn | 3/3 | pass pass pass | 12857 | 5 | within |
| rag-db-unenforced | 3/3 | pass pass pass | 7650 | 7 | within |

tier pass rate: 21/24

## behavior

- left the snapshot (scored `escaped`, never pass): 0 run(s)
- invented a node id the server did not have (scored `invented`, never pass): 1 run(s) — [('rag-stem-rules', ['internal.lexical.particles'])]
- invented node ids: 1 run(s) — ['rag-stem-rules']
- fake citations: 1 run(s) — [('rag-md-section-id', ['docs/a.md'])]
- keyword hints seen 50, followed by search_keyword next 22
- first retriever: hybrid 71 · keyword 37 · none 0
- search_keyword calls 244: empty 32 · hits without a node id 33
- after a keyword call, first consuming move (≤3 calls): keyword 91 · hybrid 29 · read_code 43 · read_code_other 17 · explore 4 · Read 7 · grep 2 · bash_read 3 · none 48
- cost self-report ratio (reported/actual, median): 0.99 over 107 run(s)
