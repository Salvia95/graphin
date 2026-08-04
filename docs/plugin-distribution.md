# 플러그인 배포 스펙 — 바이너리 동봉 자기완결 설치 (v1, draft)

graphin을 **Claude Code 플러그인 하나로 설치**해 MCP 서버·admin·계측이 바로
동작하게 만든다. 사용자가 저장소를 클론하거나 바이너리 경로를 손으로 배선하지
않는다.

**상태: 설계만 완료, 미구현.** §2부터 순서대로 구현한다.

## 0. 문제

지금은 사용자가 직접 빌드하고 절대경로를 등록해야 한다:

```sh
claude mcp add graphin -- /path/to/bin/graphin --workspace /path/to/project --admin-addr 127.0.0.1:7466
```

그 결과 **모든 소비 프로젝트가 개발자의 체크아웃을 가리킨다.** 실측된 사고:
`kinder` 프로젝트가 `/home/tipa/projects/graphin/bin/graphin`을 등록한 상태에서
저장소에 `make build`를 돌리자 **실행 중이던 kinder 세션의 바이너리가 교체됐다**
(`/proc/<pid>/exe → /home/tipa/projects/graphin/bin/graphin (deleted)`). 프로세스는
옛 inode를 물고 계속 돌았고, 사용자는 옛 UI를 보면서 그 사실을 알 방법이 없었다.

정적 에셋이 `embed.FS`로 바이너리에 들어가므로 이 결합은 admin UI 전체에 걸린다.

## 1. 확정 결정

| # | 결정 | 근거 |
|---|---|---|
| D1 | 저장소를 **public으로 전환** | private면 `go install`·릴리스 에셋·marketplace가 전부 인증을 요구해 외부 배포가 성립하지 않는다 |
| D2 | v1 플랫폼 = **linux/amd64 + linux/arm64** | 두 조합 모두 ORT 1.26.0 에셋이 존재한다. darwin/amd64는 ORT 빌드 자체가 없다 |
| D3 | 플러그인 **2개 분리** — `graphin`(MCP+admin+계측) / `graphin-guide`(SKILL+에이전트) | [usage-spec](usage-spec.md) §8이 유도 스킬 동봉을 v2로 연기했다. 무개입 베이스라인은 한번 섞이면 복구 불가 |
| D4 | 바이너리 = **릴리스 다운로드 → `go install` 폴백** | D1 덕에 `go install`이 자기완결적으로 성립한다 — 소스 사본도 체크아웃 참조도 필요 없다 |
| D5 | admin **기본 비활성**, `userConfig`로 주소를 지정할 때만 기동 | 여러 프로젝트 동시 사용 시 포트 충돌 회피 |

### 근거가 되는 플랫폼 사실

ONNX Runtime 1.26.0 릴리스 에셋 실측:

| 타깃 | ORT 에셋 | 의미 검색 | v1 |
|---|---|---|---|
| linux/amd64 | `onnxruntime-linux-x64-1.26.0.tgz` | 가능 | **배포** |
| linux/arm64 | `onnxruntime-linux-aarch64-1.26.0.tgz` | 가능 | **배포** |
| darwin/arm64 | `onnxruntime-osx-arm64-1.26.0.tgz` | 가능 | v1.1 |
| darwin/amd64 | **없음** | 영구 불가 | 배포 안 함 |
| windows | `.zip`(`extractORTLib`은 tar.gz만 읽음) | 불가 | 범위 밖 |

ORT가 없어도 바이너리는 동작한다 — `warmupSemantic`(`internal/workspace/workspace.go:309`)이
`semantic_unavailable`을 기록하고(323·330행) lexical 검색은 살아 있다. 그래서 darwin/amd64를
"그냥 배포"하고 싶어지지만, **사유 없이 반쪽만 동작하는 설치가 최악**이다. 배포한다면
`provision.Resolve`가 404 다운로드 오류가 아니라 명시적 미지원 오류를 반환해야 한다.

## 2. Phase 0 — 선행 (코드 없음)

