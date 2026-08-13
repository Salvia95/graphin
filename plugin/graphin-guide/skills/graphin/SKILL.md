---
name: graphin
description: >-
  Navigate and understand a codebase through the graphin MCP server instead of
  grep + blind file reads. Use when you need to locate where a feature/symbol
  lives, trace callers and callees, follow a dependency or call chain, do impact
  analysis before a change, or see how code connects to database tables — while
  keeping token usage low. Triggers: "where is X implemented", "what calls this",
  "what would this change break", exploring an unfamiliar or large repo, finding
  an entry point, tracing data flow, or mapping code ↔ DB relationships.
---

# Graphin — codebase navigation for agents

Graphin is a local MCP server that turns a repository into a searchable graph of
code symbols (and, optionally, database schema). It exists to answer "where /
how / what-connects-to-what" questions **without** reading whole files or
grepping line by line.

The whole design is **Progressive Disclosure**: move from cheap, low-detail
answers to expensive, high-detail ones only as far as the question requires.
You almost never need to read a file top to bottom.

## Mental model: three steps, cheap → expensive

```
1. search_hybrid("cancel a paid order")   → candidate node IDs   (names only, no code)
2. explore_graph(node_id)                 → what it uses / what uses it (+ confidence)
3. read_code(node_id)                     → the exact source of ONE node
```

Each step hands you **node IDs**. A node ID is the currency of graphin — it flows
from search into explore into read. You never invent an ID; you only ever pass
back an ID that a previous call returned.

Stop as early as you can. If `search_hybrid` names the right symbol and you only
needed to know *where* it is, you're done — don't read the code just because you
can.

## Before anything else: bootstrap

Call `bootstrap_workspace` once at the start of a session. Indexing runs in the
background:

- **Lexical/keyword search is usable almost immediately.** Semantic (meaning-based)
  matching sharpens shortly after, once the model warms up.
- If a response is tagged as still indexing (a `state="indexing"` / `..._ready`
  status), that's normal — keep going with keyword-style queries and let semantic
  catch up.
- Re-calling `bootstrap_workspace` during warmup just reports remaining progress;
  it does not re-index. Don't spam it.

## The tools

| Tool | What it gives you | Params that matter |
|---|---|---|
| `bootstrap_workspace` | Starts indexing + live file watching. | `model_type`: `english_optimal` \| `multilingual_cjk` (pick by the language of code/comments). `offline`: for air-gapped setups. |
| `search_hybrid` | Entry-point **node IDs** for a query, each with the `file` and `line` it starts at. Exact matches, keyword (BM25), and semantic results are blended and ranked. No code bodies. | `query` (required, natural language or a symbol name). `top_k` (default 5, max 20). `target`: `code` \| `docs` \| `db` — see below. |
| `explore_graph` | The graph neighborhood of a node: what it **uses** and what **uses it**, each with a `confidence`. Paginated. | `node_id` (required). `direction`: `uses` \| `used_by` \| `both` (default `both`). `min_confidence` (default `0.85`). `cursor` for the next page. |
| `read_code` | The exact source slice for one node, or for several at once. | `node_id`, **or** `node_ids` (up to 20, read in the order given). Not both. |

`diagnose_index` reports the index's own health — counts, edges whose target is
missing, files that failed to parse, semantic search state, effective settings,
disk usage — and names anything worth acting on in a `<hint>`. Reach for it when
a symbol you are sure exists doesn't come back from `search_hybrid`, or when
`explore_graph` is missing an edge you know is there: it separates "the index is
wrong" from "the code is not what you thought". It scans every node, so it is a
diagnostic, not something to call per question.

`run_local_benchmark` also exists — it measures how many bytes graphin navigation
saves versus grep for a given query. It's a demonstration/QA tool, not part of
normal exploration.

### Reading the results

- **`search_hybrid`** returns ranked nodes with an ID, a display name, a match
  type, and the `file`/`line` where the node starts. Take the top one or two that
  fit; if none look right, rephrase the query (try the concept, then a likely
  symbol name) before falling back to other tools.
- **"Where is X?" is already answered by the search result** — the location is
  right there. Only call `read_code` when you need the body, not to find out
  where something lives.
- **`explore_graph`** groups edges into `uses` and `used_by`, each edge carrying a
  `confidence` (roughly: how sure graphin is the relationship is real). Higher =
  safer. If the result says there's more and gives you a cursor, page with it only
  when you actually need more than the first page.
- **A page of `both` fills with `uses` first.** The page holds 20 edges and the
  `uses` section is laid out before `used_by`, so a node that calls a lot of
  things can push its callers onto page two — and callers are the scarce, valuable
  half when you're asking what breaks. **If you want callers, ask for them:**
  `direction="used_by"`. Don't read a short `used_by` list under `both` as "few
  callers" when `has_more` is set.
- **`read_code`** returns the source for that node with its line range. Occasionally
  it flags that a node was re-parsed on the fly (the file changed since indexing) or
  that the slice is partial — treat those as hints, not errors.
- **`read_code(node_ids=[…])`** returns whole nodes in the order you asked for and
  **never cuts inside one**. If they do not all fit the response budget it returns
  the ones that do and lists the rest as `<omitted reason="budget">` — re-request
  those in a second call. Every id you pass comes back either as a block or as an
  omission, so nothing disappears silently.

## Asking a question in a sentence: use `target="code"`

