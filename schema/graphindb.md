# graphindb 스냅샷 포맷 명세 (v1)

RDB 스키마를 graphin 그래프에 인제스트하기 위한 **저장소 커밋형 스냅샷** 규약.
graphin은 라이브 DB에 접속하지 않는다 — tbls/Atlas/`graphin dbimport`/수기 작성으로
생성된 스냅샷 파일이 저장소에 존재하면 워처가 여느 소스 파일처럼 감지·인덱싱한다.

인라인 스냅샷 외에, 프로젝트에 **이미 존재하는 SSOT**(schema.sql 덤프,
prisma 스키마, 프로젝트 고유 JSON)를 그대로 파싱하는 **매니페스트 라우팅**을
지원한다 — 아래 §매니페스트 참조.

## 매니페스트 (`graphindb.json`)

베이스네임이 정확히 `graphindb.json`인 파일(위치 자유, 루트/`db/` 권장)이
데이터소스별 SSOT를 선언한다. **SSOT 파일 자체가 파스 타깃**이라 노드 스팬이
실제 CREATE TABLE 문·model 블록을 가리키고, `read_code`가 그 원문을 반환한다.
저장소당 매니페스트 1개(중복 시 사전순 첫 파일이 활성, 나머지 경고).

```json
{
  "version": 1,
  "datasources": {
    "main": {
      "engine": "postgresql",
      "default_schema": "public",
      "sources": [
        { "path": "db/schema.sql", "format": "sql" },
        { "path": "db/main.rls.graphindb.json" }
      ]
    },
    "app":    { "engine": "postgresql", "sources": [{ "path": "prisma/schema.prisma" }] },
    "legacy": { "engine": "postgresql", "sources": [{ "path": "db/tbls.json", "format": "json" }],
                "json": { "preset": "tbls" } }
  }
}
```

- `format` 생략 시 확장자 추론: `.sql`→sql, `.prisma`→schema, `.json`→json,
  `.graphindb.json`→graphindb(인라인). 한 데이터소스가 포맷이 다른 소스를
  합칠 수 있다(예: DDL + 수기 RLS 사이드카) — FQN 병합·중복 경고는 동일.
- 경로는 저장소 상대·`/` 구분·정확 일치(글롭 없음). 서로 다른 데이터소스가
  같은 경로를 주장하면 사전순 첫 선언이 이기고 오류로 보고된다.
- 검증 오류는 `bootstrap_workspace` 응답의 `db_manifest_errors` 속성과 obs
  로그로 노출된다 — 에이전트는 이를 보고 매핑을 수정하며 수렴한다.
- 매니페스트 변경은 워처가 감지해 라우팅을 diff하고, 영향받은 파일만 강제
  재인덱싱한다(라우트가 해제된 파일은 plain 파일 노드로 강등).

### format: "sql" — 상태형 DDL

`schema.sql` 덤프·flyway 베이스라인 등 **현재 상태를 기술하는** DDL 파일.
마이그레이션 이력 재생(폴드)은 지원하지 않는다 — 이력만 있는 저장소는
베이스라인 덤프(`pg_dump --schema-only` 등)를 SSOT로 커밋할 것.

- 인식: `CREATE TABLE/VIEW/FUNCTION/PROCEDURE/TRIGGER/POLICY`,
  `ALTER TABLE … ADD CONSTRAINT … FOREIGN KEY`(같은 파일의 테이블에 병합;
  병합된 ALTER 원문은 테이블 해시에 섞여 증분을 보존한다).
  `INDEX/GRANT/COMMENT/TYPE` 등은 무시. 인용부호·주석·`$tag$` 달러 인용 인식.
- SQL 소스의 RLS는 **정책문당 1노드** `db.<ds>.<schema>.<table>.rls.<policy>`
  (인라인 사이드카의 테이블당 번들과 ID 층이 다르다).
- 구조적으로 깨진 CREATE는 건너뛰고 `partial` 마킹. 미인식 문장은 무시.

### format: "schema" — Prisma 서브셋

`model`/`view` 블록 → 테이블/뷰 노드. `@@map`(테이블명)·`@@schema`(스키마)·
필드 `@map`(컬럼명) 존중, `@relation(fields:…)` 소유측 → FK 엣지.
`enum`·composite `type`·`datasource`·`generator`는 v1 제외.

### format: "json" — 프리셋 + 셀렉터 DSL

- `"json": { "preset": "tbls" }` — tbls `schema.json`을 직접 파싱(루트
  relations의 `virtual`→`enforced:false`, VIEW 판별, 중첩 트리거 포함).
- 커스텀 구조는 최소 셀렉터 DSL로 선언한다:

```json
"json": { "mapping": {
  "tables": "tables.*",           // 컬렉션: "a.b[]" 배열, "*" 키 순회(이름 캡처)
  "table_name": "key",            // "key" | "field:<f>" ("schema.table"은 자동 분해)
  "columns": "columns[]",         // 테이블 상대 경로
  "column_name": "field:name", "column_type": "field:type",
  "foreign_keys": "fks[]",
  "fk_references": "field:references",  // [db.<ds>.]<schema>.<table>[.<col>]
  "fk_enforced": "field:enforced"       // false → 논리 참조(0.9)
} }
```

스키마 판정 우선순위: 이름의 `schema.` 프리픽스 → 중첩 `*` 캡처(바깥 키) →
`default_schema`. 매핑이 가리키는 테이블 값의 JSON 블록이 그대로 노드
스팬이다.

## 파일 규약

```
<name>[.<section>].graphindb.json
```

- 감지는 접미사 `.graphindb.json` 기준(디렉터리 위치 자유, `db/` 권장).
- `<section>` ∈ `functions` | `rls` | `triggers`. 생략 시 **메인 파일**(테이블·뷰).
- **데이터소스** 판정: `_meta.datasource`가 권위, 없으면 `<name>` 스템이 폴백.
- 한 저장소에 데이터소스 N개 공존 가능(파일 세트를 데이터소스별로 나란히 둔다).
- 같은 `_meta.datasource`를 가진 메인 파일 여러 개 허용(대형 DB의 스키마별 분할).
  단 테이블 FQN 집합은 서로소여야 하며, 중복 선언은 경고 후 last-writer-wins.
- 파일당 1MB 미만 유지(스캐너 상한). 초과 시 스키마별 파일로 분할한다.

## `_meta` (모든 파일 공통)

```json
{
  "_meta": {
    "datasource": "main",
    "engine": "postgresql",
    "synced_at": "2026-07-22T00:00:00Z",
    "source": "tbls",
    "latest_migration": "20260720020000_patch_note.sql",
    "generator": "tbls v1.79 + graphin dbimport"
  }
}
```

- `datasource`(필수 권장) · `engine`(`postgresql`|`sqlite`|`oracle`|`mysql`|…) ·
  `synced_at`(ISO 8601) · `source`(`tbls`|`atlas`|`manual`|`migration`).
- `latest_migration`은 드리프트 대조 앵커(flyway `flyway_schema_history` /
  supabase `migration list`의 최신 항목). 선택.
- 산문 이력을 `_meta`에 축적하지 않는다(프로버넌스는 필드로, 이력은 git으로).
- 미지의 필드는 무시된다(전방 호환).

## 메인 파일 — `schemas` → 테이블·뷰

```json
{
  "_meta": { "...": "..." },
  "schemas": {
    "public": {
      "tables": {
        "job_posting": {
          "comment": "채용 공고",
          "columns": [
            { "name": "id", "type": "bigint", "nullable": false,
              "default": "GENERATED ALWAYS AS IDENTITY", "constraints": ["PK"] },
            { "name": "company_id", "type": "bigint", "nullable": false },
            { "name": "title", "type": "varchar(200)", "nullable": false,
              "comment": "공고 제목" }
          ],
          "foreign_keys": [
            { "column": "company_id", "references": "public.company.id",
              "on_delete": "CASCADE" }
          ],
          "indexes": [
            { "name": "idx_job_posting_company", "columns": ["company_id"] }
          ],
          "checks": [
            { "name": "chk_status", "definition": "status IN ('open','closed')" }
          ]
        }
      },
      "views": {
        "v_active_job_posting": {
          "comment": "활성 공고",
          "definition": "SELECT ... FROM job_posting jp JOIN company c ...",
          "references": ["public.job_posting", "public.company"]
        }
      }
    }
  }
}
```

- 컬럼·인덱스·제약·체크는 테이블 블록에 **접는다**(개별 노드 아님). 상세는
  `read_code`가 해당 테이블 JSON 블록을 그대로 반환한다.
- `foreign_keys[].references` 형식:
  `[db.<datasource>.]<schema>.<table>[.<column>]`
  - `db.` 프리픽스가 없으면 같은 데이터소스 내 참조.
  - `db.<ds>.` 프리픽스는 **크로스 데이터소스**(FDW, 서비스 간 논리 참조).
  - 스냅샷에 없는 대상(예: supabase `auth.users`)도 그대로 적는다 — 엣지는
    생성되고 노드만 부재(dangling 허용, 스텁 미생성).
- `enforced: false` + `note`: 물리 FK가 아닌 **논리/다형성 참조** 표기.
  ```json
  { "column": "target_id", "references": "public.resume.id",
    "enforced": false,
    "note": "polymorphic — target_kind='resume'일 때만; 소유권은 RLS로 강제" }
  ```
