# 도구 계약 재설계 — 서브에이전트가 skill 없이 쓰는 graphin

상태: 초안 (2026-11-24) · 대상: `search_hybrid`, `search_keyword`, `explore_graph`, `read_code`

## 0. 왜

다른 프로젝트에서 graphin은 플러그인으로 들어와 **서브에이전트에 MCP 도구만 주입된 채** 쓰인다.
그 에이전트는 `graphin` skill도, 서버 `instructions`(현재 없음)도 보지 못하고 도구 설명과
응답만으로 판단한다. 지금 그 두 가지가 실패하는 지점은 셋이다.

1. **설명이 graphin 내부를 설명한다.** 색인 walk, 랭킹 방식, chunk, folding, 노드 소유 규칙이
   문장의 대부분이고, 정작 "무엇을 알아내는 도구인가 / grep·Read 대신 언제 쓰나 / 언제 쓰지
   말아야 하나"는 거의 없다.
2. **입력 형식이 정의되지 않았다.** `query`는 "Natural language or symbol query", `pattern`은
   "Text to find" 뿐이라, hybrid에 키워드 나열을 넣고 keyword에 산문 질문을 넣는다
   (실측: `search_keyword("how does search rank results")` → `files="0"`).
3. **도구가 서로에게 기댄다.** 설명이 다른 도구를 전제하고(`"Node ID from search_hybrid"`,
   `"flows straight into explore_graph and read_code"`), 응답은 ID와 위치만 돌려주며,
   `explore_graph`·`read_code`는 **불투명한 node id만** 입력으로 받는다
   (실측: `internal/mcp/tools/tools.go::keywordEmptyHint` → `NODE_NOT_FOUND`; 실제 형식은
   `internal.mcp.tools.keywordEmptyHint`). 일부 도구만 주입되거나 후속 호출이 이어지지 않으면
   답이 끊긴다. 이 저장소의 평가도 같은 원인을 기록했다 — "the second file found, named, and
   never cited" (`exploreHandler` 주석), "where is X에 두 번째 호출이 필요해 grep에 졌다"
   (`docs/eval/2026-08-07-adoption-diagnosis`).

## 1. 원칙

| # | 원칙 | 구체 규칙 |
|---|---|---|
| P1 | 목적 우선 설명 | 설명은 ①목적 ②쓸 때/쓰지 말 때(grep·Read 대비) ③입력 형식 ④돌려받는 것 ⑤해석에 필요한 기술 특성 최대 2개 순서. 구현(BM25, RRF, shard, chunk) 용어 금지. |
| P2 | 입력 형식은 예시로 못박는다 | 파라미터마다 올바른 예와 잘못된 예를 한 줄씩. |
| P3 | 도구는 독립적이다 | 설명·파라미터·`<hint>`에 **다른 graphin 도구 이름을 쓰지 않는다.** 필요하면 능력으로 말한다("an exact-text search such as grep"). |
| P4 | 사람이 읽는 이름으로 들어온다 | id를 받는 모든 곳은 심볼 이름, `Type.method`, `path`, `path:line`도 받는다. 모호하면 고르지 않고 후보를 돌려준다. |
| P5 | 연쇄 탐색이 기본값 | 응답 하나로 "판단 가능한 답"이 되도록 시그니처·스니펫·호출 위치·이웃 이름을 기본 포함한다. ID만 원하는 에이전트는 `detail="ids"`로 현행 응답을 받는다. |
| P6 | 12KB 상한 안에서 접힌다 | `mcp.MaxResponseBytes`(12KB)를 넘기면 자르지 않고 정해진 순서로 접는다(§3.5). 무엇이 접혔는지 응답이 말한다. |

## 2. 도구 설명 초안

설명은 모델이 읽으므로 영어로 유지한다(현행과 동일). 아래는 §3의 handler 변경이 들어간 뒤의
동작 기준이다. P1 단계(§4)에서 설명만 먼저 바꿀 때는 "Returns" 문단을 현행 동작으로 바꿔 쓴다.

### 2.1 `search_hybrid`

