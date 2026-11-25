# graphin

**코딩 에이전트가 파일을 하나씩 grep하는 대신, 심볼 단위로 코드를 찾고·따라가고·읽게 합니다.**

graphin은 로컬 MCP 서버이자 Claude Code 플러그인입니다. 저장소를 함수·클래스·문서 섹션·DB
테이블의 그래프로 색인해서, 에이전트가 *이건 어디 있나*, *누가 호출하나*, *그 함수만 보여 줘*를
물으면 한 번의 호출로 정확한 답을 받게 합니다.

- **로컬.** 색인·검색·(선택) 임베딩 모델이 모두 내 컴퓨터에서 돕니다. API 키, 계정, 텔레메트리가 없습니다.
- **심볼 크기로 읽기.** `read_code`는 함수·메서드·섹션·테이블 하나, 또는 `path:120-180`만 돌려줍니다. 파일 통째가 아닙니다.
- **문자열이 아니라 참조를 따라갑니다.** 호출자와 의존성을 import와 스코프로 해석하므로, 이름만 같은 남의 코드는 섞이지 않습니다.
- **코드 ↔ 데이터베이스.** 커밋된 스키마 스냅샷이 그래프 노드가 되고, 각 테이블을 읽고 쓰는 코드와 이어집니다.

[English](README.md) · [한국어](README.ko.md)

## 빠른 시작

Claude Code(2.1.83 이상)에서:

```
/plugin marketplace add Salvia95/graphin
/plugin install graphin@graphin
```

세션을 다시 시작하고, 프로젝트를 연 뒤 평소라면 grep으로 찾았을 것을 물어보세요:

> 주문 취소는 어디서 처리하고, 누가 그걸 호출해?

graphin이 처음 보는 프로젝트에서는 에이전트가 먼저 `bootstrap_workspace`를 부릅니다(다른 도구가 그렇게
안내합니다). 색인은 백그라운드에서 돌고, 어휘 검색은 몇 초 안에, 의미 검색 모델은 그다음에 준비됩니다. 이후
세션은 인덱스를 알아서 다시 엽니다. `/graphin:doctor`가 설치·플랫폼 지원·인덱스 상태를 점검합니다.

색인은 `<프로젝트>/.graphin/`에 생기므로 `.gitignore`에 `.graphin/*`를 추가하세요.

> 이미 `claude mcp add graphin`으로 직접 등록해 두었다면 지우세요
> (`claude mcp remove graphin -s local|user|project`). 두 등록이 같은 인덱스를 두고 다툽니다.
> `/graphin:doctor`가 이 상태를 감지합니다.

## 왜 필요한가

색인이 없으면 에이전트는 추측으로 탐색합니다. 단어를 grep하고, 파일을 열고, import를 따라가고,
되돌아와서 다시 시도합니다. 연 파일은 전부 컨텍스트 비용이고, 빗나간 추측은 두 번 비용을 냅니다.

| | grep + 파일 읽기 | graphin |
|---|---|---|
| "X는 어디 있나?" | grep 후 후보를 열어 확인 | 검색 한 번: 순위가 매겨진 단위와 `file:line` |
| "누가 X를 호출하나?" | 이름을 grep하고, 이름만 같은 코드를 읽어서 걸러냄 | `explore_graph` → 해석된 호출자와 각 호출이 적힌 줄 |
| "X를 보여 줘" | 파일(또는 짐작한 범위)을 읽음 | `read_code` → 정확히 그 단위 |
| "`orders` 테이블을 건드리는 코드는?" | 테이블 이름과 SQL 문자열을 grep | 테이블의 `used_by` 엣지 |

## 무엇을 물어보면 되나

graphin을 쓴다고 에이전트에게 말하는 방식이 바뀌지는 않습니다. 잘 답하는 질문과, 그때 에이전트가 고르는 도구입니다:

| 상황 | 질문 예 | 도구 |
|---|---|---|
| 하는 일은 아는데 이름을 모를 때 | "watcher는 파일이 바뀐 걸 어떻게 판단해?" | `search_hybrid` |
| 에러 메시지나 설정 키가 있을 때 | "'failed to acquire lock'은 어디서 나와?" | `search_keyword` |
| 함수를 고치기 전에 | "`applyFileResult`를 누가 호출하고, 바꾸면 뭐가 깨져?" | `explore_graph` (used_by) |
| 한 단위를 읽을 때 | "`OrderService.cancel` 보여 줘." | `read_code` |
| 스키마 질문 | "`job_posting`에 쓰는 코드는?" | 테이블에 `explore_graph` |

