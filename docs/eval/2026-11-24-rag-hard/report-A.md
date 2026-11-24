# eval-rag — rubric 1.5.0 · arm bare

corpus 43aa2ec · graphin 43aa2ec · model claude-sonnet-5-5 · runs 3 · lexical-only
agent 14c25158c013 · skill - · taskset 02fdf2e4d399 · cli 2.1.331 (Claude Code)
asked claude-sonnet-5-5 · effort high · answered by claude-sonnet-5-5

## multi-hop

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rh-auto-bootstrap | 2/3 | pass fail pass | 35409 | 9 | over_silent,within |
| rh-chunk-hitline | 3/3 | pass pass pass | 37926 | 13 | over_stated,within |
| rh-follower-route | 3/3 | pass pass pass | 21420 | 5 | within |
| rh-impact-nodeatline | 2/3 | fail pass pass | 28980 | 13 | within |
| rh-import-retarget | 3/3 | pass pass pass | 26563 | 10 | within |
| rh-scope-exclude | 3/3 | pass pass pass | 29247 | 13 | within |
| rh-ts-alias | 2/3 | pass pass fail | 26941 | 8 | within |
| rh-usage-adoption | 1/3 | fail fail pass | 42377 | 11 | over_silent,within |

tier pass rate: 19/24

## db-nav

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rh-db-code-access | 3/3 | pass pass pass | 24018 | 13 | within |
| rh-db-company-delete | 1/3 | pass fail fail | 11611 | 8 | within |
| rh-db-drop-status | 0/3 | fail fail fail | 19302 | 8 | within |
| rh-db-ssot-users | 2/3 | fail pass pass | 7096 | 8 | within |

tier pass rate: 6/12

## behavior

- left the snapshot (scored `escaped`, never pass): 0 run(s)
- invented a node id the server did not have (scored `invented`, never pass): 0 run(s)
- invented node ids: 0 run(s)
- fake citations: 0 run(s)
- keyword hints seen 18, followed by search_keyword next 11
- first retriever: hybrid 6 · keyword 24 · none 6
- search_keyword calls 61: empty 1 · hits without a node id 4
- after a keyword call, first consuming move (≤3 calls): keyword 14 · hybrid 12 · read_code 10 · read_code_other 3 · Read 12 · grep 7 · bash_read 1 · none 2
- cost self-report ratio (reported/actual, median): 0.75 over 36 run(s)