```text
Find code by what it does or by a name you remember, when you do not know the exact text in the file.

Use it for: "where is order cancellation handled", "what implements retry backoff", "how the watcher decides a file changed", or a symbol name such as applyFileResult. This is the search grep cannot do: it matches meaning and names, not only literal text.
Do not use it for: an exact string you could quote from the code (an error message, a config key, a literal) — an exact-text search answers that precisely; or for listing every occurrence of something.

Query: ONE identifier (parseConfig, OrderService.cancel) or ONE sentence describing behavior in the words code would use.
  good: "how does the lock get released after a failed write"
  good: "applyFileResult"
  bad:  "lock release write fail"   (keyword list — write it as a sentence)
  bad:  "\"lock released\""          (quoted text — that is an exact-text search)

Returns: up to top_k ranked matches, each with file:line, kind, signature and the lines around the match. The top match also carries its body (or the part around the hit, for long units) and the names of up to 3 callers, so most "where/how" questions end here. detail="ids" returns locations only.

Good to know: a sentence-shaped question also matches documentation; set target="code" for implementation only. Ranks are best guesses — check the snippet before trusting rank 1.
```

파라미터:

| 이름 | 설명 초안 |
|---|---|
| `query` | `One identifier or one sentence about behavior. Not a keyword list, quoted text or regex.` |
| `top_k` | `Max results (default 5, max 20).` |
| `target` | `code: implementation only (use for sentence questions about how code works). docs: markdown sections. db: tables/views. Omit to search everything, including config files.` |
| `detail` (신규) | `full (default): signature, snippet, top match body and callers. ids: id, file and line only.` |

### 2.2 `search_keyword`

```text
Find where an exact piece of text appears in the project, and which function or class it is in.

Use it for: text you can quote from the code — an error message, a log line, a config key, an environment variable, a constant, a route path, a string built at runtime.
Prefer it over grep when you want the enclosing function or class and the surrounding lines in one short, ranked answer.
Use grep instead when you need EVERY occurrence (renaming, counting, bulk edits — this returns at most 3 lines per file and 20 files), a case-sensitive match, or files outside the project.
Do not use it for questions or descriptions: it matches text, not meaning, and a sentence will almost never appear verbatim.

Pattern: a short fragment copied from code, 1–5 words, the part least likely to vary.
  good: "failed to acquire lock"   good: "MAX_RETRIES"   good: "/api/v1/orders"
  bad:  "how are retries configured"   (a question — describe it with a meaning search instead)
  bad:  "retry config backoff"         (a keyword list — pick the one exact string)
Matching is literal and case-insensitive. regex=true takes RE2 for a part that varies: "timeout after \d+s".

Returns: up to top_k files ranked by match count; per file up to 3 matching lines with 2 lines of context, each tied to the enclosing symbol's name, kind, signature and line range. Data files (json, lock, csv) are listed by path and count only; pass path= to see their lines.
```

파라미터:

| 이름 | 설명 초안 |
|---|---|
| `pattern` | `Exact text from the code, 1–5 words. Case-insensitive. Not a question or keyword list.` |
| `regex` | `Treat pattern as RE2. Off by default so dots and brackets in names match literally.` |
| `path` | `Only files whose path contains this substring, e.g. "internal/auth" or ".yaml".` |
| `top_k` | `Max files (default 5, max 20).` |
| `context` | `Lines around each match, 0–5 (default 2). 0 returns matched lines only.` |

### 2.3 `explore_graph`