## 실제 동작

Django, 어휘 인덱스만 사용(임베딩 모델 없음). 에이전트가 묻습니다:

> 연쇄 삭제할 때 관련 객체는 어떻게 모으지?

에이전트는 `search_hybrid`를 `target="code"`, `detail="full"`로 부르고, 첫 결과가 이미 함수 이름·위치·본문·
호출자까지 알려 줍니다(응답 일부 생략):

```xml
<results semantic_ready="false" target="code" candidates="16164" detail="full">
  <node id="django.db.models.deletion.Collector.collect" rank="1" kind="method"
        file="django/db/models/deletion.py" line="247"
        signature="def collect( self, objs, source=None, nullable=False, …">
    <body lines="247-307" partial="true" unit_lines="247-401">def collect(…): …</body>
    <callers shown="3" more="true">
      <node name="CASCADE" file="django/db/models/deletion.py" line="22" ref_line="23" />
      <node name="Model.delete" file="django/db/models/base.py" line="1125" ref_line="1133" />
      <node name="QuerySet.delete" file="django/db/models/query.py" line="1133" ref_line="1156" />
    </callers>
  </node>
  <node id="django.db.models.deletion.CASCADE" rank="2" … />
  …
</results>
```

호출 한 번이고, 다음 질문 — "이걸 바꾸면 뭐가 깨지나?" — 은 `Collector.collect`에 `direction="used_by"`로
`explore_graph`를 부르면 됩니다.

## 도구

