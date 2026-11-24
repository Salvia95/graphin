package tools

// Tool descriptions for the four navigation tools.
//
// These are read by an agent that often has nothing else: graphin is shipped
// as a plugin and injected into subagents that see neither the skill nor any
// server instructions. So each one says, in this order, what the tool is for,
// when to use it and when not (against grep and plain file reads, which every
// agent has), the exact shape of its input with good and bad examples, what
// comes back, and at most two facts needed to read the result. Implementation
// terms stay out, and no description names another graphin tool — any of them
// may be the only one the agent was given (docs/tool-contract-spec.md §1-2,
// TestDescriptionsStandAlone).

const descSearchHybrid = `Find code by what it does or by a name you remember, when you do not know the exact text in the file.

Use it for: "where is order cancellation handled", "what implements retry backoff", "how the watcher decides a file changed", or a symbol name such as applyFileResult. This is the search grep cannot do: it matches meaning and names, not only literal text.
Do not use it for: an exact string you could quote from the code (an error message, a config key, a literal) — an exact-text search answers that precisely; or for listing every occurrence of something.

Query: ONE identifier (parseConfig, OrderService.cancel) or ONE sentence describing behavior in the words code would use.
  good: "how does the lock get released after a failed write"
  good: "applyFileResult"
  bad:  "lock release write fail"   (a keyword list — write it as a sentence)
  bad:  "\"lock released\""          (quoted text — that is an exact-text search)

Returns: up to top_k ranked units (function, class, method, doc section, DB table), each with its id, name, match type and the file:line where it starts — enough to answer "where is X". No code: read the unit to see its body. For a long doc section, hit_line is the line the match is on.

Good to know: a sentence-shaped question also matches documentation; set target="code" for implementation only. Ranks are best guesses — a match_type of exact or both is stronger evidence than a single engine.`

const descSearchHybridQuery = `ONE identifier, or ONE sentence about behavior in the words code would use. Not a keyword list, quoted text or a regex.`

const descSearchHybridTarget = `code: implementation only — use it for a sentence-shaped question about how code works, which otherwise ranks documentation first. docs: markdown files and sections. db: tables, views and other schema objects. Omit to search everything, including config files. A name-shaped query rarely needs it.`

const descSearchKeyword = `Find where an exact piece of text appears in the project, and which function or class it is in.

Use it for: text you can quote from the code — an error message, a log line, a config key, an environment variable, a constant, a route path, a string built at runtime.
Prefer it over grep when you want the enclosing function or class with each match in one short, ranked answer.
Use grep instead when you need EVERY occurrence (renaming, counting, bulk edits — this returns at most 3 lines per file and 20 files), a case-sensitive match, or files outside the project.
Do not use it for questions or descriptions: it matches text, not meaning, and a sentence will almost never appear verbatim.

Pattern: a short fragment copied from code, 1–5 words, the part least likely to vary.
  good: "failed to acquire lock"   good: "MAX_RETRIES"   good: "/api/v1/orders"
  bad:  "how are retries configured"   (a question — describe it with a meaning search instead)
  bad:  "retry config backoff"         (a keyword list — pick the one exact string)
Matching is literal and case-insensitive. regex=true takes RE2 for a part that varies: "timeout after \d+s".

Returns: up to top_k files ranked by match count; per file up to 3 matching lines, each with its line number and the id of the unit it is in (a line outside every function resolves to the file, whose id is its path). context=N adds the N lines around each match. detail="full" also names the function or class each match is in, with its signature and line range, and shows 2 lines of context unless you set context. Data files (json, lock, csv) are listed by path and count only; pass path= to see their lines. Works while the project is still being indexed.`

const descSearchKeywordPattern = `Exact text from the code, 1–5 words, case-insensitive. Not a question or a keyword list.`

const descSearchKeywordContext = `Lines around each match, 0–5 (default 0). Use 2–3 to see a match in place without reading the file. A file that would overflow the response is listed as omitted; narrow with path to see it.`

const descExploreGraph = `Show how a unit of code is connected: who calls or references it, and what it calls or depends on.

Use it for: "what calls X", "what breaks if I change X" (direction="used_by"), "what does X depend on" (direction="uses"), "which code reads or writes the orders table".
Prefer it over grepping a name: it follows resolved references (imports, calls, inheritance, ORM table access, doc links), so same-named but unrelated symbols do not show up, and callers that reach it through an alias do.
Do not use it to find where something is — give it a name you already have; an unknown name comes back with suggestions.

Input: symbol = a name (cancelOrder), a qualified name (OrderService.cancel), a file path, or path:line for the unit enclosing that line. A name shared by several units returns them as candidates instead of guessing.
direction: used_by = callers and references (impact), uses = dependencies, both (default) = uses first, then used_by. When you need callers, ask for used_by: under both, a busy function's callers can fall onto the next page.

Returns: up to 20 neighbours per page, each with its id, the kind of reference (call, import, extends, implements, reference, foreign_key, contains), a confidence and the file:line where the neighbour starts. Confidence: 1.00 certain from syntax; 0.95 same package; 0.90 through an import; 0.80 a same-name guess, hidden unless min_confidence is lowered. has_more with next_cursor means more exist — a short list is not proof of few callers. detail="full" adds each neighbour's name, signature and the line where the reference is written (ref_line, ref_text), so callers can be judged without reading them.

Limits: references are known for Java, Kotlin, Python, JavaScript/TypeScript, Go, Markdown links and committed DB schemas. Dynamic dispatch, reflection and string-built calls are invisible: no edge is not proof of no relationship.`

const descExploreDirection = `used_by: callers and references (impact analysis). uses: what it depends on. both (default): uses first, then used_by — ask for used_by when callers are what you need.`

const descExploreMinConfidence = `Drop references below this confidence (default 0.85). Lower to 0.75 to include same-name guesses (0.80) — for impact analysis, where missing a caller costs more than a false one.`

const descReadCode = `Read the source of one function, class, method, doc section or DB table — exactly that unit, not the whole file.

Use it when you know which unit you need and want its body: to confirm behavior, quote it, or check a caller.
Prefer it over reading the file: it returns just that unit with its file and line range, and re-reads the file if it changed since indexing.
Do not use it to find where something is (search first), or to read a whole code file top to bottom (use a file read).

Input: symbol = a name (cancelOrder), a qualified name (OrderService.cancel), a file path, or path:line for the unit enclosing that line; symbols = up to 20 of those, read in order. lines = "120-180" reads only that stretch of a long unit (at most 200 lines; the file's own line numbers). A name shared by several units returns them as candidates instead of guessing.

Returns: the source with its file and line range. Several units come back whole, in the order asked; any that did not fit are listed with the reason, to request again. detail="full" on a single read adds up to 5 callers and 5 dependencies, each with its location and the line where the reference is written, so you can continue from the unit.`

const descExploreDetail = `ids (default): id, kind of reference, confidence and location per neighbour. full: also each neighbour's name and signature, and the line where the reference is written — use it to judge callers without reading each one.`

const descReadDetail = `code (default): the source only. full: for a single unit, also up to 5 callers and 5 dependencies with their locations — a place to continue from.`

const descSearchKeywordDetail = `ids (default): each match with its line and the id of the unit it is in. full: also that unit's name, kind, signature and line range, and 2 lines of context unless context is set.`