```text
Show how a piece of code is connected: who calls or references it, and what it calls or depends on.

Use it for: "what calls X", "what breaks if I change X" (direction="used_by"), "what does X depend on" (direction="uses"), "which code reads or writes the orders table".
Prefer it over grepping a name: it follows resolved references (imports, method calls, inheritance, ORM table access, doc links), so same-named but unrelated symbols do not show up, and callers that use an alias do.
Do not use it to find where something is — give it a name you already have; an unknown name comes back with candidates.

Input: symbol = a name (cancelOrder), a qualified name (OrderService.cancel), a file path, or path:line for the unit enclosing that line. Ambiguous names return the candidates instead of guessing.
direction: used_by = callers/references (impact), uses = dependencies, both (default) = uses first, then used_by. When you need callers, ask for used_by: under both a busy function's callers can fall onto the next page.

Returns: up to 20 neighbours per page, each with name, kind, signature, the file:line of the reference itself and the line of code that makes it, plus a confidence (1.00 resolved; 0.85–0.95 resolved by name; lower is inferred). has_more means more exist — a short list is not proof of few callers.

Limits: references are known for Java, Kotlin, Python, JavaScript/TypeScript, Go, Markdown links and committed DB schemas. Dynamic dispatch, reflection and string-built calls are invisible: no edge is not proof of no relationship.
```

파라미터:

| 이름 | 설명 초안 |
|---|---|
| `symbol` (신규) | `Name, Type.method, file path, or path:line. Ambiguous names return candidates.` |
| `node_id` | `Same as symbol; kept for compatibility.` |
| `direction` | `used_by: callers/references. uses: dependencies. both (default): uses, then used_by.` |
| `min_confidence` | `Drop references below this (default 0.85). 0.75 for impact analysis where missing a caller costs more than a false one.` |
| `cursor` | `next_cursor from a previous page.` |
| `detail` (신규) | `full (default): signature and reference line per neighbour. ids: id, file and line only.` |

### 2.4 `read_code`

```text
Read the source of one function, class, method, doc section or DB table — exactly that unit, not the whole file.

Use it when you know which symbol you need and want its body: to confirm behavior, quote it, or check a caller.
Prefer it over reading the file: it returns just that unit with its line range, re-reads the file if it changed, and names who calls the unit and what it calls so you can keep going.
Do not use it to find where something is (search first), or to read a whole code file top to bottom (use a file read).

Input: symbol = a name (cancelOrder), a qualified name (OrderService.cancel), a file path, or path:line for the unit enclosing that line; symbols = up to 20 of those, read in order. lines = "120-180" reads only that stretch (max 200 lines) of a long unit.
Ambiguous names return the candidates instead of guessing.

Returns: the source with file and line range, plus the names and file:line of up to 5 callers and 5 callees. Several units come back whole, in order; any that did not fit are listed by name with the reason, to request again.
```

파라미터:

| 이름 | 설명 초안 |
|---|---|
| `symbol` (신규) | `Name, Type.method, file path, or path:line.` |
| `symbols` (신규) | `Up to 20 of the above, read in order. Use instead of symbol.` |
| `node_id` / `node_ids` | `Same as symbol / symbols; kept for compatibility.` |
| `lines` | `Only with symbol: "start-end" or "N", the file's own line numbers, max 200, clipped to the unit.` |
| `detail` (신규) | `full (default): source plus callers/callees names. code: source only.` |

### 2.5 응답 `<hint>` 문구 (P3)

현행 hint는 다른 도구 이름을 지시한다. 해당 도구가 주입되지 않은 에이전트에게는 따를 수 없는
지시다. 능력으로 바꾼다.

**구현 결정(1단계):** 도구 이름을 완전히 빼지 않고 조건부로 남긴다 —
`an exact-text search (search_keyword if you have it, otherwise grep)` (`exactTextSearch` 상수).
도구가 없는 에이전트도 다음 수가 있고, `eval-rag.py`가 hint를 도구 이름으로 세는 지표
(`hints_seen`/followed)와 `eval-recall.py`의 hint 분류 문구("quoted string", "no indexed symbol",
"in no indexed document", "no ranked result", "touched")가 그대로 유지된다.
`TestHintsNeverDependOnAnotherGraphinTool`이 이 규칙을 고정한다. 아래 표는 방향이고, 최종 문구는
코드가 기준이다.

