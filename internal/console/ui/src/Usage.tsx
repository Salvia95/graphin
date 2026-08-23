import { useState } from "react"
import type { GroupMetrics, UsageReport } from "@/api"
import { LinkButton } from "@/components/ui/button"
import { Eyebrow, Num } from "@/components/ui/field"
import { cn } from "@/lib/utils"

// Every rate here is printed beside the two numbers it came from.
//
// That is not a style choice. docs/usage-spec.md §4.2.1 is the record of what
// happened the last time it was not: "discovery failure 57%" was half made of
// windows grepping stack traces, which graphin was never going to answer, and
// nobody could see that from the percentage. A ratio whose denominator is
// invisible is the exact defect that section exists to have fixed.
function Rate({
  label,
  num,
  den,
  note,
  tone,
}: {
  label: string
  num: number
  den: number
  note: string
  tone?: string
}) {
  const pct = den > 0 ? Math.round((num / den) * 100) : null
  return (
    <div className="rounded-lg bg-surface p-4">
      <Eyebrow className="block">{label}</Eyebrow>
      <div className={cn("num mt-2.5 text-number-xl", pct === null ? "text-muted" : (tone ?? "text-primary"))}>
        {pct === null ? "—" : `${pct}%`}
      </div>
      <div className="num mt-2 text-caption text-muted-strong">
        {num} / {den}
      </div>
      <div className="mt-1 text-pretty text-label text-muted">{note}</div>
    </div>
  )
}

/** Adoptions and fallbacks per day, drawn rather than tabulated because the
 *  question a trend answers is "which way" and a column of numbers does not
 *  answer it at a glance.
 *
 *  Fallback is not coloured as a failure. The spec is explicit that a
 *  same-intent fallback is the gold seam — the reproducible case that makes
 *  ranking better — so it takes `info`, the tone this system already uses for
 *  "something here is waiting for you to look at it". */
function Trend({ daily }: { daily: UsageReport["daily"] }) {
  const days = daily.slice(-30)
  const peak = Math.max(1, ...days.map((d) => Math.max(d.adoptions, d.fallbacks)))
  const w = 14
  const gap = 4
  const h = 96

  return (
    <div className="rounded-lg bg-surface p-5">
      <div className="mb-4 flex items-baseline justify-between">
        <Eyebrow>Daily</Eyebrow>
        <span className="flex items-center gap-4 text-label text-muted">
          <span className="flex items-center gap-1.5">
            <span className="inline-block h-2 w-2 rounded-xs bg-primary" />
            adoptions
          </span>
          <span className="flex items-center gap-1.5">
            <span className="inline-block h-2 w-2 rounded-xs bg-info" />
            fallbacks
          </span>
          <Num>peak {peak}</Num>
        </span>
      </div>
      {days.length === 0 ? (
        <p className="text-caption text-muted">No runs recorded yet.</p>
      ) : (
        <div className="overflow-x-auto">
          <svg
            width={days.length * (w + gap)}
            height={h + 20}
            role="img"
            aria-label={`Adoptions and fallbacks over the last ${days.length} days`}
          >
            {days.map((d, i) => {
              const x = i * (w + gap)
              const a = (d.adoptions / peak) * h
              const f = (d.fallbacks / peak) * h
              return (
                <g key={d.date}>
                  <title>{`${d.date} · ${d.adoptions} adopted · ${d.fallbacks} fell back`}</title>
                  <rect
                    x={x}
                    y={h - a}
                    width={w / 2 - 1}
                    height={a}
                    fill="var(--color-primary)"
                  />
                  <rect
                    x={x + w / 2}
                    y={h - f}
                    width={w / 2 - 1}
                    height={f}
                    fill="var(--color-info)"
                  />
                  {i % 5 === 0 && (
                    <text
                      x={x}
                      y={h + 14}
                      fill="var(--color-muted)"
                      fontSize="10"
                      fontFamily="var(--font-num)"
                    >
                      {d.date.slice(5)}
                    </text>
                  )}
                </g>
              )
            })}
          </svg>
        </div>
      )}
    </div>
  )
}

const GROUPS = ["all", "main", "subagent"] as const

