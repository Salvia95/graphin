---
description: graphin 채택/폴백 리포트 — usage 이벤트 로그를 집계해 헤드라인 지표와 same-intent 폴백 쌍을 요약
---

이 프로젝트의 graphin usage 리포트를 생성하고 요약하라.

1. graphin 바이너리를 찾는다: `$GRAPHIN_BIN` → `.graphin/binpath` 파일 내용 →
   PATH의 `graphin` 순서. 못 찾으면 사용자에게 graphin 빌드/설치 위치를 물어라.
2. 프로젝트 루트에서 `<graphin> usage report`를 실행한다 (기간 한정이 필요하면
   `--since 72h` 또는 `--since YYYY-MM-DD`).
3. 출력에서 다음을 요약하라:
   - 헤드라인: 채택률, 폴백 수(그중 same-intent), 늦은 전환율, 발견 실패율
   - **same-intent 폴백 쌍** — search_hybrid가 놓친 (query, pattern) 실쌍.
     이것이 인덱스/랭킹 개선의 리터럴 재현 케이스임을 짚어라
   - 퍼널 준수율(search → explore/read ID 핸드오프)과 main/서브에이전트 차이
4. "index present but no usage events" 진단이 나오면 graphin-usage 플러그인이
   설치되어 발화 중인지 확인하도록 안내하라 (plugin/graphin-usage/README.md
   트러블슈팅 참고).