1. **저장소 public 전환.** 라이선스 파일이 없으므로 공개 전 결정이 필요하다.
2. `README.md:23`의 "대상 플랫폼: Linux/macOS"를 사실에 맞게 정정한다. macOS를
   뒷받침하는 코드 경로가 현재 없다(§4).

## 3. Phase 1 — 버전 식별

대상: `cmd/graphin/main.go`, `Makefile`

### 3.1 `const` → `var`

`-ldflags -X`는 **`const`에 먹지 않고 조용히 무시된다.** 릴리스 바이너리가
`0.1.0-dev`로 찍히는 전형적 사고의 원인이다.

```go
// main.go:23
var version = "dev"
var commit = ""
var buildDate = ""
```

### 3.2 `version`은 서브커맨드로

`flag.Parse()`가 57행, `--workspace` 필수 검증이 59행이다. `flag.Bool("version", …)`로
만들면 `graphin --version`이 `--workspace is required`로 exit 2 한다. 기존
`dbimport`/`eval`/`usage` 디스패치(28·33·38행) 옆에 둔다:

```go
if len(os.Args) > 1 && (os.Args[1] == "version" || os.Args[1] == "--version") {
    // plain : graphin 1.0.0 linux/amd64 (abc1234, 2026-08-04)
    // --json: {"version","commit","os","arch","ort","semantic_supported"}
}
```

`runtime.GOOS`/`GOARCH`와 **이 플랫폼에 ORT 핀이 있는지**를 함께 낸다 —
`/graphin:doctor`가 그 한 줄로 끝나고, 사용자는 의미 검색이 왜 꺼졌는지 안다.

### 3.3 Makefile

```make
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null)
LDFLAGS  = -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.buildDate=$(shell date -u +%Y-%m-%dT%H:%M:%SZ)

build:
	$(GO) build -trimpath -ldflags "$(LDFLAGS)" -o bin/graphin ./cmd/graphin
```

`-trimpath`는 위생을 넘어 실질적이다 — 없으면 25MB 릴리스 바이너리에
`/home/runner/work/...` 빌드 경로가 박힌다.

> **`-extldflags -static`을 넣지 말 것.** `yalue/onnxruntime_go`가 libonnxruntime을
> `dlopen`으로 연다. 정적 바이너리는 `dlopen`을 못 하므로 의미 검색이 **워밍업
> 시점에만** 깨진다 — 가장 발견하기 어려운 자리다.

### 3.4 부수 개선

`main.go:107`의 admin 실패 메시지에 조치를 덧붙인다. 현재는
`admin page unavailable: bind: address already in use`만 나오고 다음 행동이 없다.
`admin_addr` 플러그인 설정을 바꾸라고 지목한다.

## 4. Phase 2 — provision 플랫폼 인지

대상: `internal/provision/{manifest.go, pins.go, download.go, provision_test.go}`

현재 `manifest.go`는 `onnxruntime-linux-x64-1.26.0.tgz` 하나를 무조건 받는다.
**`runtime.GOOS`/`GOARCH` 분기가 저장소 전체에 0건이다.**

- `manifest.go` — 단일 `ORT` var → `ortByPlatform map[string]Artifact`
  (`"linux/amd64"`, `"linux/arm64"`). `ORTLibName` 상수도 플랫폼별 값으로 바꾼다
  (macOS는 `libonnxruntime.<ver>.dylib`, Linux는 `libonnxruntime.so.<ver>`).
- `pins.go` — aarch64 아카이브 SHA256 추가. 기존 주석 규약(2026-07-21 기록) 유지.
- `download.go` — `resolveORT`(113행)와 `extractORTLib`(234·254행)이 패키지 레벨
  `ORT`/`ORTLibName`을 캡처하므로, 해석된 `Artifact`와 lib 이름을 인자로 넘긴다.
- **`ErrUnsupportedPlatform` 신설** — 핀이 없는 플랫폼에서 404 다운로드 오류 대신
  이것을 반환해야 `warmupSemantic`이 이해 가능한 사유를 남긴다.
