# 격리 강화 재베이스라인 (rubric 1.4.3, 2026-09-13)

## 왜

소유자 요청 감사에서 rag 벤치의 정답 격리에 구멍 셋이 드러났다(memory: graphin-rag-bench,
2026-09-13 감사). 셋 다 실측·카나리로 재현했고, 이번 라운드가 그중 자식 세션 격리
셋을 닫는다. 넷째(형제·docs/eval를 안 자르는 combined/wiki/scaling)는 Phase 3.

## 무엇을 바꿨나 (scripts/eval-rag.py, RUN_COMPAT 리셋 → 1.4.3)

1. **`SELF_PREFIXES`가 `eval/` 전체를 자른다** (전엔 `eval/rag`·`eval/golden`만).
   형제 벤치 정답 `eval/combined/expected.jsonl`이 `rag-hop-truncate`·`rag-semantic-gate`
   답을 그대로 옮겨 적고 있었고, **grep 팔의 Grep이 81런 중 7런에서 실제로 읽었다**
   (전부 pass). graphin 팔은 search 도구가 그 파일을 안 올려 0. 무-API materialize로
   `eval/combined/expected.jsonl`이 코퍼스에서 사라짐을 확증.
2. **`permissions.blockReadsOutsideWorkingDirectories: true`** 를 자식 `--settings`에.
   Read·Grep·Glob 도구와 Bash 하위 프로세스 읽기를 커널 경로 기준으로 스냅샷 안에
   가둔다. 봉쇄 훅(`contain.sh`)은 절대경로 토큰만 봐서 **Grep `path:".."`·`$HOME`·
   `cd ..`을 놓쳤다**(카나리로 누출 실증). 훅은 `contained` 텔레메트리로, `escaped`
   판정은 백스톱으로 남기고 집행만 이 설정으로 옮김.
3. **`enabledPlugins`에 플러그인 이름을 명시해 끈다.** 빈 `{}`는 사용자 스코프
   플러그인을 **못 껐다** — graphin·graphin-guide가 자식에 로드됐고, graphin
   플러그인의 wiki 게이트가 `.graphin/merkle.json`이 있는 graphin 팔에서만 무장해
   Bash를 풀런당 16~17회 거부했다(grep 팔 0). 한 팔만 묶던 비대칭 제거.

## 재측정 (36태스크 × 3런, graphin 팔, lexical-only)

동일 taskset `edd99ac9013e`로 직전 1.4.2 베이스라인과 A/B:

| 층 | 1.4.2 | 1.4.3 |
|---|---|---|
| answered | 32/36 | 32/36 |
| multi-hop | 22/27 | 23/27 |
| not-here | 8/9 | 9/9 |
| out-of-reach | 6/6 | 6/6 |
| budget-pressure | 6/6 | 5/6 |
| db-nav | 24/24 | 21/24 |
| **합계** | **98/108 (90.7%)** | **96/108 (88.9%)** |

- **격리 목표 달성:** `escaped` 0 · `invented(miss)` 0 · 플러그인 로드 0(108런 전부) ·
  형제 정답 코퍼스 제거.
- **하락분은 변동:** 순감 2런은 db-nav −3·budget −1을 not-here +1·multi-hop +1이
  상쇄한 것. db-nav 답은 `testdata/fixtures/dbschema/`(스냅샷 안)라 절단·blockReads
  영향권 밖이고 graphin 팔엔 Bash가 로스터에 없다. 실제 실패는 `rag-db-trigger-fn`
  1/3(기존 플래키)·`rag-db-dangling` 2/3·`rag-bp-change-flow` fake-citation 1런
  (`internal/watch/debouncer.go` 날조, 에이전트 오류). 스펙 §5의 "집계 평균으로 결론
  내지 말라" 범위.
- **게이트 0.80을 88.9%로 여유 통과.** `rag-read-omission` 0/3은 스펙 §8 의도된 유지.
- **부수 효과:** blockReads로 `rag-oor-adoption`이 더는 `find /`로 파일시스템을
  훑을 수 없어 즉답 종료 — 스펙 §7 "게이트가 5분 넘기는 이유"의 그 태스크. 스모크에서
  249~385s → 44.8s로 관측(풀런에서도 임계 경로 아님).

## 다음

- Phase 2: 네트워크 격리(공개 저장소를 grep 팔이 `git clone`으로 끌어 올 수 있다).
  bubblewrap+socat 설치 + `sandbox.network.allowedDomains`. 소유자 결정.
- Phase 3: combined/wiki/scaling 러너 대칭화(`docs/eval` 절단 + blockReads +
  플러그인 실차단). 통제 밖 파일이라 unlock 불필요.
