# 런 스냅샷 매니페스트 — 2026-09-08 36태스크 grep 대조군

[36태스크 첫 풀셋](../2026-09-08-36-baseline/findings.md)의 반대편. 확장의 목적이
비용 축의 검정력이었으므로 같은 세트에서 다시 잰다.

| 항목 | 값 |
|---|---|
| 수행일 | 2026-09-08 (108런, 오류 0, `--jobs 3`) |
| 명령 | `scripts/eval-rag.py run --out <샌드박스>/rag-2026-09-08-36-grep --arm grep --runs 3 --jobs 3 --ref b564780 --detach` |
| 소유자 지시 | "런 끝나면 채점해줘" (직전 제안한 grep 대조군) |
| 도구 | 호스트 `Read`·`Grep`·`Glob`·`Bash`, `--strict-mcp-config`로 graphin 도구 부재 |
| 프롬프트 | `GREP_AGENT_PROMPT` — `agent_sha` `237ea8f43410`, 스킬 없음 |
| 코퍼스 | **`--ref b564780`** — graphin 팔과 동일. `files.txt` 375파일 바이트 동일 |
| 태스크셋 | `edd99ac9013e` (36) — 양 팔 동일 |
| 채점 | 루브릭 **1.4.2**, graphin 팔과 같은 채점기 |
| 모델 · CLI | sonnet · Claude Code 2.1.263 |
| 산출 | 샌드박스 `~/projects/graphin-eval-sandbox/out/rag-2026-09-08-36-grep/`, 이 디렉터리의 `report.md` · `meta.json` |

`meta.graphin_commit`은 `5e15147`(HEAD)로 찍히지만 이 팔은 바이너리를 쓰지 않는다.
직전 커밋은 전부 `docs/eval/` 아래라 절단 대상이어서 코퍼스를 바꾸지 않았고,
`--ref`는 메타를 맞추기 위해 명시했다.