- `provision_test.go` — 모든 키가 64-hex SHA와 `ORTVersion` 일치 URL을 갖는지,
  미지원 플랫폼이 `ErrUnsupportedPlatform`을 내는지 테이블 테스트.

> darwin 핀을 추가할 때 **아카이브 안의 dylib 경로를 실제로 조회한 뒤** 적을 것.
> 관례상 `lib/libonnxruntime.1.26.0.dylib`이지만 확인 없이 적으면 안 된다.

Windows zip 지원은 이 단계에서 넣지 않는다 — 런처 문제가 풀리기 전까지 사장 코드다.

## 5. Phase 3 — CI와 릴리스

현재 `.github/`가 아예 없다.

### 5.1 CI 먼저 — `.github/workflows/ci.yml`

`go vet ./...` · `go test ./...` · `make test-race` · **`shellcheck -s sh`**.

이 설계의 스크립트는 전부 POSIX `sh`이고 훅에서 문법 오류가 조용히 삼켜지므로,
shellcheck가 그것을 잡는 유일한 장치다.

### 5.2 릴리스 — `.github/workflows/release.yml`

**`workflow_dispatch(version)` 입력 방식으로 한다. 태그 트리거는 닭-달걀에 걸린다** —
플러그인의 `install/manifest.json`이 아직 존재하지 않는 에셋의 SHA256을 담아야
하는데, 마켓플레이스는 태그된 트리에서 플러그인을 제공하기 때문이다.

```
job build (matrix):
  linux-amd64 → ubuntu-22.04 (또는 debian:bullseye 컨테이너)
  linux-arm64 → ubuntu-24.04-arm   # 네이티브 arm64 러너 — 크로스 툴체인 불필요
  env: CGO_ENABLED=1
  go build -trimpath -ldflags "-X main.version=${{inputs.version}} -X main.commit=$GITHUB_SHA" -o graphin ./cmd/graphin
  tar -czf graphin_${VER}_${OS}_${ARCH}.tar.gz graphin

job publish (needs: build):
  sha256sum *.tar.gz > SHA256SUMS
  → plugin/graphin/install/manifest.json 생성
  → plugin/graphin/.claude-plugin/plugin.json 버전 갱신
  → main에 커밋, 그 커밋에 v${VER} 태그
  → gh release create v${VER} --target <sha> *.tar.gz SHA256SUMS
```

에셋은 태그된 커밋의 *부모*에서 빌드된다. 둘의 차이는 매니페스트와 플러그인 버전
뿐이고 어느 쪽도 컴파일러에 닿지 않는다. **다음 사람이 "고치지" 않도록 워크플로에
주석으로 남길 것.**

> **크로스 컴파일은 선택지가 아니다.** `go-tree-sitter`와 문법 바인딩 5종이 벤더링된
> C를 cgo로 컴파일하고 `onnxruntime_go`가 `-ldl`을 쓴다. `CGO_ENABLED=1`과 C
> 툴체인이 필수라 OS별 네이티브 러너가 유일한 답이다.

> **glibc 바닥이 가장 유력한 실패 지점.** `ubuntu-latest`(24.04, glibc 2.39)에서
> 빌드하면 Debian 12·RHEL 9·AL2023에서 `GLIBC_2.38 not found`가 난다. bullseye
> 컨테이너(glibc 2.31)나 최소 `ubuntu-22.04`로 고정한다.

### 5.3 `plugin/graphin/install/manifest.json`

무결성 핀 · 플랫폼 지원표 · 버전 스탬프 · 재설치 트리거를 **한 파일이 겸한다.**
어느 항목이 바뀌어도 재설치가 걸린다 — 그게 목적이다.

