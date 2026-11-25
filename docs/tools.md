# Tool reference

<!-- Generated from the server's tool definitions — do not edit by hand.
     Regenerate: go test ./internal/mcp/tools -run TestToolReferenceUpToDate -update -->

Every MCP tool the graphin server exposes, with its full input schema. The descriptions are exactly what an agent sees. Tools marked *on demand* are kept behind the client's tool search to save prompt tokens; the rest are loaded up front.

Errors come back as `<error code="…">` in the result: `NODE_NOT_FOUND` (with suggestions), `AMBIGUOUS_REF` (with candidates), `NODE_GONE`, `NOT_BOOTSTRAPPED`, `LOCK_HELD`, `LEADER_CHANGED`, `MODEL_UNAVAILABLE`, `INTERNAL`.

| Group | Tools |
|---|---|
| Navigation | [`search_hybrid`](#search_hybrid) · [`search_keyword`](#search_keyword) · [`explore_graph`](#explore_graph) · [`read_code`](#read_code) |
| Index | [`bootstrap_workspace`](#bootstrap_workspace) · [`diagnose_index`](#diagnose_index) · [`index_scope`](#index_scope) · [`run_local_benchmark`](#run_local_benchmark) |
| Knowledge layer | [`wiki_preflight`](#wiki_preflight) · [`wiki_resolve`](#wiki_resolve) · [`wiki_propose`](#wiki_propose) · [`wiki_write_set`](#wiki_write_set) · [`wiki_edit_set`](#wiki_edit_set) |

## Navigation

What an agent uses to find, trace and read code.

### search_hybrid

Find code by what it does or by a name you half-remember — the search grep cannot do.

Query: ONE identifier (parseConfig, OrderService.cancel) or ONE sentence in the words code would use ("how the lock is released after a failed write"). Not a keyword list ("lock release fail") and not quoted text.

For exact text you can quote (an error message, a config key), use an exact-text search instead.

Returns ranked units with file:line and no code; detail="full" adds signatures, the lines around each match, and the top hit's body and callers. Sentence queries also match docs: target="code" keeps implementation only.

| Parameter | Type | Required | Default | Description |
|---|---|---|---|---|
| `query` | string | yes |  | One identifier, or one sentence about behavior. |
| `detail` | string |  | `ids` | ids (default) \| full. One of: `ids`, `full`. |
| `target` | string |  |  | code \| docs \| db. Omit to search everything. One of: `code`, `docs`, `db`. |
| `top_k` | integer |  | `5` | Max results (max 20). (maximum 20) |

### search_keyword

Find exact text from the code — an error message, log line, config key, constant, route — and the function or class each match is in.

Pattern: 1-5 words copied from code ("failed to acquire lock"). Not a question ("how are retries configured"): a sentence never appears verbatim. Literal and case-insensitive; regex=true for RE2.

Use grep instead for every occurrence (this returns at most 3 lines per file, 20 files), a case-sensitive match, or files outside the project.

Returns files ranked by match count, each matched line with its line number and enclosing unit; context=N adds the lines around it, detail="full" the unit's name and signature.

| Parameter | Type | Required | Default | Description |
|---|---|---|---|---|
| `pattern` | string | yes |  | Exact text from the code, 1-5 words. |
| `context` | integer |  | `0` | Lines around each match, 0-5 (default 0). (minimum 0, maximum 5) |
| `detail` | string |  | `ids` | ids (default) \| full. One of: `ids`, `full`. |
| `path` | string |  |  | Only paths containing this. |
| `regex` | boolean |  | `false` | Treat pattern as RE2. |
| `top_k` | integer |  | `5` | Max files (max 20). (maximum 20) |

### explore_graph

Show who calls or references a unit, and what it calls or depends on. References are resolved, so unrelated same-named symbols stay out and aliased callers come in — which grepping a name cannot do.

Use direction="used_by" for "what calls X" / "what breaks if X changes", "uses" for dependencies. The default "both" lists uses first, so a busy unit's callers can fall onto the next page: ask for used_by when callers matter.

Input: symbol = a name, Type.method, file path, or path:line. Not for finding where something is — search first.

Returns up to 20 neighbours per page with file:line and confidence (1.00 certain … 0.80 same-name guess, hidden below the 0.85 default); has_more means there are more. detail="full" adds the line where each reference is written. No edge is not proof of no relation: dynamic calls and reflection are invisible.

| Parameter | Type | Required | Default | Description |
|---|---|---|---|---|
| `cursor` | string |  |  | next_cursor of the last page. |
| `detail` | string |  | `ids` | ids (default) \| full. One of: `ids`, `full`. |
| `direction` | string |  | `both` | used_by \| uses \| both (default). One of: `uses`, `used_by`, `both`. |
| `min_confidence` | number |  | `0.85` | Default 0.85; 0.75 includes same-name guesses. |
| `node_id` | string |  |  | Alias of symbol. |
| `symbol` | string |  |  | Name, Type.method, path, or path:line. |

### read_code

Read one function, class, method, doc section or table — just that unit, not the whole file.

Use it when you know which unit you need and want its body. To find where something is, search first; to read a whole file, use a file read instead.

Input: symbol = a name, Type.method, file path, path:line (the unit enclosing it) or path:start-end (those lines); symbols = up to 20, read in order. lines="a-b" reads part of a long unit (at most 200).

Returns the source with its file and line range; units that did not fit are listed to request again. detail="full" adds callers and callees.

| Parameter | Type | Required | Default | Description |
|---|---|---|---|---|
| `detail` | string |  | `code` | code (default) \| full. One of: `code`, `full`. |
| `lines` | string |  |  | "a-b" file lines of one unit, at most 200. |
| `node_id` | string |  |  | Alias of symbol. |
| `node_ids` | string[] |  |  | Alias of symbols. (maxItems 20) |
| `symbol` | string |  |  | Name, Type.method, path, or path:line. |
| `symbols` | string[] |  |  | Up to 20 units, in order. (maxItems 20) |

## Index

Starting, inspecting and scoping the index.

### bootstrap_workspace

Index the workspace and start the file watcher. Returns immediately; progress is embedded in every subsequent tool response. Also detects RDB schema snapshots (*.graphindb.json, see schema/graphindb.md): tables/views/functions and their foreign keys index as graph nodes. When database traces exist without a snapshot, the response carries a <hint> explaining how to generate one.

| Parameter | Type | Required | Default | Description |
|---|---|---|---|---|
| `model_type` | string |  |  | Embedding model family. One of: `english_optimal`, `multilingual_cjk`. |
| `offline` | boolean |  |  | Never download; use local runtime/ artifacts only. |

### diagnose_index

*Loaded on demand.*

Report the index's own health: node/edge/shard counts, edges whose target is missing, files parsed with syntax errors, reverse-index stats, semantic search state (including a vectors.bin written by a different embedding model than the one configured), the effective startup flags, and .graphin disk usage. Any problem worth acting on comes back as a <hint>. Use it when search returns nothing for a symbol you expect to exist, when explore_graph is missing an edge you know is there, or before concluding the code is at fault — it separates "the index is wrong" from "the code is not what you thought". Counting partial nodes scans every node, so this is heavier than the other tools; it is a diagnostic, not a per-question call.

No parameters.

### index_scope

*Loaded on demand.*

Show what the index is made of — files and nodes per top-level directory and per extension, plus the exclusion patterns in effect — and edit that scope. Call with no arguments to see the composition; markdown is broken out because a .md file becomes one node per heading, which is usually what makes a node count jump. Pass `add` (gitignore syntax: "build/", "*.txt") to see what those patterns would take out: affected files, nodes, and warnings when they would cut wiki-pinned sections or DB schema snapshots. Nothing is written until you repeat the call with confirm=true, and even then the current index is untouched — search keeps answering from it until the server next starts up, which is when the nodes actually go (calling bootstrap_workspace again on the same server does not rescan). `remove` takes patterns back out so the next start indexes them again. Use it when the node count looks wrong for the project, or when scratch files, vendored copies, build output or a git worktree inside the project are being indexed.

| Parameter | Type | Required | Default | Description |
|---|---|---|---|---|
| `add` | string[] |  |  | Patterns to exclude (gitignore syntax). Previews unless confirm is true. |
| `confirm` | boolean |  |  | Write the change. Without it the call only reports what would happen. |
| `reason` | string |  |  | Why these are excluded. Recorded above the patterns so the file explains itself. |
| `remove` | string[] |  |  | Patterns to stop excluding, matched literally against the file's lines. |

### run_local_benchmark

*Loaded on demand.*

Compare grep-style context bytes vs graphin navigation bytes for a query, as a markdown report.

| Parameter | Type | Required | Default | Description |
|---|---|---|---|---|
| `expected_node` | string | yes |  |  |
| `target_query` | string | yes |  |  |

## Knowledge layer

Knowledge sets over `docs/wiki` (see the graphin-guide `knowledge` skill).

### wiki_preflight

*Loaded on demand.*

Before delegating work or starting a change, get the knowledge catalogue for it: which curated sets of documentation sections apply, with one line each and no bodies. Returns a token that must be included in the delegation prompt. An empty catalogue is a normal answer meaning no recorded knowledge applies — it still returns a token, so proceed. Use wiki_resolve to load what the catalogue names.

| Parameter | Type | Required | Default | Description |
|---|---|---|---|---|
| `role` | string |  |  | The role the work belongs to (e.g. backend). Sets tagged for this role are always included. |
| `task` | string |  |  | What the work is, in a sentence. Matched against set names, summaries and entries. |

### wiki_resolve

*Loaded on demand.*

Load the sections a knowledge set names, by set name or by node id. Each section carries a drift verdict: absent means it is byte-for-byte what it was when the set was written, "changed-since-registration" means the section was rewritten and the set's one-line summary may no longer hold, "unpinned" means nothing was recorded to compare against. reviewed="false" means an agent changed the set that serves it and no person has checked. Running this is also what clears the knowledge gate for the caller.

| Parameter | Type | Required | Default | Description |
|---|---|---|---|---|
| `node_ids` | string[] |  |  | Specific sections, when the catalogue has already been read and only some entries are wanted. |
| `sets` | string[] |  |  | Set names from a manifest. Prerequisites are pulled in automatically. |

### wiki_propose

*Loaded on demand.*

Submit a glossary candidate for review. Never writes to the glossary itself — it files a proposal and returns the mechanical verdict. A word the code index already resolves is rejected outright: that is structure, and search answers it. Cite evidence as node ids from at least two different files, or the candidate is one author's coinage.

| Parameter | Type | Required | Default | Description |
|---|---|---|---|---|
| `canonical` | string | yes |  | The one word the project should use. |
| `definition` | string | yes |  | One paragraph. What it means and why the project needs the word. |
| `evidence` | string[] | yes |  | Node ids where the term is actually used, from at least two different files. |
| `aliases` | string[] |  |  | Variants interchangeable with it in EVERY context here. Partial overlap is a separate term, not an alias. |
| `not_to_be_confused_with` | string[] |  |  | Near-misses, written as "other term — why they differ". |
| `scope` | string[] |  |  | Roles this applies to, or "all". |

### wiki_write_set

*Loaded on demand.*

Write a new knowledge set at the end of a task whose wiki_preflight returned an empty catalogue — the sections you had to find and read to do the work, with one line each saying what they claim. Judged before it lands: the task must have missed in this workspace (pass the preflight sentence verbatim), a preflight for that task must select the new set (name or alias it the way the task names the subject), every section must be a documentation heading that exists, and the wiki must be under its cap. Written at once with origin: agent and reviewed: false for a person to check; nothing is committed. Never for knowledge a task did not ask for.

| Parameter | Type | Required | Default | Description |
|---|---|---|---|---|
| `description` | string | yes |  | One sentence: what work this is for and what a reader will know after. |
| `groups` | object[] | yes |  |  |
| `name` | string | yes |  | Filename stem: letters, digits, - and _. |
| `task` | string | yes |  | The sentence given to wiki_preflight, verbatim. |
| `title` | string | yes |  | The set's heading, in the project's language. |
| `aliases` | string[] |  |  | The subject's names in other vocabularies or languages; a multi-word alias is a phrase. |
| `intro` | string |  |  | Optional paragraph under the title: when to read this set. |
| `tags` | string[] |  |  |  |

### wiki_edit_set

*Loaded on demand.*

Maintain one knowledge set without editing markdown by hand: repoint an entry whose section moved (op=repoint), rewrite an entry's one-line summary after re-reading the section (op=summarize), record that a drifted entry was re-read and its summary still holds (op=confirm), or rewrite the set's catalogue line (op=describe). Every write is judged first — the target must be a documentation section that exists and is not already listed — and is applied to the working tree at once, marking the set reviewed: false for a person to check afterwards. It never creates a set and never writes a document.

| Parameter | Type | Required | Default | Description |
|---|---|---|---|---|
| `op` | string | yes |  | What to do. One of: `repoint`, `summarize`, `confirm`, `describe`. |
| `set` | string | yes |  | Set name (the filename stem). |
| `description` | string |  |  | The set's new catalogue line (describe). One sentence. |
| `new_node_id` | string |  |  | Where the entry should point instead (repoint). A markdown section id: path.md#anchor. |
| `node_id` | string |  |  | The entry's current node id, as the set lists it (repoint, summarize, confirm). |
| `summary` | string |  |  | The one line the catalogue offers the section by: what it claims, not what it is about (summarize; optional on repoint). |
| `title` | string |  |  | New link text, if the heading's wording changed (repoint, summarize). Optional. |
