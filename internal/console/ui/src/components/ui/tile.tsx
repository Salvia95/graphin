import type { ReactNode } from "react"
import { cn } from "@/lib/utils"

/** `alert` is reserved for states costing a reader something right now — a
 *  broken link, not a stale one. */
export type Tone = "brand" | "watch" | "alert"

// The value carries the tone, not the card. Yellow here is the stat voice the
// reference gives large numbers; the status ramp takes over when something is
// actually wrong, which keeps yellow out of severity entirely.
const value: Record<Tone, string> = {
  brand: "text-primary",
  watch: "text-status-watch",
  alert: "text-status-alert",
}

export function Tile({
  label,
  value: v,
  note,
  tone = "brand",
}: {
  label: string
  value: ReactNode
  note?: ReactNode
  tone?: Tone
}) {
  return (
    <div className="rounded-xl bg-surface p-6">
      <div className="text-caption text-muted">{label}</div>
      <div className={cn("num mt-2 text-number-display", value[tone])}>{v}</div>
      {note && <div className="mt-1 text-body-sm text-muted">{note}</div>}
    </div>
  )
}
