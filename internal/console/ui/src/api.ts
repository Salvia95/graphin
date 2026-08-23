// The shapes the Go handlers marshal. Mirrors of wiki.Overview, wiki.QueueReport
// and usage.Report — the field names are the Go ones, which are the frontmatter
// keys, so the API, the file and the form cannot disagree about what something
// is called.

export type DecisionKind =
  | "dangling"
  | "glossary_full"
  | "expired"
  | "drift"
  | "approve"
  | "stale_skill"
  | "unread_set"
  | "uncovered"

export type Decision = {
  kind: DecisionKind
  severity: number
  title: string
  detail: string
  action: string
  set?: string
  node_id?: string
  canonical?: string
  count?: number
  /** Dates bounding the occurrences `count` sums. Three misses last week and
   *  three in March are not the same decision. */
  first_seen?: string
  last_seen?: string
  evidence?: string[]
  role?: string
}

export type EntryStatus = "ok" | "drift" | "dangling"

export type EntryView = {
  title: string
  node_id: string
  summary: string
  line: number
  status: EntryStatus
}

export type SetView = {
  name: string
  title: string
  summary: string
  rel_path: string
  roles: string[]
  tags: string[]
  prerequisites: string[]
  mode: string
  entries: number
  offered: number
  opened: number
  expired: boolean
  dangling: number
  drifted: number
  items: EntryView[]
}

export type Trust = "unverified" | "machine-confirmed" | "human-reviewed"

export type TermView = {
  canonical: string
  title?: string
  description?: string
  rel_path: string
  status: string
  trust: Trust
  aliases?: string[]
  evidence: number
  expired: boolean
}

export type Overview = {
  present: boolean
  health: {
    decisions: number
    dangling: number
    drifted: number
    expired: number
    awaiting: number
    sets: number
    entries: number
    /** Misses a set now matches. They left the backlog, and a number that
     *  shrinks with no trace is a number nobody trusts twice. */
    answered: number
  }
  glossary: { count: number; cap: number }
  decisions: Decision[]
  sets: SetView[]
  terms: TermView[]
}

/** The headline counters for one population. Every rate the screen shows is
 *  derived from these at render time and printed next to its own denominator —
 *  docs/usage-spec.md §4.2.1 is the record of what happens when it is not. */
export type GroupMetrics = {
  windows: number
  windows_with_search: number
  windows_with_symbol_search: number
  windows_with_graphin: number
  adoptions: number
  fallbacks: number
  same_intent_fallbacks: number
  inconclusive: number
  late_switches: number
  discovery_failures: number
  funnel_searches: number
  funnel_adherent: number
}

/** Run outcomes split by what the run touched. The three are NOT a partition:
 *  a run that surfaced more than one kind counts in each. */
export type TargetMetrics = {
  runs: number
  adoptions: number
  fallbacks: number
  same_intent_fallbacks: number
  inconclusive: number
}

export type FallbackPair = {
  ts: string
  query: string
  pattern: string
  same_intent: boolean
}

export type DayTrend = { date: string; adoptions: number; fallbacks: number }

export type UsageReport = {
  events: number
  sessions: number
  sessions_with_graphin: number
  /** -1 means no session used graphin at all — not zero calls. */
  median_calls_to_first_nav: number
  groups: Record<string, GroupMetrics>
  targets: Record<string, TargetMetrics>
  fallback_pairs: FallbackPair[]
  search_shapes: Record<string, number>
  daily: DayTrend[]
  problems?: string[]
}

export type Workspace = { root: string; name: string }

/** One queued proposal in full. The queue list carries none of this on purpose;
 *  the form that needs it opens one candidate at a time. */
export type Candidate = {
  canonical: string
  title?: string
  description?: string
  body?: string
  tags?: string[]
  aliases?: string[]
  evidence?: string[]
  status?: string
  file: string
  seen: number
}

export type Edits = {
  title?: string
  description?: string
  tags?: string[]
  aliases?: string[]
  body?: string
}

export type Approved = {
  term: { canonical: string; status: string; reviewed?: { by: string; at: string }[] }
  file: string
  note: string
}

export type RepinResult = {
  added: number
  updated: number
  dropped: number
  problems: { kind: string; set: string; node_id: string }[]
  path: string
  wrote: boolean
}

async function json<T>(res: Response): Promise<T> {
  if (!res.ok) {
    // Failures stay JSON on the Go side precisely so this branch can say
    // something a person can act on instead of "request failed".
    const body = await res.json().catch(() => null)
    throw new Error(body?.error ?? `${res.status} ${res.statusText}`)
  }
  return res.json() as Promise<T>
}

const post = (path: string, body?: unknown) =>
  fetch(path, {
    method: "POST",
    headers: body ? { "content-type": "application/json" } : undefined,
    body: body ? JSON.stringify(body) : undefined,
  })

export const api = {
  wiki: () => fetch("/api/wiki").then(json<Overview>),
  usage: () => fetch("/api/usage").then(json<UsageReport>),
  workspace: () => fetch("/api/workspace").then(json<Workspace>),
  candidate: (canonical: string) =>
    fetch(`/api/queue/${encodeURIComponent(canonical)}`).then(json<Candidate>),
  approve: (canonical: string, edits: Edits) =>
    post(`/api/queue/${encodeURIComponent(canonical)}/approve`, edits).then(json<Approved>),
  discard: async (canonical: string) => {
    const res = await post(`/api/queue/${encodeURIComponent(canonical)}/discard`)
    if (!res.ok) {
      const body = await res.json().catch(() => null)
      throw new Error(body?.error ?? `${res.status} ${res.statusText}`)
    }
  },
  /** No scope repins everything; a (set, node) pair repins the one entry a
   *  person just re-read. */
  repin: (scope?: { set: string; node_id: string }) =>
    post("/api/wiki/repin", scope).then(json<RepinResult>),
}
