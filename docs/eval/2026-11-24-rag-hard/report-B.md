# eval-rag — rubric 1.5.0 · arm bare

corpus 43aa2ec · graphin 43aa2ec · model claude-sonnet-5-5 · runs 3 · lexical-only
agent 14c25158c013 · skill - · taskset 02fdf2e4d399 · cli 2.1.331 (Claude Code)
asked claude-sonnet-5-5 · effort high · answered by claude-sonnet-5-5

## multi-hop

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rh-auto-bootstrap | 2/3 | fail pass pass | 38445 | 11 | over_silent,within |
| rh-chunk-hitline | 3/3 | pass pass pass | 36723 | 15 | within |
| rh-follower-route | 3/3 | pass pass pass | 13131 | 4 | within |
| rh-impact-nodeatline | 2/3 | pass pass fail | 17310 | 16 | within |
| rh-import-retarget | 3/3 | pass pass pass | 34928 | 11 | within |
| rh-scope-exclude | 3/3 | pass pass pass | 20502 | 10 | within |
| rh-ts-alias | 2/3 | pass pass fail | 24306 | 7 | within |
| rh-usage-adoption | 0/3 | fail fail fail | 41315 | 12 | over_silent,within |

tier pass rate: 18/24

## db-nav

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rh-db-code-access | 3/3 | pass pass pass | 20932 | 10 | within |
| rh-db-company-delete | 2/3 | fail pass pass | 10736 | 8 | within |
| rh-db-drop-status | 0/3 | fail fail fail | 9512 | 6 | within |
| rh-db-ssot-users | 3/3 | pass pass pass | 6997 | 8 | within |

tier pass rate: 8/12

## behavior

- left the snapshot (scored `escaped`, never pass): 0 run(s)
- invented a node id the server did not have (scored `invented`, never pass): 0 run(s)
- invented node ids: 0 run(s)
- fake citations: 0 run(s)
- keyword hints seen 17, followed by search_keyword next 7
- first retriever: hybrid 7 · keyword 18 · none 11
- search_keyword calls 68: empty 0 · hits without a node id 4
- after a keyword call, first consuming move (≤3 calls): keyword 21 · hybrid 8 · read_code 3 · read_code_other 11 · explore 3 · Read 14 · grep 5 · none 3
- cost self-report ratio (reported/actual, median): 0.77 over 36 run(s)