```json
{
  "schema": 1,
  "version": "1.0.0",
  "repo": "Salvia95/graphin",
  "tag": "v1.0.0",
  "platforms": {
    "linux/amd64": { "asset": "graphin_1.0.0_linux_amd64.tar.gz", "sha256": "…", "bin": "graphin" },
    "linux/arm64": { "asset": "graphin_1.0.0_linux_arm64.tar.gz", "sha256": "…", "bin": "graphin" }
  },
  "go_install": { "module": "github.com/Salvia95/graphin/cmd/graphin", "min_go": "1.26.5" },
  "notes": { "darwin/amd64": "no onnxruntime 1.26.0 build exists; semantic search unavailable" }
}
```

*v2 후보*: `SHA256SUMS`를 minisign/cosign으로 서명하고 공개키를 플러그인에 동봉.
커밋백 루프가 사라지고 무결성이 "태그된 트리를 신뢰"에서 실제 서명으로 올라간다.
v1에서는 사용자 머신에 바이너리 의존성이 하나 더 생기므로 보류.

## 6. Phase 4 — `plugin/graphin/`

```
plugin/graphin/
├── .claude-plugin/plugin.json
├── .mcp.json
├── bin/graphin-launch.sh
├── install/{manifest.json, install.sh}
├── hooks/{hooks.json, session-start.sh, usage.sh}
├── commands/{report.md, setup.md, doctor.md, admin.md}
└── README.md
```

### 6.1 바이너리 설치 위치

`${CLAUDE_PLUGIN_DATA}` = `~/.claude/plugins/data/graphin-graphin/`.

**`${CLAUDE_PLUGIN_ROOT}`에 두면 안 된다** — 플러그인 업데이트마다 경로가 바뀌고
정리되므로 25MB를 매번 다시 받는다. `PLUGIN_DATA`는 업데이트를 넘어 살아남는다.

```
${CLAUDE_PLUGIN_DATA}/
├── bin/graphin-1.0.0-linux-amd64      # 버전 박힌 실제 파일
├── bin/graphin -> graphin-1.0.0-linux-amd64
├── state/{manifest.json, installed.json, last-error.txt, install.lock/}
└── logs/install.log
```

**버전 파일명 + 심볼릭 링크는 장식이 아니다:**

- 실행 중인 바이너리를 제자리에서 덮으면 Linux에서 `ETXTBSY`가 난다.
- 링크만 갈아끼우면 돌던 서버는 옛 inode로 무사히 끝난다.
- 원자적 교체: `ln -s "$t" "$l.tmp.$$" && mv -f "$l.tmp.$$" "$l"`.
  `$l`이 디렉터리가 아님을 먼저 확인할 것 — 아니면 `mv`가 *안으로* 옮긴다.
- 잠금은 `mkdir` 기반으로. `flock`은 macOS 이식성이 없다.

### 6.2 설치 결정 트리 (`install/install.sh`)

```
0. $GRAPHIN_BIN 또는 binary_path userConfig가 실행 가능 → 그대로 사용
   (개발자·폐쇄망·미지원 플랫폼의 탈출구)
1. 빠른 경로: 심볼릭 링크 존재 && cmp -s manifest.json → 즉시 exit 0
   (stat 2회 + 1KB 비교. fork 없음)
2. 재링크: 해당 버전 파일이 이미 있고 SHA 일치 → 링크만 교체
   (업그레이드→롤백→업그레이드 반복에서 재다운로드 회피)
3. 다운로드: curl -fsSL --proto '=https' --tlsv1.2 --retry 3 --max-time 600
   → SHA256 불일치면 임시파일 삭제하고 중단 (부분 설치 절대 금지)
4. go install 폴백:
   command -v go && cc/gcc/clang 존재 확인
   GOTOOLCHAIN=auto CGO_ENABLED=1 go install …/cmd/graphin@v$VER
   → `graphin version --json`으로 버전 확인 후 채택 (source="go-install")
   go.mod가 go 1.26.5라 구버전 Go는 GOTOOLCHAIN=auto로 자가 승급해야 한다.
   사용자 환경에 GOTOOLCHAIN=local이 있으면 실패하므로 감지해 안내한다.
5. 실패: state/last-error.txt에 사유와 조치를 쓰고 exit 1
```

