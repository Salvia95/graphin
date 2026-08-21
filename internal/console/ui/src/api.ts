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
  evidence?: string[]
  role?: string
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
}

export type TermView = {
  canonical: string
  title?: string
  description?: string
  rel_path: string
  status: string
  trust: "unverified" | "machine-confirmed" | "human-reviewed"
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
  }
  glossary: { count: number; cap: number }
  decisions: Decision[]
  sets: SetView[]
  terms: TermView[]
}

export type UsageReport = {
  events: number
  sessions: number
  sessions_with_graphin: number
  groups?: Record<string, { adoptions?: number; fallbacks?: number }>
}

export type Edits = {
  title?: string
  description?: string
  tags?: string[]
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
  approve: (canonical: string, edits: Edits) =>
    post(`/api/queue/${encodeURIComponent(canonical)}/approve`, edits).then(json<Approved>),
  discard: async (canonical: string) => {
    const res = await post(`/api/queue/${encodeURIComponent(canonical)}/discard`)
    if (!res.ok) {
      const body = await res.json().catch(() => null)
      throw new Error(body?.error ?? `${res.status} ${res.statusText}`)
    }
  },
  repin: () => post("/api/wiki/repin").then(json<RepinResult>),
}
