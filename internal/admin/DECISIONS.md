# admin 디자인 적용 결정 — graphin 종속 판단

[DESIGN.md](DESIGN.md)는 디자인 **시스템**(원칙·토큰·컴포넌트 규격)이다. 이 문서는
그 시스템을 **graphin에 적용하면서 내린 판단**만 기록한다 — 채택/보류 현황, 승인된
예외, graphin에서만 성립하는 값.

두 문서를 섞지 않는다. DESIGN.md가 바뀌면 디자인 규격이 바뀐 것이고, 이 문서가
바뀌면 graphin의 적용 판단이 바뀐 것이다. **DESIGN.md 규격과 현재 코드가 다르면
버그가 아니라 §2의 의도적 차이일 수 있다 — 여기를 먼저 확인한다.**

**적용 완료 (2026-08-04).** §6 구현 순서 1~6단계를 모두 적용했다. 남은 미적용은
쓰기 지원이 없어서 대상 자체가 없는 항목뿐이다(§5).

---

## 1. 결정 로그 (2026-08-04)

| # | 결정 | 근거 |
|---|---|---|
| D1 | **§5 좌측 사이드바 레이아웃을 채택한다** | 상단 스티키 네비에서 전환. `layout.html` 전면 개편 + 7개 페이지 재배치가 따라온다. |
| D2 | **CDN `@import`를 쓰지 않는다. Pretendard 가변 폰트를 `static/`에 벤더링한다** | `embed.go`가 `templates`·`static`을 바이너리에 넣는다 — admin은 오프라인 동작이 요구사항이다. 게다가 `server.go:86`이 `Content-Security-Policy: default-src 'self'`를 강제하므로 CDN 요청은 **선택이 아니라 차단**된다(§4-E3). 실제 벤더링 크기는 D5 참조. |
| D3 | **디자인 시스템과 적용 결정을 문서로 분리한다** | DESIGN.md는 원문 규격을 그대로 보존하고, graphin 종속 판단은 전부 이 문서로 모은다. |
| D4 | **저장 위치는 `internal/admin/`** | 규격을 적용할 코드 바로 옆. `embed.go`의 지시자는 `//go:embed templates static`이라 이 `.md`들은 바이너리에 들어가지 않는다 — 문서를 여기 둬도 바이너리가 커지지 않는다. |
| D5 | **Pretendard는 전체 빌드가 아니라 Std 빌드를 벤더링한다** | 전체 가변 폰트는 **2.1MB**로 D2에서 예상한 ≈1MB의 두 배였다. Std(KS X 1001 한글 2780자)는 **292KB** — 7배 작다. Pretendard가 그리는 것은 UI 크롬뿐이고 식별자·경로·코드는 전부 모노스페이스 스택이라 전체 자모가 필요 없다. 미수록 글자는 스택의 시스템 한글 폰트로 폴백된다. |
| D6 | **자체 스크립트 `static/admin.js`를 둔다** | v1의 "JS는 벤더링한 htmx뿐" 방침에서 벗어나는 결정. CSP가 `hx-on:*`(eval 필요)를 막으므로 규격이 요구하는 동작 — 팝오버 닫기(§4.3), 트리 접기(§4.4), 코드 복사(§4.5), 테마 토글(§5) — 을 구현하려면 외부 파일이 유일한 경로다. 의존성은 없고 문서 레벨 위임 하나로 처리해 htmx가 DOM을 갈아끼워도 다시 붙일 필요가 없다. |
| D7 | **`/browse` 루트는 트리, `?pkg=`는 표로 남긴다** | §7의 "주 표현 = 들여쓰기 트리, 보조 A = 관계 표"를 그대로 구현한 형태. 대시보드·노드 breadcrumb가 이미 `/browse?pkg=`로 깊은 링크를 걸고 있어 표 뷰를 없애면 그 링크가 전부 깨진다. |

---

## 2. 적용 현황

토큰은 `static/theme.css`(§2 전용), 컴포넌트와 도메인 클래스는
`static/custom.css`. 로드 순서는 `pico.min.css → theme.css → custom.css`.

