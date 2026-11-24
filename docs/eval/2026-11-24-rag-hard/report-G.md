# eval-rag — rubric 1.5.0 · arm grep

corpus 43aa2ec · graphin 43aa2ec · model claude-sonnet-5-5 · runs 3 · lexical-only
agent 237ea8f43410 · skill - · taskset 02fdf2e4d399 · cli 2.1.331 (Claude Code)
asked claude-sonnet-5-5 · effort high · answered by claude-sonnet-5-5

## multi-hop

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rh-auto-bootstrap | 2/3 | fail pass pass | 44244 | 9 | over_silent,over_stated,within |
| rh-chunk-hitline | 2/3 | pass pass fail | 36884 | 13 | within |
| rh-follower-route | 3/3 | pass pass pass | 17365 | 7 | within |
| rh-impact-nodeatline | 3/3 | pass pass pass | 30784 | 13 | within |
| rh-import-retarget | 3/3 | pass pass pass | 15741 | 9 | within |
| rh-scope-exclude | 3/3 | pass pass pass | 33881 | 8 | within |
| rh-ts-alias | 2/3 | fail pass pass | 32663 | 14 | within |
| rh-usage-adoption | 2/3 | pass fail pass | 35188 | 13 | within |

tier pass rate: 20/24

## db-nav

| task | pass | verdicts | bytes(med) | calls(med) | budget |
|---|---|---|---|---|---|
| rh-db-code-access | 3/3 | pass pass pass | 13769 | 10 | within |
| rh-db-company-delete | 3/3 | pass pass pass | 11230 | 9 | within |
| rh-db-drop-status | 0/3 | fail fail fail | 9457 | 7 | within |
| rh-db-ssot-users | 2/3 | pass fail pass | 7013 | 8 | within |

tier pass rate: 8/12

## behavior

- left the snapshot (scored `escaped`, never pass): 0 run(s)
- invented a node id the server did not have (scored `invented`, never pass): 0 run(s)
- invented node ids: 0 run(s)
- calls the containment hook denied: 1 in 1 run(s)
- fake citations: 0 run(s)
- keyword hints seen 0, followed by search_keyword next 0
- first retriever: hybrid 0 · keyword 0 · none 36
- cost self-report ratio (reported/actual, median): 0.89 over 36 run(s)