**버전 판별은 매니페스트 파일 동일성(`cmp -s`)으로 한다.** 공식 문서가 권장하는
`diff -q package.json` 패턴 그대로다. 그래서 매 세션 재실행 비용이 사실상 0이고,
`--version`은 **외부에서 들어온 바이너리**에만 쓰인다 — `binary_path`, PATH의
`graphin`, `go install` 직후 확인, `/graphin:doctor`. 이 넷이 전부다.

### 6.3 실행 순서 위험 — 런처가 권위를 갖는다

**MCP 서버 spawn과 SessionStart 훅의 순서는 공식 문서에 명시되어 있지 않다.**
서버가 먼저 뜨면 설치 직후 첫 세션이 깨진다. 순서에 무관하도록 설계한다:

- `bin/graphin-launch.sh`가 **권위** — 바이너리가 없으면 자기가 동기 설치 후 `exec`.
- SessionStart 훅은 **예열기** — 보통 먼저 이겨서 다운로드를 MCP 기동 타임아웃
  밖으로 밀어낸다.
- 훅은 모든 경로에서 `exit 0`. 실패는 `additionalContext`로 산문 전달해 Claude가
  사용자에게 말하게 한다.
- 훅 `matcher`에서 `compact`를 제외한다 — 압축이 25MB 다운로드를 불러선 안 된다.
- 첫 실행이 느린 링크에서 MCP 기동 타임아웃을 넘길 수 있다. 대응은 `/mcp` 재연결
  또는 `MCP_TIMEOUT` 상향. README에 적는다.
- `/graphin:setup`(강제 재설치, verbose)을 제공해 복구가 명령 하나로 끝나게 한다.

```json
// hooks/hooks.json
{
  "hooks": {
    "SessionStart": [
      { "matcher": "startup|resume|clear",
        "hooks": [{ "type": "command",
                    "command": "\"${CLAUDE_PLUGIN_ROOT}\"/hooks/session-start.sh",
                    "timeout": 300 }] }
    ],
    "PostToolUse": [
      { "matcher": "*",
        "hooks": [{ "type": "command",
                    "command": "\"${CLAUDE_PLUGIN_ROOT}\"/hooks/usage.sh",
                    "timeout": 10 }] }
    ]
  }
}
```

### 6.4 `.mcp.json` — args 없이 전부 env로

```json
{
  "mcpServers": {
    "graphin": {
      "command": "${CLAUDE_PLUGIN_ROOT}/bin/graphin-launch.sh",
      "env": {
        "GRAPHIN_PLUGIN_ROOT":        "${CLAUDE_PLUGIN_ROOT}",
        "GRAPHIN_PLUGIN_DATA":        "${CLAUDE_PLUGIN_DATA}",
        "GRAPHIN_PROJECT_DIR":        "${CLAUDE_PROJECT_DIR}",
        "GRAPHIN_ADMIN_ADDR":         "${user_config.admin_addr}",
        "GRAPHIN_MODEL_TYPE":         "${user_config.model_type}",
        "GRAPHIN_WORKSPACE_SUBDIR":   "${user_config.workspace_subdir}",
        "GRAPHIN_OFFLINE":            "${user_config.offline}",
        "GRAPHIN_MODEL_DIR":          "${user_config.model_dir}",
        "GRAPHIN_SEMANTIC_MAX_NODES": "${user_config.semantic_max_nodes}",
        "GRAPHIN_BINARY_PATH":        "${user_config.binary_path}"
      }
    }
  }
}
```

**`args`가 없는 것이 핵심이다.** 이것이 "정적 JSON은 `--admin-addr`를 조건부로 뺄 수
없다"는 제약을 해소한다 — 런처가 env를 읽어 빈 값을 건너뛰며 argv를 조립한다.
`--offline` 같은 `flag.Bool`은 치환만으로는 애초에 표현할 수 없다.

`--workspace`는 `${CLAUDE_PROJECT_DIR}`로 해결된다.

