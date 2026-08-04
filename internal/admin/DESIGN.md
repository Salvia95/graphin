# MCP Admin — Design System

> **Scope.** This document defines the design system only — principles, tokens,
> component specs. Which of it graphin has adopted, what is deferred, and which
> exceptions are approved live in **[DECISIONS.md](DECISIONS.md)**. Read both
> before implementing: where this spec and the current code disagree, check the
> adoption table in DECISIONS.md first — it may be a deferred item, not a bug.

UI specification for the MCP server admin console. Built on **htmx 2 + Pico CSS 2 (classless)**.
Keep Pico's classless defaults and **override CSS variables only**. Do not introduce classes beyond the ones defined here.

- Audience: developers who maintain the codebase
- Data: relationship graphs, configuration values, source code
- Reference viewport: 1440×900 (key metrics must fit without scrolling)
- Base type size: 14px, line-height 1.55
- UI copy language: Korean, with the English technical term alongside (see §3)

---

## 1. Principles

| # | Principle | Implementation rule |
|---|---|---|
| P1 | **Separate key from value** | Keys: sans, muted, fixed-width left column. Values: mono, full-contrast foreground. Distinguish on three axes at once — color, typeface, alignment. |
| P2 | **Size encodes hierarchy** | The higher a component sits in the tree, the larger its font, weight and padding. Never step outside the 4-level type scale (lv1–lv4). |
| P3 | **Z-pattern placement** | Top-left = identity and status. Bottom-right = actions. Destructive actions always sit at the far right. |
| P4 | **One screen, one glance** | At 1440×900 the key metrics must be visible without scrolling. Details are deferred behind htmx lazy loading (collapse/expand). |
| P5 | **Grid alignment** | Spacing in multiples of 4px. Information on the same axis shares the key-column width (`--key-col`), the baseline, and right-aligned numerics. |
| P6 | **Help next to terminology** | Put ⓘ next to proper nouns and MCP concepts; clicking loads the explanation via htmx. Hover is never the trigger. |

**Prohibited**: box shadows (separate surfaces with a 1px line plus a background step only); communicating state through color alone; any layout where the value column shifts horizontally between read and edit states.

---

## 2. Tokens

Save as `static/css/mcp-theme.css` and load it after Pico.

Assets are served from the binary, so the stylesheets are vendored rather than
imported from a CDN. Load order is fixed by the `<link>` order in the layout:

```html
<link rel="stylesheet" href="/static/pico.min.css">   <!-- Pico 2 classless, vendored -->
<link rel="stylesheet" href="/static/theme.css">      <!-- the token layer below -->
```

The Pretendard variable font is vendored into `static/` and declared with
`@font-face` at the top of the token file:

```css
@font-face {
  font-family: "Pretendard Variable";
  src: url("/static/PretendardVariable.woff2") format("woff2-variations");
  font-weight: 45 920; font-style: normal; font-display: swap;
}
```

