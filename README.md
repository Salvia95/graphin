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
tree-sitter). 배포 플랫폼: **linux/amd64 · linux/arm64**. macOS·Windows는
소스 빌드로 동작하지만 onnxruntime 1.26.0 핀이 없어 의미 검색은 `--ort-lib`로
직접 지정해야 하며(없으면 lexical 검색만), darwin/amd64는 해당 릴리스 에셋
자체가 없다. 현재 실행 중인 바이너리가 어느 쪽인지는
`graphin version --json`의 `semantic_supported`로 확인한다.

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

**코드↔DB 크로스 도메인 엣지**: JPA `@Table(name=)`/`@Entity`, SQLAlchemy
`__tablename__`, Django `Meta.db_table`, TypeORM `@Entity("x")`, Prisma
client 멤버 접근(`prisma.<model>.`), 그리고 SQL 문맥이 확실한 문자열 리터럴
(`SELECT…FROM`/`JOIN`/`INSERT INTO`/`UPDATE…SET`, `@Query` 포함)이 감지되면
코드 노드 → 테이블 노드 `reference` 엣지가 생성된다(명시 물리명 1.0 /
client·SQL 0.9 / 클래스명 관례 0.8, 레지스트리 실존 대상 한정 — 명시
매핑만 단일 데이터소스에서 dangling 허용). 테이블 노드의 `used_by` 한 번으로
"이 테이블을 건드리는 코드"에 도달한다. 부트스트랩 이후 스냅샷이 추가·삭제
되면 영향받는 코드 파일만 재해석되어 엣지가 따라 움직인다(재임베딩 없음).
설계: [`docs/phase7-spec.md`](docs/phase7-spec.md).

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

> **주의**: 이 방식은 등록한 프로젝트가 **이 체크아웃의 빌드 산출물을 절대경로로
> 직접 참조**한다 — 저장소에서 `make build`를 돌리면 실행 중인 다른 프로젝트의
> 바이너리가 교체된다. 플러그인 하나로 설치가 끝나는 자기완결 배포는 설계까지
> 마쳤고 구현 전이다: [`docs/plugin-distribution.md`](docs/plugin-distribution.md).

에이전트가 `bootstrap_workspace`를 호출하면 인덱싱과 File Watcher가 시작된다.
`initialize`는 인덱싱과 무관하게 즉시 응답하며, 준비 전 응답에는
`<system_status state="indexing" lexical_ready=... semantic_ready=... />`가 동봉된다.
lexical이 준비되기 전 워처 이벤트는 버퍼링되었다가 준비 직후 도착 순서대로
재생된다 — 초기 스캔이 읽어둔 (낡은) 파일 내용이 동시 편집을 덮어쓰지 못하게
하는 순서 보장이다. 임베딩은 유계 큐 대신 백로그로 처리되어 대형 저장소의 콜드
부트스트랩에서도 벡터가 유실되지 않으며, 워밍업 중 `bootstrap_workspace`를
재호출하면 응답에 `embed_pending`(남은 임베딩 수)이 동봉되어 진행 상황을
확인할 수 있다(lexical 검색은 그 사이에도 사용 가능).

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
`--ort-lib` · `--workers <n>` · `--verbose` · `--admin-addr <host:port>`

서브커맨드: `dbimport` · `usage` · `eval` · `version`. `graphin version --json`은
버전·커밋·`os`/`arch`·ORT 버전과 **이 플랫폼에서 의미 검색이 가능한지**
(`semantic_supported`)를 한 줄로 낸다.

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
├── agent-nav.log                       # JSONL 구조화 로그
├── binpath                             # 서버 바이너리 절대경로 (usage 훅의 해석용)
└── usage/events.jsonl                  # graphin-usage 플러그인의 툴콜 이벤트 (32MiB 회전)
```

## 관리자 페이지 (admin)

MCP 서버에 내장된 읽기 전용 로컬 웹 페이지. 사람이 브라우저로 그래프 상태를
모니터링하는 용도다 — AI 에이전트의 도구 경로(MCP)와 같은 프로세스에서 같은
라이브 워크스페이스를 본다.

```sh
# MCP 등록 인자에 플래그 추가 (루프백 주소만 허용)
claude mcp add graphin -- /path/to/bin/graphin \
  --workspace /path/to/project --admin-addr 127.0.0.1:7466
