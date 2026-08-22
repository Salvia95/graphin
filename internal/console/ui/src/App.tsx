import { useCallback, useEffect, useMemo, useState } from "react"
import {
  api,
  type Overview,
  type RepinResult,
  type UsageReport,
  type Workspace,
} from "@/api"
import { ApproveDrawer } from "@/ApproveDrawer"
import { DecisionGroup } from "@/DecisionGroup"
import { HealthyPanel } from "@/HealthyPanel"
import { SetDrawer } from "@/SetDrawer"
import { SetsGrid, SetsRail } from "@/Sets"
import { Tiles } from "@/Tiles"
import { WikiMap } from "@/WikiMap"
import { Wordmark } from "@/components/Wordmark"
import { Button } from "@/components/ui/button"
import { Eyebrow, Num } from "@/components/ui/field"
import { BACKLOG_GROUPS, TIER, TIER_ORDER, type Tier, isQueue, tierOf } from "@/lib/tiers"
import { cn } from "@/lib/utils"

type View = "queue" | "backlog" | "map"
type Filter = Tier | "all"

/** Backlog groups fold. `uncovered` grows one row per unanswered question and
 *  can reach the hundreds — a list that long stops being a list. Ten is enough
 *  to see what kind of thing is in there. */
const BACKLOG_CAP = 10

function Tab({
  active,
  label,
  count,
  onClick,
}: {
  active: boolean
  label: string
  count: number
  onClick: () => void
}) {
  return (
    <button
      onClick={onClick}
      className={cn(
        "-mb-px flex items-center gap-2 border-b-2 pb-3 text-title-sm transition-colors",
        active ? "border-primary text-on-dark" : "border-transparent text-muted hover:text-body",
      )}
    >
      <span>{label}</span>
      <span
        className={cn(
          "num rounded-sm bg-surface px-[7px] py-0.5 text-number-sm",
          active ? "text-muted-strong" : "text-muted",
        )}
      >
        {count}
      </span>
    </button>
  )
}

function FilterChip({
  active,
  glyph,
  label,
  count,
  tone,
  onClick,
}: {
  active: boolean
  glyph?: string
  label: string
  count: number
  tone: string
  onClick: () => void
}) {
  return (
    <button
      onClick={onClick}
      className={cn(
        "flex h-[30px] items-center gap-2 rounded-md border px-3 text-body-sm whitespace-nowrap transition-colors",
        active
          ? "border-hairline-strong bg-elevated text-body"
          : cn("border-hairline bg-transparent hover:border-hairline-strong", tone),
      )}
    >
      {glyph && (
        <span className="opacity-75" aria-hidden>
          {glyph}
        </span>
      )}
      <span>{label}</span>
      <span className="num opacity-70">{count}</span>
    </button>
  )
}