```css
:root {
  /* --- Typefaces --- */
  --pico-font-family: "Pretendard Variable", Pretendard, -apple-system,
                      "Apple SD Gothic Neo", "Malgun Gothic", sans-serif;
  --pico-font-family-monospace: "D2Coding", "JetBrains Mono", "SFMono-Regular",
                                Consolas, monospace;
  --pico-font-size: 87.5%;            /* 14px base */
  --pico-line-height: 1.55;

  /* --- Density --- */
  --pico-spacing: 1rem;
  --pico-block-spacing-vertical: .75rem;
  --pico-form-element-spacing-vertical: .375rem;
  --pico-form-element-spacing-horizontal: .625rem;
  --pico-border-radius: 5px;

  /* --- Layout constants --- */
  --key-col: 180px;      /* key column width, shared across all screens (150px in dense panels) */
  --indent: 20px;        /* tree indent per depth level */
  --sidebar: 220px;

  /* --- Color (light) --- */
  --c-bg: #f6f7f9;  --c-surface: #ffffff;  --c-surface-2: #f0f2f5;
  --c-code-bg: #f4f6f8;
  --c-line: #dfe3e8;  --c-line-strong: #c3cad3;
  --c-fg: #12161c;  --c-fg-muted: #5a6472;  --c-fg-dim: #8a94a3;
  --c-accent: #2c5fd6;  --c-accent-bg: #e8eeff;
  --c-ok:   #17714a;  --c-ok-bg:   #dff3e8;
  --c-warn: #8a5a00;  --c-warn-bg: #fdf0d5;
  --c-err:  #b3261e;  --c-err-bg:  #fce8e6;
  --c-info: #1f6a83;  --c-info-bg: #e0f2f7;

  --pico-primary: var(--c-accent);
  --pico-background-color: var(--c-bg);
  --pico-card-background-color: var(--c-surface);
  --pico-muted-color: var(--c-fg-muted);
  --pico-muted-border-color: var(--c-line);
}

[data-theme="dark"] {
  --c-bg: #0e1116;  --c-surface: #151a21;  --c-surface-2: #1b2129;
  --c-code-bg: #11161d;
  --c-line: #262e38;  --c-line-strong: #3a4553;
  --c-fg: #e6ebf2;  --c-fg-muted: #9aa6b5;  --c-fg-dim: #6c7786;
  --c-accent: #7aa2f7;  --c-accent-bg: #1b2740;
  --c-ok:   #5ec98b;  --c-ok-bg:   #14291f;
  --c-warn: #e0b04a;  --c-warn-bg: #2b2312;
  --c-err:  #f2807c;  --c-err-bg:  #2e1817;
  --c-info: #63c1dc;  --c-info-bg: #10262d;
}
```

Theme follows the Pico convention: `<html data-theme="light|dark">`. To honor the OS setting when the attribute is absent, duplicate the dark values into
`@media (prefers-color-scheme: dark) { :root:not([data-theme]) { … } }`.

### Spacing and radius

Use only `4 / 8 / 12 / 16 / 24 / 32 / 48` (multiples of 4px).
Radius: `4px` badges and inputs, `8px` blocks, `10px` panels.

---

## 3. Typography

| Token | Size / weight | Used for |
|---|---|---|
| lv1 | 26–28px / 800, ls −0.02em | Page title, server name |
| lv2 | 18–19px / 700, ls −0.01em | Panel heading |
| lv3 | 15.5px / 650 | Card and group heading, tree depth 1 |
| lv4 | 13px / 500 | Property labels, tree leaves |
| body | 14px / 400 | Prose, descriptions |
| meta | 11.5–12px / 500, `--c-fg-dim` | Timestamps, secondary info |
| section-eyebrow | 11.5px / 700, ls 0.08–0.12em, uppercase | Section divider label |

**Use monospace for**: identifiers, paths and URIs; configuration values; every numeric; status badge labels; code blocks; all columns of the log table.
Numerics always get `font-variant-numeric: tabular-nums` and right alignment.

Bilingual labeling: the Korean label comes first, the English source term follows inside `<code>` — e.g. `전송 방식 <code>transport</code>`. Korean strings in the snippets below are literal UI copy; ship them as written.

---

## 4. Components

### 4.1 Status badge — `.badge`

**The status vocabulary itself is defined by the implementer.** The design system only fixes its shape:

- At most 5 values, lowercase, monospace
- Color maps to exactly five semantic axes — healthy `ok`, caution `warn`, failure `err`, neutral, in-progress `info`
- Never color alone: always a dot plus the text label

```css
.badge { display:inline-flex; align-items:center; gap:6px;
  font-family: var(--pico-font-family-monospace); font-size:12px; font-weight:500;
  padding:3px 9px; border-radius:4px; border:1px solid currentColor;
  line-height:1.2; }
.badge::before { content:""; width:6px; height:6px; border-radius:50%;
  background: currentColor; }
.badge.ok      { color:var(--c-ok);   background:var(--c-ok-bg); }
.badge.warn    { color:var(--c-warn); background:var(--c-warn-bg); }
.badge.err     { color:var(--c-err);  background:var(--c-err-bg); }
.badge.info    { color:var(--c-info); background:var(--c-info-bg); }
.badge.neutral { color:var(--c-fg-dim); background:var(--c-surface-2);
                 border-color:var(--c-line-strong); }
/* dot-less tag form (enabled / overridden, etc.) */
.tag { font-family:var(--pico-font-family-monospace); font-size:11.5px;
  padding:1px 6px; border-radius:4px;
  background:var(--c-surface-2); color:var(--c-fg-muted); }
.tag.overridden { border:1px dashed var(--c-line-strong); }
```