### 6.5 런처 (`bin/graphin-launch.sh`)

책임: ① 바이너리 해석(없으면 설치 호출) ② 워크스페이스 해석 ③ argv 조립 ④ `exec`.

```sh
#!/bin/sh
set -eu
DATA="${GRAPHIN_PLUGIN_DATA:-${XDG_DATA_HOME:-$HOME/.local/share}/graphin}"
ROOT="${GRAPHIN_PLUGIN_ROOT:?}"

# 미설정 user_config가 빈 문자열인지 리터럴로 남는지 미확인 — 양쪽 다 미설정으로 본다.
val() { case "$1" in ""|'${'*) return 1;; esac; printf '%s' "$1"; }

BIN="$(val "${GRAPHIN_BINARY_PATH:-}" || true)"; [ -n "${BIN:-}" ] || BIN="$DATA/bin/graphin"
if [ ! -x "$BIN" ] || ! cmp -s "$ROOT/install/manifest.json" "$DATA/state/manifest.json"; then
  "$ROOT/install/install.sh" >>"$DATA/logs/install.log" 2>&1 || {
    cat "$DATA/state/last-error.txt" >&2; exit 1; }
fi

WS="${GRAPHIN_PROJECT_DIR:-$PWD}"
sub="$(val "${GRAPHIN_WORKSPACE_SUBDIR:-}" || true)"; [ -n "${sub:-}" ] && WS="$WS/$sub"

set -- --workspace "$WS"
v="$(val "${GRAPHIN_ADMIN_ADDR:-}" || true)" && [ -n "${v:-}" ] && set -- "$@" --admin-addr "$v"
# … model-type / model-dir / semantic-max-nodes 동일 패턴 …
case "$(printf '%s' "${GRAPHIN_OFFLINE:-}" | tr 'A-Z' 'a-z')" in 1|true|yes|on) set -- "$@" --offline;; esac

exec "$BIN" "$@"
```

**두 규칙을 어기면 프로토콜이 깨진다:**

- **`exec`로 넘긴다.** MCP 전송은 `os.Stdin`/`os.Stdout` 생 stdio다(`main.go:119`).
  중간에 버퍼링하거나 재포맷하는 래퍼가 남으면 프로토콜이 오염된다.
- **stdout에 절대 쓰지 않는다.** 진단은 stderr로만.

### 6.6 `plugin.json` userConfig

`admin_addr`(기본 `""` = 비활성) · `model_type` · `offline` · `model_dir` ·
`semantic_max_nodes` · `workspace_subdir` · `binary_path`.

`binary_path`는 **자기 체크아웃 빌드를 계속 쓰려는 개발자의 착지점**이다. 플러그인이
단일 등록 지점으로 남으면서도 바이너리는 어디서든 올 수 있다.

### 6.7 계측 훅 이동 — 해석 순서를 바꿔야 한다

`plugin/graphin-usage/hooks/handler.sh` → `plugin/graphin/hooks/usage.sh`.
바이너리 해석 순서를 다음으로 바꾼다:

```
1. $GRAPHIN_BIN
2. $CLAUDE_PLUGIN_OPTION_BINARY_PATH
3. ${CLAUDE_PLUGIN_DATA}/bin/graphin      ← 신규. binpath보다 위여야 한다
4. <root>/.graphin/binpath                ← 레거시
5. command -v graphin
```

**이유가 미묘하고, 놓치면 조용히 죽는다.** `os.Executable()`은 Linux에서
`/proc/self/exe`를 읽어 **심볼릭 링크를 해석한다.** 그래서 `$DATA/bin/graphin`으로
기동된 서버는 `binpath`에 `$DATA/bin/graphin-1.0.0-linux-amd64`(버전 박힌 실제
파일)를 쓴다. 업그레이드가 그 파일을 정리하면 `binpath`는 없는 곳을 가리키고,
계측이 죽으며 `usage report`는 "인덱스는 있는데 이벤트가 없다"만 출력한다.
심볼릭 링크를 위에 두면 자가 치유된다.

