# graphin

**Let your coding agent find, trace and read code by symbol — instead of grepping file after file.**

graphin is a local MCP server and Claude Code plugin. It indexes a repository
into a graph of functions, classes, doc sections and database tables, so an
agent can ask *where is this*, *what calls it* and *show me just that function*
and get an exact answer in one call.

- **Local.** Indexing, search and the optional embedding model run on your machine. No API key, no account, no telemetry.
- **Symbol-sized reads.** `read_code` returns one function, method, section or table — or `path:120-180` — not the whole file.
- **Follows references, not strings.** Callers and dependencies are resolved through imports and scopes, so same-named strangers stay out.
- **Code ↔ database.** Committed schema snapshots become graph nodes, linked to the code that reads and writes each table.

[English](README.md) · [한국어](README.ko.md)

## Quick start

In Claude Code (2.1.83 or later):

```
/plugin marketplace add Salvia95/graphin
/plugin install graphin@graphin
```

Restart the session, open a project and ask something you would normally
grep for:

> Where is order cancellation handled, and what calls it?

On a project graphin has never seen, the agent first calls
`bootstrap_workspace` (the other tools tell it to); indexing runs in the
background, lexical search is ready within seconds and the semantic model
warms up after it. Later sessions reopen the index on their own. Run
`/graphin:doctor` to check the install, platform support and index state.

Add `.graphin/*` to your `.gitignore` — the index lives in `<project>/.graphin/`.

> Already registered graphin by hand (`claude mcp add graphin`)? Remove it
> (`claude mcp remove graphin -s local|user|project`). Two registrations fight
> over the same index; `/graphin:doctor` detects this.

## Why

Without an index, an agent explores by guessing: grep a word, open a file,
follow an import, back out, try again. Every file it opens is paid for in
context, and every guess that misses is paid for twice.

| | grep + read | graphin |
|---|---|---|
| "Where is X?" | grep, then open candidates to check | one search: ranked units with `file:line` |
| "What calls X?" | grep the name, filter out same-named strangers by reading | `explore_graph` → resolved callers with the line of each call |
| "Show me X" | read the file (or a guessed range) | `read_code` → exactly that unit |
| "Which code touches the `orders` table?" | grep table names and SQL strings | the table's `used_by` edges |

## What to ask

graphin does not change how you talk to the agent. These are the questions it
answers well, and the tool the agent reaches for:

| Situation | Ask | Tool |
|---|---|---|
| You know what it does, not what it is called | "How does the watcher decide a file changed?" | `search_hybrid` |
| You have an error message or config key | "Where does 'failed to acquire lock' come from?" | `search_keyword` |
| Before changing a function | "What calls `applyFileResult`, and what would break?" | `explore_graph` (used_by) |
| Reading one unit | "Show me `OrderService.cancel`." | `read_code` |
| Schema questions | "Which code writes to `job_posting`?" | `explore_graph` on the table |

## See it in action

Django, lexical index only (no embedding model). The agent asks:

> How are related objects collected for cascade deletion?

It calls `search_hybrid` with `target="code"` and `detail="full"`; the first
result already names the function, its location, its body and who calls it
(response trimmed):

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

One call, and the next step — "what would changing it break?" — is
`explore_graph` on `Collector.collect` with `direction="used_by"`.

## Tools

