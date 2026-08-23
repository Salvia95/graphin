import type { Overview, UsageReport } from "@/api"
import { Eyebrow } from "@/components/ui/field"
import { cn } from "@/lib/utils"

function Tile({
  label,
  onClick,
  children,
}: {
  label: string
  onClick?: () => void
  children: React.ReactNode
}) {
  const body = (
    <>
      <div className="mb-3.5 flex items-baseline justify-between">
        <Eyebrow>{label}</Eyebrow>
        {onClick && <span className="text-label text-muted">›</span>}
      </div>
      {children}
    </>
  )
  if (!onClick) return <div className="rounded-xl bg-surface p-5">{body}</div>
  return (
    <button
      onClick={onClick}
      className="rounded-xl bg-surface p-5 text-left transition-colors hover:bg-elevated"
    >
      {body}
    </button>
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
  onOpenUsage,
}: {
  o: Overview
  u: UsageReport | null
  queue: number
  backlog: number
  onOpenUsage: () => void
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

      {/* The number here is the queue, not every decision. It sat beside a tab
          reading "Decisions 6" while itself reading 12, which is one word
          meaning two things on one screen. The backlog is real and stays
          visible — one line down, where it cannot be mistaken for work that is
          waiting today. */}
      <Tile label="Decisions">
        <Stat>{queue}</Stat>
        <Sub>
          {backlog} in backlog · {kinds}/8 kinds across both
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

      {/* Session-level adoption, and the denominator is on the line below it
          because a rate without one is the defect usage-spec §4.2.1 was written
          to fix. The headline rate — adopted over adopted-plus-fell-back — is a
          different number and lives in the usage view this opens. */}
      <Tile label="Adoption" onClick={onOpenUsage}>
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
