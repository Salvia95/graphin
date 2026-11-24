# eval-rag — rubric 1.5.0 · arm bare

corpus 342d29c · graphin 69ea715 · model claude-sonnet-5-5 · runs 3 · lexical-only
agent 14c25158c013 · skill - · taskset 02fdf2e4d399 · cli 2.1.331 (Claude Code)
asked claude-sonnet-5-5 · effort high · answered by claude-sonnet-5-5

## multi-hop

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rh-auto-bootstrap | 2/3 | fail pass pass | 34171 | 7 | over_silent,within |
| rh-chunk-hitline | 2/3 | fail pass pass | 34743 | 13 | over_silent,within |
| rh-follower-route | 3/3 | pass pass pass | 22525 | 5 | within |
| rh-impact-nodeatline | 3/3 | pass pass pass | 22533 | 13 | within |
| rh-import-retarget | 3/3 | pass pass pass | 24414 | 9 | within |
| rh-scope-exclude | 3/3 | pass pass pass | 30471 | 11 | within |
| rh-ts-alias | 3/3 | pass pass pass | 21774 | 6 | within |
| rh-usage-adoption | 1/3 | fail fail pass | 58272 | 13 | over_silent,over_stated |

tier pass rate: 20/24

## db-nav

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rh-db-code-access | 2/3 | pass pass fail | 26644 | 12 | within |
| rh-db-company-delete | 2/3 | pass fail pass | 9812 | 7 | within |
| rh-db-drop-status | 0/3 | fail fail fail | 10627 | 9 | within |
| rh-db-ssot-users | 2/3 | pass fail pass | 7115 | 8 | within |

tier pass rate: 6/12

## behavior

- left the snapshot (scored `escaped`, never pass): 0 run(s)
- invented a node id the server did not have (scored `invented`, never pass): 0 run(s)
- invented node ids: 0 run(s)
- fake citations: 0 run(s)
- keyword hints seen 18, followed by search_keyword next 11
- first retriever: hybrid 8 · keyword 17 · none 11
- search_keyword calls 48: empty 2 · hits without a node id 2
- after a keyword call, first consuming move (≤3 calls): keyword 6 · hybrid 6 · read_code_other 13 · explore 1 · Read 15 · grep 6 · none 1
- cost self-report ratio (reported/actual, median): 0.76 over 36 run(s)
