# graphin

AI 코딩 에이전트를 위한 로컬 코드베이스 탐색 MCP 서버.

`grep` 기반 선형 탐색 대신 **점진적 정보 공개(Progressive Disclosure)** 3단계로
에이전트의 토큰 소모를 최소화한다:

```
1. search_hybrid("결제 취소 로직")   → 진입점 노드 ID 후보 (본문 없음)
2. explore_graph(노드 ID)           → uses / used_by 관계 + confidence
3. read_code(노드 ID)               → 해당 노드의 원본 코드만 정확히 슬라이싱
```

지원 언어: **Java, Kotlin, Python, JavaScript, TypeScript**(JSX/TSX 포함,
tree-sitter). 대상 플랫폼: Linux/macOS.

JS/TS는 파일 경로 기반 모듈 ID(`src/order/service.ts` →
`src.order.service.OrderService`)를 사용하며, 상대 경로 import·re-export·
`require()`는 파싱 시점에 같은 dotted 모듈 공간으로 정규화되어 스코프 랭킹에
반영된다. tsconfig `paths` 별칭은 해석하지 않는다(전역 티어 0.80으로 폴백).
`.d.ts`는 선언 시그니처를 인덱싱하고, TS 오버로드 시그니처는 구현부 하나로
접힌다. `.min.js`·`dist/`·`.next/`·`coverage/`·`node_modules/`는 기본 제외.

**탐색 커버리지**: 심볼 노드 외에도 ① 메서드/클래스 **본문 토큰**(문자열
리터럴·주석·필드)이 lexical 인덱스에 포함되고, ② YAML/properties/SQL/MD/
Gradle/Dockerfile 등 **앵커 없는 텍스트 파일은 파일 단위 노드**(ID = 상대
경로)로 승격되어 검색·`read_code`가 가능하다. 파일 노드는 그래프 엣지를
만들지 않으며, 락파일(package-lock.json 등)은 기본 제외된다.

**RDB 스키마 스냅샷**: `<name>[.<section>].graphindb.json` 규약의 스냅샷
파일을 저장소에 커밋하면 테이블·뷰·함수·프로시저가 개별 노드로, FK가
`ForeignKey` 엣지로 인덱싱된다(컬럼·인덱스·제약은 테이블 노드에 접힘,
RLS·트리거는 사이드카 파일로 분리). graphin은 라이브 DB에 접속하지 않는다 —
스냅샷은 tbls/Atlas 출력을 `graphin dbimport`로 변환하거나 수기로 작성한다.
데이터소스(파일 단위) N개 공존, 크로스 데이터소스 참조(`db.<ds>.` 프리픽스),
supabase `auth.users` 같은 스냅샷 밖 대상(dangling 엣지)을 지원한다.
전체 명세: [`schema/graphindb.md`](schema/graphindb.md).

```sh
# Postgres/MySQL/SQLite: tbls 이트로스펙션 → 스냅샷 변환
tbls out -t json --dsn "postgres://..." | graphin dbimport --from tbls --datasource main -
graphin dbimport --init main            # 수기 작성용 빈 뼈대
```

프로젝트에 스키마 SSOT가 이미 있으면 변환 없이 **매니페스트로 직접 라우팅**할
수 있다: 베이스네임 `graphindb.json` 매니페스트가 데이터소스별 SSOT 파일을
`sql`(상태형 DDL 덤프) · `schema`(prisma 서브셋) · `json`(tbls 프리셋 또는
셀렉터 매핑) 포맷으로 선언하면 SSOT 파일 자체가 파스 타깃이 되어
`read_code`가 실제 CREATE TABLE·model 블록을 반환한다. 매니페스트 오류는
`db_manifest_errors` 속성으로 에이전트에게 피드백된다.

부트스트랩 시 DB 흔적(마이그레이션 디렉터리·prisma·docker-compose 등)은
있는데 스냅샷도 매니페스트도 없으면 `bootstrap_workspace` 응답에
`db_sources_detected`/`db_snapshots` 속성과 안내 `<hint>`가 동봉된다.
Supabase의 RLS·트리거 사이드카와 Oracle(Atlas inspect 참고)은 수기 작성한다.
tbls의 virtual relation은 `enforced:false` 논리 참조로 변환된다.

## 빌드 & 등록

```sh
make build                    # → bin/graphin
# Claude Code 등록 예시
claude mcp add graphin -- /path/to/bin/graphin --workspace /path/to/project
```

에이전트가 `bootstrap_workspace`를 호출하면 인덱싱과 File Watcher가 시작된다.
`initialize`는 인덱싱과 무관하게 즉시 응답하며, 준비 전 응답에는
`<system_status state="indexing" lexical_ready=... semantic_ready=... />`가 동봉된다.

## 도구 (MCP tools)

| 도구 | 역할 |
|---|---|
| `bootstrap_workspace` | 최초 인덱싱 + watcher 기동 (`model_type`: `english_optimal` \| `multilingual_cjk`) |
| `search_hybrid` | Tier-0 정확 일치 → BM25 ∥ 벡터 RRF(k=60) 병합. 원시 점수 비노출 |
| `explore_graph` | 결정론 정렬(confidence↓ → 동일 패키지 → FQN), 20엣지/페이지 seek-key 커서 |
| `read_code` | 바이트 오프셋 슬라이싱. 파일 해시 불일치 시 인라인 재파싱(`reparsed="true"`) |
| `run_local_benchmark` | Grep Full / Grep -C20 / graphin 3-시나리오 바이트·토큰 절감 리포트 |

## 실행 플래그

`--workspace <path>`(필수) · `--model-type` · `--offline` · `--model-dir` ·
`--ort-lib` · `--workers <n>` · `--verbose`

시멘틱 모델(e5 계열 INT8 ONNX)과 onnxruntime 1.26.0은 최초 부트스트랩 시
SHA256 검증과 함께 자동 프로비저닝된다(`~/.cache/graphin/artifacts` 캐시).
폐쇄망은 `--offline` + `--model-dir`/`--ort-lib`를 사용한다.

## 데이터 레이아웃

```
<workspace>/.graphin/
├── search/{vectors.bin, lexical.idx}   # 벡터(머클 헤더) + BM25 스냅샷
├── graph/{chunk_<pkg>.fb, reverse_base.bin, reverse_delta.log}
├── merkle.json                         # BLAKE3 파일/서브트리 해시
├── runtime/                            # 검증 완료된 모델 + ORT
├── lockfile                            # PID + 3s heartbeat
└── agent-nav.log                       # JSONL 구조화 로그
```

## 개발

```sh
make test        # go vet + go test ./...
make test-race   # 동시성 패키지 race 검사
make fbs         # schema/graph.fbs 재생성 (flatc 25.12.19, scripts/fetch-flatc.sh)

# 실제 ONNX 추론 스모크 (모델 다운로드/캐시 필요)
GRAPHIN_ORT_SMOKE=1 go test ./internal/semantic/onnx/ -run TestRealONNX -v
```

- 토크나이저는 HF tokenizer.json 서브셋(WordPiece + Unigram)의 자체 구현이며,
  `testdata/tokenizer/`의 HF 레퍼런스 토큰 ID와 완전 일치를 테스트로 강제한다.
  픽스처 재생성: `scripts/gen_tokenizer_fixtures.py`.
- 핵심 회귀: 파일 상단 import 추가 후에도 미변경 메서드의 `read_code`가
  정확해야 한다(2-Track: 오프셋은 무조건 갱신, 임베딩·엣지 연산은 스킵).
