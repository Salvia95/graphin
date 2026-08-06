# 플러그인 배포 스펙 — 바이너리 동봉 자기완결 설치 (v1, draft)

graphin을 **Claude Code 플러그인 하나로 설치**해 MCP 서버·admin·계측이 바로
동작하게 만든다. 사용자가 저장소를 클론하거나 바이너리 경로를 손으로 배선하지
않는다.

**상태 (2026-08-06): Phase 0~7 전부 완료, v0.1.0 릴리스 완료.** 저장소는 public,
라이선스는 Apache-2.0, CI·릴리스 워크플로와 두 플러그인(`graphin` ·
`graphin-guide`)이 들어왔고, v0.1.0이 발행되어 `install/manifest.json`이
커밋됐다 — 플러그인은 자기완결이다. 옛 `graphin-usage`는 0.2.0 묘비가 됐다.

**목표는 달성됐다**: 체크아웃도 자격증명도 없는 머신에서
`/plugin install graphin@graphin` 하나로 설치가 끝난다.

남은 것은 실사용으로만 확인되는 것들뿐이다 — §11-2(MCP spawn과 SessionStart
순서), §10 마지막 항목(제3의 머신에서의 설치), 그리고 v1.1 후보(darwin 핀,
서명, admin `:0` + 주소 파일).

## 0. 문제

지금은 사용자가 직접 빌드하고 절대경로를 등록해야 한다:

```sh
claude mcp add graphin -- /path/to/bin/graphin --workspace /path/to/project --admin-addr 127.0.0.1:7466
```

그 결과 **모든 소비 프로젝트가 개발자의 체크아웃을 가리킨다.** 실측된 사고:
어떤 소비 프로젝트가 `<graphin-체크아웃>/bin/graphin`을 등록한 상태에서
저장소에 `make build`를 돌리자 **실행 중이던 그 세션의 바이너리가 교체됐다**
(`/proc/<pid>/exe → <graphin-체크아웃>/bin/graphin (deleted)`). 프로세스는
옛 inode를 물고 계속 돌았고, 사용자는 옛 UI를 보면서 그 사실을 알 방법이 없었다.

정적 에셋이 `embed.FS`로 바이너리에 들어가므로 이 결합은 admin UI 전체에 걸린다.

## 1. 확정 결정

| # | 결정 | 근거 |
|---|---|---|
| D1 | 저장소를 **public으로 전환** | private면 `go install`·릴리스 에셋·marketplace가 전부 인증을 요구해 외부 배포가 성립하지 않는다 |
| D2 | v1 플랫폼 = **linux/amd64 + linux/arm64** | 두 조합 모두 ORT 1.26.0 에셋이 존재한다. darwin/amd64는 ORT 빌드 자체가 없다 |
| D3 | 플러그인 **2개 분리** — `graphin`(MCP+admin+계측) / `graphin-guide`(SKILL+에이전트) | [usage-spec](usage-spec.md) §8이 유도 스킬 동봉을 v2로 연기했다. 무개입 베이스라인은 한번 섞이면 복구 불가 |
| D4 | 바이너리 = **릴리스 다운로드 → `go install` 폴백** | D1 덕에 `go install`이 자기완결적으로 성립한다 — 소스 사본도 체크아웃 참조도 필요 없다 |
| D5 | admin **기본 비활성**, 주소를 지정할 때만 기동. 우선순위는 `<ws>/.graphin/admin-addr` **파일 → `userConfig`** | plugin option은 CC 2.1.207부터 **user settings에서만** 읽힌다(§11-7). 즉 `admin_addr`는 프로젝트별로 줄 수 없는 전역 값 하나여서, 켜는 순간 모든 프로젝트가 같은 포트를 노린다. 파일 override가 그 구멍을 메우면서도 §12의 "자동 포트 할당 금지"를 지킨다 |
| D6 | 라이선스 = **Apache-2.0** (2026-08-05 확정, `LICENSE` 커밋됨) | 특허 조항이 있어 기업 채택 마찰이 적고, tree-sitter·onnxruntime 등 의존성 생태계의 관행과 맞는다 |

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

1. ~~라이선스 결정~~ → **Apache-2.0 확정, `LICENSE` 커밋 완료**(D6). 저작권자
   표기는 `Salvia95` — 법인/실명이 필요하면 그 줄만 고치면 된다.
2. ~~`README.md:23` 플랫폼 표기 정정~~ → **완료.** 배포 타깃(linux/amd64·arm64)과
   그 밖 플랫폼에서 `--ort-lib` 없이는 lexical-only라는 사실을 명시했다.