```html
<span class="badge ok">running</span>
<span class="badge neutral">stopped</span>
```

### 4.2 Key / value

All three patterns share one rule: key in sans, muted, fixed at `--key-col`; value in mono, foreground color.

**A. Two-column grid (default)** — reuse Pico's `<dl>` as-is.

```css
dl { display:grid; grid-template-columns: var(--key-col) 1fr; gap:.5rem 1.25rem;
     margin:0; }
dt { color: var(--c-fg-muted); font-size:13px; }
dd { margin:0; font-family: var(--pico-font-family-monospace); font-size:13px;
     font-variant-numeric: tabular-nums; }
dd.str  { color: var(--c-accent); }   /* "stdio" */
dd.bool { color: var(--c-ok); }       /* true */
dd.off  { color: var(--c-err); }      /* false */
```

**B. Stacked** — for long values (arrays, commands) or values that carry an explanation. Key 12px above, value 13px mono below, `word-break: break-all`.

**C. Row-divided — `.kv-table`** — for configuration lists where each value also carries a source, a state and an action.

```css
.kv-table { display:grid; grid-template-columns: var(--key-col) 1fr 120px 110px; }
.kv-table > * { padding:10px 24px; border-bottom:1px solid var(--c-line);
                align-self:center; }
.kv-table .head { background:var(--c-surface-2); font-size:12px;
                  color:var(--c-fg-dim); padding-block:8px; }
.kv-table .row-changed { background: var(--c-accent-bg); }
```
Columns: key / value / source (`default`, `env`, `override`) / action (right aligned).

### 4.3 Term help — ⓘ

Click-triggered popover. Load the body over htmx so the initial HTML stays small.

```html
<span class="term">
  전송 방식 <code>transport</code>
  <button class="help"
          hx-get="/help/transport"
          hx-target="next .popover"
          hx-swap="innerHTML"
          aria-label="transport 설명">i</button>
  <div class="popover" role="tooltip"></div>
</span>
```

```css
.term { position:relative; }
.help { width:16px; height:16px; padding:0; line-height:1; font-size:11px;
  border-radius:50%; border:1px solid var(--c-line-strong);
  background:transparent; color:var(--c-fg-dim); cursor:pointer;
  vertical-align:1px; margin-left:5px; }
.help:hover { border-color:var(--c-accent); color:var(--c-accent); }
.popover:empty { display:none; }
.popover { position:absolute; top:26px; left:0; z-index:20; width:320px;
  background:var(--c-surface); border:1px solid var(--c-line-strong);
  border-radius:8px; padding:14px 16px; font-size:13px;
  color:var(--c-fg-muted); text-wrap:pretty; }
/* short terms get a dotted underline only */
abbr[title] { border-bottom:1px dotted var(--c-fg-dim); cursor:help;
              text-decoration:none; }
```

The server returns a fragment shaped like `<div class="popover-title">transport</div><p>…</p>`. Close an open popover from a document-level click handler or `hx-on:click`.

### 4.4 Hierarchy tree (chosen approach: indented tree)

20px indent per depth level plus a left guide line. Node label in mono; the kind badge sits in a left-aligned column; metrics sit in a right-aligned column. Font size shrinks with depth (P2).

```html
<li data-depth="2">
  <button hx-get="/tree/tools/search_repo/children"
          hx-target="closest li" hx-swap="beforeend"
          aria-expanded="false">▸</button>
  <code>search_repo</code>
</li>
```

