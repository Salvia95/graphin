# eval-rag — rubric 1.4.2 · arm graphin

corpus worktree · graphin 6eab151 (dirty) · model sonnet · runs 1 · lexical-only
agent c11b4614ad78 · skill 1fa366d41424 · taskset a9c42218b5cb · cli 2.1.263 (Claude Code)

## answered

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-fk-edges | 1/1 | pass | 16699 | 11 | within |
| rag-hint-conditions | 1/1 | pass | 18601 | 11 | within |
| rag-lit-arity | 1/1 | pass | 7756 | 5 | within |
| rag-lit-debounce | 1/1 | pass | 3167 | 6 | within |
| rag-lit-stoplist | 1/1 | pass | 9310 | 4 | within |
| rag-lit-tracks | 1/1 | pass | 30428 | 13 | within |
| rag-lock-steal | 1/1 | pass | 7518 | 9 | within |
| rag-md-section-id | 1/1 | pass | 10849 | 7 | within |
| rag-read-omission | 0/1 | fail | 0 | 0 | within |
| rag-semantic-gate | 1/1 | pass | 18238 | 8 | within |
| rag-stem-rules | 1/1 | pass | 19430 | 18 | within |
| rag-usage-rotation | 1/1 | pass | 16054 | 13 | within |

tier pass rate: 11/12

## multi-hop

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rag-chain-model-mismatch | 1/1 | pass | 19085 | 13 | within |
| rag-chain-pin-drift | 1/1 | pass | 23425 | 17 | within |
| rag-chain-redirect | 1/1 | pass | 9087 | 8 | within |
| rag-chain-tokenizer | 1/1 | pass | 8649 | 7 | within |
| rag-chain-wiki-token | 1/1 | pass | 26820 | 13 | within |
| rag-hop-grep-baseline | 1/1 | pass | 7464 | 9 | within |
| rag-hop-patternshape | 0/1 | fail | 6362 | 6 | within |
| rag-hop-stem-sides | 1/1 | pass | 15565 | 7 | within |
| rag-hop-truncate | 1/1 | pass | 14124 | 10 | within |

tier pass rate: 8/9

## behavior

- left the snapshot (scored `escaped`, never pass): 0 run(s)
- invented a node id the server did not have (scored `invented`, never pass): 0 run(s)
- invented node ids: 2 run(s) — ['rag-lock-steal', 'rag-chain-pin-drift']
- fake citations: 0 run(s)
- keyword hints seen 13, followed by search_keyword next 8
- first retriever: hybrid 14 · keyword 6 · none 1
- search_keyword calls 76: empty 3 · hits without a node id 14
- after a keyword call, first consuming move (≤3 calls): keyword 33 · hybrid 14 · read_code 8 · read_code_other 8 · explore 1 · Read 1 · none 11
- cost self-report ratio (reported/actual, median): 0.93 over 20 run(s)
