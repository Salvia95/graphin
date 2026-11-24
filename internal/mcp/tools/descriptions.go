package tools

// Tool descriptions for the four navigation tools.
//
// These are read by an agent that often has nothing else: graphin is shipped
// as a plugin and injected into subagents that see neither the skill nor any
// server instructions. Each one says what the tool is for, when to use
// something else instead (grep or a file read, which every agent has), the
// input shape with one good and one bad example, and what comes back. No
// description names another graphin tool — any of them may be the only one
// the agent was given (TestDescriptionsStandAlone).
//
// They are also paid for on every turn of every session: the tool list is
// part of the prompt. The first long version added ~2.3K tokens per turn and
// cost +10-15% input tokens per run with no accuracy change
// (docs/eval/2026-11-24-tool-contract §토큰). Keep them short; put detail in
// the responses, which are paid for only when asked.

const descSearchHybrid = `Find code by what it does or by a name you half-remember — the search grep cannot do.
Query: ONE identifier (parseConfig, OrderService.cancel) or ONE sentence in the words code would use ("how the lock is released after a failed write"). Not a keyword list ("lock release fail") and not quoted text.
For exact text you can quote (an error message, a config key), use an exact-text search instead.
Returns ranked units with file:line and no code; detail="full" adds signatures, the lines around each match, and the top hit's body and callers. Sentence queries also match docs: target="code" keeps implementation only.`

const descSearchHybridQuery = `One identifier, or one sentence about behavior.`

const descSearchHybridTarget = `code | docs | db. Omit to search everything.`

const descSearchHybridDetail = `ids (default) | full.`

const descSearchKeyword = `Find exact text from the code — an error message, log line, config key, constant, route — and the function or class each match is in.
Pattern: 1-5 words copied from code ("failed to acquire lock"). Not a question ("how are retries configured"): a sentence never appears verbatim. Literal and case-insensitive; regex=true for RE2.
Use grep instead for every occurrence (this returns at most 3 lines per file, 20 files), a case-sensitive match, or files outside the project.
Returns files ranked by match count, each matched line with its line number and enclosing unit; context=N adds the lines around it, detail="full" the unit's name and signature.`

const descSearchKeywordPattern = `Exact text from the code, 1-5 words.`

const descSearchKeywordContext = `Lines around each match, 0-5 (default 0).`

const descSearchKeywordDetail = `ids (default) | full.`

const descExploreGraph = `Show who calls or references a unit, and what it calls or depends on. References are resolved, so unrelated same-named symbols stay out and aliased callers come in — which grepping a name cannot do.
Use direction="used_by" for "what calls X" / "what breaks if X changes", "uses" for dependencies. The default "both" lists uses first, so a busy unit's callers can fall onto the next page: ask for used_by when callers matter.
Input: symbol = a name, Type.method, file path, or path:line. Not for finding where something is — search first.
Returns up to 20 neighbours per page with file:line and confidence (1.00 certain … 0.80 same-name guess, hidden below the 0.85 default); has_more means there are more. detail="full" adds the line where each reference is written. No edge is not proof of no relation: dynamic calls and reflection are invisible.`

const descExploreDirection = `used_by | uses | both (default).`

const descExploreMinConfidence = `Default 0.85; 0.75 includes same-name guesses.`

const descExploreDetail = `ids (default) | full.`

const descReadCode = `Read one function, class, method, doc section or table — just that unit, not the whole file.
Use it when you know which unit you need and want its body. To find where something is, search first; to read a whole file, use a file read instead.
Input: symbol = a name, Type.method, file path, path:line (the unit enclosing it) or path:start-end (those lines); symbols = up to 20, read in order. lines="a-b" reads part of a long unit (at most 200).
Returns the source with its file and line range; units that did not fit are listed to request again. detail="full" adds callers and callees.`

const descReadDetail = `code (default) | full.`
