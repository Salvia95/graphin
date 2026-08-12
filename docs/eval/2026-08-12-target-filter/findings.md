# target 필터 (2026-08-12) — 산문 질의의 슬롯 70%가 코드가 아니었다

진단 권고 ②·③이 지목한 "다단계 탐색에서 놓친다 / 부른 뒤 답이 안 착지한다"의
제품 쪽 뿌리 하나를 재현하고, 고치고, 다시 쟀다.

D 구간에서 이 저장소에 처음 물어본 코드 질문이 계기였다. 실행 중인 0.2.8 서버에
`search_hybrid("cross-file edge invalidation when caller file changes")`를 던지자
1·3·5위가 마크다운, 5위는 `scores.json`, 코드 히트 2·4위는 둘 다 테스트 함수였다.
**구현은 한 자리도 못 받았다.**

## 왜 그런가 — 구조가 그렇게 되어 있다

마크다운과 plain 텍스트는 **파일 하나가 노드 하나**다(scope.md §1). 그래서 문서
전체가 하나의 임베딩이 되고, 산문 질의에 대해 **작은 코드 노드를 구조적으로
이긴다.** 함수 하나는 자기 이름과 시그니처뿐이지만 문서는 그 함수가 하는 일을
문장으로 적어 둔 물건이다. 게다가 이 저장소의 `docs/eval/`은 정확히 그 주제의
산문이라 자석이 된다.

`.json`·`.yml`·`.toml`도 같은 통(`plainExtensions`, parse.go)에 들어가 있어
`scores.json`이 산문 질의에 **semantic**으로 뜬다. 산문이 아닌 파일이 산문
매칭으로 올라오는 것은 그냥 노이즈다.

그리고 부를 때 **"코드만"이라고 말할 방법이 없었다** — `search_hybrid`의
파라미터는 `query`와 `top_k` 둘뿐이었다.

계측은 이 구분을 이미 하고 있었다. `usage report`는 문서 조회가 코드 채택률에
섞이지 않도록 target(code/db/docs)으로 쪼갠다(usage-spec, 커밋 `bb262d7`).
**분리가 필요하다는 걸 측정에서는 인정하고 제품에서는 안 했다.**

## 측정

- 코퍼스: 이 저장소의 커밋된 트리(`git archive HEAD`, 소스 커밋 `eb3486f`).
  워킹 트리가 아니다 — 이 변경으로 추가한 테스트 픽스처의 산문이 첫 질의와
  겹쳐서 1차 측정을 오염시켰다(재현 시 주의).
- 바이너리: 이 변경을 포함해 빌드. semantic 준비 완료(`semantic_ready="true"`)
  상태에서만 질의했다.
- 질의 8개, 전부 **코드에 대한 산문 질문**(심볼형이 아니다 — 심볼형은 Tier-0가
  이미 잘 처리한다):

  1. cross-file edge invalidation when caller file changes
  2. how are exact matches pinned to the top of search results
  3. where is the semantic node count gate enforced
  4. how does the watcher decide a file changed
  5. where does the markdown parser build section slugs
  6. how is the workspace lock acquired and released
  7. what writes the vectors file header
  8. how does read_code repair stale byte offsets

- 분류는 결과의 `file` 속성으로 한다: `_test.go`는 test, `.md`는 docs,
  `.json/.yml/.toml/.txt`는 text, 나머지가 impl. top_k=5 × 8질의 = **40슬롯**.

| | impl | test | docs | text |
|---|---:|---:|---:|---:|
| 기존(필터 없음) | **7** | 5 | 21 | 7 |
| `target=code` | **22** | 18 | – | – |

**코드가 아닌 것이 슬롯의 70%(28/40)를 먹고 있었다.** 필터를 걸면 구현 히트가
7 → 22로 **3.1배**가 된다. 8질의 중 4개는 기존에 구현을 한 건도 못 냈고
(`0/…`), 그중 3개가 필터를 걸면 2~4건을 낸다.

## 새로 드러난 것 — 다음 층은 테스트다