3. ~~저장소 public 전환~~ → **완료** (2026-08-05). 이로써 D1·D4가 성립한다:
   릴리스 에셋을 인증 없이 받을 수 있고 `go install` 폴백이 자기완결적이다.

## 3. Phase 1 — 버전 식별 ✅ 구현 완료

대상: `cmd/graphin/main.go`, `Makefile`

### 3.1 `const` → `var`

`-ldflags -X`는 **`const`에 먹지 않고 조용히 무시된다.** 릴리스 바이너리가
`0.1.0-dev`로 찍히는 전형적 사고의 원인이다.

```go
// main.go:31
var version = "dev"
var commit = ""
var buildDate = ""
```

**`go install` 경로가 이 셋을 비운다 — 폴백이 없으면 설치가 실패한다.**
`go install …/cmd/graphin@v1.0.0`로 만든 바이너리에는 `-ldflags`가 아예 붙지
않아 `version`이 `dev`로 남는다. §6.2 4단계가 그 값을 보고 방금 설치한
정상 바이너리를 거부하게 된다. `buildIdentity()`가 `debug.ReadBuildInfo()`의
`Main.Version`(모듈 버전)과 `vcs.revision`으로 폴백한다.

### 3.2 `version`은 서브커맨드로

`flag.Parse()`가 76행, `--workspace` 필수 검증이 79행이다. `flag.Bool("version", …)`로
만들면 `graphin --version`이 `--workspace is required`로 exit 2 한다. 기존
`dbimport`/`eval`/`usage` 디스패치(47·52·57행) 옆에 둔다:

```go
if len(os.Args) > 1 && (os.Args[1] == "version" || os.Args[1] == "--version") {
    // plain : graphin 1.0.0 linux/amd64 (abc1234, 2026-08-04)
    //         미지원 플랫폼이면 뒤에 [lexical only: …]가 붙는다
    // --json: {"version","commit","build_date","os","arch","ort","semantic_supported"}
}
```

`runtime.GOOS`/`GOARCH`와 **이 플랫폼에 ORT 핀이 있는지**를 함께 낸다 —
`/graphin:doctor`가 그 한 줄로 끝나고, 사용자는 의미 검색이 왜 꺼졌는지 안다.
`--json` 키 집합은 `install.sh`와 doctor가 파싱하는 계약이라
`cmd/graphin/version_test.go`가 고정한다. 알 수 없는 인자는 exit 2 +
**stdout 무출력**(파서가 엉뚱한 단어를 버전으로 읽는 것보다 오류가 낫다).

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

### 3.4 부수 개선 ✅

admin 바인드 실패 메시지가 주소를 바꿀 수 있는 **세 자리를 모두** 지목한다:
`--admin-addr` 플래그, 플러그인의 `admin_addr` 옵션, 그리고 D5의
`<ws>/.graphin/admin-addr` 파일.

## 4. Phase 2 — provision 플랫폼 인지 ✅ 구현 완료

대상: `internal/provision/{manifest.go, pins.go, download.go, provision_test.go}`

현재 `manifest.go`는 `onnxruntime-linux-x64-1.26.0.tgz` 하나를 무조건 받는다.
**`runtime.GOOS`/`GOARCH` 분기가 저장소 전체에 0건이다.**

- `manifest.go` — 단일 `ORT` var → `ortByPlatform map[string]ortPlatform`
  (`"linux/amd64"`, `"linux/arm64"`). 아카이브와 **그 안의 lib 이름이 한 쌍으로
  묶여 있다** — macOS는 `libonnxruntime.<ver>.dylib`, Linux는
  `libonnxruntime.so.<ver>`로 갈리기 때문이다. 접근자는 `ORTFor(goos, goarch)`와
  `SemanticSupported(goos, goarch)`.
- `pins.go` — aarch64 SHA256 `34ff1c2d…be01a` 추가(2026-08-05 실측). 기존 주석
  규약(2026-07-21 기록) 유지.
  **리눅스 두 타깃의 `.so` 이름은 동일하다**(`libonnxruntime.so.1.26.0`, 아카이브
  실측) — 그래서 v1은 lib 이름이 실질적으로 하나지만, 구조는 darwin을 위해
  플랫폼별로 열어 뒀다.
- `download.go` — `resolveORT`/`extractORTLib`이 해석된 `Artifact`와 lib 이름을
  인자로 받는다. **`--ort-lib` 검사는 플랫폼 조회보다 앞에 둔다** — 핀 없는
  플랫폼이 의미 검색을 쓸 수 있는 유일한 통로라, 플랫폼 조회가 먼저 거부하면
  탈출구가 막힌다. `Options.Platform`은 `BaseURL`과 같은 성격의 테스트 훅이다.