A whole markdown file is **one node**, and a document about a function matches a
prose question better than the function itself ever can — the function only has
its name and signature to match with, while the document is that behavior
written out in sentences. So a sentence-shaped question about code comes back
full of documentation.

Measured on graphin's own repository (8 prose questions, top 5 each, 40 slots):

| | implementation | tests | markdown | config/data |
|---|---:|---:|---:|---:|
| no `target` | 7 | 5 | **21** | 7 |
| `target="code"` | **22** | 18 | – | – |

**70% of the slots were not code.** So:

- **Sentence-shaped question about implementation → pass `target="code"`.**
  "how does the watcher decide a file changed", "where is the lock released".
- **Symbol-shaped query → don't bother.** `applyFileResult` already lands on an
  exact match at rank 1; the filter has nothing to fix.
- **Looking for prose or a decision record → `target="docs"`.** Returns markdown
  files and sections only.
- **Looking for a table or view → `target="db"`.**
- **Omit it to search everything** — that's the default, and it's also how you
  reach config files (`.json`, `.yml`, `.toml`), which count as neither code nor
  docs.

Two things to know:

- **A filtered response echoes the filter** (`<results … target="code">`). If you
  passed `target` and the echo is missing, you are talking to an older graphin
  server that ignored it — the results are unfiltered, so read them that way.
- **Filtering does not fix everything.** With docs out of the way, test functions
  take a large share of what's left: a test name like
  `TestContainsRefreshesWhenChildAdded` is practically the question written as an
  identifier, while the implementation's name is not. If the top code hits are
  all tests, the implementation is usually one hop away — `explore_graph` the
  test and read its `uses`.

## Tuning recall vs. precision

`explore_graph`'s `min_confidence` is your main knob:

- **Default `0.85`** favors precision — fewer, higher-quality edges. Good default.
- **Lower to ~`0.75`** when you're doing impact analysis or hunting an elusive
  connection and want maximum recall (accept some noise).
- **Raise toward `0.95`** when a node is very central and you're drowning in edges.

## When to use graphin

- You don't yet know **where** the relevant code is.
- You need to **trace** a call chain, dependency, or "who calls this" relationship.
- You're doing **impact analysis** — "what could this change break?"
- You want to see **how code maps to database tables** (see below).
- The repo is **large or unfamiliar** and grepping would flood context.

## When NOT to use it (anti-patterns)

- **Don't invent node IDs.** Only pass IDs that search/explore returned. A guessed
  ID will miss.
- **Don't skip search and guess.** If you don't have an ID, start at `search_hybrid`.
- **Don't use `read_code` as a file reader.** It returns one node's slice, not a
  whole file. (Config/plain-text files are indexed as single file-level nodes, so
  reading those *does* return the file — but code files give you the symbol.)
- **Don't crank `min_confidence` to the floor by default** — you'll get plausible-
  but-wrong edges. Lower it deliberately, for recall, not as a habit.
- **Don't re-bootstrap in a loop** to "make it faster." Indexing finishes on its own.
- **Don't expect runtime facts.** Graphin knows structure (who references whom),
  not values, execution traces, or live state.

## Databases (optional)

If the project commits a schema snapshot or a schema manifest, graphin also indexes
**tables, views, functions, and procedures as nodes**, with foreign keys as edges —
and it links code to the tables it touches (via ORM annotations, ORM client access,
and recognizable SQL). Practical payoff:

- `search_hybrid("orders table")` → the table node.
- `explore_graph(table_node, direction="used_by")` → **every piece of code that
  touches that table**, in one hop.

Graphin never connects to a live database; it reads committed schema files only. So
it reflects the schema as checked in, not the current production state.

## Scope & limits (know these before you trust an answer)

- **Graph edges exist for:** Java, Kotlin, Python, JavaScript, TypeScript (incl.
  JSX/TSX), **Go**, and **Markdown**. Other file types are still **searchable and
  readable** as text nodes, but they won't have `uses` / `used_by` edges — so
  `explore_graph` is thin for them.
- **Markdown is a graph too.** Every heading becomes a section node whose id is
  `path/to/doc.md#github-style-slug`, linked to its parent by a `contains` edge.
  So a search over docs returns the *section* that answers the question, not the
  whole file, and `read_code` on it gives you that section alone. The file node
  still exists — search it by name when you want the document itself.
- **A missing edge is not proof of no relationship.** Dynamic dispatch, reflection,
  string-built calls, and unresolved import aliases can hide links. Low/absent edges
  = "look closer," not "definitely unrelated."
- **DB view is snapshot-based** (no live connection, no migration-history replay).

## Quick recipes

- **"Where is feature X?"** → `search_hybrid("X in plain words", target="code")` —
  the answer is the `file`/`line` on the top result. `read_code` only if you need
  the body.
- **"How does this repo do Y?"** (a sentence, not a symbol) →
  `search_hybrid("...", target="code")`. Without the filter most of the list is
  documentation about Y rather than the code doing Y.
- **"What calls this function?"** → get its ID via search →
  `explore_graph(id, direction="used_by")`.
- **"What does this depend on?"** → `explore_graph(id, direction="uses")`.
- **"What breaks if I change this?"** → `explore_graph(id, direction="used_by",
  min_confidence=0.75)`, then `read_code` the callers that matter.
- **"What code touches the `orders` table?"** → search the table →
  `explore_graph(table_id, direction="used_by")`.