| 위치 | 현행 | 초안 |
|---|---|---|
| `searchHint` quoted | `search_keyword finds the exact string` | `an exact-text search (grep, or this server's keyword search) finds the string itself` |
| `searchHint` unnamed ident | `search_keyword points at the declaration` | `an exact-text search for it finds the declaration` |
| `searchHint` empty | `search_keyword finds an exact string the ranking cannot` | `if you know exact text from the code, search for that text instead` |
| `keywordEmptyHint` | `If it is a name, search_hybrid finds the symbol` | `If you meant a concept or a name you are unsure of, a meaning search fits better than exact text` |
| `read_code` 미해결 이름 | (없음 — `NODE_NOT_FOUND`) | §3.1의 후보 목록 + `no unit is named X; search for what it does` |

## 3. handler 변경 설계

### 3.1 공통: 참조 해석기 `resolveRef` (P4)

`internal/mcp/tools/resolve.go` 신설. `explore_graph`, `read_code`가 공유한다.

```go
type Ref struct {
    IDs        []string // 1개면 확정, 여러 개면 후보
    LineFilter int      // path:line 입력일 때의 줄
}
func resolveRef(ws *workspace.Workspace, in string) (Ref, error)
```

해석 순서(첫 성공에서 멈춤):

1. **정확한 node id** — `ws.NodeLocation(id)` ok, 또는 `ws.ResolveNodeID`로 별칭 해석된 id.
2. **`path:line`** — 정규식 `^(.+):(\d+)$` 이고 path가 색인된 파일이면
   `ws.NodesInFiles([path])` + `workspace.NodeAtLine`으로 그 줄을 감싸는 가장 좁은 노드.
   감싸는 노드가 없으면 파일 노드.
3. **파일 경로** — 파일 노드 id는 경로이므로 1과 같은 경로로 해석된다(명시적으로 둔다).
4. **단순 이름** — `ws.Sym.Lookup(name)`.
5. **한정 이름 `A.b`, `pkg.A.b`, `A#b`, `A::b`** — 구분자를 `.`로 정규화하고 마지막 세그먼트로
   `Lookup` 한 뒤, id가 `"."+정규화문자열`로 끝나는 것만 남긴다.
6. **실패** — `NODE_NOT_FOUND` 대신 `<unresolved ref="..">`에 라우터 검색 상위 5개를
   후보로 담는다(`ws.Router.SearchFilteredStats(in, 5, nil)` 재사용). 에이전트는 다른 도구 없이
   다음 입력을 고를 수 있다.

후보가 2개 이상이면 **고르지 않는다.** 단, 비-테스트 후보가 정확히 하나이고 나머지가 모두
테스트 파일(`_test.go`, `test_*.py`, `*.spec.ts`, `src/test/` 등 — `isTestPath`)이면 그 하나를
고르고 `<resolved ... candidates="N" chose="non-test" />`를 남긴다 — 그 외 자동 선택은 없다.

**구현(2단계):** `internal/mcp/tools/resolve.go`, `internal/workspace/resolve.go`.
이름이 아닌 id로 들어오지 않은 호출은 응답 맨 앞에 `<resolved ref=".." id=".." />`를 붙여, 에이전트가
해석된 단위를 인용할 수 있게 한다. 정확한 id 입력은 이 줄이 없어 기존 응답과 같다. 모호하면
`AMBIGUOUS_REF` + `<candidates>`, 미해결이면 기존 `NODE_NOT_FOUND` 줄 그대로 + `<suggestions>`.
다중 읽기에서 모호한 ref는 `<omitted reason="ambiguous">`. usage 로그는 해석된 id를 `node_id`로,
입력한 이름을 `ref`로 기록해 search funnel 지표가 이어진다. `eval-rag.py`의 `invented_ids`는
`node_id` 입력만 센다 — `symbol`로 이름을 넘기는 것은 이제 정당한 입력이라 셀 대상이 아니다.

```xml
<ambiguous ref="Close" candidates="14">
  <candidate id="internal.workspace.Workspace.Close" kind="method" file="internal/workspace/workspace.go" line="512" signature="func (w *Workspace) Close()" />
  ...최대 10개, 비-테스트 우선, 그 다음 경로순
  <hint>several units are named Close; pass a qualified name such as Workspace.Close, or path:line</hint>
</ambiguous>
```

### 3.2 공통: 요약 빌더 `nodeSummary` (P5)