- **`ErrUnsupportedPlatform` 신설** — 핀이 없는 플랫폼에서 404 다운로드 오류 대신
  이것을 반환해야 `warmupSemantic`이 이해 가능한 사유를 남긴다. 메시지에 반드시
  플랫폼 키를 넣는다(운영자가 `semantic_unavailable` 로그에서 읽는 값이다).
- `provision_test.go` — 핀 테이블의 모든 키가 `<goos>/<goarch>` 꼴이고 64-hex
  SHA와 `ORTVersion` 일치 URL·lib 이름을 갖는지, D2가 약속한 두 플랫폼이 실제로
  들어 있는지, darwin/amd64가 `ErrUnsupportedPlatform`을 내는지, 그 위에서도
  `--ort-lib`가 이기는지.

> darwin 핀을 추가할 때 **아카이브 안의 dylib 경로를 실제로 조회한 뒤** 적을 것.
> 관례상 `lib/libonnxruntime.1.26.0.dylib`이지만 확인 없이 적으면 안 된다.

Windows zip 지원은 이 단계에서 넣지 않는다 — 런처 문제가 풀리기 전까지 사장 코드다.

## 5. Phase 3 — CI와 릴리스 ✅ 워크플로 작성 완료 (첫 릴리스 미실행)

### 5.1 CI 먼저 — `.github/workflows/ci.yml`

`go vet ./...` · `go test ./...` · `make test-race` · **shellcheck**.

훅에서 문법 오류가 조용히 삼켜지므로(모든 경로가 `exit 0`, stderr도 버려진다)
shellcheck가 그것을 잡는 유일한 장치다.

**`-s sh`를 강제하지 않는다 — 셰방으로 판별시킨다.** 플러그인의 훅과 런처는
`#!/bin/sh`를 선언하므로 셰방 판별만으로 bashism이 잡힌다. 반대로 `-s sh`를
강제하면 의도적으로 bash인 `scripts/fetch-flatc.sh`가 `SC3040`(pipefail)로
걸린다 — 실측 확인.

러너는 `ubuntu-22.04`로 고정한다. CI가 릴리스보다 새로운 glibc로 컴파일하면
CI 통과가 배포 가능성을 뜻하지 않게 된다.

> 워크플로 자체는 `actionlint`로 검증한다(run 블록의 shellcheck 포함).
> `go run github.com/rhysd/actionlint/cmd/actionlint@latest`

### 5.2 릴리스 — `.github/workflows/release.yml`

**`workflow_dispatch(version)` 입력 방식으로 한다. 태그 트리거는 닭-달걀에 걸린다** —
플러그인의 `install/manifest.json`이 아직 존재하지 않는 에셋의 SHA256을 담아야
하는데, 마켓플레이스는 태그된 트리에서 플러그인을 제공하기 때문이다.

```
job build (matrix, 둘 다 debian:bullseye 컨테이너 안에서):
  linux-amd64 → ubuntu-22.04
  linux-arm64 → ubuntu-22.04-arm   # 네이티브 arm64 러너 — 크로스 툴체인 불필요
  env: CGO_ENABLED=1
  make build VERSION=$VER COMMIT=… BUILDDATE=…   # 빌드 플래그의 단일 출처는 Makefile
  ./graphin version --json 로 자기 검증 → tar -czf graphin_${VER}_linux_${ARCH}.tar.gz

job publish (needs: build):
  버전 입력 semver 검증 · 기존 태그 재사용 거부
  sha256sum *.tar.gz > SHA256SUMS
  → plugin/graphin/install/manifest.json 생성 (jq)
  → plugin/graphin/.claude-plugin/plugin.json 버전 갱신 (없으면 notice 후 건너뜀)
  → main에 커밋, 그 커밋에 v${VER} 태그
  → gh release create v${VER} *.tar.gz SHA256SUMS
```

에셋은 태그된 커밋의 *부모*에서 빌드된다. 둘의 차이는 매니페스트와 플러그인 버전
뿐이고 어느 쪽도 컴파일러에 닿지 않는다. **다음 사람이 "고치지" 않도록 워크플로
맨 위에 주석으로 남겼다.**

빌드 잡의 **자기 검증 단계가 이 워크플로의 핵심 안전장치**다. `version --json`이
`version == 입력값 && arch == 매트릭스 && semantic_supported`를 만족하지 못하면
거기서 멈춘다 — `-ldflags`가 조용히 빗나간 바이너리(§3.1)가 릴리스로 나가는 경로를
막는다. `objdump -T`로 실제 glibc 하한도 로그에 남긴다.

