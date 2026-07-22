# Phase 7 스펙 초안 — 코드↔DB 크로스 도메인 엣지 + 평가 체계 (v0, draft)

코드 그래프와 graphindb 그래프를 잇는 **크로스 도메인 엣지**(7a/7b)와,
SWE-Explore 기반 **반증 가능한 탐색 품질 평가 하니스**(7c)를 정의한다.
7c는 7a/7b와 코드 경로가 독립이라 병행 진행한다.

## 0. 목표와 근거

- 현재 DB 노드는 DB 노드끼리만 연결된다(FK·트리거·RLS). 코드 노드 →
  테이블 엣지가 생기면 "orders 테이블을 건드리는 코드"라는 질문에
  `explore_graph(db.main.public.orders, used_by)` 한 번으로 답할 수 있다.
  라우트 → 핸들러 → 리포지토리 → 테이블 종단 추적의 마지막 조각.
- 평가 근거: [SWE-Explore](https://arxiv.org/abs/2606.07297)는 (1) 에이전트가
  파일은 찾지만 라인 스팬을 놓치고, (2) **누락된 근거가 잡음보다 치명적**
  임을 보였다. graphin의 `min_confidence`·`top_k` 기본값이 리콜을 깎는지
  데이터로 검증한다. [CodeCompass](https://arxiv.org/abs/2602.20048)의
  내비게이션 역설(정보 과잉이 성능을 깎음)에 따라 **새 MCP 도구는 추가하지
  않는다** — 기존 5개 도구의 응답이 풍부해질 뿐이다.

## 1. Phase 7a — 크로스 도메인 엣지 코어

### 1.1 엣지 의미론

| 소스 | 대상 | 타입 | 비고 |
|---|---|---|---|
| class/method/function | table/view | `Reference` | 방향은 항상 코드 → DB (uses). 테이블 → 코드는 역인덱스(used_by)가 제공 |

- **`.fbs` 변경 없음.** 기존 `Reference` 타입을 재사용한다 — 대상 노드
  kind(`table`/`view`)로 크로스 도메인임이 구분되므로 새 EdgeType이 불필요하고,
  샤드 하위호환이 유지된다.
- DB 함수/프로시저 호출(코드에서 `SELECT fn_x(...)`)은 v1 제외 — §5 열린 질문.

### 1.2 confidence 티어 (§2.1.3 체계에 편입)

| 티어 | 조건 |
|---|---|
| **1.0** | 명시 물리명 매핑이고 물리명이 DB 레지스트리 전체에서 유일: JPA `@Table(name="x")`, Prisma `@@map` 경유 별칭, SQLAlchemy `__tablename__`, Django `Meta.db_table`, TypeORM `@Entity("x")` |
| **0.9** | SQL 리터럴 컨텍스트 매칭(7b): `FROM`/`JOIN`/`INSERT INTO`/`UPDATE`/`DELETE FROM` 직후 식별자가 레지스트리 테이블·뷰와 일치. Prisma client 멤버 접근(`prisma.orderItem` → 모델 `OrderItem`, SSOT 라우팅된 경우) |
| **0.8** | 관례 추론: 명시 물리명 없는 엔티티 클래스명 → 동일명/snake_case 후보. 물리명이 복수 데이터소스에 중복될 때의 강등 티어 |

- 팬아웃 가드: 동일 물리명 후보가 `maxGlobalCandidates`(5) 초과 시 전부 배제
  (기존 전역 동명 규칙과 동일). 레지스트리가 닫힌 세계(실존 테이블만)라
  코드 쪽 stoplist는 불필요.
- 관례 추론(0.8)은 **레지스트리에 실존하는 테이블만** 대상으로 한다 —
  dangling 없음. dangling은 명시 매핑에만 허용(§1.5).

### 1.3 추출 규칙 — 언어 × ORM 매트릭스 (v1 스코프)

extractor가 파싱 시점에 AST에서 감지한다(BodyTokens 사후 매칭이 아님 —
토큰은 캡·소문자화로 정보가 손실된다).

| 언어 | v1 (7a) | 7b | 제외(후보) |
|---|---|---|---|
| Java | JPA/Hibernate `@Entity`+`@Table(name=)` (클래스 노드에 emit) | 문자열 리터럴 SQL (`@Query(nativeQuery)` 포함) | MyBatis XML 매퍼, JPA implicit naming strategy |
| Kotlin | 동일 JPA 어노테이션 | 동일 | Exposed `Table("x")` |
| Python | SQLAlchemy `__tablename__ = "x"`, `Table("x", …)` 1st arg, Django `class Meta: db_table` | 문자열 리터럴 SQL | Django 관례명(`app_model`) 합성 |
| JS/TS | TypeORM `@Entity("x")`, Prisma client 멤버 접근 | 템플릿 리터럴 SQL (`sql\`\``, `knex.raw`) | Sequelize, Drizzle |

- **Prisma 별칭**: `dbprisma.go`가 모델을 파싱할 때 테이블 def에 **모델명
  별칭**(`OrderItem`, lcfirst `orderItem`)을 함께 등록한다. 코드 쪽은
  `prisma.<member>.` / `this.prisma.<member>.` 리시버 패턴에서 `<member>`를
  DBRef(convention 아님, 0.9)로 emit. `@@map` 유무와 무관하게 별칭이 물리
  테이블 def를 가리키므로 해석은 동일 경로.
- 명시 매핑은 **클래스(엔티티) 노드**에 붙는다. 메서드 단위 귀속은 하지
  않는다 — repository 메서드는 엔티티 클래스 경유 2-hop으로 도달 가능하고,
  스팬 과잉 귀속은 내비게이션 역설을 부른다.

### 1.4 해석 파이프라인 변경

```
parse.Node                nodeRecord              resolveEdges (코드 분기)
+ DBRefs []DBRef   →      + DBRefs []DBRef   →    step 4: DB 대상 해석
```

- `parse.DBRef { Name string; Source uint8 /* explicit | client | convention */ }`.
  스키마/데이터소스는 코드에서 알 수 없으므로 담지 않는다(해석 시 판정).
- `graph.ApplyFile`: Changed 노드에서 `rec.DBRefs = n.DBRefs` (Supers/RawCalls와
  동일 대우 — Track B에서만 갱신).
- `resolveEdges` step 4 (기존 1~3 뒤): 각 DBRef에 대해
  1. `res.candidates(name)` → `KindTable`/`KindView`만 필터 (+ Oracle 대비
     `ToUpper` 폴백, 기존 DB 휴리스틱과 동일).
  2. convention Source는 snake_case 변형도 조회.
  3. 후보 데이터소스 유일 → §1.2 티어로 emit. 복수 데이터소스 → 전부 0.8
     (팬아웃 가드 적용).
- 초기 스캔은 전 파일 `ApplyFile` 후 일괄 `Flush`이므로 스냅샷·코드 등록
  순서 무관하게 해석이 성립한다(추가 동기화 불필요). 증분 시나리오는 §1.6.

### 1.5 데이터소스 모호성 · dangling 정책

- **DB 도메인이 비활성**(데이터소스 0개)이면 step 4는 no-op — DB 없는
  저장소에 노이즈를 만들지 않는다.
- 명시 매핑(1.0 티어)인데 레지스트리에 물리명이 없으면: 데이터소스가
  정확히 1개일 때만 `db.<ds>.<default_schema>.<name>`으로 **dangling 엣지**
  emit(기존 FK dangling과 동일 철학 — 스텁 미생성). 데이터소스 복수면 FQN을
  합성할 수 없으므로 skip + obs 이벤트 `db_xref_unresolved` (스냅샷 누락
  신호로 활용 가능).

### 1.6 무효화 — 스냅샷 후행 추가 (7b로 이관)

부트스트랩 이후 스냅샷/매니페스트가 처음 추가되면, 기존 코드 노드는
`needsResolve`가 아니어서 크로스 엣지가 생기지 않는다(재파싱 전까지).
7a에서는 이 한계를 **문서화만** 하고, 7b에서 해소한다:

- DB 파일 인덱싱으로 테이블 def 집합이 바뀌면, 신규/제거된 물리명(+별칭)을
  **lexical 인덱스 역조회**로 검색 → 히트한 노드들의 소속 파일을 강제
  재인덱싱(`forceEmbed`와 유사한 경로, 단 임베딩은 스킵하고 파스만).
  BM25가 이미 본문 토큰을 색인하므로 추가 인덱스 없이 후보를 좁힐 수 있다.
- 매니페스트 라이브 리로드(`reloadDBRoutesLocked`)와 같은 배치에서 처리.

### 1.7 저장 포맷·MCP 표면 영향

- `.fbs`·`lexical.idx`·`vectors.bin` 포맷 변경 없음. `merkle.json` 변경 없음
  (DBRefs는 소스 바이트 해시에 이미 반영됨).
- MCP 도구 5개 유지. `explore_graph` 응답에 크로스 엣지가 자연 편입된다
  (대상 kind로 식별). 신규 속성·도구 없음.

### 1.8 테스트 계획

- 픽스처: `testdata/fixtures/dbxref/` — java(JPA)·python(SQLAlchemy/Django)·
  typescript(Prisma/TypeORM) 각각 + 스냅샷/SSOT 페어. 모호성 케이스(두
  데이터소스에 같은 테이블명), dangling 케이스(supabase `auth.users` 스타일).
- 유닛: extractor별 DBRef emit, resolveEdges step 4 티어·팬아웃·dangling
  (`dbedges_test.go` 패턴 준용).
- E2E: 부트스트랩 → `explore_graph(테이블, used_by)`에 엔티티 클래스 등장 →
  `read_code` 라운드트립. 스냅샷 후행 추가 시나리오는 7b에서 추가.

## 2. Phase 7b — SQL 리터럴 감지 + 무효화 + db-trace 벤치

- **SQL 리터럴**: extractor가 노드 본문의 문자열/템플릿 리터럴 중
  SQL 키워드 문맥(`FROM|JOIN|INSERT INTO|UPDATE|DELETE FROM` — 대소문자
  무관, 단어 경계)이 있는 것만 골라, 키워드 직후 식별자
  (`[schema.]table`)를 DBRef(Source=sql)로 emit. 레지스트리 실존 대상만
  0.9로 해석(§1.2) — 자유 문자열발(發) dangling은 만들지 않는다.
- **무효화**: §1.6 설계 구현 + E2E(부트스트랩 후 스냅샷 추가 → 크로스 엣지
  출현 확인).
- **db-trace 벤치 시나리오**: 구현 결과 `run_local_benchmark`의 제네릭
  3-step 경로(검색 → explore both → read_code)가 테이블 기대 노드를 그대로
  처리하므로 도구 변경 없이 E2E(`TestDBTraceBenchmark`)로 회귀만 고정했다 —
  테이블 노드의 explore가 크로스 엣지를 동봉하므로 grep 베이스라인
  (마이그레이션·엔티티·SQL 전부 히트) 대비 절감이 리포트에 잡힌다.

## 3. Phase 7c — SWE-Explore 평가 하니스

### 3.1 데이터셋

[SWE-Explore-Bench](https://github.com/Qiushao-E/SWE-Explore-Bench)
(HF: `SWE-Explore-Bench/SWE-Explore-Bench`). 태스크 = 실제 이슈 + 저장소
스냅샷(커밋), 제출 = **고정 라인 예산 하의 ranked (file, line-range) 목록**,
채점 = 성공 궤적에서 증류한 라인 수준 ground truth 대비 file hit /
line recall. 저장소는 SWE-bench 계열 Python — graphin의 Python 파싱으로
커버된다. 데이터셋·클론은 커밋하지 않고 `~/.cache/graphin/eval/` 캐시.

### 3.2 결정론 탐색 정책 (LLM 불개입)

`graphin eval swe-explore` 서브커맨드(dbimport와 같은 분기)가 태스크별로:

1. **질의 생성**: 이슈 제목+본문에서 결정론적 규칙으로 질의 도출 —
   코드 스팬·식별자 토큰 우선, 상위 Q개(기본 3).
2. **시드**: 각 질의를 `search_hybrid`(라이브러리 직호출)에 투입, top_k 수집.
3. **확장**: 시드별 `explore_graph` 1-hop(uses+used_by), confidence 내림차순.
4. **산출**: 방문 순서대로 노드 스팬을 (file, start-line, end-line)으로 펼쳐
   라인 예산까지 절단 → 제출 포맷 출력.

전 단계 결정론(타이브레이크는 노드 ID 사전순) — 같은 입력이면 같은 제출.
`--lexical-only` 플래그로 모델 프로비저닝 없는 CI 실행을 지원한다.

### 3.3 메트릭 · 스윕 · 베이스라인

- **채점**: SWE-Explore 공식 스코어러 재사용(하니스는 제출 파일만 생성).
  부가 메트릭: 시뮬레이션 툴콜 수, 응답 총바이트(기존 §3.5 정신).
- **스윕**(핵심 목적 — 기본값의 리콜 비용 검증):
  `min_confidence ∈ {0, 0.5, 0.8}` × `top_k ∈ {5, 10, 20}` ×
  `RRF k ∈ {20, 60, 100}` × `{lexical-only, hybrid}`.
- **베이스라인**: `internal/bench`의 GrepFull/GrepContext를 ranked region
  산출로 확장해 같은 스코어러로 비교 — "grep 대비 X% 바이트로 Y% 리콜"을
  README에 실을 수 있는 형태로 마크다운 리포트 생성.

### 3.4 성공 기준 (가설 — 결과로 기각 가능)

- H1: hybrid가 lexical-only보다 line recall 우위 (모델 비용 정당화).
- H2: `min_confidence=0.5` 기본값은 0 대비 recall 손실 ≤ 2%p
  (아니면 기본값을 낮추고 응답 크기 영향을 재평가).
- H3: 동일 라인 예산에서 graphin ≥ GrepContext(-C20) recall, 바이트 ≤ 30%.

결과 수치는 `docs/eval/`에 리포트로 커밋하고, 기본값 변경은 이 리포트를
근거로만 수행한다.

### 3.5 구현 노트 (7c 확정 사항)

- CLI: `graphin eval swe-explore --bench <jsonl> --repos <dir> --out <dir>`
  (+ `--sweep`/`--policy grep`/`--semantic`/`--tasks N`). 구현:
  `internal/eval/sweexplore/`, 진입 분기: `cmd/graphin/eval.go`.
- 데이터셋 필드: `instance_id`·`repo_dir`(상대면 `--repos` 기준, 없으면
  `<repos>/<instance_id>` 폴백)·이슈 텍스트는
  `problem_statement|issue|meta.*` 순으로 탐색.
- 제출 포맷: 라인당 `{"instance_id": …, "regions": [{"path","start","end"}…]}`
  — 스코어러의 `list[(path,start,end)]` 입력으로 바로 변환 가능.
- 태스크당 인덱싱 1회, 스윕 설정은 영속 인덱스 복원 경로로 리플레이.
  whole-file 노드(`kind=file`)와 `--max-region-lines` 초과 스팬은 라인 예산
  보호를 위해 제외.

### 3.6 비고

SWE-Explore는 코드 탐색 품질만 측정한다 — DB 크로스 엣지의 가치는 §2의
db-trace 시나리오가 담당한다(공개 벤치에 DB 스키마 저장소가 없으므로
자체 픽스처 기반이 정직한 최선).

## 4. 비목표

- 라이브 DB 접속(불변 원칙) · 컬럼 수준 엣지(테이블 접힘 유지) ·
  새 MCP 도구 · LLM-in-the-loop 평가 · ORM 전 종 지원(§1.3 제외 열은
  수요 확인 후) · tsconfig paths 해석(별도 트랙).

## 5. 열린 질문

1. JPA implicit naming(어노테이션 없는 `@Entity` → 물리명 전략 camel→snake)
   을 0.8 관례 티어에 포함할지 — Spring 기본 전략만 지원하는 절충안.
2. 코드 → DB 함수/프로시저 `Call` 엣지(`SELECT fn_x(...)` 감지) — 7b의 SQL
   리터럴 파서가 자연 확장점이나, 오탐률을 픽스처로 먼저 측정.
3. SWE-Explore 스코어러의 라이선스·제출 포맷 버전 고정(하니스에 커밋 해시 핀).
4. 관례 추론의 복수형 처리(`User` → `users`) — 영어 복수화 규칙을 넣을지,
   레지스트리 실존 매칭만으로 충분한지.

## 6. 마일스톤

| 단계 | 내용 | 의존성 |
|---|---|---|
| 7a | DBRef 추출(명시 매핑) + step 4 해석 + 픽스처/E2E | — |
| 7b | SQL 리터럴 + 무효화 + db-trace 벤치 | 7a |
| 7c | SWE-Explore 하니스 + 스윕 리포트 + 기본값 튜닝 | — (7a/7b와 병행 가능) |

7c의 스윕 결과가 `min_confidence`·`top_k` 기본값을 바꾸면 7a/7b의 크로스
엣지 노출량에도 소급 적용된다(같은 confidence 체계이므로 자동).