`internal/workspace/summary.go` 신설.

```go
type Summary struct {
    ID, Name, Kind, File string
    Line, EndLine        int
    Signature            string // 1줄, ≤200자
}
func (w *Workspace) Summary(id string) (Summary, bool)
func (w *Workspace) Snippet(id string, around, radius int) (from, to int, lines []string)
func (w *Workspace) RefLine(holderID, targetName string) (line int, text string, ok bool)
```

- **Signature**: 노드 소스 앞부분에서 언어별 규칙으로 잘라낸다.
  코드 = 첫 `{` 또는 `:`(Python) 직전까지, 여러 줄이면 공백 정규화해 1줄로.
  Markdown = 헤딩 줄. DB = `table orders (12 columns)`. 기타 텍스트 = 첫 비어있지 않은 줄.
  1단계는 `ReadCode` 슬라이스에서 **지연 계산**하고 프로세스 내 LRU(키: id+FileHash, 4096개)에
  캐시한다. 2단계(선택)에서 파서가 `parse.Node`에 기록하고 `NodeMeta.Signature`로 영속화한다.
- **RefLine (호출 위치)**: 엣지에 위치가 없으므로(`graph.Edge`는 Target/Type/Confidence뿐)
  holder 노드의 슬라이스에서 대상 단순 이름을 단어 경계로 찾는 첫 줄을 쓴다. 못 찾으면
  (ORM 암묵 접근, 상속 등) 생략하고 `ref_line` 속성을 내지 않는다 — 추측 줄은 내지 않는다.
  2단계(선택)에서 resolver가 엣지에 `SiteLine uint32`를 기록하면 이 휴리스틱을 대체한다.
- 비용: 파일 읽기는 노드 슬라이스 단위이고 응답당 최대 20+5개. 같은 파일은 요청 내에서
  한 번만 읽도록 요청 스코프 캐시(`map[relPath][]byte`)를 둔다.

### 3.3 도구별 변경

#### `search_hybrid`

| 항목 | 현행 | 변경 (`detail=full`, 기본) |
|---|---|---|
| 결과 노드 | id, display_name, rank, match_type, file, line, hit_line | + `kind`, `signature`, `<snippet>`(매칭 줄 ±3, hit_line 없으면 시그니처부터 6줄) |
| 1위 결과 | 동일 | + `<body>`: 노드 전체가 80줄·4KB 이하면 전체, 아니면 hit_line(또는 시작) 기준 60줄 창, `partial="true"` |
| 1위 결과 | 없음 | + `<callers>`: used_by(min 0.85) 상위 3개 이름·file:line |
| `detail=ids` | — | 현행 응답과 **바이트 단위 동일** |

```xml
<results semantic_ready="true" target="code" candidates="212" detail="full">
  <node id="internal.watch.Watcher.changed" name="changed" kind="method" rank="1" match_type="both"
        file="internal/watch/watcher.go" line="88" signature="func (w *Watcher) changed(ev fsnotify.Event) bool">
    <body lines="88-121"><![CDATA[...]]></body>
    <callers>
      <ref name="Watcher.loop" file="internal/watch/watcher.go" line="140" />
    </callers>
  </node>
  <node id="..." name="..." kind="func" rank="2" ... signature="...">
    <snippet lines="40-46"><![CDATA[...]]></snippet>
  </node>
  ...
</results>
```

#### `search_keyword`

| 항목 | 현행 | 변경 |
|---|---|---|
| `context` 기본값 | 0 | **2** (`context=0`이면 현행과 동일 출력) |
| 매칭 줄의 소유 노드 | `id`만 | + `owner` 이름, `kind`, `signature`, `owner_lines="a-b"` (파일 노드면 생략) |
| 색인 전(`node_ids="false"`) | id 없음 | 동일. 소유 정보 없이 줄과 문맥만 — 이 도구의 무색인 동작은 유지 |

변경 후 기본 응답은 바이트가 늘어난다. 파일당 예산 접기(`omitted="budget"`)는 기존 로직이
그대로 처리한다. 다만 기본 `context=2`에서 5개 파일이 12KB에 들어가는지 §5에서 측정한다.