`plugin.json`이 아직 없는 Phase 4 이전에도 이 워크플로는 돈다(버전 갱신만 건너뛴다).

#### 5.2.1 v0.1.0 실행으로 확인된 것 (2026-08-06)

첫 실행이 한 번에 통과했고, 미지수였던 넷이 함께 풀렸다:

- **`ubuntu-22.04-arm` 러너가 존재한다**(public 저장소 무료 티어). arm64 네이티브
  빌드에 크로스 툴체인이 필요 없다.
- **`debian:bullseye` 컨테이너 안에서 Node 24 액션이 돈다.** 러너가 컨테이너에
  주입하는 Node가 glibc 2.31에서 동작한다(v4 액션들이 Node 20 deprecation으로
  Node 24에 강제 실행되는 상태였다).
- **워크플로의 `permissions: contents: write`가 저장소 기본값 `read`를 덮는다.**
  저장소 설정은 상한이 아니라 기본값이다 — publish 잡의 커밋·태그 푸시가 성립한다.
- **실측 glibc 하한은 2.17이다.** bullseye가 2.31인데도 cgo가 실제로 요구하는
  심볼이 그보다 낮아, 사실상 현행 리눅스 전부를 덮는다.

산출물도 설계대로다: 태그 `v0.1.0`은 매니페스트 커밋을, 바이너리가 보고하는
`commit`은 그 **부모**를 가리킨다. `SHA256SUMS`와 매니페스트의 해시가 일치한다.

> **크로스 컴파일은 선택지가 아니다.** `go-tree-sitter`와 문법 바인딩 5종이 벤더링된
> C를 cgo로 컴파일하고 `onnxruntime_go`가 `-ldl`을 쓴다. `CGO_ENABLED=1`과 C
> 툴체인이 필수라 OS별 네이티브 러너가 유일한 답이다.

> **glibc 바닥이 가장 유력한 실패 지점 — `ubuntu-22.04`도 충분하지 않다.**
> `ubuntu-latest`(24.04, glibc 2.39)는 물론이고, 러너 이미지에서 직접 빌드하면
> 22.04라도 glibc 2.35가 하한이 되어 **RHEL 9·AL2023(둘 다 2.34)이 깨진다.**
> 그래서 두 아키텍처 모두 `debian:bullseye` 컨테이너(2.31) 안에서 빌드한다.

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

## 6. Phase 4 — `plugin/graphin/` ✅ 구현 완료

```
plugin/graphin/
├── .claude-plugin/plugin.json
├── .mcp.json
├── bin/graphin-launch.sh
├── install/install.sh          # manifest.json은 릴리스 워크플로가 커밋한다
├── hooks/{hooks.json, session-start.sh, usage.sh}
├── commands/{report.md, setup.md, doctor.md, admin.md}
└── README.md
```

`install/manifest.json`은 **저장소에 없다** — §5.2의 publish 잡이 생성해 커밋한다.
첫 릴리스 이전에는 `install.sh`가 그 사실을 사유로 적고 `binary_path`를 쓰라고
안내한다(추측해서 설치하지 않는다).

검증: `claude plugin validate plugin/graphin --strict`(마켓플레이스는
`claude plugin validate . --strict`) · `shellcheck` · `go test ./e2e/ -run
'TestLauncherArgv|TestPluginUsageHook'`.

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
   ldflags가 없는 경로라 §3.1의 ReadBuildInfo 폴백이 여기서 값을 만든다.
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

**실측 반영 (CC 2.1.221 기준):**

- `GRAPHIN_PLUGIN_ROOT`/`GRAPHIN_PLUGIN_DATA` 별칭은 **불필요하다.** stdio 서버의
  env에는 `CLAUDE_PLUGIN_ROOT`·`CLAUDE_PLUGIN_DATA`가 자동 주입된다. 런처가 그대로
  읽는다.
- **`.mcp.json`이 참조하는 `${user_config.KEY}`는 전부 `plugin.json`의 `userConfig`에
  선언돼 있어야 하고, `required: true`를 붙이면 안 된다.** 미선언 키나
  `required`+`default` 없는 키는 빈 문자열이 아니라 **예외**가 되고, 서버 설정
  전체가 로드에 실패한다(`Plugin option "x" isn't set`). 선언된 optional 키는
  미설정 시 **빈 문자열**로 렌더된다 — §6.5의 `val()`이 기대하는 그대로다.
- boolean은 `"true"`/`"false"` 문자열로 렌더된다.

### 6.5 런처 (`bin/graphin-launch.sh`)

책임: ① 바이너리 해석(없으면 설치 호출) ② 워크스페이스 해석 ③ argv 조립 ④ `exec`.

