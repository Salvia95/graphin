// The shapes the Go handlers marshal. These mirror wiki.QueueReport and
// usage.Report; the field names are the frontmatter keys for the same reason
// they are on the Go side — one set of names for the API, the file and the form.

export type QueuedProposal = {
  canonical: string
  file: string
  seen: number
  evidence: string[]
}

export type Miss = { ts: string; task?: string; role?: string }

export type QueueReport = {
  glossary: { count: number; cap: number }
  awaiting_review: QueuedProposal[]
  misses: Miss[]
  unread_sets: { set: string; offered: number }[]
  drifted: { node: string; served: number }[]
}

export type UsageReport = {
  events: number
  sessions: number
  sessions_with_graphin: number
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

async function json<T>(res: Response): Promise<T> {
  if (!res.ok) {
    // Failures stay JSON on the Go side precisely so this branch can say
    // something a person can act on instead of "request failed".
    const body = await res.json().catch(() => null)
    throw new Error(body?.error ?? `${res.status} ${res.statusText}`)
  }
  return res.json() as Promise<T>
}

export const api = {
  queue: () => fetch("/api/queue").then(json<QueueReport>),
  usage: () => fetch("/api/usage").then(json<UsageReport>),
  approve: (canonical: string, edits: Edits) =>
    fetch(`/api/queue/${encodeURIComponent(canonical)}/approve`, {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify(edits),
    }).then(json<Approved>),
  discard: async (canonical: string) => {
    const res = await fetch(`/api/queue/${encodeURIComponent(canonical)}/discard`, {
      method: "POST",
    })
    if (!res.ok) {
      const body = await res.json().catch(() => null)
      throw new Error(body?.error ?? `${res.status} ${res.statusText}`)
    }
  },
}