필터를 건 뒤 남은 40슬롯에서 **18개가 테스트 함수**다. 문서를 걷어내자 그 자리를
테스트가 가져갔다. 질의 1·8은 필터를 걸고도 구현이 1건뿐이고 나머지 넷이 전부
테스트다.

이유는 ④에서 이미 본 것과 같은 모양이다 — 진단 권고 ④는 **정의가 제 호출부에
밀린다**였고(호출부가 질의의 산문 토큰까지 갖고 있어서), 여기서는 **구현이 제
테스트에 밀린다.** 테스트 함수 이름은 `TestContainsRefreshesWhenChildAdded`처럼
질의를 거의 문장으로 담고 있어서, 그 이름을 토큰으로 쪼개면 산문 질의와 정확히
겹친다. 구현 함수 이름은 그렇지 않다.

이건 이번 변경의 범위 밖이라 고치지 않았다. 다음 후보 둘:

- 테스트를 target의 하위 모집단으로 갈라 `code`에서 뺀다. 문제: 테스트가 답인
  질문("이 동작을 어디서 단언하나")이 실제로 있다.
- 랭킹에서 테스트 노드를 감점한다. 문제: 감점은 기본값 변경이라 이 디렉터리의
  리포트를 근거로만 해야 하고(README 규약), k20 precision 경고
  (2026-08-08-tier0-ranking, p=0.022)와 같은 통에 들어간다.

**지금 수치를 "검색이 좋아졌다"로 인용하지 말 것** — 잰 것은 *슬롯 배분*이지
정답률이 아니다. 정답 노드를 고정한 골든셋이 아직 없다.

## 무엇을 바꿨나

`search_hybrid`에 선택 파라미터 `target`(`code` | `docs` | `db`)을 넣었다.
생략하면 기존과 완전히 동일하다(테스트로 고정: `TestNilFilterMatchesUnfilteredSearch`).

- 분류는 `nodeid.Target(kind, id)`. **kind만으로는 안 된다** — `KindFile`이
  마크다운 루트와 앵커 없는 설정 텍스트의 공용 폴백이라 README와 scores.json을
  구별하지 못한다. 그래서 kind와 ID를 같이 본다.
- `.json`·`.yml` 류는 `code`도 `docs`도 아닌 `text`로 떨어진다. 이름으로 부를 수
  있는 모집단이 아니므로 열거형에서는 뺐다 — 필터를 생략하면 그대로 검색된다.
- 필터는 **Tier-0를 포함한 모든 후보 스트림**에 걸린다. 이름이 정확히 맞았다는
  이유로 문서가 code 슬롯을 가져가면 안 된다.
- **후보 풀을 넓힌다**(슬롯당 3 → 24). 안 넓히면 상위가 전부 문서일 때 다섯을
  물어 둘을 받는다 — 필터가 존재하는 이유가 바로 그 질의 모양이다. 넓히는 비용은
  사실상 0이다: semantic은 topK와 무관하게 질의 임베딩 1회를 내고, BM25는 힙만
  커진다.
- 필터 없는 경로의 후보 수는 **그대로 뒀다.** lexical-only 스윕 451건이 그 풀에서
  측정됐다.
- 응답이 `target="code"`를 되돌려 준다. 짧은 목록이 "이 저장소엔 이게 별로 없다"로
  읽히면 안 된다.
- 모르는 target은 무시하지 않고 **거절한다.** 조용히 전체 검색을 하면 호출자는
  코드만 봤다고 믿는다.

## 남은 것

1. 위 §새로 드러난 것 — 테스트가 구현을 가린다.
2. 골든셋. 이 저장소는 우리가 정답을 아는 질문의 보고이고, 그게 있어야 "슬롯
   배분이 좋아졌다"를 "답을 더 잘 찾는다"로 올려 말할 수 있다.
3. 에이전트가 이 파라미터를 실제로 쓰는가. 도구 설명에 언제 쓰는지 적어 뒀지만,
   진단이 관찰한 행동은 "추가 작업을 안 한다"였다. `graphin-guide` SKILL에 넣을지는
   베이스라인 보존과 얽혀 있어 별도 판단이 필요하다.