| 도구 | 용도 |
|---|---|
| `search_hybrid` | 의미나 어렴풋한 이름으로 코드 찾기(정확 일치 + BM25 + 임베딩 결합). |
| `search_keyword` | 정확한 텍스트(메시지·키·상수) 찾기, 그리고 각 매칭이 속한 단위. |
| `explore_graph` | 한 단위를 누가 호출·참조하고 무엇에 의존하는지, confidence와 함께. |
| `read_code` | 한 단위, 여러 단위, 또는 줄 범위 읽기 — 실수로 파일 통째를 읽지 않습니다. |
| `bootstrap_workspace` · `diagnose_index` · `index_scope` | 인덱스 시작, 상태 점검, 색인 범위 확인·변경. |
| `wiki_*` | `docs/wiki` 위의 선택형 지식 계층([graphin-guide](#선택-graphin-guide) 참고). |

탐색 도구는 id뿐 아니라 이름(`cancelOrder`, `OrderService.cancel`, `src/order.py:120`)도 받고,
`detail="full"`을 주면 시그니처·호출 위치·호출자를 같은 응답에 담아 줍니다.
모든 파라미터는 코드에서 생성한 **[도구 레퍼런스](docs/tools.md)**에 있습니다.

## 동작 원리

```
소스 파일 ──tree-sitter──▶ 심볼 + 참조 ──▶ ┌ BM25 인덱스 (심볼, 본문, 문서 조각)
                                           ├ 벡터 인덱스 (선택, 로컬 ONNX 모델)
스키마 스냅샷 ──────────▶ 테이블·뷰·FK ──▶ └ 그래프 샤드 (호출, import, 상속,
                                              테이블 접근, 문서 섹션) + 역인덱스
파일 감시 ──▶ 머클 diff ──▶ 바뀐 것만 다시 파싱
```

- 검색은 정확 일치 계층·BM25·벡터 결과를 RRF로 결합하고, 원시 점수 대신 위치를 돌려줍니다.
- 엣지에는 confidence가 붙습니다: 문법상 확실하면 1.00, 같은 패키지 0.95, import를 거치면 0.90,
  같은 이름 추측 0.80(기본으로 숨김).
- 편집은 파일 감시로 반영되고, 파일이 바뀐 노드는 읽을 때 다시 파싱되므로 `read_code`가 낡은 위치를 돌려주지 않습니다.
- 한 프로젝트를 여러 세션이 열어도 인덱스는 하나입니다. 먼저 뜬 서버가 인덱스를 들고, 나머지는 그쪽으로 호출을 넘깁니다.

## 근거

공개 벤치마크와 이 저장소의 과제 세트로 측정했습니다. 방법과 원자료는 [`docs/eval/`](docs/eval/README.md)에 있습니다.

| 측정 | 결과 | 범위 |
|---|---|---|
| [SWE-Explore](https://github.com/Qiushao-E/SWE-Explore-Bench), 3개 언어 — lucene(Java)·element-web(TypeScript)·django(Python)의 40태스크 | recall@300 **0.118 대 0.000**(grep ±20), 제출 줄은 **2.1%**; graphin이 33태스크에서 앞서고 뒤진 태스크 없음 | LLM 없는 결정론 탐색기, 같은 질의, 어휘 인덱스 · [상세](docs/eval/2026-11-25-swe-explore-3lang/findings.md) |
| SWE-Explore, Python 451태스크(SWE-bench Verified) | recall@300 **0.109 대 0.004**, 제출 줄 2.6% | 같음 · [상세](docs/eval/2026-07-25-h1-reverify/findings.md) |
| 이 저장소의 에이전트 벤치 — 36문항 × 3회, graphin 에이전트 대 grep만 쓰는 에이전트 | **정답률 동일**(각 98/108) | [상세](docs/eval/2026-09-08-36-grep/findings.md) |
| 같은 벤치, 도구 설명 외에 아무 안내도 없는 서브에이전트 | grep 에이전트와 정답률·**토큰 비용 모두 동일** | [상세](docs/eval/2026-11-24-rag-hard/findings.md) |

SWE-Explore의 언어별 recall@300은 Java 0.129, Python 0.130, TypeScript 0.091로, 지금은 TypeScript가 가장 약합니다.

이 수치가 말하는 것과 말하지 않는 것: 정해진 읽기 예산 안에서 graphin은 grep이 못 하는 방식으로 맞는 코드를
앞에 올립니다. 끈기 있게 grep하는 에이전트도 같은 답에 도달하며, 이 저장소 크기(파일 수백 개)에서는 토큰 비용도
비슷합니다. graphin은 기본적으로 토큰 절약기가 아니라, 덜 헤매고 맞는 코드에 닿는 수단이고, 그 차이는 저장소가
클수록 커집니다.

## 지원 언어와 내용

| 내용 | 심볼 | 참조(그래프 엣지) |
|---|---|---|
| Java, Kotlin, Python | ✓ | 호출, import, 상속 |
| Go | ✓ | 호출, import, 인터페이스 임베딩 |
| JavaScript, TypeScript (JSX/TSX) | ✓ | 호출, import(`tsconfig` 경로 별칭 포함), JSX 컴포넌트 |
| Markdown | 헤딩마다 노드 하나 | 섹션 계층 |
| DB 스키마 스냅샷 | 테이블, 뷰, 함수, 프로시저 | FK, 코드 → 테이블 접근(JPA, SQLAlchemy, Django, TypeORM, Prisma, SQL 문자열) |
| 그 밖의 텍스트(YAML, SQL, Dockerfile, …) | 파일마다 노드 하나 | — |

DB 지원은 커밋된 스냅샷만 읽습니다 — graphin은 라이브 DB에 접속하지 않습니다. `tbls`/Atlas 출력에서
`graphin dbimport`로 만들거나, 매니페스트로 기존 Prisma 스키마나 SQL 덤프를 가리키면 됩니다:
[`schema/graphindb.md`](schema/graphindb.md).

## 선택: graphin-guide

두 번째 플러그인이 에이전트에게 graphin을 잘 쓰는 법을 가르치고, 읽기 전용 리서치 서브에이전트를 더합니다:

```
/plugin install graphin-guide@graphin
```

| 구성 | 하는 일 |
|---|---|
| `graphin` 스킬 | 언제 검색하고, 따라가고, 읽고 — 언제 멈추는지. |
| `graphin-rag` 서브에이전트 | 코드베이스 질문을 통째로 맡아 컨텍스트 예산 안에서 일하고, 답·인용·확인하지 못한 것을 함께 돌려줍니다. |
| `knowledge` 스킬 · `wiki-maintainer` | `docs/wiki` 위의 지식 계층: 문서 섹션을 묶은 세트, 용어집, 편집 전 사전 점검 게이트. |

일부러 별도 플러그인입니다. graphin은 에이전트가 스스로 graphin을 채택하는지를 (로컬에) 기록하는데, 유도를
섞으면 그 기준선이 사라집니다. 자세한 내용: [`plugin/graphin-guide/README.md`](plugin/graphin-guide/README.md).

## 설정

모두 선택 사항이며 `/plugin` → Manage → graphin → Configure에 있습니다:

| 옵션 | 용도 |
|---|---|
| `model_type` | 임베딩 모델: `multilingual_cjk`(기본) 또는 `english_optimal` |
| `semantic_max_nodes` | 노드가 이 수를 넘으면 의미 검색을 끕니다(기본 40,000; 어휘 검색은 유지) |
| `workspace_subdir` | 프로젝트 루트 대신 하위 디렉터리를 색인 |
| `offline` · `model_dir` | 다운로드 금지, 로컬 모델 디렉터리 사용(폐쇄망) |
| `binary_path` | 릴리스 바이너리 대신 직접 빌드한 것을 사용 |

경로 제외는 `.graphin/ignore`(gitignore 문법)로 하거나 에이전트에게 `index_scope`를 쓰게 하세요.
플러그인 옵션은 프로젝트별이 아니라 전역(user settings) 값입니다.

**플랫폼:** linux/amd64·linux/arm64는 릴리스 바이너리를 제공합니다. macOS(Apple Silicon)는 `go install`로
자동 빌드합니다(Go와 clang 필요). Intel macOS도 같은 방식으로 빌드되지만 의미 검색은 쓸 수 없습니다. Windows는
지원하지 않습니다.

## 개인정보

모든 데이터는 `<프로젝트>/.graphin/`과 `~/.cache/graphin/`에 남습니다. 네트워크 접근은 릴리스 바이너리와,
첫 부트스트랩 때의 임베딩 모델·ONNX 런타임 다운로드뿐입니다(SHA256 검증, `offline`으로 끌 수 있음).

플러그인은 채택 여부를 재려고 도구 호출 기록을 로컬에 남깁니다(`.graphin/usage/events.jsonl`) — 도구 이름,
graphin 질의, grep 패턴, 파일 경로만 적고, 파일 내용·도구 응답·전체 셸 명령은 적지 않습니다. `/graphin:report`가
요약하며, 어디로도 전송하지 않습니다. [기록 항목](plugin/graphin/README.md#계측-무엇을-기록하나).

## 한계

- **작은 저장소:** 파일 수백 개 규모에서는 grep 기반 에이전트도 같은 답을 비슷한 토큰 비용으로 찾습니다. graphin의
  이점은 저장소가 크고 질문이 여러 파일에 걸칠수록 커집니다.
- **정적 분석만:** 동적 디스패치, 리플렉션, 문자열로 만든 호출은 보이지 않습니다. 엣지가 없다고 관계가 없다는
  증거는 아닙니다.
- **스키마는 커밋된 그대로:** 라이브 DB가 아니라 스냅샷입니다.
- **플랫폼:** 위 참고. Intel macOS에서는 의미 검색을 쓸 수 없습니다.

## 문제 해결

| 증상 | 해결 |
|---|---|
| 뭔가 이상할 때 | `/graphin:doctor`가 바이너리·플랫폼·인덱스·중복 등록을 점검합니다 |
| 첫 세션에서 서버 시작이 시간 초과 | 25MB 바이너리를 받는 중입니다. `/mcp`에서 재연결하거나 `MCP_TIMEOUT`을 올리세요(예: `60000`) |
| 있는 줄 아는 심볼을 못 찾을 때 | 에이전트에게 `diagnose_index`를 돌리게 하거나 정확한 텍스트로 검색하세요 |
| 노드가 너무 많거나 무관한 파일이 색인될 때 | `index_scope`가 색인 범위를 보여 주고 제외를 미리 봅니다 |

## 업데이트와 제거

릴리스는 마켓플레이스를 새로 고쳐야 받습니다:

```sh
claude plugin marketplace update graphin
claude plugin update graphin@graphin
```

그다음 세션을 다시 시작해야 바이너리가 바뀝니다. 제거: `/plugin uninstall graphin@graphin` 후
`<프로젝트>/.graphin/`을 지우세요.

버전 번호: 0.x에서 **minor**(0.X.0)는 사용자가 뭔가 고쳐야 할 수 있다는 뜻이고(도구·옵션·플래그 이름 변경),
**patch**는 할 일이 없습니다. 플러그인 버전과 바이너리 버전은 항상 같은 숫자입니다.

## 개발

```sh
make build        # bin/graphin
make test         # go vet + go test ./...
./bin/graphin version --json
```

설계와 내부: [내부 동작](docs/internals.md) · [배포](docs/plugin-distribution.md) ·
[평가](docs/eval/README.md) · [스키마 스냅샷](schema/graphindb.md) · [도구 레퍼런스](docs/tools.md).

## 라이선스

[Apache-2.0](LICENSE)
