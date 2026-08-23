import { useState } from "react"
import type { Decision, SetView } from "@/api"
import { Button } from "@/components/ui/button"
import { Chip, Eyebrow } from "@/components/ui/field"
import { copy } from "@/lib/clipboard"
import { TIER, kindLabel, locator, metaOf, tierOf, titleIsMono } from "@/lib/tiers"
import { cn } from "@/lib/utils"

/** The only thing the console can do for a decision it cannot resolve itself.
 *
 *  Seven of the eight kinds end in an editor: fixing a link, rewriting a
 *  summary, retiring a term. The console does not open editors and will not
 *  pretend to — so the button hands over the one thing that is tedious to
 *  retype, the node ID or the set file's path, and the "Next" line above says
 *  what to do with it.
 */
function CopyLocator({ text }: { text: string }) {
  const [done, setDone] = useState(false)
  return (
    <Button
      variant="outline"
      size="sm"
      className="px-3.5"
      title={text}
      onClick={async () => {
        if (await copy(text)) {
          setDone(true)
          window.setTimeout(() => setDone(false), 1400)
        }
      }}
    >
      {done ? "Copied" : "Copy"}
    </Button>
  )
}

/** The action the drift card's own instruction describes.
 *
 *  "Re-read it, confirm the summary still holds, then repin" is about one
 *  section. Repin-all is still there for the person who read everything, but it
 *  belongs to the group, not to a card — pressing it from here would clear the
 *  warnings on entries nobody opened. */
function RepinEntry({ d, onRepin }: { d: Decision; onRepin: RepinFn }) {
  const [busy, setBusy] = useState(false)
  const [done, setDone] = useState(false)
  return (
    <Button
      variant="outline"
      size="sm"
      className="px-3.5"
      disabled={busy || done}
      onClick={async () => {
        setBusy(true)
        const ok = await onRepin({ set: d.set!, node_id: d.node_id! })
        setBusy(false)
        setDone(ok)
      }}
    >
      {done ? "Repinned" : "Repin this"}
    </Button>
  )
}

export type RepinFn = (scope: { set: string; node_id: string }) => Promise<boolean>

export function DecisionCard({
  d,
  sets,
  onReview,
  onRepin,
}: {
  d: Decision
  sets: SetView[]
  onReview: (canonical: string) => void
  onRepin: RepinFn
}) {
  const t = TIER[tierOf(d)]
  const meta = metaOf(d)

  return (
    <article className={cn("rounded-lg bg-surface px-5 py-4", t.bar)}>
      <div className="flex items-start gap-4">
        {/* Fixed gutter so the kind column reads down the page as a column,
            not as eight ragged labels. */}
        <div className="w-22 shrink-0 pt-0.5">
          <div className="flex items-center gap-1.5">
            <span className={cn("text-label leading-none", t.text)} aria-hidden>
              {t.glyph}
            </span>
            <span className={cn("num text-tag tracking-tag uppercase", t.text)}>
              {kindLabel(d.kind)}
            </span>
          </div>
        </div>

        <div className="min-w-0 flex-1">
          <h3
            className={cn(
              "text-title-xs break-words text-on-dark",
              titleIsMono(d) && "num",
            )}
          >
            {d.title}
          </h3>
          <p className="mt-1.5 max-w-[76ch] text-pretty text-body-md text-muted-strong">
            {d.detail}
          </p>
          <p className="mt-2.5 flex items-baseline gap-2">
            <Eyebrow className="shrink-0">Next</Eyebrow>
            <span className="text-body-md text-body">{d.action}</span>
          </p>
          {meta.length > 0 && (
            <div className="mt-3 flex flex-wrap gap-1.5">
              {meta.map((m) => (
                <Chip key={m}>{m}</Chip>
              ))}
            </div>
          )}
        </div>

        <div className="flex shrink-0 items-center gap-2 pt-0.5">
          {d.kind === "approve" ? (
            <Button size="sm" onClick={() => onReview(d.canonical ?? d.title)}>
              Review candidate
            </Button>
          ) : (
            <>
              <CopyLocator text={locator(d, sets)} />
              {d.kind === "drift" && d.set && d.node_id && (
                <RepinEntry d={d} onRepin={onRepin} />
              )}
            </>
          )}
        </div>
      </div>
    </article>
  )
}