아울러 `usage.sh` 가드가 `$CLAUDE_PLUGIN_OPTION_WORKSPACE_SUBDIR`를 존중하게 한다 —
이제 서버와 훅이 한 플러그인에 있으므로 워크스페이스 설정을 공유할 수 있고,
`plugin/graphin-usage/README.md` §3이 기록한 상향 탐색 한계가 해소된다.

## 7. Phase 5 — `plugin/graphin-guide/`

D3에 따라 분리한다. **베이스라인 보존이 목적이므로 `graphin` 플러그인에 넣지 않는다.**

```
plugin/graphin-guide/
├── .claude-plugin/plugin.json
├── skills/graphin/SKILL.md      # examples/skills/graphin/SKILL.md 이동
└── agents/graphin-explorer.md   # examples/agents/graphin-explorer.md 이동
```

- SKILL의 "EXAMPLE / TEMPLATE — 복사해 쓰라" HTML 주석 제거(이제 배포물이다).
- 에이전트 19행이 `examples/skills/graphin/SKILL.md`를 참조 — 경로 갱신.
- 에이전트의 `disallowedTools`는 **그대로 둔다.** 플러그인이 서버 키를 `graphin`으로
  고정해도 플러그인 제공 MCP 도구가 추가 네임스페이싱되는지 미확인이라(§11-4),
  화이트리스트로 바꾸면 기동 실패 위험이 있다.
- `examples/`는 플러그인을 가리키는 얇은 포인터로 남기거나 삭제한다. 두 벌 유지 금지.

`graphin-guide` 설치 시점을 `<ws>/.graphin/usage/guidance.json`에
`{"plugin","version","since"}`로 기록해 두면 `usage report --since`로 유도 전후를
분리해 볼 수 있다. D3로 베이스라인은 이미 보존되므로 선택 사항이다.

## 8. Phase 6 — 마이그레이션

### 8.1 기존 수동 등록과의 충돌 — 최대 리스크

플러그인도 `graphin` 키로 MCP 서버를 등록한다. 사용자 스코프
`claude mcp add graphin`과 충돌할 때 무엇이 이기는지 **공식 문서에 없다. 릴리스 전
실측 필수**(§11-1).

- `/graphin:doctor`가 `claude mcp list`를 확인해 비-플러그인 `graphin` 등록이 있으면
  경고한다. 출시 후 가장 유력한 지원 이슈이고, 감지 비용은 싸다.
- README에 제거 절차를 명시: `claude mcp remove graphin -s {local,user,project}`.
- 충돌이 파괴적으로 밝혀지면 서버 키를 바꾸는 것이 대안이지만, 그러면 도구 이름이
  `mcp__graphin__*`에서 바뀌어 SKILL 산문과 사용자 습관이 함께 깨진다. 문서로 푸는
  쪽을 우선한다.

### 8.2 `graphin-usage` 기존 설치

두 플러그인이 공존하면 PostToolUse가 2회 발화한다. 그러나
`internal/usage/stream.go:77-81`이 읽기 시점에 `tool_use_id`로 디듀프하므로
**지표는 무해하고 디스크만 낭비된다.** 따라서 점진 마이그레이션이 안전하다.

`graphin-usage` 0.2.0을 묘비로 낸다 — `hooks/hooks.json` 삭제(중복 중단),
`commands/report.md`는 `/graphin:report`를 가리키게, description을 deprecated로.
갱신하지 않은 사용자는 계속 동작한다(시끄러울 뿐).

`internal/usage/run.go:60`의 진단 문자열이 `plugin/graphin-usage/README.md`를
가리키므로 함께 고친다.

### 8.3 `.graphin/binpath`

`main.go:80-83`의 기록은 **유지한다.** 갱신하지 않은 기존 `graphin-usage` 설치가
의존한다. 해석 순서만 §6.7처럼 바꾼다.

## 9. Phase 7 — 문서