#### `explore_graph`

| 항목 | 현행 | 변경 |
|---|---|---|
| 입력 | `node_id`(정확한 id) | `symbol` / `node_id` → `resolveRef`. 모호·미해결은 §3.1 응답 |
| 대상 | `<graph_context target="id">` | + 대상 자신의 `name`, `kind`, `file`, `line`, `signature` |
| 이웃 | id, type, confidence, file, line(이웃 **시작** 줄) | + `name`, `kind`, `signature`, `ref_line`·`<ref_text>`(참조가 일어나는 줄, §3.2) |
| 기본 페이지 | 20 | 20 유지. 12KB를 넘으면 signature부터 접는다(§3.5) |
| `detail=ids` | — | 현행 응답과 바이트 단위 동일(입력 해석만 확장) |

`ref_line` 기준: `used_by` 이웃은 **이웃(호출자) 안에서** 대상 이름이 나오는 줄,
`uses` 이웃은 **대상 안에서** 이웃 이름이 나오는 줄. 두 경우 모두 "참조가 적힌 곳"으로 의미가
같다.

```xml
<graph_context target="internal.mcp.tools.keywordEmptyHint" name="keywordEmptyHint" kind="func"
               file="internal/mcp/tools/tools.go" line="466" signature="func keywordEmptyHint(regex bool) string">
  <used_by>
    <node id="internal.mcp.tools.keywordHandler" name="keywordHandler" kind="func" type="call" confidence="1.00"
          file="internal/mcp/tools/tools.go" line="475" ref_line="624"
          signature="func keywordHandler(ws *workspace.Workspace) mcp.ToolHandler">
      <ref_text>fmt.Fprintf(&amp;sb, "\n  &lt;hint&gt;%s&lt;/hint&gt;", mcp.EscapeText(keywordEmptyHint(a.Regex)))</ref_text>
    </node>
  </used_by>
</graph_context>
```

#### `read_code`

| 항목 | 현행 | 변경 |
|---|---|---|
| 입력 | `node_id` / `node_ids` | + `symbol` / `symbols` → `resolveRef`. `path:line`이면 감싸는 노드를 읽음 |
| 출력 | `<code_block>` | + 블록 뒤 `<neighbors>`: used_by 상위 5, uses 상위 5의 이름·file:line (min 0.85) |
| 다중 읽기 | 예산 내 통째, 나머지 `<omitted reason="budget">` | 동일. `<neighbors>`는 **단일 읽기에서만** 붙여 예산을 코드에 쓴다 |
| `detail=code` | — | 현행 응답과 바이트 단위 동일 |

### 3.4 `detail` 값 정리

| 도구 | 기본 | 현행 동작 |
|---|---|---|
| `search_hybrid` | `full` | `ids` |
| `search_keyword` | `context=2` | `context=0` (별도 `detail` 불필요) |
| `explore_graph` | `full` | `ids` |
| `read_code` | `full` | `code` |

현행 동작 모드는 **바이트 단위로 기존 응답과 같아야 한다.** 기존 e2e 테스트는 이 모드로 옮겨
회귀 방지용으로 남긴다.

### 3.5 12KB 접기 순서 (P6)

응답이 `keywordBudget`(=`MaxResponseBytes`-512)을 넘으면 아래 순서로 접고, 접은 사실을
`<folded what="..." />`로 남긴다. 중간에서 잘라내는 `Truncate`는 이 경로에서 발생하지 않아야 한다.

- `search_hybrid`: ① 2위 이하 snippet → 시그니처만 ② 1위 body → hit 기준 30줄 창
  ③ callers 제거 ④ 하위 결과 id·위치만.
- `explore_graph`: ① ref_text 제거 ② signature 제거 ③ 그래도 넘치면 페이지 크기를 줄이고
  `has_more`로 넘김(엣지는 절대 조용히 버리지 않음).
- `read_code`: ① neighbors 제거 ② 이후 기존 규칙.

### 3.6 응답 상단 상태