```sh
#!/bin/sh
set -eu
DATA="${CLAUDE_PLUGIN_DATA:-${XDG_DATA_HOME:-$HOME/.local/share}/graphin}"
ROOT="${CLAUDE_PLUGIN_ROOT:?}"

# 미설정 user_config는 빈 문자열로 렌더된다(실측). 리터럴 잔존 케이스는 선언된
# 키에서는 발생하지 않지만, 값싼 보험으로 남긴다.
val() { case "$1" in ""|'${'*) return 1;; esac; printf '%s' "$1"; }

BIN="$(val "${GRAPHIN_BINARY_PATH:-}" || true)"; [ -n "${BIN:-}" ] || BIN="$DATA/bin/graphin"
if [ ! -x "$BIN" ] || ! cmp -s "$ROOT/install/manifest.json" "$DATA/state/manifest.json"; then
  "$ROOT/install/install.sh" >>"$DATA/logs/install.log" 2>&1 || {
    cat "$DATA/state/last-error.txt" >&2; exit 1; }
fi

WS="${GRAPHIN_PROJECT_DIR:-$PWD}"
sub="$(val "${GRAPHIN_WORKSPACE_SUBDIR:-}" || true)"; [ -n "${sub:-}" ] && WS="$WS/$sub"

set -- --workspace "$WS"
# D5: 프로젝트별 파일이 전역 옵션을 이긴다. plugin option은 user settings에만
# 저장되므로 이 파일이 없으면 모든 프로젝트가 같은 포트를 노린다.
v=""
[ -r "$WS/.graphin/admin-addr" ] && v="$(head -n1 "$WS/.graphin/admin-addr" | tr -d '[:space:]')"
[ -n "$v" ] || v="$(val "${GRAPHIN_ADMIN_ADDR:-}" || true)"
[ -n "${v:-}" ] && set -- "$@" --admin-addr "$v"
# … model-type / model-dir / semantic-max-nodes 동일 패턴 …
case "$(printf '%s' "${GRAPHIN_OFFLINE:-}" | tr 'A-Z' 'a-z')" in 1|true|yes|on) set -- "$@" --offline;; esac

exec "$BIN" "$@"
```

**두 규칙을 어기면 프로토콜이 깨진다:**

- **`exec`로 넘긴다.** MCP 전송은 `os.Stdin`/`os.Stdout` 생 stdio다(`main.go:149`).
  중간에 버퍼링하거나 재포맷하는 래퍼가 남으면 프로토콜이 오염된다.
- **stdout에 절대 쓰지 않는다.** 진단은 stderr로만.

### 6.5.1 구현하며 확정된 것

- `userConfig` 필드 스키마는 `.strict()`다: `type`(`string`|`number`|`boolean`|
  `directory`|`file`) · `title` · `description`이 필수이고 `required` · `default` ·
  `multiple` · `sensitive` · `min` · `max`가 선택. **모르는 필드는 거부된다.**
  키는 `^[A-Za-z_]\w*$`여야 한다(`CLAUDE_PLUGIN_OPTION_<KEY>`가 되기 때문).
- `model_type`·`semantic_max_nodes`에는 **`default`를 주지 않았다.** 기본값을
  적어 두면 서버의 기본값과 두 벌이 되어 언젠가 어긋난다. 비워 두면 플래그 자체가
  붙지 않아 서버 기본값이 유일한 출처로 남는다.
- 매니페스트 파서는 jq가 없는 사용자 머신을 가정해 `sed`/`awk`로 짰다. 우리가
  생성한 레이아웃만 읽는 전용 리더이고, 실제 `jq` 출력으로 검증했다.
- 오래된 버전 파일은 정리한다(각 25MB). 실행 중인 파일을 unlink해도 리눅스에서는
  안전하고, 그 때문에 끊기는 `binpath`는 §6.7의 해석 순서가 자가 치유한다.

### 6.6 `plugin.json` userConfig

`admin_addr`(기본 `""` = 비활성) · `model_type` · `offline` · `model_dir` ·
`semantic_max_nodes` · `workspace_subdir` · `binary_path`.

**일곱 개 전부 `required: false`여야 한다**(§6.4 실측). 하나라도 required로
선언하고 사용자가 값을 비워 두면 서버 설정이 통째로 로드에 실패한다.

`binary_path`는 **자기 체크아웃 빌드를 계속 쓰려는 개발자의 착지점**이다. 플러그인이
단일 등록 지점으로 남으면서도 바이너리는 어디서든 올 수 있다.

