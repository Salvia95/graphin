import { useCallback, useEffect, useState } from "react"
import { api, type Approved, type QueueReport, type UsageReport } from "@/api"
import { Candidate } from "@/Candidate"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Badge } from "@/components/ui/field"

function Section({
  title,
  count,
  children,
}: {
  title: string
  count: number
  children: React.ReactNode
}) {
  return (
    <section className="space-y-3">
      <h2 className="flex items-center gap-2 text-sm font-semibold uppercase tracking-wide text-slate-500">
        {title} <Badge>{count}</Badge>
      </h2>
      {children}
    </section>
  )
}

export default function App() {
  const [queue, setQueue] = useState<QueueReport | null>(null)
  const [usage, setUsage] = useState<UsageReport | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [done, setDone] = useState<Approved[]>([])

  const reload = useCallback(() => {
    api.queue().then(setQueue).catch((e: Error) => setError(e.message))
    // Adoption is a nice-to-have here; a workspace with no usage log yet is an
    // ordinary state and must not blank the page that has the real work on it.
    api.usage().then(setUsage).catch(() => setUsage(null))
  }, [])

  useEffect(reload, [reload])

  if (error) {
    return (
      <main className="mx-auto max-w-3xl p-8">
        <p className="rounded-md border border-red-300 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900 dark:bg-red-950 dark:text-red-400">
          {error}
        </p>
      </main>
    )
  }
  if (!queue) return <main className="p-8 text-sm text-slate-500">읽는 중…</main>

  return (
    <main className="mx-auto max-w-3xl space-y-8 p-6 sm:p-8">
      <header className="space-y-1">
        <h1 className="text-xl font-semibold">graphin console</h1>
        <p className="text-sm text-slate-500">
          용어집 {queue.glossary.count} / {queue.glossary.cap}
          {usage && ` · 이벤트 ${usage.events} · 세션 ${usage.sessions_with_graphin}/${usage.sessions}`}
        </p>
      </header>

      {done.length > 0 && (
        <Card className="border-emerald-300 dark:border-emerald-900">
          <CardHeader>
            <CardTitle className="text-sm">
              {done.length}건 처리됨 — 아직 커밋되지 않았습니다
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-1 text-xs">
            {done.map((d) => (
              <p key={d.file} className="font-mono text-slate-600 dark:text-slate-400">
                {d.file}
              </p>
            ))}
            <p className="pt-1 text-slate-500">
              워킹 트리에만 쓰였습니다. <code>git diff</code>로 확인하고 직접 커밋하십시오.
            </p>
          </CardContent>
        </Card>
      )}

      <Section title="승인 대기" count={queue.awaiting_review.length}>
        {queue.awaiting_review.length === 0 ? (
          <p className="text-sm text-slate-500">없음</p>
        ) : (
          <div className="space-y-3">
            {queue.awaiting_review.map((p) => (
              <Candidate
                key={p.canonical}
                p={p}
                onDone={(result) => {
                  if (result) setDone((d) => [...d, result])
                  reload()
                }}
              />
            ))}
          </div>
        )}
      </Section>

      <Section title="위키가 답하지 못한 작업" count={queue.misses.length}>
        {queue.misses.length === 0 ? (
          <p className="text-sm text-slate-500">없음</p>
        ) : (
          <ul className="space-y-1 text-sm">
            {queue.misses.slice(0, 20).map((m, i) => (
              <li key={`${m.ts}-${i}`} className="flex gap-2">
                <Badge>{m.role || "-"}</Badge>
                <span className="text-slate-700 dark:text-slate-300">{m.task}</span>
              </li>
            ))}
          </ul>
        )}
      </Section>

      {queue.unread_sets.length > 0 && (
        <Section title="제시됐지만 아무도 안 연 세트" count={queue.unread_sets.length}>
          <ul className="space-y-1 text-sm">
            {queue.unread_sets.map((s) => (
              <li key={s.set}>
                <span className="font-mono">{s.set}</span>
                <span className="text-slate-500"> — {s.offered}회 제시, 0회 열림</span>
              </li>
            ))}
          </ul>
        </Section>
      )}

      {queue.drifted.length > 0 && (
        <Section title="낡은 핀으로 서빙된 절" count={queue.drifted.length}>
          <ul className="space-y-1 text-sm">
            {queue.drifted.map((d) => (
              <li key={d.node} className="font-mono text-xs">
                {d.node} <span className="text-slate-500">({d.served})</span>
              </li>
            ))}
          </ul>
          <p className="text-xs text-slate-500">
            각각 다시 읽고 요약이 여전히 맞는지 확인한 뒤 <code>graphin wiki repin</code>.
          </p>
        </Section>
      )}
    </main>
  )
}
