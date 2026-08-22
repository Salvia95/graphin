import type { ReactNode } from "react"
import type { Decision, SetView } from "@/api"
import { DecisionCard } from "@/DecisionCard"
import { TIER, type Tier } from "@/lib/tiers"
import { cn } from "@/lib/utils"

/** A group heading is the fourth axis severity travels on: position. Even with
 *  the colours stripped out, "Fix now" above a block of cards says the same
 *  thing the red bar does. */
export function DecisionGroup({
  tier,
  label,
  note,
  items,
  sets,
  cap,
  expanded,
  onToggle,
  onReview,
  action,
}: {
  tier: Tier
  label: string
  note: string
  items: Decision[]
  sets: SetView[]
  /** How many to show before folding. Undefined never folds. */
  cap?: number
  expanded: boolean
  onToggle: () => void
  onReview: (canonical: string) => void
  action?: ReactNode
}) {
  const t = TIER[tier]
  const folds = cap !== undefined && items.length > cap
  const shown = folds && !expanded ? items.slice(0, cap) : items

  return (
    <section className="mb-8">
      <header className="mb-3 flex items-center gap-3">
        <span className={cn("text-caption leading-none", t.text)} aria-hidden>
          {t.glyph}
        </span>
        <h2 className={cn("text-label font-semibold tracking-group uppercase", t.text)}>{label}</h2>
        <span className="num text-label text-muted">{items.length}</span>
        <span className="h-px flex-1 bg-hairline" />
        <span className="text-label text-muted">{note}</span>
        {action}
      </header>

      <div className="flex flex-col gap-2">
        {shown.map((d, i) => (
          <DecisionCard key={`${d.kind}-${d.title}-${i}`} d={d} sets={sets} onReview={onReview} />
        ))}
        {folds && (
          <button
            onClick={onToggle}
            className="rounded-lg border border-dashed border-hairline p-3 text-left text-body-sm text-muted-strong transition-colors hover:border-hairline-strong hover:text-body"
          >
            {expanded ? "Collapse" : `Show ${items.length - (cap ?? 0)} more`}
          </button>
        )}
      </div>
    </section>
  )
}