`admin_addr`는 **전역 값 하나**다(user settings에만 저장, CC 2.1.207+). 프로젝트별
주소는 `<ws>/.graphin/admin-addr` 파일이 담당한다(D5). 우선순위와 두 자리 모두를
바인드 실패 메시지가 지목한다(§3.4).

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

`$CLAUDE_PLUGIN_OPTION_<KEY>`를 쓰는 것은 편의가 아니라 **강제**다. 훅의
shell-form `command`가 `${user_config.*}`를 참조하면 치환값이 셸로 재파싱되는
것을 막기 위해 **플러그인 로드 자체가 에러로 실패한다**(실측). 값은 훅 환경의
`CLAUDE_PLUGIN_OPTION_<KEY>`로만 읽는다.

아울러 `usage.sh` 가드가 `$CLAUDE_PLUGIN_OPTION_WORKSPACE_SUBDIR`를 존중하게 한다 —
이제 서버와 훅이 한 플러그인에 있으므로 워크스페이스 설정을 공유할 수 있고,
`plugin/graphin-usage/README.md` §3이 기록한 상향 탐색 한계가 해소된다.

## 7. Phase 5 — `plugin/graphin-guide/` ✅ 구현 완료

D3에 따라 분리한다. **베이스라인 보존이 목적이므로 `graphin` 플러그인에 넣지 않는다.**

```
plugin/graphin-guide/
├── .claude-plugin/plugin.json
├── skills/graphin/SKILL.md      # examples/skills/graphin/SKILL.md 이동
├── agents/graphin-explorer.md   # examples/agents/graphin-explorer.md 이동
└── README.md
```

- SKILL의 "EXAMPLE / TEMPLATE — 복사해 쓰라" HTML 주석 제거(이제 배포물이다).
  본문은 한 글자도 바꾸지 않았다.
- 에이전트의 `disallowedTools`는 **그대로 둔다.** 이유는 바뀌었다 — §11-4가
  해결되어 플러그인 도구가 `mcp__plugin_graphin_graphin__*`임이 확정됐지만,
  화이트리스트로 적으면 **직접 등록한 서버에서는 그 이름이 아니다.** 이름에
  의존하지 않는 쪽이 두 경우 모두에서 옳다. 주석을 이 사실로 갱신했다.
- `examples/`는 플러그인을 가리키는 포인터 README만 남기고 두 파일을 삭제했다.
  사본이 갈라지면 어느 쪽이 진짜인지 알 수 없게 된다.
- `skills/`·`agents/`는 기본 폴더라 `plugin.json`에 경로를 적지 않았다. 적으면
  기본 폴더 탐색이 꺼져(`shadowed-by-manifest`) 두 곳을 동기화해야 한다.

`graphin-guide` 설치 시점을 `<ws>/.graphin/usage/guidance.json`에 기록하는 안은
**넣지 않았다.** D3로 베이스라인이 이미 보존되고, 전후 비교는 설치일 기준
`usage report --since`로 낼 수 있어 훅 하나를 더 다는 값을 못 한다.

## 8. Phase 6 — 마이그레이션 ✅ 구현 완료

### 8.1 기존 수동 등록과의 충돌 — 최대 리스크

**실측 결과: 섀도잉이 아니라 중복이다. 그리고 조용히 반쪽이 죽는다.**

Claude Code는 플러그인 MCP 서버를 억제할 때 **`command`+`args`가 같은지만** 본다
(env는 비교에서 제외 — CC 2.1.152 체인지로그가 명시). 수동 등록은 바이너리를
직접 부르고 플러그인은 런처 스크립트를 부르므로 **커맨드가 달라 억제가 걸리지
않는다.** 둘 다 뜬다.

그 결과가 나쁘다: 같은 워크스페이스를 두 프로세스가 잡으려 하고, 늦게 뜬 쪽이
`workspace.go:198`의 `lock.Acquire`에서 `ErrLockHeld`를 받는다. 에이전트에게는
도구 두 벌이 보이는데 한 벌은 인덱싱을 못 한다.

- `/graphin:doctor`가 `claude mcp list`를 확인해 비-플러그인 `graphin` 등록이 있으면
  경고한다. **선택이 아니라 필수다.**
- README에 제거 절차를 명시: `claude mcp remove graphin -s {local,user,project}`.
- 서버 키를 바꾸는 것은 대안이 못 된다 — 아래 8.1.1대로 **도구 이름은 어차피
  바뀐다.**

#### 8.1.1 도구 이름은 플러그인화의 불가피한 귀결이다

플러그인이 제공하는 MCP 서버는 레지스트리 키가 `plugin:<플러그인>:<서버>`가 되고,
도구 이름은 `mcp__` + (키에서 `[^a-zA-Z0-9_-]`를 `_`로 치환) + `__` + 도구명이다.
따라서:

```
mcp__graphin__search_hybrid  →  mcp__plugin_graphin_graphin__search_hybrid
```

영향 범위는 다행히 좁다:

- **계측: 무사.** `internal/usage/event.go:55`의 `^mcp__.+__([a-zA-Z0-9_]+)$`
  접미사 매칭이 그대로 통과한다([usage-spec](usage-spec.md) §3).
- **SKILL: 무사.** 산문이 `search_hybrid` 같은 맨 이름만 쓴다.
- ~~고칠 곳: 에이전트 주석의 `mcp__graphin__*` 언급~~ → Phase 5에서 갱신 완료
  (`plugin/graphin-guide/agents/graphin-explorer.md`). `disallowedTools`는 이름
  비의존이라 그대로 둔다 — 직접 등록한 서버에서는 이름이 또 다르기 때문이다.
- 훅 matcher를 쓸 일이 생기면 `mcp__plugin_graphin_graphin__.*` 꼴이어야 한다
  (CC 2.1.195부터 하이픈 식별자는 정확 매칭).

### 8.2 `graphin-usage` 기존 설치 ✅ 완료

두 플러그인이 공존하면 PostToolUse가 2회 발화한다. 그러나
`internal/usage/stream.go:77-81`이 읽기 시점에 `tool_use_id`로 디듀프하므로
**지표는 무해하고 디스크만 낭비된다.** 따라서 점진 마이그레이션이 안전하다.

`graphin-usage` 0.2.0을 묘비로 냈다 — `hooks/`를 통째로 삭제(중복 중단),
`commands/report.md`는 `/graphin:report`로 안내, description·README를
deprecated로. 갱신하지 않은 사용자는 계속 동작한다(시끄러울 뿐).

**`handler.sh`도 함께 지웠다.** `hooks.json`이 없으면 호출되지 않는 죽은
스크립트다. 다만 그 위에 있던 e2e 픽스처 스위트(가드·malformed·bash 검색·결과
id 추출·순차 append)는 **살아 있는 `plugin/graphin/hooks/usage.sh`로 옮겼다** —
죽은 스크립트를 테스트를 붙들기 위해 남겨 두는 것은 거꾸로다. 레거시 binpath
해석 경로도 그 스위트가 계속 덮는다.

사용자에게 보이는 문자열 중 옛 플러그인을 가리키던 셋도 함께 고쳤다:
`internal/usage/run.go`의 "no usage events" 진단(→ `/graphin:doctor`),
admin `/usage` 페이지의 빈 상태(→ `/plugin install graphin@graphin`),
`cmd/graphin/main.go`의 binpath 주석.

### 8.3 `.graphin/binpath`

`main.go:96-101`의 기록은 **유지한다.** 갱신하지 않은 기존 `graphin-usage` 설치가
의존한다. 해석 순서만 §6.7처럼 바꾼다.

## 9. Phase 7 — 문서 ✅ 구현 완료

- `README.md` — "빌드 & 등록"을 **"설치"**로 교체했다. 첫 화면이
  `/plugin install graphin@graphin`이고 최소 Claude Code 2.1.83을 명시한다.
  `make build`는 개발자 절이 되었고, 자기 빌드를 쓰는 길은 `claude mcp add`가
  아니라 `binary_path` 옵션이라고 못 박았다 — 절대경로 등록이 §0의 사고를
  다시 부르기 때문이다. admin 절도 프로젝트별 `admin-addr` 파일 방식으로 바꿨다.
- [usage-spec.md](usage-spec.md) — §1 트리를 `plugin/graphin/`으로 갱신,
  §6 릴리스 게이트와 네임스페이싱을 `/graphin:report`로, §8 D3 기록.
- 각 플러그인 README 3종(`graphin`·`graphin-guide`·`graphin-usage` 묘비).
  프라이버시 표(무엇을 기록하고 무엇을 기록하지 않는가)는 묘비가 가리키는
  `plugin/graphin/README.md`로 옮겼다 — 링크만 남기고 내용을 잃으면 안 된다.

## 10. 검증