| 영역 | 규격 (DESIGN.md) | 현재 코드 | 상태 |
|---|---|---|---|
| 에셋 | Pico 2 classless 벤더링 | `static/pico.min.css` **v2.1.1** | 충족 |
| htmx | htmx 2 | `static/htmx.min.js` **v2.0.6** | 충족 |
| 서버 응답 | HTML 프래그먼트만 | `partials/` 전용, JSON 렌더링 없음 | 충족 |
| 병기 규칙 | 한국어 우선 + 원어 `<code>` | `labels.go` 단일 용어 사전 | 충족 |
| 폰트 | Pretendard `@font-face` | `PretendardStdVariable.woff2` 벤더링 | 충족 (D5) |
| 기준 크기 | `--pico-font-size: 87.5%` | theme.css에서 87.5% | 충족 |
| 토큰 | `--c-*` 색 토큰 체계 | theme.css `:root` + 다크 2경로 | 충족 |
| 레이아웃 | 사이드바 220px, `body` 100vh | `.sidebar` + `main` 2행 그리드 | 충족 (D1·§3.5) |
| 배지 | 점 + 라벨, radius 4px | `.badge` + `.badge::before` 점 | 충족 |
| 키/값 | `dl` 2열, 값 좌측 mono | 전 화면 `dl`, 수치만 `.num` | 충족 |
| kv-table | 4열(키/값/출처/액션) | 3열 — 액션 열 제외 | 의도적 차이 (§5) |
| 트리 | `.tree` 들여쓰기 트리 | `/browse` 3단계 지연 로드 트리 | 충족 |
| 소스 뷰어 | `.code` 48px 행번호 열 | `codeLines`로 실제 행번호 + 복사 버튼 | 충족 |
| 로그 | `.log` 5열 그리드 | `.log` **3열** — 레코드가 3필드뿐 | 의도적 차이 |
| 용어 도움말 | `.help` + `.popover` htmx | `/help/{term}` + `helpTerms` 14종 | 충족 |
| 폼 컨트롤 | §4.7 전체 | 포커스 링·mono만 (편집 폼 없음) | 부분 (§5) |

### 규격을 벗어난 클래스

DESIGN.md는 "여기 정의된 것 외의 클래스는 새로 만들지 않는다"고 못박는다.
`.topbar` `.navsep` `.navside` `.state` `.logtable`은 §5·§4 적용으로 제거됐다.

존속하는 것은 **규격에 대응 컴포넌트가 없는 도메인 클래스**뿐이다 — `.cards`
`.tabs` `.controls` `.edgelist` `.confbadge` `.etype` `.chart` `.legend`
`.bigram` `.crumbs` `.el` `.ncirc` `.nlabel` `.egolegend`, 그리고 규격 컴포넌트를
graphin 데이터에 맞춰 조립하는 구조 클래스(`.sidebar` `.pagehead` `.pagebody`
`.tnode` `.tlabel` `.codehead` 등). 전부 §4 예외 아래에 있다.

`.hl`(코드 강조 행)·`.str`/`.bool`/`.off`(dd 변형)·`.field-error`는 규격에 정의돼
있으나 아직 쓰는 화면이 없다. 규격 원문에 있는 것이라 CSS에 남겨 둔다.

---

## 3. graphin 종속 규격

DESIGN.md가 "구현자가 정의한다"고 남겨둔 자리를 채운 값.

### 3.1 사이드바 그룹 (DESIGN.md §5)

규격의 Operations / Registry / Configuration을 graphin 화면에 매핑한다:

| 그룹 | 항목 | 라우트 |
|---|---|---|
| 관측 | 대시보드 / 진단 / 로그 / 계측 | `/`, `/diagnostics`, `/logs`, `/usage` |
| 구조 | 구조 / 검색 | `/browse`, `/search` |
| 설정 | 설정 | `/settings` |

`/node`는 구조·검색의 상세 화면이므로 사이드바 항목이 아니다.
사이드바 최상단에는 `graphin` + 워크스페이스 상태 배지 + `Root` 경로를, 최하단에는
버전과 테마 토글을 둔다(P3).

### 3.2 상태 어휘 (DESIGN.md §4.1)

