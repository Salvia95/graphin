# 재현 좌표 — rag 베이스라인 (2026-09-01)

graphin-rag 행동 골든셋(`eval/rag`) 첫 베이스라인. 벤치의 정의는
`docs/rag-bench-spec.md`, 실행·채점은 `scripts/eval-rag.py`.

| 좌표 | 값 |
|---|---|
| 수행 | 2026-09-01 07:49 KST 시작, 총 55분 (57런 순차) |
| 소스 | graphin `e7f9999` + 미커밋 벤치 파일들 (`--worktree`) |
| 코퍼스 | 워크트리 스냅샷에서 **측정 장치 제외** (`eval/`·골든셋 스킬·`scripts/eval-*`) |
| 태스크셋 | 19태스크 × 3런, sha256 `0c1c39bae800b5ae…` |
| 에이전트 | `plugin/graphin-guide/agents/graphin-rag.md` sha256 `b83715c342ad…` |
| 스킬 | `plugin/graphin-guide/skills/graphin/SKILL.md` sha256 `e7b2b89cae31…` |
| 합성 프롬프트 | sha256 `efc0d81ae806…` |
| 모델 | `sonnet` → claude-sonnet-5 (에이전트 frontmatter와 동일) |
| CLI | Claude Code 2.1.252, `--strict-mcp-config` + 훅·플러그인 off |
| 인덱스 | lexical-only (`--ort-lib /nonexistent-ort`), 사전 인덱스 1회 |
| 루브릭 | 기록 1.0.2 · 채점 1.0.3 (채점 전용 수정, `RUN_COMPAT` 호환) |
| 비용 | $7.57 (57런 합) |

원시 트랜스크립트(57개 stream-json)는 관례대로 커밋하지 않는다 — 위 좌표로
재현한다. 단 LLM이 도는 벤치라 재현은 통계적 재현이다: 같은 좌표에서 층별
pass율이 비슷하게 나오는 것이지 런이 같아지는 것이 아니다.

같은 날 같은 세트로 루브릭 1.0.0·1.0.1 런 두 번이 선행했다(각 57런,
$7~8/런셋). 숫자는 39/57 → 43/57 → 49/57로 움직였지만 이것은 에이전트
개선이 아니라 **측정 교정**이다 — 무엇이 교정됐는지는 findings의 교정 이력
절이 담는다.
