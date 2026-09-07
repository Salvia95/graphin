# 런 스냅샷 매니페스트 — 2026-09-08 grep 대조군 (1.4.1 대칭 이후 첫 대조)

[141-baseline](../2026-09-08-141-baseline/findings.md)의 반대편. 1.4.1이 봉쇄 훅을
양 팔로 대칭화한 뒤 **두 팔을 같은 조건에서 나란히 놓는 첫 셋**이다. 직전 대조
([2026-09-06-grep-control](../2026-09-06-grep-control/findings.md))는 grep 팔에만
훅이 걸린 상태의 숫자였다.

| 항목 | 값 |
|---|---|
| 수행일 | 2026-09-08 (81런, 오류 0, `--jobs 3`으로 실경과 약 35분) |
| 명령 | `scripts/eval-rag.py run --out <샌드박스>/rag-2026-09-08-grep-control --arm grep --runs 3 --jobs 3 --ref 8c0d392 --detach` |
| 소유자 지시 | "반대편 측정도 진행해줘" |
| 도구 | 호스트 `Read`·`Grep`·`Glob`·`Bash`. `--mcp-config` 없이 `--strict-mcp-config`라 graphin 도구가 존재하지 않는다 |
| 프롬프트 | 채점기의 `GREP_AGENT_PROMPT` 상수 — `agent_sha` **`237ea8f43410`**, 스킬 없음 |
| 코퍼스 | **`--ref 8c0d392`** — graphin 팔과 맞추기 위해 명시했다(§1 참조). `files.txt` 375파일 바이트 동일 |
| 채점 | 루브릭 **1.4.2**, graphin 팔과 같은 채점기 하나 |
| 모델 · CLI | sonnet · Claude Code 2.1.263 — 양 팔 동일 |
| 비용 | 벽시계 합 약 97분 |
| 산출 | 샌드박스 `~/projects/graphin-eval-sandbox/out/rag-2026-09-08-grep-control/`, 이 디렉터리의 `report.md` · `meta.json` |

## 1. `--ref`를 명시한 이유

graphin 팔은 `8c0d392`에서 돌았고, 그 뒤 승격 커밋 `c8981dd`가 HEAD를 옮겼다.
그 커밋이 바꾼 파일 중 셋(`docs/rag-bench-spec.md`, `docs/wiki/sets/rag-bench.md`,
`docs/wiki/pins.lock`)은 **`SELF_PREFIXES` 절단에 걸리지 않아 스냅샷에 남는다.**
기본값인 HEAD로 돌렸다면 두 팔이 다른 코퍼스를 본 셈이 된다. `--ref 8c0d392`로
graphin 팔이 본 트리에 정확히 맞췄고, `files.txt` 375줄이 바이트 동일함을 확인했다.

`meta.graphin_commit`은 `c8981dd`(현재 HEAD)로 찍히지만 이 팔은 graphin 바이너리를
쓰지 않으므로 비교에 무관하다. 비교에 쓰이는 것은 `corpus`이고 양 팔 모두 `8c0d392`다.

`meta.rubric_version`도 다르게 찍힌다(graphin 팔 1.4.1, 이 팔 1.4.2). 두 버전은
러너가 바이트 동일한 채점 전용 차이라 `RUN_COMPAT`에 함께 있고, **채점은 1.4.2
채점기 하나로** 했다.
