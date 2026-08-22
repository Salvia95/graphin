# 디자인 시스템 — 참조와 적용

graphin의 화면(현재는 `graphin console` 하나)이 따르는 시각 언어다. 본문 §1부터는
**Binance에서 추출한 참조 명세**를 그대로 싣는다. 그 앞의 §0이 graphin에 적용할 때
갈리는 지점과 그 이유를 적는다.

문서를 따로 둔 이유는 일관성이다. 화면이 하나뿐인 지금도 컴포넌트는 여러 사람이
여러 세션에 걸쳐 손대고, 색과 간격은 근거 없이 흔들리기 가장 쉬운 것들이다. 근거를
코드 옆이 아니라 문서에 두면 다음 사람이 그걸 읽고 같은 결정을 반복할 수 있다.

## 0. graphin 적용 규칙

### 0.1 토큰이 실제로 사는 곳

참조 명세의 `{colors.x}` 같은 표기는 YAML을 가리키지만, graphin에는 그 YAML이 없다.
**구현은 `internal/console/ui/src/index.css`의 Tailwind v4 `@theme` 블록**이고, 토큰
이름이 그대로 CSS 변수와 유틸리티 이름이 된다 — `{colors.canvas-dark}`는
`--color-canvas-dark`이고 `bg-canvas-dark`로 쓴다. 문서가 이름을 정하고 CSS가 값을
정한다.

### 0.2 폰트 — 외부 요청은 금지, 자체 호스팅은 가능, CJK가 진짜 제약

BinanceNova와 BinancePlex는 라이선스 폰트라 쓸 수 없다. 참조 명세가 지목한 대체를
따른다: 편집용은 **Inter**, 숫자용은 **IBM Plex Mono / JetBrains Mono** 계열.

**외부 호스트에 폰트를 요청하지 않는다.** 이것이 진짜 규칙이다. 콘솔은 사용자 기계에서
도는 로컬 도구이고, 로컬 도구가 CDN에 연결을 거는 것은 오프라인에서 깨지고 온라인에서는
설명할 이유가 없다. Google Fonts를 포함해 어떤 원격 자산도 쓰지 않는다.

**자체 호스팅은 금지가 아니다.** 이 절은 원래 "바이너리 크기 때문에 폰트 파일을 안
넣는다"고 적혀 있었는데, 재보니 그 근거가 틀렸다 — Latin woff2 서브셋 60KB는 21.3MB
바이너리의 **0.27%**다. 크기는 이 결정을 못 가른다.

**가르는 것은 CJK다.** 화면에 지나가는 데이터는 사용자가 쓴 것이라 한글이 섞인다(§0.7).
Latin 서브셋은 그걸 못 덮고, 한글을 덮는 웹폰트는 서브셋을 해도 수백 KB에서 MB 단위로
간다 — 여기서는 크기가 실제로 결정을 가른다. 그래서 **어떤 폰트를 넣든 한글은 시스템
폴백으로 렌더된다.** 디자인은 그 혼용 상태에서 성립해야 한다.

지금 상태는 폰트 파일 없음, 시스템 스택이다. 넣기로 한다면 Latin만 자체 호스팅하고
한글 폴백과의 조합을 눈으로 확인해야 한다.

숫자의 **기능**은 대체 폰트로도 지킨다. 참조 명세가 BinancePlex에 요구한 것은 장식이
아니라 자릿수 정렬이므로, 숫자 유틸리티에 `font-variant-numeric: tabular-nums`를 건다.

### 0.3 다크 단일 모드

참조 명세는 마케팅 표면을 다크, 거래 표면을 라이트로 나눈다. graphin 콘솔은 대시보드
하나이고, 명세가 대시보드에 배정한 것은 다크다. **라이트 모드를 만들지 않는다** —
어중간하게 둘 다 지원하면 두 벌 다 덜 다듬어진다.

### 0.4 거래 토큰의 재범위 — 이 문서의 유일한 의도적 이탈

참조 명세는 `{colors.trading-up}` / `{colors.trading-down}`을 **가격 방향 전용**으로
못 박고 일반적인 성공·실패로 전용하지 말라고 한다. 그 규칙이 존재하는 이유는 그
제품에 가격이 있기 때문이다. **graphin에는 가격이 없다.** 초록과 빨강이 다른 뜻으로
쓰일 여지 자체가 없으므로, 같은 색값을 상태 토큰으로 재범위한다:

| 참조 토큰 | graphin에서의 이름 | 뜻 |
|---|---|---|
| `trading-up` #0ecb81 | `status-good` | 사람이 확인함 (human-reviewed) |
| `trading-down` #f6465d | `status-alert` | 지금 독자에게 손해를 입히는 상태 (끊어진 링크, 용어집 포화) |

이 이탈은 여기 한 번만 적는다. 다른 토큰을 같은 논리로 재해석하려면 그 근거를 여기
추가해야 한다.

### 0.5 노랑의 희소성 — 심각도에 쓰지 않는다