| Tool | What it is for |
|---|---|
| `search_hybrid` | Find code by meaning or a half-remembered name (exact match + BM25 + embeddings, fused). |
| `search_keyword` | Find exact text — messages, keys, constants — and the unit each match is in. |
| `explore_graph` | Who calls or references a unit, and what it depends on, with confidence scores. |
| `read_code` | Read one unit, several in one call, or a line range — never a whole file by accident. |
| `bootstrap_workspace` · `diagnose_index` · `index_scope` | Start the index, check its health, see and change what it covers. |
| `wiki_*` | The optional knowledge layer over `docs/wiki` (see [graphin-guide](#optional-graphin-guide)). |

Navigation tools accept plain names (`cancelOrder`, `OrderService.cancel`,
`src/order.py:120`) as well as ids, and take `detail="full"` to return
signatures, call sites and callers in the same response.
Every parameter is in the generated **[tool reference](docs/tools.md)**.

## How it works

```
source files ──tree-sitter──▶ symbols + references ──▶ ┌ BM25 index (symbols, bodies, doc chunks)
                                                       ├ vector index (optional, local ONNX model)
schema snapshots ─────────────▶ tables, views, FKs ──▶ └ graph shards (calls, imports, inheritance,
                                                          table access, doc sections) + reverse index
file watcher ──▶ merkle diff ──▶ re-parse only what changed
```

- Search fuses an exact-name tier, BM25 and vector results (RRF); responses
  carry locations, not raw scores.
- Edges carry a confidence: 1.00 when certain from syntax, 0.95 same package,
  0.90 through an import, 0.80 a same-name guess (hidden by default).
- Edits are picked up by a file watcher; a node whose file changed is re-parsed
  on read, so `read_code` never returns stale offsets.
- Several sessions on one project share one index: the first server holds it,
  the others forward their calls to it.

## Evidence

Measured on public benchmarks and this repository's own task sets. Methods and
raw results are in [`docs/eval/`](docs/eval/README.md).

| Measurement | Result | Scope |
|---|---|---|
| [SWE-Explore](https://github.com/Qiushao-E/SWE-Explore-Bench), 3 languages — 40 tasks from lucene (Java), element-web (TypeScript), django (Python) | recall@300 **0.118 vs 0.000** for grep ±20, with **2.1%** of the lines; graphin ahead on 33 tasks, behind on none | deterministic explorers, same derived queries, lexical index · [details](docs/eval/2026-11-25-swe-explore-3lang/findings.md) |
| SWE-Explore, 451 Python tasks (SWE-bench Verified) | recall@300 **0.109 vs 0.004**, with 2.6% of the lines | same · [details](docs/eval/2026-07-25-h1-reverify/findings.md) |
| Agent benchmark on this repository — 36 questions × 3 runs, graphin agent vs a grep-only agent | **same accuracy** (98/108 each) | [details](docs/eval/2026-09-08-36-grep/findings.md) |
| Same, sub-agent with no guidance beyond the tool descriptions | same accuracy and **same token cost** as the grep-only agent | [details](docs/eval/2026-11-24-rag-hard/findings.md) |

Per language on SWE-Explore, recall@300 was 0.129 (Java), 0.130 (Python) and
0.091 (TypeScript) — TypeScript is the weakest so far.

What this does and does not show: under a fixed reading budget, graphin puts
the right code first where grep cannot. An agent that greps patiently reaches
the same answers; on a repository the size of this one (a few hundred files),
it does so at about the same token cost. graphin is not a token-saver by
default — it is a way to land on the right code with less searching, and that
gap widens with repository size.

## Supported languages and content

| Content | Symbols | References (graph edges) |
|---|---|---|
| Java, Kotlin, Python | ✓ | calls, imports, inheritance |
| Go | ✓ | calls, imports, interface embedding |
| JavaScript, TypeScript (JSX/TSX) | ✓ | calls, imports (incl. `tsconfig` path aliases), JSX components |
| Markdown | one node per heading | section hierarchy |
| Database schema snapshots | tables, views, functions, procedures | foreign keys, code → table access (JPA, SQLAlchemy, Django, TypeORM, Prisma, SQL strings) |
| Other text (YAML, SQL, Dockerfile, …) | one node per file | — |

Database support reads committed snapshots only — graphin never connects to a
live database. Generate one from `tbls`/Atlas with `graphin dbimport`, or point a
manifest at an existing Prisma schema or SQL dump: [`schema/graphindb.md`](schema/graphindb.md).

## Optional: graphin-guide

A second plugin teaches the agent to use graphin well and adds a read-only
research subagent:

```
/plugin install graphin-guide@graphin
```

| Part | What it does |
|---|---|
| `graphin` skill | When to search, explore or read — and when to stop. |
| `graphin-rag` subagent | Takes a whole codebase question, works to a context budget, and returns the answer with citations and what it did not verify. |
| `knowledge` skill · `wiki-maintainer` | A knowledge layer over `docs/wiki`: curated sets of doc sections, a glossary, and a pre-flight gate before edits. |

It is a separate plugin on purpose: graphin records (locally) whether agents
adopt it on their own, and mixing in guidance would erase that baseline.
Details: [`plugin/graphin-guide/README.md`](plugin/graphin-guide/README.md).

## Configuration

All optional, under `/plugin` → Manage → graphin → Configure:

| Option | Purpose |
|---|---|
| `model_type` | Embedding model: `multilingual_cjk` (default) or `english_optimal` |
| `semantic_max_nodes` | Turn semantic search off above this many nodes (default 40,000; lexical stays on) |
| `workspace_subdir` | Index a subdirectory instead of the project root |
| `offline` · `model_dir` | Never download; use a local model directory (air-gapped setups) |
| `binary_path` | Use your own build instead of the released binary |

Exclude paths with `.graphin/ignore` (gitignore syntax) or ask the agent to
use `index_scope`. Plugin options are global (user settings), not per project.

**Platforms:** release binaries for linux/amd64 and linux/arm64. macOS
(Apple Silicon) builds from source automatically via `go install` (needs Go and
clang); Intel macOS builds the same way but without semantic search. Windows is
not supported.

## Privacy

Everything stays in `<project>/.graphin/` and `~/.cache/graphin/`. The only
network access is downloading the release binary and, on first bootstrap, the
embedding model and ONNX runtime (SHA256-verified; skip with `offline`).

The plugin keeps a local log of tool calls (`.graphin/usage/events.jsonl`) to
measure adoption — tool names, graphin queries, grep patterns and file paths;
never file contents, tool output or full shell commands. `/graphin:report`
summarizes it; nothing is sent anywhere. [What is recorded](plugin/graphin/README.md#계측-무엇을-기록하나).

## Limits

- **Small repositories:** in a repository of a few hundred files, a grep-based
  agent finds the same answers at about the same token cost. graphin's
  advantage grows with size and with questions that span files.
- **Static analysis only:** dynamic dispatch, reflection and string-built
  calls are invisible. A missing edge is not proof of no relationship.
- **Schema is as committed:** snapshots, not your live database.
- **Platforms:** see above; semantic search is unavailable on Intel macOS.

## Troubleshooting

| Symptom | Fix |
|---|---|
| Anything odd | `/graphin:doctor` checks binary, platform, index and duplicate registrations |
| First session times out starting the server | the 25 MB binary is downloading; reconnect in `/mcp` or raise `MCP_TIMEOUT` (e.g. `60000`) |
| A symbol you know exists is not found | ask the agent to run `diagnose_index`, or use exact-text search |
| Too many nodes / irrelevant files indexed | `index_scope` shows what is indexed and previews exclusions |

## Update and uninstall

Releases reach you only after refreshing the marketplace:

```sh
claude plugin marketplace update graphin
claude plugin update graphin@graphin
```

Then restart the session — that is what swaps the binary. To remove:
`/plugin uninstall graphin@graphin`, and delete `<project>/.graphin/`.

Version numbers: in 0.x, a **minor** bump (0.X.0) means you may have to change
something (a tool, option or flag was renamed); a **patch** needs nothing.
The plugin and binary versions are always the same number.

## Development

```sh
make build        # bin/graphin
make test         # go vet + go test ./...
./bin/graphin version --json
```

Design and internals: [internals](docs/internals.md) ·
[distribution](docs/plugin-distribution.md) · [evaluation](docs/eval/README.md) ·
[schema snapshots](schema/graphindb.md) · [tool reference](docs/tools.md).

## License

[Apache-2.0](LICENSE)
