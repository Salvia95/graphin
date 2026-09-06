# 런 스냅샷 매니페스트 — 2026-09-06 grep-control (rag 벤치 v2 대조군)

rag 벤치의 **grep 에이전트 대조군**. 09-01 베이스라인 findings의 "다음에 볼 것"에
적혀 있던 v2이고, [2026-09-04 종합](../2026-09-04-swe-explore-synthesis.md) §4.2-10이
"grep 에이전트 대조군이 없다"고 적은 그 자리를 채운다. 같은 27태스크, 같은 채점기,
같은 계약(종료 상태 셋·인용 규칙·예산 고지) — 그리고 graphin이 없다.

| 항목 | 값 |
|---|---|
| 수행일 | 2026-09-06 |
| 장치 | `scripts/eval-rag.py run --arm grep --runs 3 --jobs 3` (루브릭 **1.3.2**, 이 팔을 위해 신설) |
| 소유자 허가 | 세션에서 명시 요청("grep 에이전트 대조군(v2)부터 돌려보자") → unlock → `lock --approved-by salvia95` → unlock 삭제 |
| 코퍼스 | `--ref HEAD`(`eae65b6`), 측정 장치 제외. **인덱스 없음** — grep 팔은 부트스트랩하지 않으므로 `.graphin/`이 스냅샷에 생기지 않는다 |
| 도구 로스터 | `Read` · `Grep` · `Glob` · `Bash`. `--strict-mcp-config`에 `--mcp-config` 없음 → graphin MCP 도구가 존재하지 않는다(트랜스크립트 init 이벤트로 확인: mcp 도구 0). 위임·스킬·네트워크·쓰기 도구는 graphin 팔과 같이 거부 |
| 프롬프트 | `GREP_AGENT_PROMPT`(채점기 안의 상수, sha `237ea8f43410`). graphin-rag 계약에서 검색기 서술을 빼고 도구 절만 바꾼 것 — 역할, 예산(40,000B, 2/3에서 닫기), 루프 4단계, 종료 상태 셋, 점검 셋, 금지 셋, 리포트 4절 |
| 봉쇄 | PreToolUse 훅(`contain.sh`, combined 벤치의 것과 동일)이 `Bash|Read|Grep|Glob`의 스냅샷 밖 절대 경로·`~`를 거부한다. graphin 팔에는 없다 — 그쪽은 81런 중 이탈 0이라 훅이 무력했을 것이고, grep 루프는 스케일링 벤치에서 36런에 33회 시도했다. 거부 횟수는 채점기가 `contained`로 센다. 사후 `escaped` 판정은 양쪽 공통 |
| 모델 · CLI | sonnet · Claude Code 2.1.261 |
| 스모크 | `rag-nh-redis` 1런으로 배관 확인 — MCP 도구 0, Grep 4·Bash 6, 훅 거부 0, 47초 |

## 비교 대상

| 팔 | 디렉터리 | 성격 |
|---|---|---|
| graphin, 변경 전 (77/81, 429 중단) | [2026-09-05-keyword-context](../2026-09-05-keyword-context/findings.md) §2.2.1 | 관찰 지표만 |
| graphin, 프롬프트만 변경 (81) | 같은 곳 §2.2.2 | pass 69/81 |
| graphin, 09-01 재베이스라인 (81) | [2026-09-01-rag-merged-agent](../2026-09-01-rag-merged-agent/findings.md) | pass 76/81, 코퍼스는 그때의 트리 |

세 graphin 팔은 코퍼스·시점이 조금씩 다르다. 가장 가까운 비교는 **같은 코퍼스
(`eae65b6`)·같은 날·같은 채점기 계열**인 09-05의 두 팔이다.

## 규약 주석

- 1.3.2는 기본 팔의 명령줄·프롬프트·로스터를 바이트 하나 바꾸지 않았다. 그래서
  `RUN_COMPAT`에 붙고, 기존 graphin 팔 트랜스크립트는 그대로 비교 대상이다.
- grep 팔의 not-here 판정에는 graphin 팔과 같은 함정이 있다 — 09-01 rag 기록이
  코퍼스에 들어 있어 태스크 id를 찾아내면 "이건 벤치 문항"이라고 알아본다. 판정은
  양쪽에 같은 규칙으로 걸리므로 비교에는 공정하지만, 절대값은 그만큼 낮다.