참조 명세의 핵심 규칙은 `{colors.primary}`(#FCD535)를 **주 액션·브랜드 주장·워드마크**
에만 쓰는 것이다. 희소성이 그 색의 힘이다. 그래서 콘솔의 심각도 표시에 노랑을 쓰지
않는다. 결정 큐의 네 단계는 이렇게 간다:

| 단계 | 토큰 | 쓰이는 곳 |
|---|---|---|
| alert | `status-alert` #f6465d | 끊어진 링크, 용어집 포화 |
| watch | `status-watch` #f0b90b | 만료, 드리프트, 낡은 스킬 |
| info | `info` #3b82f6 | 승인 대기 — 당신을 기다리는 액션 |
| neutral | `muted` #707a8a | 안 읽히는 세트, 답 없던 작업 |

`status-watch`는 참조 명세의 `{colors.primary-active}`와 같은 값이다. 밝은 CTA 노랑과
구분되는 깊은 호박색이라 경고로 읽히고, 새 색을 들이지 않는다.

### 0.7 화면의 언어는 영어다

**UI 문자열은 전부 영어로 쓴다.** 라벨·버튼·탭·표 헤더·빈 상태 문구, 그리고 Go가
만드는 결정 카피(`internal/wiki/overview.go`의 `Detail`·`Action`)까지 포함한다.

한글은 **문서와 데이터에만** 남는다:

| 자리 | 언어 |
|---|---|
| `docs/**.md`, `docs/wiki/sets/**` | 한글 |
| 커밋 메시지 | 한글 (저장소 관례) |
| UI 문자열, 결정 카피 | **영어** |
| 화면에 지나가는 데이터(작업 문구, 세트 이름, 노드 ID) | 원문 그대로 |

이유는 번역 품질이다. 이 시스템의 용어 — drift, dangling, pin, set, trust, canonical —
는 코드와 문서에서 이미 영어로 확정돼 있고, 화면에서만 한글로 옮기면 같은 개념이 두
이름을 갖는다. 옮긴 말이 어색한 것은 표면 증상이고, 진짜 비용은 **화면과 코드가 서로
다른 어휘를 쓰게 되는 것**이다.

데이터는 번역하지 않는다. 사용자가 한글로 쓴 작업 문구는 한글로 뜨는 것이 맞다 —
그건 우리 카피가 아니라 그들의 내용이다.

### 0.6 숫자는 항상 숫자 폰트로

참조 명세가 "시스템 위반"이라고 부른 유일한 항목이다. 카운트·비율·통계는 전부 숫자
스택으로 렌더한다. 결정 개수, 제시/열람 횟수, 용어집 압력, 채택 지표가 전부 여기 든다.

숫자 스택은 **식별자에도 쓴다.** 노드 ID·세트 이름·canonical은 한 글자씩 읽히는
것이고, 그 점에서 숫자와 같은 것이다. `.num`이 둘 다 덮는 이유가 여기 있다.

### 0.8 심각도는 색 하나로 나르지 않는다 (2026-08-22, 시안 반영)

`docs/design-brief.md` §6.2가 지목한 약점이다 — alert(#f6465d)와 watch(#f0b90b)는
인접한 심각도인데 적록 색각 이상에서 가장 헷갈리는 조합이다. 완성된 시안의 답은
**같은 정보를 네 축에 동시에 싣는 것**이고, 그중 어느 하나만 읽혀도 답이 나온다.

| 축 | alert | watch | info | neutral |
|---|---|---|---|---|
| 색 | `status-alert` | `status-watch` | `info` | `muted` |
| 글리프 | ▲ | ◐ | ◆ | · |
| 라벨 | Fix now | Verify | Waiting on you | Signals |
| 왼쪽 띠 | 4px | 3px | 2px | 1px |
| 위치 | 그룹 1 | 그룹 2 | 그룹 3 | 백로그 탭 |

라벨이 **무엇이 잘못됐나가 아니라 무엇을 하라는 말**인 것이 핵심이다. 무엇이
잘못됐는지는 바로 아래 카드가 이미 말한다.

8종 → 4단계 매핑은 `internal/console/ui/src/lib/tiers.ts`에 있고, 브리프 §4.1의
톤 표와 같다. 노랑은 이 표 어디에도 없다(§0.5).

### 0.9 화면의 뼈대 — 시안이 확정한 것

브리프 §6이 열어 둔 질문 다섯의 답이다. 뒤집으려면 여기에 근거를 추가한다.

- **탭은 `Decisions`와 `Backlog` 둘.** neutral 두 종(안 읽히는 세트, 답 없던 작업)은
  오늘 아무것도 막지 않으므로 큐에서 내린다. 사라지는 게 아니라 자기 탭으로 간다.
- **`Wiki map`은 탭이 아니라 링크다.** 세트 패널 헤더에서 연다. 아무도 카탈로그를
  훑으려고 이 콘솔을 열지 않는다(브리프 §6.5).
- **세트는 큐 옆의 레일로 간다.** 단, 큐가 비면 레일이 비켜설 이유가 없으므로
  전폭 그리드로 바뀐다. 갈림은 `큐 길이 > 0` 하나다.
- **빈 큐는 초록 테두리의 주장이다.** "0 broken links"를 네 개 보여주는 것이
  "할 일 없음"보다 강하다 — 0은 근거고 빈 화면은 어깨짓이다(브리프 §6.3).
- **폼은 서랍이다.** 승인 카드가 인라인으로 펼쳐지면 아래 결정이 전부 밀린다.
  서랍은 목록의 위치를 지키고, 닫으면 떠난 줄로 정확히 돌아온다(브리프 §6.4).
  세트 상세도 같은 이유로 서랍이다 — 둘 다 장소가 아니라 작업이라 라우트가 아니다.

시안을 그대로 따르지 않은 곳은 셋이고, 전부 데이터나 계약이 없어서다.

- **`Last indexed` 줄을 안 넣었다.** 콘솔은 인덱스를 열 수 없다(console-spec §3).
  세트 수와 엔트리 수로 대체했다.
- **`Canonical` 입력은 읽기 전용이다.** `Store.Approve`가 canonical을 정체성으로
  못 박는다 — 다른 낱말은 다른 용어다. 편집 가능한 것처럼 그려 두면 조용히
  무시되는 필드가 된다.
- **카드 오른쪽 버튼은 `Copy`다.** 시안의 `Open set file`·`Retire a term`은 콘솔이
  할 수 없는 일이다. 대신 다시 치기 번거로운 것(노드 ID, 세트 파일 경로)을 넘긴다.
  `approve`만 진짜 액션(`Review candidate`)을 갖는다.

---

> **여기서부터는 참조 명세다.** Binance에서 추출한 것이고 원문을 보존한다.
> graphin에 적용할 때의 이탈은 §0에만 적는다 — 아래 본문을 고쳐 두면 참조와 적용이
> 뒤섞여, 다음 사람이 무엇이 원본이고 무엇이 우리 결정인지 못 가른다.

## Overview

Binance reads like a financial trading platform that wants to feel both authoritative and energetic. The base atmosphere is **deep near-black canvas** (`{colors.canvas-dark}` — #0b0e11) holding white type and a single, ubiquitous accent: **Binance Yellow** (`{colors.primary}` — #FCD535). That yellow does almost all of the brand's heavy lifting — it carries every primary CTA, every value-claim headline ("FUNDS ARE SAFU"), every "Sign Up" pill, every featured tier indicator, and the wordmark itself. There is no secondary brand color. The system trusts the yellow voltage to do the brand work, and it carries it.

Type runs Binance's custom **BinanceNova** (display + body) and **BinancePlex** (numerical / financial display) stack. BinanceNova carries display headlines, section titles, and body copy. BinancePlex appears on price tickers, large stat numbers (transaction volumes, user counts, prize pools) — anywhere a number wants to feel "tabular and reliable." Both run at modest weights — display sizes use weight 600-700 (bolder than typical marketing because trading platforms need numbers to read at a glance), body stays at 400.

The product is **multi-theme**: marketing surfaces (homepage, smart-money, futures arena) default to dark, while transactional surfaces (buy crypto, deposit, withdraw) flip to a light theme. The same yellow CTAs and gray-blue hairlines (`{colors.hairline-on-light}` — #eaecef) thread through both — only canvas, surface, and text tones flip. Trading **green** (`{colors.trading-up}` — #0ecb81) and **red** (`{colors.trading-down}` — #f6465d) signal price direction in tables, charts, and price tickers across both modes.

**Key Characteristics:**
- Single accent color: `{colors.primary}` (#FCD535) does all brand voltage — primary CTAs, hero headlines, brand mark, badges. Used scarcely on dark for emphasis, ubiquitously on transactional dialogs.
- Custom type stack: `BinanceNova` (display + body) and `BinancePlex` (numbers, prices, financial data). Big stat numbers always render in BinancePlex for tabular consistency.
- Multi-theme: marketing pages default dark (`{colors.canvas-dark}`); transactional pages flip light (`{colors.canvas-light}`). Yellow CTAs and trading green/red are shared across both.
- Light footer on dark body: the homepage uses `{colors.surface-soft-light}` (#fafafa) for the footer even when the body above it is dark — a deliberate inversion that visually closes the page.
- Trading semantics: green up / red down (`{colors.trading-up}` / `{colors.trading-down}`) for price changes, applied as text color rather than badge background.
- Card surfaces: `{colors.surface-card-dark}` (#1e2329) for elevated cards on dark; `{colors.canvas-light}` for cards on light. No gradient surfaces, no atmospheric backdrops — flat color blocks throughout.
- Border radius is small to medium: `{rounded.md}` (6px) for primary buttons, `{rounded.lg}` (8px) for inputs and content cards, `{rounded.xl}` (12px) for elevated card containers, `{rounded.pill}` for prominent feature CTAs.
- Spacing follows a 4-multiple scale; major editorial bands sit at `{spacing.section}` (80px) — slightly tighter than typical marketing-only sites because product pages need denser layouts.

## Colors

### Brand & Accent
- **Binance Yellow** (`{colors.primary}` — #FCD535): The single brand color. Used for primary CTA backgrounds, the wordmark, brand-claim headlines ("FUNDS ARE SAFU"), trust badges ("No.1 Trading Volume"), large stat numbers in `{component.stat-callout-card}`, and inline links.
- **Binance Yellow Active** (`{colors.primary-active}` — #f0b90b): The press / hover-darker variant. Slightly more saturated yellow.
- **Binance Yellow Disabled** (`{colors.primary-disabled}` — #3a3a1f): A desaturated dark-yellow used on disabled CTAs over dark canvas.
- **Accent Turquoise** (`{colors.accent-turquoise}` — #2dbdb6): A small secondary accent used very sparingly on Smart Money's "Check Now" CTA over dark surfaces. Treat as a single-product accent, not a system color.

### Surface

The system has two canvas modes that map to product context:

**Dark mode (marketing default):**
- **Canvas Dark** (`{colors.canvas-dark}` — #0b0e11): The primary page floor. Near-black with a slight warm tint — never pure black.
- **Surface Card Dark** (`{colors.surface-card-dark}` — #1e2329): Cards, navigation dropdowns, secondary buttons over dark canvas, markets table.
- **Surface Elevated Dark** (`{colors.surface-elevated-dark}` — #2b3139): One step lighter, used for nested cards, hovered nav items, and chart background panels.

**Light mode (transactional):**
- **Canvas Light** (`{colors.canvas-light}` — #ffffff): The page floor on transactional pages (buy crypto, deposit forms, account dialogs).
- **Surface Soft Light** (`{colors.surface-soft-light}` — #fafafa): Footer surface and disabled states.
- **Surface Strong Light** (`{colors.surface-strong-light}` — #f5f5f5): Form input backgrounds in muted contexts.

### Hairlines & Borders
- **Hairline on Light** (`{colors.hairline-on-light}` — #eaecef): The 1px border tone on light surfaces. Dembrandt's frequency analysis confirms this as the highest-count token (1022 occurrences) — Binance uses hairlines liberally.
- **Hairline on Dark** (`{colors.hairline-on-dark}` — #2b3139): The 1px border tone on dark surfaces. Same hex as `{colors.surface-elevated-dark}` — borders feel like surface steps, not ink lines.
- **Border Strong** (`{colors.border-strong}` — #cdd1d6): A heavier border tone used on disabled secondary buttons.

### Text
- **Ink** (`{colors.ink}` — #181a20): The strongest text on light surfaces. Display headlines on transactional pages.
- **Body on Dark** (`{colors.body}` — #eaecef): Default running-text on dark canvas — deliberately not pure white, slightly cooler.
- **Body on Light** (`{colors.body-on-light}` — #181a20): Same as ink — light-mode body text reuses the ink token.
- **Muted** (`{colors.muted}` — #707a8a): Footer links, breadcrumbs, captions, table column headers. Works on both light and dark canvas.
- **Muted Strong** (`{colors.muted-strong}` — #929aa5): A second-tier muted for emphasized labels.
- **On Primary** (`{colors.on-primary}` — #181a20): Black text on yellow primary CTAs.
- **On Dark** (`{colors.on-dark}` — #ffffff): Pure white for high-contrast headlines on dark canvas.

### Trading Semantics
- **Trading Up** (`{colors.trading-up}` — #0ecb81): Price-up green, used as text color in tables, charts, and inline ticker arrows. Never as a button background.
- **Trading Down** (`{colors.trading-down}` — #f6465d): Price-down red. Same usage rules as trading-up.

### Info / Focus
- **Info** (`{colors.info}` — #3b82f6): Inline info badges and the focus-ring base. The Tailwind `--tw-ring-color` token surfaced by dembrandt — used on input focus.

## Typography

### Font Family
The system runs **BinanceNova** for display and body, and **BinancePlex** for numerical / financial data. Both are licensed Binance custom typefaces. The fallback stack walks `-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif`.

The split is functional, not decorative:
- BinanceNova → editorial type (headlines, paragraphs, button labels, nav)
- BinancePlex → tabular numerical type (prices, volumes, percentages, stat counters, prize pools)

Mixing them is not optional — BinanceNova on a price ticker would lose the trading-platform character; BinancePlex on a paragraph would feel monospace-cold.

### Hierarchy

| Token | Size | Weight | Line Height | Letter Spacing | Use |
|---|---|---|---|---|---|
| `{typography.hero-display}` | 64px | 700 | 1.1 | -1px | Homepage h1 ("316,258,026 USERS TRUST US") |
| `{typography.display-lg}` | 48px | 700 | 1.1 | -0.5px | Brand-claim headlines ("FUNDS ARE SAFU"), prize-pool hero ("Futures Masters Arena") |
| `{typography.display-md}` | 40px | 600 | 1.15 | -0.3px | Section heads on long-scroll pages |
| `{typography.display-sm}` | 32px | 600 | 1.2 | 0 | CTA band headlines ("Secure, Low-Fee Trading on Binance") |
| `{typography.title-lg}` | 24px | 600 | 1.3 | 0 | Sub-section titles |
| `{typography.title-md}` | 20px | 600 | 1.35 | 0 | QR-promo cards, feature card titles |
| `{typography.title-sm}` | 16px | 600 | 1.4 | 0 | Trust badges, FAQ rows, step labels |
| `{typography.number-display}` | 40px | 700 | 1.1 | -0.3px | Big stat numbers (15,000 BTC, $429,423,449) — BinancePlex |
| `{typography.number-md}` | 16px | 500 | 1.4 | 0 | Markets table prices, table cells — BinancePlex |
| `{typography.number-sm}` | 14px | 500 | 1.4 | 0 | Inline prices, %  changes — BinancePlex |
| `{typography.body-md}` | 14px | 400 | 1.5 | 0 | Default running-text — BinanceNova |
| `{typography.body-sm}` | 13px | 400 | 1.5 | 0 | Cookie consent text, footer body |
| `{typography.caption}` | 12px | 500 | 1.4 | 0 | Small meta labels |
| `{typography.button}` | 14px | 600 | 1 | 0 | Standard CTA button labels |
| `{typography.nav-link}` | 14px | 500 | 1.4 | 0 | Top nav menu items |

### Principles
Display sizes use weight 700 — heavier than most marketing systems. This makes sense for a trading platform: numbers need to read at a glance, headlines need to compete with chart visualizations and dense data tables. The system will not soften display weight to 400 the way Airtable or Stripe does.

`{typography.number-display}` and the smaller number variants always use **BinancePlex**, even when surrounding body type uses BinanceNova. Prices, volumes, and stat counters render in BinancePlex regardless of context — it is the system's "trustworthy number" voice.

### Note on Font Substitutes
If BinanceNova and BinancePlex are unavailable, **Inter** is the closest open-source substitute for BinanceNova and **JetBrains Mono** or **IBM Plex Sans** is the closest substitute for BinancePlex (depending on whether tabular monospace fidelity matters more than humanist proportions). Adjust display headlines down by ~3% in line-height to match BinanceNova's tighter cap height.

## Layout

### Spacing System
- **Base unit:** 4px.
- **Tokens:** `{spacing.xxs}` 4px · `{spacing.xs}` 8px · `{spacing.sm}` 12px · `{spacing.md}` 16px · `{spacing.lg}` 24px · `{spacing.xl}` 32px · `{spacing.xxl}` 48px · `{spacing.section}` 80px.
- **Section padding (vertical):** `{spacing.section}` (80px) — slightly tighter than airy marketing sites (96px) because Binance pages mix marketing bands with dense product surfaces (markets tables, FAQ accordions).
- **Card internal padding:** `{spacing.lg}` (24px) for content cards and markets tables; `{spacing.xl}` (32px) for QR-promo cards and CTA bands; `{spacing.md}` (16px) for trust badges and table rows.
- **Gutters:** `{spacing.lg}` (24px) between cards in 3-up grids; `{spacing.md}` (16px) inside footer column gutters and dense FAQ lists.

### Grid & Container
- **Max content width:** ~1280px centered on marketing pages; ~1440px on product surfaces (markets, smart-money tables) where horizontal density matters.
- **Editorial body:** Single 12-column grid; product pages often use 8/4 split (main panel + side rail).
- **Markets table:** 5-column header (Pair / Last Price / 24h Change / 24h Volume / Action), with the first column carrying coin icon + symbol pair.
- **Footer:** 6-column link list at desktop, wrapping to 2-up at tablet and 1-up on mobile.

### Whitespace Philosophy
Binance is denser than typical marketing sites — long-scroll pages mix hero bands with markets tables, FAQ accordions, and feature grids without much breathing room between them. The system trusts contrast (yellow vs. dark canvas, green vs. red price cells) to do the visual separation work, not whitespace. Where whitespace appears, it's always uniform — `{spacing.section}` between every major band.

## Elevation & Depth

| Level | Treatment | Use |
|---|---|---|
| Flat | No shadow, no border | Body sections, top nav, hero bands, footer |
| Soft hairline | 1px `{colors.hairline-on-dark}` or `{colors.hairline-on-light}` | Inputs, table dividers, FAQ row separators, secondary buttons |
| Card surface | `{colors.surface-card-dark}` background on dark canvas, `{colors.canvas-light}` on light context — no shadow | All elevated cards (markets-table-card, QR-promo-card, feature-photo-card, trust-badges) |
| Subtle drop shadow | Faint shadow visible only when a card sits over imagery | Used sparingly on the buy-crypto-amount-card on transactional pages |
| Focus ring | `0 0 0 2px {colors.info-ring}` at 50% alpha | Input + button keyboard focus state |

The elevation philosophy is **flat surfaces with color-block separation**. Binance does not use heavy drop shadows or glassmorphism — depth comes from the contrast between `{colors.canvas-dark}` and `{colors.surface-card-dark}` (a 12-step lightness jump that reads as a clear elevation boundary).

### Decorative Depth
- **Yellow → dark vertical gradient backdrop** on the Futures Arena hero: `{colors.primary}` fading down to `{colors.canvas-dark}`. This is a single-page treatment used for product-launch / event hero surfaces, not a system-wide signature.
- **Coin-stack illustrations** flanking large stat blocks (3D rendered crypto coins, trophy icons). These are illustrations, not tokens — treat as content rather than design system surface.

## Shapes

### Border Radius Scale

| Token | Value | Use |
|---|---|---|
| `{rounded.xs}` | 2px | Almost no use — reserved for very small badges |
| `{rounded.sm}` | 4px | Small inline buttons (subscribe, trading-up / trading-down inline) |
| `{rounded.md}` | 6px | Standard CTA buttons, primary buttons, primary input fields |
| `{rounded.lg}` | 8px | Search input, content cards, trust badges, sub-cards |
| `{rounded.xl}` | 12px | Elevated card containers (markets-table-card, QR-promo-card, CTA bands) |
| `{rounded.pill}` | 9999px | Prominent feature CTAs ("Sign Up" pill on dark, futures-arena "Join Now") |
| `{rounded.full}` | 9999px / 50% | Coin icons, avatars |

Binance's radius hierarchy is tighter than typical marketing systems — most surfaces sit at 6-12px. The pill radius is a deliberate exception used to signal "this is a top-of-page action."

### Photography & Iconography
- Coin icons render as 24×24 or 32×32 rounded glyphs (often 50% radius on circular outline + the coin's brand color inside).
- 3D rendered coin stacks and trophy illustrations are full-color illustrations with a slight floor shadow — not flat icons.
- Photographic content (people-using-the-app section) crops to `{rounded.xl}` (12px) corners, full-bleed on mobile.

## Components

### Top Navigation

**`top-nav-dark`** — The marketing top nav on dark canvas. 64px tall, `{colors.canvas-dark}` background. Carries the yellow Binance wordmark at left, primary horizontal menu (Buy Crypto, Markets, Trade, Futures, Earn, Square, Smart Money, Campaigns), right-side cluster with language selector, light/dark toggle, "Log In" text link, "Sign Up" `{component.button-primary}`. The wordmark uses `{colors.primary}` for "BINANCE" type.

**`top-nav-light`** — The transactional top nav on light canvas (buy crypto, deposit pages). Same layout but `{colors.canvas-light}` background and `{colors.ink}` menu items.

### Buttons

**`button-primary`** — The signature primary CTA. Background `{colors.primary}`, text `{colors.on-primary}` (black on yellow — the system's iconic combination), type `{typography.button}`, padding 12px × 24px, height 40px, rounded `{rounded.md}` (6px). Press state: `button-primary-active` darkens to `{colors.primary-active}` (#f0b90b). Disabled state: `button-primary-disabled` desaturates to `{colors.primary-disabled}`.

**`button-primary-pill`** — A larger pill variant of the primary CTA used for top-of-page sign-up moments and product-launch heroes (Futures Arena "Join Now"). Same yellow + black combination, padding 14px × 32px, rounded `{rounded.pill}` (9999px). Use sparingly — the pill is a "this is THE action" signal.

**`button-secondary-on-dark`** — Used over `{colors.canvas-dark}` for less-emphasized actions. Background `{colors.surface-card-dark}`, text `{colors.on-dark}`, rounded `{rounded.md}`.

**`button-secondary-on-light`** — Light-canvas equivalent. Background `{colors.canvas-light}` with `{colors.hairline-on-light}` 1px border, text `{colors.ink}`.

**`button-tertiary-text`** — Inline text button with no background. Used for "Log In" in the top nav and inline "Read More" links.

**`button-trading-up`** — A solid green button used on price-up signals (Buy / Long actions). Background `{colors.trading-up}`, text `{colors.on-dark}`, rounded `{rounded.sm}` (4px), padding 8px × 20px. Smaller and tighter than `{component.button-primary}` because it appears in dense trading interfaces.

**`button-trading-down`** — Symmetric red variant for Sell / Short actions. Same shape, background `{colors.trading-down}`.

**`button-subscribe`** — Compact yellow CTA used in the Smart Money traders table to subscribe to a top trader. Smaller height (28px) and tighter padding than the primary CTA — fits inside dense table rows. Same yellow + black combination.

**`text-link`** — Inline body links in `{colors.primary}` (yellow on dark, also yellow on light). No underline by default. Type inherits `{typography.body-md}`.

### Cards & Containers

**`hero-band-dark`** — Full-width dark band carrying the homepage h1 + sub-headline + dual CTA pair. Background `{colors.canvas-dark}`, padding `{spacing.section}` (80px). The h1 ("316,258,026 USERS TRUST US") uses `{typography.hero-display}` at 64px / 700 — the system's largest type role.

**`stat-callout-card`** — Inline yellow stat numbers (15,000 BTC, 7,488,223, $429,423,449). Transparent background, text `{colors.primary}`, type `{typography.number-display}` in BinancePlex. Used as a flat layout block, not a card with surface — the yellow text alone carries the visual weight.

**`trust-badge`** — Small dark cards holding "No.1 Customer Service" / "No.1 Trading Volume" claims. Background `{colors.surface-card-dark}`, rounded `{rounded.lg}` (8px), padding 16px × 20px. Yellow numeric or word badge ("No.1") sits next to a short label.

**`markets-table-card`** — The right-side markets table on the homepage. Background `{colors.surface-card-dark}`, rounded `{rounded.xl}` (12px), padding `{spacing.lg}` (24px). Carries a tab row (Popular / New listing / Top gainers), then a 5-column row of coin pairs with last price, 24h change %, action button. Each row uses `{component.markets-row}`.

**`markets-row`** — A single row inside the markets table. Transparent background, 12px vertical padding, hairline divider between rows. Coin icon (32×32) + symbol on left; last price in `{typography.number-md}` (BinancePlex); 24h change cell colored by direction (`{component.price-up-cell}` or `{component.price-down-cell}`); right-aligned chevron icon for "view detail."

**`price-up-cell`** / **`price-down-cell`** — Colored text cells for price changes. Transparent background, text `{colors.trading-up}` or `{colors.trading-down}`, type `{typography.number-md}` in BinancePlex. Always paired with a small triangle arrow indicating direction.

**`feature-photo-card`** — The "Trade on the go" section's photo strip — 3 lifestyle photos showing people using the Binance app. Background `{colors.surface-card-dark}`, rounded `{rounded.xl}`. Photos crop edge-to-edge, no internal padding around the image.

**`qr-promo-card`** — The "Trade on the go. Anywhere, anytime." card with QR code. Background `{colors.surface-card-dark}`, rounded `{rounded.xl}`, padding `{spacing.xl}` (32px). Contains an h2 in `{typography.title-md}`, a body paragraph, app store badges (iOS / Android), and a centered QR code.

**`funds-safu-band`** — The yellow-headlined "FUNDS ARE SAFU" band. Background stays `{colors.canvas-dark}`, but the headline uses `{colors.primary}` at `{typography.display-lg}`. Below the headline, three large `{component.stat-callout-card}` numbers anchor the band: total BTC reserves, users helped, funds recovered.

**`faq-row`** — A single FAQ accordion row. Transparent background, padding 20px vertical, hairline divider between rows. Closed state: question in `{typography.title-sm}` + chevron icon at right. Open state: question + answer body in `{typography.body-md}`.

**`cta-band-dark`** — The "Secure, Low-Fee Trading on Binance" pre-footer CTA band. Background `{colors.surface-card-dark}` (one step elevated from canvas), rounded `{rounded.xl}`, padding `{spacing.xxl}` (48px). Carries an h2 in `{typography.display-sm}` and a `{component.button-primary}` aligned right.

### Light-Mode Transactional Components

**`buy-crypto-amount-card`** — The right-rail card on the Buy BTC page. Background `{colors.canvas-light}`, rounded `{rounded.lg}` (8px), padding `{spacing.lg}` (24px). Carries an editable amount input in `{typography.number-display}` (BinancePlex), a currency selector, and a yellow `{component.button-primary}` for "Continue" / "Confirm Order."

**`steps-card`** — The "How to Buy Crypto" 3-up cards (Enter Amount → Confirm Order → Receive Crypto). Background `{colors.canvas-light}`, rounded `{rounded.lg}`, padding `{spacing.lg}`. Each card has a small numbered icon, a `{typography.title-sm}` step name, and a body description.

**`price-chart-card`** — The "Bitcoin Markets" card carrying the BTC price chart. Background `{colors.canvas-light}`, rounded `{rounded.lg}`. Top row carries pair selector ($79,065.04, +0.45%); main area is a candlestick / line chart in `{colors.trading-up}` and `{colors.trading-down}`; bottom row carries timeframe selector (24H / 1W / 1M / 3M / 1Y / ALL).

**`conversion-cell`** — A single row in the BTC ↔ USD conversion table. Transparent background, text `{colors.body-on-light}`, type `{typography.body-md}`. Pair label on left (BTC, USDT, etc.); USD equivalent on right.

### Inputs & Forms

**`search-input-on-dark`** — The "Search currencies" input on the homepage hero. Background `{colors.surface-card-dark}`, text `{colors.on-dark}`, rounded `{rounded.lg}` (8px), padding 10px × 16px, height 40px. Carries a yellow `{component.button-primary-pill}` on the right side ("Sign Up").

**`text-input-on-light`** — Standard input on transactional pages. Background `{colors.canvas-light}`, 1px `{colors.hairline-on-light}` border, rounded `{rounded.md}` (6px), padding 10px × 16px, height 40px. Focus state inherits the focus-ring shadow.

**`cookie-consent-card`** — The cookie banner card visible on the homepage. Background `{colors.canvas-light}`, rounded `{rounded.lg}`, padding `{spacing.md}` (16px). Body text in `{typography.body-sm}` (13px / 400) with three stacked button options (Accept Cookies & Continue / Reject Additional Cookies / Manage Cookies).

### Smart Money Sub-System

**`trader-row`** — A single row in the top-traders table on /smart-money. Transparent background, padding 12px vertical, hairline divider between rows. Avatar + trader name + private/public badge on left; ROI %, AUM, mint date columns; yellow `{component.button-subscribe}` on right.

### Signature Components

**`arena-hero-gradient`** — The Futures Arena product-launch hero. A vertical gradient from `{colors.primary}` at top to `{colors.canvas-dark}` at bottom, with the prize-pool headline (4,000,000 USDT) in `{typography.display-lg}` centered. A `{component.button-primary-pill}` ("Join Now") sits below the headline. Used only on product-launch event surfaces — do not generalize to other heroes.

### Footer

**`footer-light`** — The light-gray footer that closes every page (including dark-canvas pages). Background `{colors.surface-soft-light}` (#fafafa), text `{colors.body-on-light}`. 6-column link list at desktop covering Community / About Us / Products / Business / Service / Learn columns. Vertical padding 64px. The deliberate light footer on a dark page is one of Binance's most distinctive layout choices — it visually closes the page with a "marketing reset" surface.

## Do's and Don'ts

### Do
- Reserve `{colors.primary}` (Binance Yellow) for primary actions, brand-claim headlines, and the wordmark. Never use it for secondary or decorative purposes — yellow's scarcity is what makes it powerful.
- Keep `{component.button-primary}` (yellow with black text) as the universal primary CTA across both dark and light modes. The same button appears identically on `{colors.canvas-dark}` and `{colors.canvas-light}`.
- Use `{component.button-trading-up}` (green) and `{component.button-trading-down}` (red) only for explicit Buy/Sell or Long/Short actions. Never use them for general "confirm" or "cancel" because they carry semantic price-direction meaning.
- Use BinancePlex for every number. Prices, volumes, percentages, stat counters — all BinancePlex. Mixing BinanceNova into a number ticker breaks the trading-platform character.
- Choose canvas mode by surface intent: dark for marketing / product showcase / trading dashboards; light for transactional dialogs (buy / deposit / withdraw / form submission).
- Anchor every editorial band with `{spacing.section}` (80px). Binance is denser than airy marketing sites — 80px is the right rhythm.

### Don't
- Don't introduce a second brand color. The system has exactly one accent (`{colors.primary}`) and any expansion dilutes the brand identity. The turquoise on Smart Money is a single-product experiment, not a system token.
- Don't use yellow for body text or large surface fills. It is for focal-point CTAs and headlines only.
- Don't use `{colors.trading-up}` / `{colors.trading-down}` as background fills on cards. They are price-direction signals, expressed as text color or small badge fill — never as a card surface.
- Don't soften display weight. `{typography.hero-display}` and `{typography.display-lg}` are intentionally weight 700 — going to 400 reads as design-portfolio, not trading platform.
- Don't add atmospheric gradients to the canvas (mesh, aurora, glow effects). Binance trusts color-block contrast — adding atmospheric depth muddies the trading-platform feel.
- Don't invert `{component.button-primary}`'s text color. Black on yellow is the system's signature — white text on yellow loses contrast and brand recognition.

## Responsive Behavior

### Breakpoints

| Name | Width | Key Changes |
|---|---|---|
| Mobile | < 768px | Top nav collapses to hamburger; hero h1 drops from 64px to ~36px; markets table converts to a horizontally-scrollable card list; demo grids drop to 1-up; footer 6 columns wrap to 2 |
| Tablet | 768–1024px | Top nav stays horizontal but tightens, secondary menu items hide behind a "More" dropdown; markets table 2-up; pricing/feature grids 2-up |
| Desktop | 1024–1440px | Full top-nav with all primary menu items; 5-column markets table; trading dashboards in 8/4 split (chart + side rail) |
| Wide | > 1440px | Same as desktop with more outer breathing room; max content width caps at 1280-1440px depending on surface |

### Touch Targets
- Primary CTAs render at minimum 40 × 40px (`{component.button-primary}` height + padding) — meets WCAG AAA's 44 × 44 with surrounding spacing.
- Subscribe / inline action buttons are 28 × 28 — denser than ideal but matches industry trading platform norms.
- Coin icons in markets tables are 32 × 32px, with the entire row tappable for 44px+ effective target.

### Collapsing Strategy
- Top nav collapses to hamburger at < 768px; the menu opens as a full-screen sheet with the same yellow accent CTAs anchored to the bottom of the sheet.
- Markets table reflows to a horizontally-scrollable single card per coin pair on mobile.
- The hero stat numbers ("316M USERS") shrink proportionally rather than wrapping — Binance's biggest claim must always read as a single block.
- Trading dashboards switch from chart + side-rail to chart-only with a separate "Trade" tab on mobile.
- The light footer stays full-bleed at every breakpoint — it does not collapse to a separate dark variant.

### Image Behavior
- Coin icons stay at fixed 24/32px sizes regardless of breakpoint.
- Lifestyle photos in the "Trade on the go" section crop responsively — wider at desktop, taller (vertical) at mobile.
- 3D coin-stack illustrations are fixed-aspect-ratio assets that scale uniformly without cropping.

## Iteration Guide

1. Focus on ONE component at a time. Reference its YAML key directly (`{component.button-primary}`, `{component.markets-row}`).
2. When adding a new component, decide first whether it lives in dark mode (marketing / product) or light mode (transactional). The same component appears in both with surface tone flipped.
3. Variants of an existing component (`-active`, `-disabled`) live as separate entries in `components:` — never as nested state objects.
4. Use `{token.refs}` everywhere prose mentions a color, a radius, a typography role, or a spacing value.
5. Never document hover. The system documents Default and Active/Pressed states only.
6. Numbers always use BinancePlex; copy always uses BinanceNova. Mixing them is a system violation.
7. Trading green / red are semantic price tokens — never repurpose them for "success" or "error" generic states.

## Known Gaps

- The dembrandt frequency analyzer captured `#eaecef` (light hairline, count 1022) as the highest-frequency token. The brand-defining `{colors.primary}` (#FCD535) appears far less frequently because it's used scarcely as accent — its system role had to be confirmed from screenshots.
- BinanceNova and BinancePlex weight-axis values are not formalized as variable-font tokens — only the static weights observed in screenshots are documented.
- Animation and transition timings (chart redraws, price-change flashes) are not in scope.
- Form validation states beyond `{component.text-input-on-light}` defaults are not extracted — error / success input variants would need a sign-up or order-confirmation flow to confirm.
- The trading dashboard surfaces (Spot / Futures / Margin) were not in the analyzed URL set; their order book, candlestick chart configuration, and position-management cards are not documented here.
- The light/dark theme toggle behavior (whether transactional pages can be forced dark by user preference) is product behavior, not extracted from the marketing surfaces.