`internal/workspace/fsm.go`의 3상태를 규격의 5색 축에 매핑한다 — 5개 이하 제한
충족:

| raw | 한국어 라벨 (`labels.go`) | 배지 |
|---|---|---|
| `not_bootstrapped` | 부트스트랩 전 | `.badge.neutral` |
| `indexing` | 인덱싱 중 | `.badge.info` |
| `ready` | 준비됨 | `.badge.ok` |

라벨은 한국어, 배지 텍스트도 한국어로 낸다. `labels.go` 주석의 정책대로 CSS
클래스·URL 파라미터·노드 ID에는 raw 값을 유지한다.

### 3.3 트리 지표 열의 의미 (DESIGN.md §4.4)

규격의 트리 4열은 `노드 / 종류 / 호출 / 상태`이고 세 번째는 "노드당 지표"다.
graphin 트리는 깊이마다 행의 성격이 달라 한 지표로 안 묶인다:

| 깊이 | 행 | 지표 |
|---|---|---|
| 0 | 패키지(샤드) | 품고 있는 노드 수 |
| 1 | 파일 | 품고 있는 노드 수 |
| 2 | 노드 | 나가는 참조 수 (`len(Uses)`) |

헤더 한 칸으로 둘 다 정확히 부를 수 없으므로 **각 칸이 `title`로 무엇을 센
값인지 밝힌다**("노드 12개" / "참조 3개"). 헤더에도 두 의미를 적은 `title`을
단다. `TestTreeMetricNamesWhatItCounts`가 이 성질을 지킨다.

### 3.4 사이드바 상태 배지는 폴링하지 않는다 (DESIGN.md §5)

규격은 사이드바 최상단에 서버 정체성 + 상태 배지를 요구한다. graphin은 그 배지를
**페이지 렌더 시점 값으로만** 낸다 — 인덱싱 중 페이지를 열어 두면 낡는다.

폴링을 붙이지 않은 이유는 §4-E4다: 규격이 실시간 갱신을 "실제로 필요한 곳에만"
허용하고, graphin은 그 자리를 대시보드 상태 카드(2s)와 로그(3s) 둘로 못박았다.
세 번째 폴러를 전 페이지에 깔면 그 결정이 무의미해진다. 대신 살아 있는 값이
필요한 시나리오(USE_CASES.md UC-1)는 대시보드를 보라고 문서에 명시했다.

### 3.5 "페이지 무스크롤"의 적용 범위 (DESIGN.md §5)

규격은 페이지 자체가 스크롤하지 않는다고 규정한다. graphin에서 이 원칙이 성립하는
범위:

- **적용**: 사이드바, 헤더, KPI 영역, 최근 트레이스 — 1440×900에서 스크롤 없이 전부 보인다.
- **내부 스크롤 허용**: 트리/목록 패널, 상세 패널 본문, 소스 뷰어(`pre.code`가
  이미 `max-height: 34rem`), 로그 본문, 검색 결과.

소스 코드·로그·구조 목록은 본질적으로 길이 제한이 없으므로 이들을 페이지 스크롤로
흘리지 않고 패널 내부 스크롤로 가둔다 — 규격 위반이 아니라 규격이 말한 "트리와
패널 본문 안에서만 허용"의 graphin판 목록이다.

---

## 4. 승인된 예외

### E1. ego-graph SVG — 노드-엣지 다이어그램 (DESIGN.md §7)

규격은 "노드-엣지 다이어그램은 채택하지 않는다(비결정적 레이아웃, 라벨 가독성)"고
쓴다. **graphin은 예외로 유지한다.**

`internal/admin/ego.go`의 ego-graph는 v1의 핵심 기능이고, 규격이 지적한 두 단점에
해당하지 않는다: 중심 노드 1홉 고정 배치라 레이아웃이 결정적이고, 노드 수가
상한으로 잘려 라벨이 겹치지 않는다. 규격이 배제한 건 임의 위상의 force-directed
그래프다.

단 §7의 **보조 표현은 함께 제공한다** — 관계 표(`.edgelist`)가 이미 그 역할이며,
ego-graph는 보조가 아니라 병렬 표현으로 둔다.