```css
.tree { display:grid; grid-template-columns: 1fr 92px 84px 76px; } /* node / kind / calls / state */
.tree li[data-depth="0"] > code { font-size:14.5px; font-weight:700; }
.tree li[data-depth="1"] > code { font-size:13.5px; font-weight:600; padding-left:20px; }
.tree li[data-depth="2"] > code { font-size:13px;   font-weight:400; padding-left:40px; }
.tree li[data-depth]     { border-left:1px solid var(--c-line); }
.tree li:hover           { background: var(--c-surface-2); }
```

Caret glyphs: collapsed `▸`, expanded `▾`, leaf `·`. While loading, `hx-indicator` swaps a spinner into the caret slot.
Beyond depth 4, offer a column browser or relation table as a secondary view (§7).

### 4.5 Source viewer

Line numbers in a right-aligned 48px fixed column. Highlighted lines get a 4px left bar plus a background tint. 12.5px / 1.65.

```css
.code { background:var(--c-code-bg); border:1px solid var(--c-line);
        border-radius:10px; overflow:hidden; }
.code .line { display:grid; grid-template-columns:48px 1fr;
              font-family:var(--pico-font-family-monospace);
              font-size:12.5px; line-height:1.65; }
.code .ln   { text-align:right; padding-right:14px; color:var(--c-fg-dim);
              user-select:none; }
.code .src  { padding-left:12px; white-space:pre; }
.code .line.hl { background:var(--c-warn-bg); }
.code .line.hl .ln  { color:var(--c-warn); }
.code .line.hl .src { padding-left:8px; border-left:4px solid var(--c-warn); }
```
The header bar carries `path · L18–24 · language` (mono, 12.5px) with a copy button on the right.

### 4.6 Log / trace

Every column is mono with `tabular-nums`. Timestamp, level, latency and status have fixed widths; only the message flexes. The level is scannable via a 4px left bar.

```css
.log { display:grid; grid-template-columns:132px 58px 1fr 74px 60px; }
.log > .row { display:contents; }
.log .cell { padding:5px 24px; border-bottom:1px solid var(--c-line);
             font-family:var(--pico-font-family-monospace); font-size:12.5px;
             font-variant-numeric: tabular-nums; }
.log .num { text-align:right; }
.row.warn  .cell:first-child { box-shadow: inset 4px 0 0 var(--c-warn); }
.row.error .cell:first-child { box-shadow: inset 4px 0 0 var(--c-err); }
```
Append new rows with `hx-swap="afterbegin"` plus `hx-trigger="every 5s"` — only where live updates are actually needed.

### 4.7 Form controls

The label column uses the **same** width as the read-only view (`--key-col`), so the value never moves horizontally when switching between read and edit (P1, P5).

```css
input, select, textarea { font-family: var(--pico-font-family-monospace);
  font-size:13px; }
input[type="number"], input.num { width:110px; text-align:right;
  font-variant-numeric: tabular-nums; }
:is(input,select):focus { border-color:var(--c-accent);
  box-shadow:0 0 0 3px var(--c-accent-bg); outline:none; }
[aria-invalid="true"] { border-color: var(--c-err); }
.field-error { font-size:12px; color: var(--c-err); margin-top:4px; }
```
The action bar sits at the bottom right of the panel: supporting text on the left, then `revert`, then `save` (P3).

---

## 5. Layout (chosen approach: left sidebar)

```
┌─ aside 220px ─┬─ main ───────────────────────────────┐
│ server + state│ h1 title            [alt][alt][danger]│  header  auto
│ ───────────── │ ───────────────────────────────────── │
│ Operations    │ KPI ×4 (1px grid dividers)            │  auto
│ Registry      │ ───────────────────────────────────── │
│ Configuration │ tree (1fr)      │ config form (460px) │  1fr
│               │ ───────────────────────────────────── │
│ v0.9.2   ☾    │ recent traces, 3–4 rows               │  auto
└───────────────┴───────────────────────────────────────┘
```

```css
body { display:grid; grid-template-columns: var(--sidebar) 1fr;
       height:100vh; overflow:hidden; }
main { display:grid; grid-template-rows:auto auto 1fr auto; min-width:0; }
```

