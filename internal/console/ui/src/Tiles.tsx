import type { Overview, UsageReport } from "@/api"
import { Eyebrow } from "@/components/ui/field"
import { cn } from "@/lib/utils"

function Tile({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="rounded-xl bg-surface p-5">
      <Eyebrow className="mb-3.5 block">{label}</Eyebrow>
      {children}
    </div>
  )
}

/** The stat voice: brand yellow, tabular, oversized. It is the one non-action
 *  place yellow is allowed, and the reason severity never uses it. */
function Stat({ children, tone }: { children: React.ReactNode; tone?: string }) {
  return <div className={cn("num text-number-display", tone ?? "text-primary")}>{children}</div>
}

function Sub({ children, tone }: { children: React.ReactNode; tone?: string }) {
  return <div className={cn("mt-2 text-caption", tone ?? "text-muted")}>{children}</div>
}

/** Wiki health is three numbers rather than one, because they are three
 *  different problems: a broken link is wrong today, a drifted pin may be wrong,
 *  an expired one is simply old. Rolling them into a single score would make the
 *  tile read the same for all three. */
function Count({ n, label, tone }: { n: number; label: string; tone: string }) {
  return (
    <div>
      <div className={cn("num text-number-xl", n > 0 ? tone : "text-muted")}>{n}</div>
      <div className="mt-1.5 text-label text-muted">{label}</div>
    </div>
  )
}

export function Tiles({
  o,
  u,
  queue,
  backlog,
}: {
  o: Overview
  u: UsageReport | null
  queue: number
  backlog: number
}) {
  const h = o.health
  const kinds = new Set(o.decisions.map((d) => d.kind)).size
  const full = o.glossary.count >= o.glossary.cap
  const free = o.glossary.cap - o.glossary.count
  const adoption =
    u && u.sessions > 0 ? Math.round((u.sessions_with_graphin / u.sessions) * 100) : null

  return (
    <div className="grid grid-cols-4 gap-4 px-8 pt-6 pb-2">
      <Tile label="Wiki health">
        <div className="flex gap-6">
          <Count n={h.dangling} label="broken" tone="text-status-alert" />
          <Count n={h.drifted} label="drift" tone="text-status-watch" />
          <Count n={h.expired} label="expired" tone="text-status-watch" />
        </div>
      </Tile>

      <Tile label="Decisions">
        <Stat>{h.decisions}</Stat>
        <Sub>
          {kinds} of 8 kinds · {queue} in the queue, {backlog} in backlog
        </Sub>
      </Tile>

      <Tile label="Glossary">
        <Stat>
          {o.glossary.count}/{o.glossary.cap}
        </Stat>
        <Sub tone={full ? "text-status-alert" : undefined}>
          {full ? "full — approvals blocked" : `room for ${free} more`}
        </Sub>
      </Tile>

      <Tile label="Adoption">
        <Stat tone={adoption === null ? "text-muted" : undefined}>
          {adoption === null ? "—" : `${adoption}%`}
        </Stat>
        <Sub>
          {u && u.sessions > 0
            ? `${u.sessions_with_graphin} of ${u.sessions} sessions · ${u.events.toLocaleString()} events`
            : "no usage log yet"}
        </Sub>
      </Tile>
    </div>
  )
}