- `README.md` — 설치 절차(96–100·126–133행)를 `/plugin install graphin@graphin`으로
  교체, 23행 플랫폼 표기 정정, 최소 Claude Code 버전 명시.
- [usage-spec.md](usage-spec.md) — §1 트리 갱신, §8에 D3 결정(유도 분리 유지) 기록.
- 각 플러그인 README.

## 10. 검증

```sh
# Phase 1
make build && ./bin/graphin version --json        # dev가 아니어야 한다

# Phase 2
go test ./internal/provision/

# Phase 3 — 구형 glibc에서 실행되는지가 핵심
docker run --rm -v $PWD:/w debian:12 /w/graphin version

# Phase 4 — 로컬 플러그인 개발 모드
claude --plugin-dir ./plugin/graphin
#   /mcp 에서 graphin 연결 확인 → search_hybrid 호출
#   admin_addr 설정 후 페이지 확인 → 빈 값으로 되돌려 비활성 확인

# Phase 5
claude --plugin-dir ./plugin/graphin-guide
#   에이전트의 skills:[graphin] 이 실제로 해석되는지 확인 (§11-3)

# Phase 7 — 유일하게 목표를 증명하는 테스트
# graphin 체크아웃도 자격증명도 없는 머신에서:
/plugin marketplace add Salvia95/graphin
/plugin install graphin@graphin
```

## 11. 구현 전에 반드시 실측할 것 (추측 금지)

1. **수동 `claude mcp add graphin`과 플러그인 제공 `graphin`의 충돌 동작.**
   섀도잉인지 중복인지 오류인지. 출시 후 가장 유력한 지원 이슈다.
2. **MCP spawn과 SessionStart 훅의 순서.** 설계는 순서 무관이지만 첫 실행 UX가
   갈린다(매끄러운가, 재연결이 필요한가).
3. **`skills: [graphin]`이 플러그인 제공 스킬을 맨 이름으로 해석하는지.**
   네임스페이싱(`graphin-guide:graphin`)이 필요할 수 있다. 조용히 실패하면
   에이전트가 도구 레퍼런스를 통째로 잃고 산문으로 퇴화한다.
4. **플러그인 제공 MCP 도구가 추가 네임스페이싱되는지**(`mcp__graphin__*` 여부).
   계측은 [usage-spec](usage-spec.md) §3이 접미사 매칭을 규정해 무사하지만, SKILL
   산문과 도구 화이트리스트는 영향받는다.
5. **`${user_config.KEY}`가 미설정일 때** 빈 문자열인지 리터럴이 남는지, `boolean`이
   `true`/`1` 중 무엇으로 렌더되는지. §6.5의 `val()` 가드가 양쪽을 막지만 확인한다.
6. **darwin ORT 아카이브 안의 dylib 경로**(v1.1). 관례에서 추정하지 말고 조회한다.
7. **`${CLAUDE_PLUGIN_DATA}`·`userConfig`의 최소 Claude Code 버전.** 최근 기능이다.

## 12. 하지 말 것

- **바이너리를 플러그인 디렉터리에 커밋** — 설치 시 `~/.claude/plugins/cache`로 전체
  복사되고, 25MB × 플랫폼 수 × 모든 버전이 git 히스토리에 영구히 남는다.
- **`-extldflags -static`** — `dlopen` 파괴. 워밍업에서만 드러난다.
- **`ubuntu-latest`로 linux 릴리스 빌드** — glibc 바닥이 너무 높다.
- **darwin/amd64 배포** — ORT 1.26.0 에셋 부재. 영구 lexical-only를 문서로만 알리는
  꼴이 된다.
- **admin 포트 자동 할당** — 조용한 포트 드리프트가 보이는 바인드 실패보다 나쁘다.
  원한다면 `:0` + `.graphin/admin-addr` 파일로 발견 가능하게 만든다(v1.1).
- **SessionStart 훅만을 유일한 설치 경로로 삼기** — MCP spawn이 먼저면 첫 세션이
  항상 깨진다.