- Sidebar groups: Operations / Registry / Configuration. Group headings use section-eyebrow; counts and warning badges sit on the right of each item.
- Server identity plus status badge go at the top of the sidebar (the top-left of P3).
- Header action order: non-destructive, non-destructive, destructive (outline only, `--c-err`).
- Separate panels with a `gap:1px; background:var(--c-line)` grid rather than borders per panel.
- Internal scrolling is allowed only inside the tree and the config form body. The page itself never scrolls.

---

## 6. Configuration editing (chosen approach: grouped form)

Grouped form with group-level save. Safer when values validate against each other.

- Group headings use section-eyebrow (General / Limits / Security).
- When a value differs from its default, promote the label to `--c-fg` at weight 500, show the original value next to it in struck-through mono, and attach an `overridden` tag.
- While unsaved changes exist, show a warning tag reading the change count next to the panel heading.
- Pinned bottom action bar: supporting note on the left, then `revert` and `save all`.

```html
<form hx-put="/config/runtime" hx-target="#config-panel" hx-swap="outerHTML">
  <fieldset>
    <legend>한도 Limits</legend>
    <label for="rps">rate_limit_rps</label>
    <input id="rps" name="rate_limit_rps" class="num" value="120"
           aria-describedby="rps-src">
    <span id="rps-src"><s>60</s> <span class="tag overridden">overridden</span></span>
  </fieldset>
</form>
```

Optional secondary modes: a **JSON editor behind an "advanced" tab** for bulk edits and diff review — using it as the primary surface breaks P1. On screens where single-value edits dominate, inline editing is allowed (`hx-get` swaps the cell for an input, `hx-patch` writes it back) provided width and alignment are identical before and after the swap.

---

## 7. Graph presentation (chosen approach: indented tree)

- **Primary**: the indented tree from §4.4 — exact for ownership relations (server → tools → tool), fits depth 4 without scrolling.
- **Secondary A — relation table**: two columns, `from → to`. For many-to-many and reverse lookups ("which tools use this resource?"). Sortable and filterable.
- **Secondary B — column browser**: three-column left-to-right drilldown. Fastest when depth is uniform.

Node-edge diagrams are not adopted: non-deterministic layout and poor label legibility.

---

## 8. htmx conventions

| Situation | Pattern |
|---|---|
| Load tree children | `hx-get` + `hx-target="closest li"` + `hx-swap="beforeend"`; response is a `<li data-depth="n">` fragment |
| Term help | `hx-get="/help/{term}"` + `hx-target="next .popover"` |
| Save form | `hx-put` targeting the panel with `outerHTML` — the server re-renders the whole panel including states and tags |
| Inline edit | read cell `hx-get .../edit` → input cell `hx-patch`, `hx-trigger="keyup[key=='Enter'], blur"` |
| Dangerous action | `hx-confirm` with a Korean confirmation string |
| Loading | `hx-indicator` — spinner only at the trigger site (caret, button); never a global overlay |
| Errors | On `htmx:responseError`, insert a `role="alert"` banner at the top of the panel and keep the submitted form values |

The server always returns **HTML fragments**. Do not mix in JSON plus client-side rendering.

---

## 9. Accessibility

- State is double-encoded as color plus text. Never replace badge text with an icon alone.
- Tree toggles carry `aria-expanded`; the container uses `role="tree"`, rows use `role="treeitem"` with `aria-level`.
- Help buttons carry an `aria-label` naming the term; popovers use `role="tooltip"`.
- Form errors set `aria-invalid="true"` and link the message via `aria-describedby`.
- Focus ring is a 3px `--c-accent` glow. Never `outline:none` on its own.
- Contrast: `--c-fg-dim` is for secondary info at 12px and above only. Body text uses `--c-fg-muted` or stronger.

---

## 10. Reference files

- `MCP Admin Design System.dc.html` — live token and component catalog
- `MCP Admin Dashboard.dc.html` — 1440×900 dashboard mockup (sidebar layout, grouped-form editing, indented tree)

Neither file is checked into this repository. If they are brought in, note that
`embed.go` embeds only `templates` and `static`, so a file placed elsewhere under
`internal/admin/` is not compiled into the binary.