### E2. 도메인 색상표 10종 (DESIGN.md §1 금지·§4.1 5색 제한)

규격은 색을 5개 의미 축으로 제한하고 "색만으로 상태 전달"을 금지한다. graphin은
그 위에 **의미 축이 아닌 분류 축** 색을 쓴다:

- confidence 4종 — `certain` / `samepkg` / `imported` / `global`
- 엣지 유형 6종 — `call` / `import` / `extends` / `implements` / `reference` / `foreign_key`

이건 상태가 아니라 범주라서 5색 축에 접히지 않는다. 대신 **색만으로 전달하지
않는다는 규칙은 그대로 지킨다**:

- `.confbadge`는 숫자를 함께 낸다 → 색 + 텍스트.
- `.etype`은 유형명 텍스트를 함께 낸다 → 색 + 텍스트.
- ego-graph SVG의 엣지 선(`.el.*`)은 **범례 + 선별 `<title>`** 로 해소했다
  (2026-08-04). `.egolegend`가 그려진 유형만 골라 색 견본과 한국어 이름을 짝지어
  보여주고, 각 `<line>`에 `<title>호출 · 신뢰도 0.95</title>`가 붙는다.

  당초 계획했던 **유형별 dash 패턴은 채택하지 않았다** — `.el.edashed`는 이미
  낮은 신뢰도(≤0.80)를 뜻하고 `ego_test.go`가 그 의미를 단언한다. 같은 채널에
  유형을 얹으면 두 축이 충돌한다. 신뢰도는 dash·선 굵기·투명도 세 채널을
  이미 쓰고 있으므로, 유형에는 패턴 대신 텍스트를 줬다.
  `TestEgoEdgeTypeIsNotColourOnly`가 이 성질을 지킨다.

### E3. 엄격 CSP가 규격을 제약하는 지점 (DESIGN.md §2·§4.3)

`server.go:86`이 모든 응답에 `Content-Security-Policy: default-src 'self'`를
붙인다. `unsafe-inline`도 `unsafe-eval`도 없고, `polish_test.go`가 이를 단언한다.
규격 중 여기 걸리는 것:

- **CDN `@import`·외부 폰트** — 차단된다. D2가 취향이 아니라 필수인 이유.
- **인라인 `<style>`** — 차단된다. 토큰은 반드시 외부 `theme.css`로 낸다(규격도
  그렇게 쓴다). 값이 동적인 스타일은 SVG 표현 속성이나 `<progress>`로 우회한다 —
  ego-graph와 usage 차트가 이미 그 방식이다.
- **§4.3의 `hx-on:click`** — htmx의 `hx-on:*`는 속성 문자열을 함수로 컴파일하므로
  `unsafe-eval` 없이는 동작하지 않는다. 팝오버 닫기는 규격이 함께 제시한
  **document 레벨 클릭 핸들러**로 구현하고, 그 핸들러는 벤더링한 외부 `.js`에 둔다.

### E4. 실시간 폴링 (DESIGN.md §4.6)

규격은 "실시간 갱신이 실제로 필요한 곳에만" 폴링을 허용한다. 현재 두 곳이 이
조건을 충족한다고 본다:

- `dashboard.html` `#status` — `every 2s`. 인덱싱 진행 상태라 갱신이 목적 그 자체.
- `logs.html` `#logbody` — `every 3s`. 로그 tail.

그 외 화면에는 폴링을 추가하지 않는다.

---

## 5. 보류 — 쓰기 지원 전까지 미적용

admin은 **읽기 전용**이다 — v1 확정 결정이고, `server.go`의 라우트가 전부 `GET`이다.
다음 규격은 규격으로서 유효하되 적용 대상이 아직 없다:

- **§6 설정 편집 (그룹 폼)** — `/settings`는 값을 표시만 한다. `hx-put`/`revert`/
  `save all`/`row-changed`는 쓰기 지원이 생길 때 적용한다. 단 §6의 **기본값 대비
  표시**(라벨 승격 + 취소선 원값 + `overridden` 태그)는 읽기 전용으로도 뜻이
  통하므로 지금 적용했다.