/** Adoption as the project actually measures it.
 *
 *  The tile on the dashboard shows session-level adoption — how many sessions
 *  touched graphin at all — because that is the number a glance can use. It is
 *  not the headline rate. This view exists so the two are never confused: the
 *  headline is adoptions over adoptions-plus-fallbacks, per run, and it lives
 *  next to the populations it was computed from.
 */
export function Usage({ u, onBack }: { u: UsageReport | null; onBack: () => void }) {
  const [group, setGroup] = useState<(typeof GROUPS)[number]>("all")

  if (!u) {
    return (
      <div>
        <Header onBack={onBack} />
        <p className="max-w-[70ch] text-pretty text-body-md text-muted">
          No usage log in this workspace. Events are written by the instrumentation plugin, so an
          empty log usually means the plugin is not installed or never fired — not that nobody used
          graphin.
        </p>
      </div>
    )
  }

  const g: GroupMetrics | undefined = u.groups?.[group]
  const shapes = u.search_shapes ?? {}
  const targets = u.targets ?? {}
  const pairs = u.fallback_pairs ?? []

  return (
    <div>
      <Header onBack={onBack} u={u} />

      <div className="mb-4 flex items-center gap-2">
        <Eyebrow className="mr-1">population</Eyebrow>
        {GROUPS.map((k) => (
          <button
            key={k}
            onClick={() => setGroup(k)}
            className={cn(
              "num h-7 rounded-sm px-3 text-caption transition-colors",
              group === k ? "bg-elevated text-body" : "text-muted hover:text-body",
            )}
          >
            {k}
          </button>
        ))}
      </div>

      {!g || g.windows === 0 ? (
        <p className="text-body-md text-muted">Nothing recorded for this population.</p>
      ) : (
        <>
          <div className="grid grid-cols-4 gap-3">
            <Rate
              label="Adoption"
              num={g.adoptions}
              den={g.adoptions + g.fallbacks}
              note="runs that ended in a read or an edit, over runs that ended either way"
            />
            <Rate
              label="Same-intent fallback"
              num={g.same_intent_fallbacks}
              den={g.fallbacks}
              tone="text-info"
              note="fallbacks whose grep repeated the graphin query — the reproducible cases"
            />
            <Rate
              label="Late switch"
              num={g.late_switches}
              den={g.windows_with_graphin}
              tone="text-status-watch"
              note="windows that grepped twice before reaching for graphin"
            />
            <Rate
              label="Discovery failure"
              num={g.discovery_failures}
              den={g.windows_with_symbol_search}
              tone="text-status-watch"
              note="symbol searches that never considered graphin — prose and regex are not in the denominator"
            />
          </div>

          <div className="mt-3 grid grid-cols-4 gap-3">
            <Rate
              label="Funnel adherence"
              num={g.funnel_adherent}
              den={g.funnel_searches}
              note="searches whose returned ids were actually explored or read"
            />
            <div className="rounded-lg bg-surface p-4">
              <Eyebrow className="block">Inconclusive</Eyebrow>
              <div className="num mt-2.5 text-number-xl text-muted">{g.inconclusive}</div>
              <div className="mt-2 text-pretty text-label text-muted">
                runs the window ended before anything followed them — counted, never guessed at
              </div>
            </div>
            <div className="rounded-lg bg-surface p-4">
              <Eyebrow className="block">Calls to first nav</Eyebrow>
              <div className="num mt-2.5 text-number-xl text-muted">
                {u.median_calls_to_first_nav < 0 ? "—" : u.median_calls_to_first_nav}
              </div>
              <div className="mt-2 text-pretty text-label text-muted">
                {u.median_calls_to_first_nav < 0
                  ? "no session reached graphin at all"
                  : "median tool calls before the first graphin call"}
              </div>
            </div>
            <div className="rounded-lg bg-surface p-4">
              <Eyebrow className="block">Search shapes</Eyebrow>
              <div className="mt-2.5 flex flex-col gap-1">
                {["symbol", "regex", "literal", "none"].map((k) => (
                  <div key={k} className="flex items-baseline justify-between text-label">
                    <span className="text-muted">{k}</span>
                    <Num className="text-muted-strong">{shapes[k] ?? 0}</Num>
                  </div>
                ))}
              </div>
            </div>
          </div>

          <div className="mt-6">
            <Trend daily={u.daily ?? []} />
          </div>

          <div className="mt-6">
            <div className="mb-3 flex items-baseline gap-3">
              <Eyebrow>By target</Eyebrow>
              <span className="text-label text-muted">
                overlapping populations, not a split — a run touching two kinds counts in both
              </span>
            </div>
            <div className="overflow-hidden rounded-lg border border-hairline">
              <table className="w-full text-body-sm">
                <thead>
                  <tr className="bg-surface text-left">
                    {["target", "runs", "adopted", "fell back", "same-intent", "inconclusive"].map(
                      (h) => (
                        <th key={h} className="px-4 py-2.5 text-label tracking-eyebrow text-muted uppercase">
                          {h}
                        </th>
                      ),
                    )}
                  </tr>
                </thead>
                <tbody>
                  {["code", "db", "docs"].map((k) => {
                    const t = targets[k]
                    return (
                      <tr key={k} className="border-t border-hairline">
                        <td className="num px-4 py-2.5 text-body">{k}</td>
                        {[
                          t?.runs ?? 0,
                          t?.adoptions ?? 0,
                          t?.fallbacks ?? 0,
                          t?.same_intent_fallbacks ?? 0,
                          t?.inconclusive ?? 0,
                        ].map((v, i) => (
                          <td key={i} className="num px-4 py-2.5 text-muted-strong">
                            {v}
                          </td>
                        ))}
                      </tr>
                    )
                  })}
                </tbody>
              </table>
            </div>
          </div>

          <div className="mt-6">
            <div className="mb-3 flex items-baseline gap-3">
              <Eyebrow>Fallback pairs</Eyebrow>
              <Num className="text-label text-muted">{pairs.length}</Num>
              <span className="text-label text-muted">
                the query graphin was given, and the grep that followed it
              </span>
            </div>
            {pairs.length === 0 ? (
              <p className="text-caption text-muted">None recorded.</p>
            ) : (
              <div className="flex flex-col gap-px overflow-hidden rounded-lg border border-hairline bg-elevated">
                {pairs.map((p, i) => (
                  <div key={`${p.ts}-${i}`} className="bg-canvas px-4 py-3">
                    <div className="flex items-baseline justify-between gap-4">
                      <span className="num text-caption break-words text-body">{p.query}</span>
                      <span
                        className={cn(
                          "num shrink-0 text-tag tracking-tag uppercase",
                          p.same_intent ? "text-info" : "text-muted",
                        )}
                      >
                        {p.same_intent ? "same intent" : "new intent"}
                      </span>
                    </div>
                    <div className="num mt-1 flex items-baseline gap-2 text-caption text-muted-strong">
                      <span className="text-muted">→</span>
                      <span className="break-words">{p.pattern}</span>
                    </div>
                    <div className="num mt-1 text-label text-muted">{p.ts}</div>
                  </div>
                ))}
              </div>
            )}
          </div>
        </>
      )}

      {u.problems && u.problems.length > 0 && (
        <p className="mt-6 rounded-lg border-l-2 border-l-status-watch bg-surface px-4 py-3 text-caption text-muted-strong">
          <Num>{u.problems.length}</Num> log line{u.problems.length === 1 ? "" : "s"} could not be
          read. The reader is deliberately lenient — a broken line is counted, not fatal.
        </p>
      )}
    </div>
  )
}

function Header({ onBack, u }: { onBack: () => void; u?: UsageReport }) {
  return (
    <div className="mb-5 flex items-baseline justify-between border-b border-hairline pb-3">
      <div className="flex items-baseline gap-3">
        <h2 className="text-title-sm text-on-dark">Usage</h2>
        {u && (
          <Num className="text-caption text-muted">
            {u.events.toLocaleString()} events · {u.sessions_with_graphin} of {u.sessions} sessions
          </Num>
        )}
      </div>
      <LinkButton onClick={onBack}>← Decisions</LinkButton>
    </div>
  )
}