```sh
# Phase 1 ✅  dev가 아니어야 하고, semantic_supported가 이 플랫폼과 맞아야 한다
make build && ./bin/graphin version --json
go test ./cmd/graphin/                            # --json 키 집합 계약

# Phase 2 ✅  핀 테이블 well-formed + 미지원 플랫폼이 명시적 오류를 내는지
go test ./internal/provision/

# Phase 3 — 워크플로 문법·run 블록은 actionlint로 (푸시 전)
go run github.com/rhysd/actionlint/cmd/actionlint@latest
# 릴리스 후, 구형 glibc에서 실행되는지가 핵심
docker run --rm -v $PWD:/w debian:12 /w/graphin version

# Phase 4 ✅ 실제 릴리스에서 설치되는지 — 빈 데이터 디렉터리로 전 과정을 돌린다
CLAUDE_PLUGIN_ROOT=$PWD/plugin/graphin CLAUDE_PLUGIN_DATA=$(mktemp -d) \
  sh plugin/graphin/install/install.sh
#   v0.1.0 실측: 첫 설치 2.7초, 두 번째 실행은 7ms 무출력(빠른 경로)

# Phase 4 — 로컬 플러그인 개발 모드
claude --plugin-dir ./plugin/graphin
#   /mcp 에서 graphin 연결 확인 → 도구 이름이 mcp__plugin_graphin_graphin__* 인지 (§8.1.1)
#   admin_addr 설정 후 페이지 확인 → 빈 값으로 되돌려 비활성 확인
#   <ws>/.graphin/admin-addr 가 전역 admin_addr를 이기는지 (D5)
#   SessionStart 훅과 MCP spawn 중 무엇이 먼저인지 관찰 (§11-2)

# Phase 5
claude --plugin-dir ./plugin/graphin-guide
#   에이전트의 skills:[graphin] 이 실제로 해석되는지 확인 (§11-3)

# Phase 7 — 유일하게 목표를 증명하는 테스트
# graphin 체크아웃도 자격증명도 없는 머신에서:
/plugin marketplace add Salvia95/graphin
/plugin install graphin@graphin
```

## 11. 실측 (추측 금지)

측정 방법: 설치된 Claude Code 2.1.221 바이너리의 플러그인 로더 코드와 공식
CHANGELOG 대조. 2·3은 정적으로 결론이 나지 않아 Phase 4·5 스모크로 넘긴다.

| # | 항목 | 결과 (2026-08-05, CC 2.1.221) |
|---|---|---|
| 1 | 수동 등록과의 충돌 | ✅ **중복.** 억제는 `command`+`args` 동일 시에만(env 제외). 런처 경유라 커맨드가 달라 둘 다 뜨고, 늦은 쪽이 `ErrLockHeld` → §8.1 |
| 2 | MCP spawn vs SessionStart 순서 | ⏳ 미해결. 설계는 순서 무관(런처가 권위) — Phase 4 스모크에서 첫 실행 UX만 확인 |
| 3 | `skills: [graphin]` 해석 | ✅ **맨 이름으로 동작한다.** 해석기는 ①정확 일치 → ②`<에이전트 네임스페이스>:<이름>` → ③`:<이름>` 접미사 순이라, 플러그인 에이전트는 ②에서 같은 플러그인의 스킬을 먼저 집는다. 실패해도 완전 침묵이 아니라 `Skill '…' specified in frontmatter was not found` 경고를 남긴다 |
| 4 | 도구 네임스페이싱 | ✅ **바뀐다.** `mcp__plugin_graphin_graphin__*` → §8.1.1. 계측·SKILL은 무사 |
| 5 | 미설정 `${user_config.KEY}` | ✅ 선언된 optional 키는 **빈 문자열**. 미선언·`required`+`default` 없음은 **예외로 서버 로드 실패**. boolean은 `"true"`/`"false"` → §6.4 |
| 6 | darwin dylib 경로 | ⏳ v1.1. 관례에서 추정하지 말고 조회한다 |
| 7 | 최소 CC 버전 | ✅ **2.1.83** (`userConfig`). `${CLAUDE_PLUGIN_DATA}`는 2.1.78. 2.1.207부터 plugin option은 **user settings에서만** 읽힘 → D5 |

추가로 확인된 것:

- 훅의 shell-form `command`에서 `${user_config.*}` 참조는 **로드 실패**를 부른다.
  `$CLAUDE_PLUGIN_OPTION_<KEY>`가 유일한 경로다(§6.7).
- stdio 서버 env에는 `CLAUDE_PLUGIN_ROOT`·`CLAUDE_PLUGIN_DATA`가 자동 주입된다(§6.4).
- `${CLAUDE_PLUGIN_DATA}` 실경로는 `~/.claude/plugins/data/<플러그인>-<마켓플레이스>/`
  — 이 머신의 `graphin-usage-graphin`으로 확인. 즉 `graphin-graphin`이 맞다.
- `Setup` 훅 이벤트가 존재하나 `--init`/`--init-only`/`--maintenance`에서만 발화하므로
  세션 설치 경로를 대체하지 못한다. `/graphin:setup`의 보조로는 쓸 수 있다.

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