# 브라우저에서 http://127.0.0.1:7466
```

| 화면 | 내용 |
|---|---|
| 대시보드 | 인덱싱 진행률·임베딩 백로그(2s 폴링), 노드/엣지/샤드 카운트, 헬스 요약 |
| 구조 | 패키지(샤드) → 파일 → 노드 드릴다운 — 검색 없이 그래프를 둘러보는 진입점 |
| 검색 | Tier-0 → BM25 ∥ 벡터 RRF (MCP `search_hybrid`와 동일 경로), match 배지 |
| 노드 상세 | ego-graph SVG(1홉, confidence 기반 스타일), uses/used_by 목록(min_conf 필터·커서 페이지네이션), 코드 뷰 |
| 진단 | 끊어진(dangling) 엣지(코드/DB 필터), partial 노드, semantic 상태, 역인덱스 통계 |
| 로그 | `agent-nav.log` tail(3s 갱신) — 워처 배치·재인덱싱·임베딩 이벤트, 에러 강조·이벤트 필터 |
| 계측 | graphin-usage 채택 지표(`usage report`와 동일 산식) — 헤드라인·폴백 페어·바이그램·일별 추이 차트 |
| 설정 | 유효 기동 플래그·모델 스펙·게이트 상태·저장소 용량 (읽기 전용) |

운영자가 각 화면에서 무엇을 확인하고 어떤 값이 정상인지, 이상 신호에 어떤 조치를
취하는지는 [`internal/admin/USE_CASES.md`](internal/admin/USE_CASES.md)에 유스케이스
9종으로 정리돼 있다. UI 규격은 [`DESIGN.md`](internal/admin/DESIGN.md),
graphin 종속 적용 판단은 [`DECISIONS.md`](internal/admin/DECISIONS.md).

v1은 어떤 변경도 수행하지 않는다(전 라우트 GET). 바인드 실패 시 경고만 남기고
MCP 서버는 계속 동작한다. 페이지는 루프백 바인드 + Host 헤더 검증으로 로컬
전용이며, 정적 자산은 바이너리에 임베드되어 오프라인에서 완결된다.

서드파티: [htmx](https://htmx.org) v2.0.6 (Zero-Clause BSD,
`internal/admin/static/htmx.LICENSE`)을 벤더링한다.

## 평가 (SWE-Explore 하니스)

탐색 품질을 반증 가능하게 재는 결정론(LLM 불개입) 하니스.
[SWE-Explore-Bench](https://github.com/Qiushao-E/SWE-Explore-Bench)의 벤치
JSONL과 저장소 스냅샷을 준비한 뒤:

```sh
graphin eval swe-explore --bench bench.jsonl --repos ./repos --out eval-out
graphin eval swe-explore ... --sweep          # top_k × RRF k × min_confidence 27점 매트릭스
graphin eval swe-explore ... --policy grep    # Grep -C20 베이스라인 (같은 질의 유도)
graphin eval swe-explore ... --semantic       # 하이브리드 모드 (모델 워밍업 대기)
```

이슈 텍스트에서 질의를 결정론적으로 유도(제목 → 백틱 스팬 → 식별자 빈도)해
search → explore 1-hop → read_code 스팬을 ranked `(path,start,end)` JSONL로
출력한다. 태스크당 인덱싱 1회, 스윕 설정은 영속 인덱스를 재사용한다. 채점은
벤치 공식 스코어러(`eval.py`) 몫이며, 하니스는 제출 파일과 `summary.md`만
만든다. 설계·가설: [`docs/phase7-spec.md`](docs/phase7-spec.md) §3.

## 채택 계측 (graphin-usage 플러그인)

실세션에서 graphin이 채택되는지/어디서 폴백하는지 재는 Claude Code 플러그인.
PostToolUse 훅이 인덱싱된 프로젝트의 툴콜을 `.graphin/usage/events.jsonl`에
쌓고, 인접 시퀀스에서 헤드라인 4종 — 채택(`graphin → Read/Edit`), 폴백
(`graphin → Grep`, same-intent 쌍은 인덱스 개선의 실측 재현 케이스),
늦은 전환, 발견 실패 — 을 집계한다.

```sh
claude --plugin-dir ./plugin/graphin-usage   # 로컬 시험 (세션 한정)
graphin usage report [--since 72h] [--json]  # 집계 (세션 안: /graphin-usage:report)
```

설치·프라이버시·트러블슈팅: [`plugin/graphin-usage/README.md`](plugin/graphin-usage/README.md) ·
설계: [`docs/usage-spec.md`](docs/usage-spec.md).

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
