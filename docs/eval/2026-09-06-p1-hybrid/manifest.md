# 런 스냅샷 매니페스트 — 2026-09-06 p1-hybrid (D 팔: P1 바이너리 + 하이브리드)

[인수인계 §7-2](../../handoff-2026-09-06.md#7-2-세-번째-팔-d--p1-바이너리--하이브리드-허가-후-2시간-12달러)의
세 번째 팔. grep 대조군(C)의 **진짜 비교 상대**이고, B(프롬프트만 변경)와의 차이가
도구 효과(P1 `context` + P3 힌트)다. 채점은 같은 날 고친 루브릭 1.3.3
([rescore](../2026-09-06-rescore-1.3.3/findings.md))으로, 네 팔 모두.

| 항목 | 값 |
|---|---|
| 수행일 | 2026-09-06 10:49 → 12:01 KST (1시간 13분, 81런 오류 0) |
| 명령 | `scripts/eval-rag.py run --out <샌드박스>/rag-p1-hybrid --runs 3 --jobs 1 --semantic --ref eae65b6 --detach` |
| 소유자 허가 | 세션에서 명시("세 작업 차례대로 진행해줘") |
| 바이너리 | `bin/graphin` v0.4.11-2-g7bf8826-dirty — P1 `context`·P3 힌트·P4 계측 포함(`make build` 직후) |
| 코퍼스 | `--ref eae65b6` — A·B·C와 동일. 기본값 HEAD(`7bf8826`)는 09-06 기록·인수인계를 코퍼스에 넣어 not-here 오염이 늘므로 쓰지 않았다 |
| 스킬 · 에이전트 | 스킬 `5d8a9a59fe08`(B와 동일, `context` 레시피 포함) · 에이전트 `f67281fc2db3`(A·B와 동일) · 태스크셋 `5ca2a37b2b92` |
| 검색 | `--semantic` — 그러나 **부분 하이브리드**(아래) |
| 워커 | `--jobs 1` — 하이브리드 서버 RSS ~1.4GB, 8GB에서 3은 OOM(인수인계 §8) |
| 모델 · CLI | sonnet · Claude Code 2.1.261 |
| 비용 | $10.80 · 벽시계 합 72분 |
| 산출 | 샌드박스 `~/projects/graphin-eval-sandbox/out/rag-2026-09-05/rag-p1-hybrid/`(트랜스크립트·meta·report), 분석 스크립트 `…/tools/{compare,kwnext,regrade,salvage}.py`, 이 디렉터리의 `scores.json`(네 팔 pass·부호검정·태스크별·행동 지표·실패 목록) |

## `--semantic`이 부분적인 이유 (이 런에서 처음 확인)

러너는 스냅샷을 시맨틱까지 프리인덱스하지만, **런마다 claude가 새 MCP 서버를
띄우고** 그 서버가 ONNX 모델을 다시 로드한다. 로드가 끝나는 **8~15초** 전까지
응답은 `semantic_ready="false"`이고 `search_hybrid`는 어휘만 돈다. 콜이 전부 10초
안에 끝나는 짧은 런은 시맨틱을 한 번도 못 본다.

| 지표 | 값 |
|---|--:|
| `search_hybrid` 응답의 `<results semantic_ready>` true / false | **90 / 56 — 하이브리드 콜의 62%만 시맨틱** |
| 시맨틱을 한 번이라도 본 런 | 47 / 81 |

(처음 적은 "32%"는 상태 접두 `<system_status semantic_ready>`까지 섞어 센 값이었다 —
FSM 표기는 엔진보다 늦게 뒤집혀 false가 과대 계상된다. 정본은 `<results>` 속성이다.)

그러므로 D는 "P1 + 하이브리드"가 아니라 **"P1 + 하이브리드-준비되면"**이다. D−B의
차이는 대부분 P1·P3의 것이고, 하이브리드의 기여는 이 런으로 재지 못했다. 깨끗한
하이브리드 팔에는 서버의 준비 대기 옵션(러너/제품 변경, `RUN_COMPAT` 리셋 여부
판단)이 필요하다 — 소유자 결정.

## 규약 주석

- 스킬·에이전트 md는 런 도중 편집하지 않았다(러너가 시작 시 읽는다).
- `meta.graphin_commit`은 `7bf8826 (dirty)`로 찍힌다 — 바이너리의 커밋이고, 코퍼스는
  `corpus: eae65b6`이 정본이다. 리포트 헤더가 둘을 따로 보여 준다.
- 8GB에서 D와 `go test`를 겹치지 않았다. Go 테스트는 D 완주 뒤 `go vet`·26패키지 ok.