- **§4.7 폼 컨트롤** — 편집 입력이 없다. 검색·필터의 `input`/`select`에 서체·
  포커스 링 규칙만 적용했다(`.field-error`·`aria-invalid`는 정의만 남김).
- **§8의 `hx-put`·인라인 편집·`hx-confirm` 행** — 위와 같다.
- **§4.2-C `.kv-table`** — 출처(`default`/`env`/`override`)·액션 열이 필요한
  화면이 아직 없다. `/settings`에는 액션 열을 뺀 읽기 전용형으로 적용한다.

읽기 전용 유지 자체가 결정이다 — 관찰 모드에서 admin이 상태를 바꾸면 계측 신호가
오염된다(`docs/usage-spec.md` §8과 같은 이유).

---

## 6. 구현 기록 (2026-08-04 완료)

1. **토큰 레이어** — `static/theme.css` 신설(§2 전량), Pretendard Std 벤더링
   (+`pretendard.LICENSE`, OFL 1.1), `layout.html` `<link>` 추가, 기준 크기
   93.75%→87.5%. `--pico-*` 매핑은 Pico의 테마 블록과 같은 특이도로 재선언해야
   덮이지 않는다(theme.css §3 주석 참조).
2. **레이아웃** — `layout.html`을 `aside.sidebar` + `main` 구조로 개편.
   각 페이지가 `{{define "pagehead"}}`로 h1을 넘기고, 본문만 `.pagebody`에서
   스크롤한다. 사이드바 배지·카운트를 위해 `pageVM`에 `State`·`Nav`를 추가.
3. **공통 컴포넌트** — `.badge`(점+라벨 5종), `.tag`, `dl` 2열. `.state`와 알약형
   배지 제거, `dl.kv` 우측 정렬을 `.num` 옵트인으로 전환.
4. **화면별** — `.log` 3열 그리드, `.code` 행번호+복사(`codeLines`/`langOf`),
   `/browse` 지연 로드 트리(`handlers_tree.go` + `/partial/tree`),
   `/settings` 읽기 전용 `.kv-table`(`cfgRows`).
5. **용어 도움말** — `/help/{term}` + `helpTerms` 14종 + `{{term}}` 헬퍼.
6. **E2 해소** — ego 범례 + 선별 `<title>` (dash 패턴은 채택 안 함, §4-E2).

새로 추가한 회귀 방지 테스트:

- `help_test.go` — 템플릿의 `{{term "키"}}`가 전부 사전에 있는지(오타 가드),
  전 용어의 `/help` 200, 미등록 용어 404, 라벨 이스케이프.
- `ego_test.go` — `TestEgoEdgeTypeIsNotColourOnly`, `TestBuildLegendOrderAndDedup`.
- `logs_test.go` — 오류 행이 클래스와 '오류' 태그로 이중 표시되는지.
- `dbxref_test.go` — UC-8(코드↔DB 추적). 테이블 노드가 검색되는지, `used_by`
  한 홉이 JPA 엔티티와 SQL 리터럴 양쪽에 닿는지, 그리고 두 경로의 신뢰도가
  USE_CASES.md가 설명하는 티어(명시 물리명 1.00 / SQL·ORM 0.90)와 같은지.
  마지막 단언이 핵심이다 — 티어가 흔들리면 문서가 거짓이 되므로 여기서 먼저
  실패해야 한다. 픽스처는 `testdata/fixtures/dbxref`를 재사용한다.

### 남은 후속 작업

- **성능**: `pageVM`이 모든 페이지에서 `cachedHealth()`(O(nodes+edges))를 부른다.
  10초 메모이즈라 연속 이동은 싸지만, 캐시가 식은 첫 요청은 전수 순회를 문다.
  대형 저장소에서 체감되면 사이드바 카운트를 별도 폴링 파셜로 분리한다.
- **미사용 규격**: `.hl`(코드 강조 행)을 쓸 화면이 없다 — 검색 결과에서 노드로
  이동할 때 해당 라인을 강조하면 자연스럽게 쓰인다.
- **1440×900 실측**: 브라우저가 없는 환경이라 렌더 마크업으로만 검증했다.
  P4("핵심 지표가 스크롤 없이")는 실제 뷰포트에서 한 번 확인이 필요하다.
