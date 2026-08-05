---
description: graphin 채택/폴백 리포트 — usage 이벤트 로그를 집계해 헤드라인 지표와 same-intent 폴백 쌍을 요약
allowed-tools: Bash(*), Read
---

이 프로젝트의 graphin usage 리포트를 생성하고 요약하라.

1. graphin 바이너리를 찾는다. 이 플러그인이 설치한 것이 먼저다:
   `$GRAPHIN_BIN` → `${CLAUDE_PLUGIN_DATA}/bin/graphin` → `.graphin/binpath` 내용 →
   PATH의 `graphin`. 못 찾으면 `/graphin:doctor`를 돌리라고 안내하라.
2. 프로젝트 루트에서 `<graphin> usage report`를 실행한다 (기간 한정이 필요하면
   `--since 72h` 또는 `--since YYYY-MM-DD`).
3. 출력에서 다음을 요약하라:
   - 헤드라인: 채택률, 폴백 수(그중 same-intent), 늦은 전환율, 발견 실패율
   - **same-intent 폴백 쌍** — search_hybrid가 놓친 (query, pattern) 실쌍.
     이것이 인덱스/랭킹 개선의 리터럴 재현 케이스임을 짚어라
   - 퍼널 준수율(search → explore/read ID 핸드오프)과 main/서브에이전트 차이
4. "index present but no usage events" 진단이 나오면 `/graphin:doctor`로 훅이
   발화 중인지 확인하도록 안내하라.