export default function App() {
  const [o, setO] = useState<Overview | null>(null)
  const [u, setU] = useState<UsageReport | null>(null)
  const [ws, setWs] = useState<Workspace | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [view, setView] = useState<View>("queue")
  const [filter, setFilter] = useState<Filter>("all")
  const [expanded, setExpanded] = useState<Record<string, boolean>>({})
  const [approving, setApproving] = useState<string | null>(null)
  const [openSet, setOpenSet] = useState<string | null>(null)
  const [written, setWritten] = useState<string[]>([])
  const [repin, setRepin] = useState<RepinResult | null>(null)
  const [busy, setBusy] = useState(false)

  const reload = useCallback(() => {
    api.wiki().then(setO).catch((e: Error) => setError(e.message))
    // Adoption is context, not the work. A workspace with no usage log yet is an
    // ordinary state and must not blank the page the decisions are on.
    api.usage().then(setU).catch(() => setU(null))
    api.workspace().then(setWs).catch(() => setWs(null))
  }, [])
  useEffect(reload, [reload])

  const { queue, backlog } = useMemo(() => {
    const all = o?.decisions ?? []
    return { queue: all.filter(isQueue), backlog: all.filter((d) => !isQueue(d)) }
  }, [o])

  if (error) {
    return (
      <main className="mx-auto max-w-3xl p-10">
        <p className="rounded-lg border border-status-alert/40 px-6 py-4 text-body-md text-status-alert">
          {error}
        </p>
      </main>
    )
  }
  if (!o) return <main className="p-10 text-body-md text-muted">Loading…</main>

  if (!o.present) {
    return (
      <main className="mx-auto max-w-2xl space-y-4 p-10">
        <Wordmark height={22} className="text-body" />
        <p className="text-body-md text-muted-strong">
          No <span className="num text-body">docs/wiki</span> in this workspace. Without a
          knowledge layer there is nothing to decide, and the knowledge gate stays disarmed.
        </p>
      </main>
    )
  }

  // The rail exists to make room for a queue. With nothing in the queue there is
  // nothing to make room for, and the catalogue takes the width instead.
  const problem = queue.length > 0
  const isBacklog = view === "backlog"
  const isMap = view === "map"
  const source = isBacklog ? backlog : queue
  const shown = filter === "all" ? source : source.filter((d) => tierOf(d) === filter)

  const counts = { all: source.length, alert: 0, watch: 0, info: 0, neutral: 0 }
  for (const d of source) counts[tierOf(d)]++
  const tiers = TIER_ORDER.filter((t) => counts[t] > 0)
  const showFilters = !isBacklog && tiers.length > 1

  const toggle = (key: string) => setExpanded((e) => ({ ...e, [key]: !e[key] }))

  async function doRepin() {
    setBusy(true)
    try {
      const r = await api.repin()
      setRepin(r)
      setWritten((w) => [...new Set([...w, r.path])])
      reload()
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(false)
    }
  }

  const groups = isBacklog
    ? BACKLOG_GROUPS.map(({ kind, label, note }) => {
        const items = shown.filter((d) => d.kind === kind)
        if (items.length === 0) return null
        return (
          <DecisionGroup
            key={kind}
            tier="neutral"
            label={label}
            note={note}
            items={items}
            sets={o.sets}
            cap={BACKLOG_CAP}
            expanded={!!expanded[kind]}
            onToggle={() => toggle(kind)}
            onReview={setApproving}
          />
        )
      })
    : TIER_ORDER.map((tier) => {
        const items = shown.filter((d) => tierOf(d) === tier)
        if (items.length === 0) return null
        return (
          <DecisionGroup
            key={tier}
            tier={tier}
            label={TIER[tier].label}
            note={TIER[tier].note}
            items={items}
            sets={o.sets}
            expanded
            onToggle={() => toggle(tier)}
            onReview={setApproving}
            action={
              tier === "watch" && o.health.drifted > 0 ? (
                <Button variant="ghost" size="xs" disabled={busy} onClick={doRepin}>
                  Repin all
                </Button>
              ) : undefined
            }
          />
        )
      })

  return (
    <div className="min-w-[1280px]">
      <header className="sticky top-0 z-20 flex h-16 items-center justify-between border-b border-hairline bg-canvas px-8">
        <Wordmark className="text-body" />
        <div className="flex items-center gap-3">
          <Eyebrow>workspace</Eyebrow>
          <span className="num text-caption text-body" title={ws?.root}>
            {ws?.name ?? "—"}
          </span>
        </div>
      </header>

      <Tiles o={o} u={u} queue={queue.length} backlog={backlog.length} />

      {written.length > 0 && (
        // The console's whole safety story is that it stops at the working tree.
        // Saying so once in a doc is not the same as saying it at the moment a
        // file changes, so this appears the instant one does.
        <div className="px-8 pt-2">
          <div className="rounded-lg border-l-2 border-l-info bg-surface px-5 py-4">
            <p className="text-body-sm text-body">
              <Num>{written.length}</Num> file{written.length === 1 ? "" : "s"} written to the
              working tree — <span className="text-on-dark">not committed</span>. Review with{" "}
              <span className="num">git diff</span> and commit them yourself.
            </p>
            <div className="mt-2 flex flex-col gap-1">
              {written.map((f) => (
                <span key={f} className="num text-label text-muted">
                  {f}
                </span>
              ))}
            </div>
            {repin && (
              <p className="mt-2 text-label text-muted">
                repin: <Num>{repin.added}</Num> added · <Num>{repin.updated}</Num> updated ·{" "}
                <Num>{repin.dropped}</Num> dropped
              </p>
            )}
          </div>
        </div>
      )}

      <div
        className={cn(
          "grid items-start gap-6 px-8 pt-4 pb-16",
          problem && !isMap ? "grid-cols-[minmax(0,1fr)_340px]" : "grid-cols-[minmax(0,1fr)]",
        )}
      >
        <div>
          {isMap ? (
            <WikiMap o={o} onOpenSet={setOpenSet} onBack={() => setView("queue")} />
          ) : (
            <>
              <div className="mb-5 flex items-center gap-5 border-b border-hairline">
                <Tab
                  active={view === "queue"}
                  label="Decisions"
                  count={queue.length}
                  onClick={() => {
                    setView("queue")
                    setFilter("all")
                  }}
                />
                <Tab
                  active={isBacklog}
                  label="Backlog"
                  count={backlog.length}
                  onClick={() => {
                    setView("backlog")
                    setFilter("all")
                  }}
                />
                <span className="flex-1" />
                <span className="pb-3 text-caption whitespace-nowrap text-muted">
                  {isBacklog
                    ? "Unread sets and unanswered questions — nothing here blocks you today."
                    : "Sorted by what it costs the reader right now."}
                </span>
              </div>

              {showFilters && (
                <div className="mb-6 flex flex-wrap gap-2">
                  <FilterChip
                    active={filter === "all"}
                    label="All"
                    count={counts.all}
                    tone="text-muted-strong"
                    onClick={() => setFilter("all")}
                  />
                  {tiers.map((t) => (
                    <FilterChip
                      key={t}
                      active={filter === t}
                      glyph={TIER[t].glyph}
                      label={TIER[t].label}
                      count={counts[t]}
                      tone={TIER[t].text}
                      onClick={() => setFilter(filter === t ? "all" : t)}
                    />
                  ))}
                </div>
              )}

              {!problem && !isBacklog && <HealthyPanel o={o} backlog={backlog.length} />}

              {groups}

              {shown.length === 0 && (problem || isBacklog) && (
                <p className="text-body-md text-muted">
                  {isBacklog ? "Nothing in the backlog." : "None of this kind."}
                </p>
              )}

              {!problem && (
                <SetsGrid
                  sets={o.sets}
                  glossaryEmpty={o.terms.length === 0}
                  onOpenSet={setOpenSet}
                  onOpenMap={() => setView("map")}
                />
              )}
            </>
          )}
        </div>

        {problem && !isMap && (
          <SetsRail
            sets={o.sets}
            terms={o.terms}
            count={o.glossary.count}
            cap={o.glossary.cap}
            onOpenSet={setOpenSet}
            onOpenMap={() => setView("map")}
          />
        )}
      </div>

      <SetDrawer s={o.sets.find((s) => s.name === openSet) ?? null} onClose={() => setOpenSet(null)} />

      <ApproveDrawer
        canonical={approving}
        glossaryFull={o.glossary.count >= o.glossary.cap}
        onClose={() => setApproving(null)}
        onDone={(file) => {
          if (file) setWritten((w) => [...new Set([...w, file])])
          reload()
        }}
      />
    </div>
  )
}