`<system_status>`는 지금 `state!="ready"`일 때 모든 응답에 붙는다. 서브에이전트에게 필요한 건
"결과를 믿어도 되나"뿐이므로, `semantic_ready="false"`일 때 `search_hybrid`에만
`<hint>meaning search is still warming up; name-based matches only — prefer exact names for now</hint>`
를 붙이고 나머지 도구는 현행대로 둔다.

## 4. 단계

| 단계 | 내용 | 기본 응답 변화 | 위험 |
|---|---|---|---|
| **S1** | §2 설명 교체("Returns"는 현행 동작 기준), §2.5 hint 문구, `resolveRef`, `symbol` 파라미터 | 입력만 넓어짐. 출력 동일 | 낮음 |
| **S2** | `Summary`/`RefLine`, `explore_graph` full, `read_code` neighbors, `search_keyword` context=2 + owner | 바이트 증가 | 중간 — e2e·벤치 재기준 |
| **S3** | `search_hybrid` full(body·callers), 설명 "Returns" 최종본 | 가장 큰 바이트 증가 | 중간 — 비용 지표와 충돌 |
| S4 (선택) | 파서에 Signature, resolver에 엣지 SiteLine 영속화 | 없음(정확도 향상) | 스키마 변경 |

S1은 단독으로 가치가 있다. 오용(입력 형식)과 ID 추측 실패를 바로 줄인다. S2와 S3은 §5의 측정을
통과해야 기본값을 뒤집는다. 그 전에는 `detail=full`을 opt-in으로 둔다.

## 5. 측정과 승인 기준

`scripts/eval-rag.py`(서브에이전트 실제 실행)를 3개 설정으로 돌린다.
A = 현행, B = S1, C = S1+S2+S3. 모두 skill 없이 도구만 주입한다.

| 지표 | 의미 | C의 승인 기준 |
|---|---|---|
| 답변 완결률 | 인용 가능한 file:line을 담은 답의 비율 | A 대비 +10%p 이상 |
| 체인 단절률 | 검색 결과의 id/위치를 받고도 후속 근거 없이 끝난 비율 | A 대비 절반 이하 |
| 도구 호출 수/과제 | | A 이하 |
| 총 응답 바이트/과제 | `<cost>` 합계 | A 대비 +30% 이내 |
| 입력 오용률 | hybrid 키워드 나열 · keyword 산문 질의 비율(`internal/usage` Shape 분류 재사용) | B·C 모두 A 대비 절반 이하 |
| `NODE_NOT_FOUND` 비율 | | B·C 모두 거의 0 |

`scripts/eval-recall.py`는 id를 채점하므로 `detail=ids`로 고정해 회귀만 확인한다.
결과는 `docs/eval/<date>-tool-contract/`에 두고 `docs/eval/gate-log.md`에 기록한다.

## 6. 함께 바꿔야 하는 곳

- **e2e**: `e2e/search_keyword_test.go` 등은 기본 응답을 바이트 단위로 고정한다
  (`keywordHandler` 주석). 현행 모드(`context=0`, `detail=ids|code`)로 옮기고, 새 기본값은
  구조 검사 테스트로 추가한다.
- **스키마 테스트**: `TestInputSchemasMarshalValid`에 새 파라미터를 포함한다.
- **usage 수집**: `internal/usage/ingest.go:262-264`가 `node_id`만 기록한다. `symbol`과
  `symbols`도 기록해야 지표가 끊기지 않는다.
- **skill·agent 문서**: `plugin/graphin-guide/skills/graphin/SKILL.md`,
  `plugin/graphin-guide/agents/graphin-rag.md`의 "ID는 search가 준 것만", "where는 read_code
  없이" 같은 규칙을 새 기본값에 맞춘다. skill은 이제 필수가 아니라 심화 가이드가 된다.
- **설명 길이**: 초안은 현행보다 길다(도구당 약 900~1,300자). 서브에이전트 주입 시 도구 목록
  토큰이 늘어나므로, S1 측정에서 설명 길이와 오용률의 관계를 같이 본다.
