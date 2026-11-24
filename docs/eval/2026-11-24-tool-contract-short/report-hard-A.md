# eval-rag — rubric 1.5.0 · arm bare

corpus 342d29c · graphin 69ea715 · model claude-sonnet-5-5 · runs 3 · lexical-only
agent 14c25158c013 · skill - · taskset 02fdf2e4d399 · cli 2.1.331 (Claude Code)
asked claude-sonnet-5-5 · effort high · answered by claude-sonnet-5-5

## multi-hop

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rh-auto-bootstrap | 1/3 | pass fail fail | 42595 | 10 | over_silent,within |
| rh-chunk-hitline | 3/3 | pass pass pass | 35944 | 12 | within |
| rh-follower-route | 3/3 | pass pass pass | 15977 | 5 | within |
| rh-impact-nodeatline | 3/3 | pass pass pass | 22244 | 14 | within |
| rh-import-retarget | 3/3 | pass pass pass | 23440 | 11 | within |
| rh-scope-exclude | 3/3 | pass pass pass | 24207 | 11 | within |
| rh-ts-alias | 2/3 | fail pass pass | 27526 | 9 | within |
| rh-usage-adoption | 0/3 | fail fail fail | 43771 | 11 | over_silent |

tier pass rate: 18/24

## db-nav

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rh-db-code-access | 2/3 | pass pass fail | 26169 | 14 | within |
| rh-db-company-delete | 2/3 | pass fail pass | 10203 | 6 | within |
| rh-db-drop-status | 0/3 | fail fail fail | 19123 | 6 | within |
| rh-db-ssot-users | 3/3 | pass pass pass | 9530 | 10 | within |

tier pass rate: 7/12

## behavior

- left the snapshot (scored `escaped`, never pass): 0 run(s)
- invented a node id the server did not have (scored `invented`, never pass): 0 run(s)
- invented node ids: 0 run(s)
- fake citations: 0 run(s)
- keyword hints seen 19, followed by search_keyword next 8
- first retriever: hybrid 5 · keyword 27 · none 4
- search_keyword calls 82: empty 1 · hits without a node id 7
- after a keyword call, first consuming move (≤3 calls): keyword 25 · hybrid 10 · read_code 8 · read_code_other 7 · explore 1 · Read 12 · grep 8 · bash_read 5 · none 6
- cost self-report ratio (reported/actual, median): 0.71 over 36 run(s)