- `views[].references`를 생략하면 `definition` 토큰에서 휴리스틱 추출된다
  (명시가 항상 우선, confidence도 명시가 높다).

## `functions` 파일 — 함수·프로시저

```json
{
  "_meta": { "...": "..." },
  "schemas": {
    "public": {
      "functions": {
        "fn_company_job_count": {
          "args": "company_id bigint",
          "returns": "integer",
          "language": "sql",
          "comment": "회사별 공고 수",
          "references": ["public.job_posting"],
          "definition": "SELECT count(*) FROM job_posting WHERE ..."
        }
      },
      "procedures": {
        "prc_archive_job_posting": {
          "args": "before date",
          "language": "plpgsql",
          "definition": "..."
        }
      }
    }
  }
}
```

- `security`(`definer`|`invoker`) 선택 필드(supabase 트리거 함수 관행 기록용).
- `references` 생략 시 `definition` 토큰 매칭으로 휴리스틱 추출.

## `rls` 파일 — 테이블별 RLS 정책

```json
{
  "_meta": { "...": "..." },
  "schemas": {
    "public": {
      "resume": {
        "enabled": true,
        "policies": [
          { "name": "resume_self_read", "command": "SELECT",
            "roles": ["authenticated"],
            "using": "auth.uid() = user_id" },
          { "name": "resume_self_insert", "command": "INSERT",
            "roles": ["authenticated"],
            "with_check": "auth.uid() = user_id" }
        ]
      },
      "company": { "enabled": false, "policies": [] }
    }
  }
}
```

- 정책 묶음이 **테이블당 1노드**(`…<table>.rls`)가 된다. `enabled: false`인
  테이블도 기록(RLS 미적용이 정보다) — 단 노드는 정책이 있거나 enabled일 때만.

## `triggers` 파일 — 테이블별 트리거

```json
{
  "_meta": { "...": "..." },
  "schemas": {
    "public": {
      "job_posting": [
        { "name": "trg_job_posting_updated_at",
          "timing": "BEFORE", "events": ["UPDATE"], "level": "ROW",
          "function": "public.tg_job_posting_set_updated_at",
          "definition": "CREATE TRIGGER ..." }
      ]
    }
  }
}
```

- `function`은 `[db.<ds>.]<schema>.<function>` FQN — 트리거→함수 Call 엣지의 근거.

## 노드·엣지 매핑 (인덱서 동작 규정)

| 소스 | 노드 | kind | ID | display_name |
|---|---|---|---|---|
| 테이블 | 1/테이블 | `table` | `db.<ds>.<schema>.<table>` | `<ds>.<schema>.<table>` |
| 뷰 | 1/뷰 | `view` | 동일 스킴 | 동일 스킴 |
| 함수 | 1/함수 | `db_function` | `db.<ds>.<schema>.<fn>` | 동일 스킴 |
| 프로시저 | 1/프로시저 | `procedure` | 동일 스킴 | 동일 스킴 |
| RLS 묶음 | 1/테이블 | `rls_policy` | `db.<ds>.<schema>.<table>.rls` | `<ds>.<schema>.<table>.rls` |
| 트리거 | 1/트리거 | `trigger` | `db.<ds>.<schema>.<table>.<trigger>` | `<ds>.<schema>.<table>.<trigger>` |

엣지(§2.1.3 confidence 체계에 편입):

| 엣지 | 타입 | confidence |
|---|---|---|
| 테이블 → FK 대상 테이블 | `ForeignKey` | 1.0 (FQN 직접 해석) / `enforced:false`는 0.9 |
| 뷰·함수·프로시저 → 명시 `references` | `Reference` | 1.0 |
| 뷰·함수·프로시저 → definition 토큰 매칭 | `Reference` | 0.8 |
| RLS 노드 → 테이블 | `Reference` | 1.0 |
| 트리거 → 테이블 | `Reference` | 1.0 |
| 트리거 → 함수 | `Call` | 1.0 |

- 샤드 키 = `db.<datasource>` (사이드카 파일 전부 같은 샤드에 합류).
- 검색: 컬럼명·타입·주석·정책 내용은 노드 BodyTokens(JSON 블록 토큰화)로
  BM25에, 합성 요약문(`table … : columns …; references …`)으로 벡터 인덱스에
  들어간다.

## 결정성 (생성기 의무)

스냅샷 **생성기**(dbimport 등)는 재실행 시 바이트 동일 출력을 보장해야 한다:
키 정렬(스키마·테이블·함수 이름 사전순, `_meta` 최상단), 2-스페이스 들여쓰기,
후행 개행 1개. 흔들리는 출력은 전 노드를 "변경"으로 오판시켜 재임베딩을
유발한다(머클이 바이트 해시 기준). 수기 편집은 규칙 자유이나 같은 이유로
불필요한 재정렬을 피할 것.
