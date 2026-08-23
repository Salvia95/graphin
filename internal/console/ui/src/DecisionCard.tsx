import { useState } from "react"
import type { Decision, SetView, Workspace } from "@/api"
import { Button } from "@/components/ui/button"
import { Chip, Eyebrow } from "@/components/ui/field"
import { copy } from "@/lib/clipboard"
import { editorHref } from "@/lib/editor"
import { TIER, kindLabel, locator, metaOf, tierOf, titleIsMono } from "@/lib/tiers"
import { cn } from "@/lib/utils"

/** Everything a card can set in motion. Bundled because the list grew past the
 *  point where threading each one through the group was readable. */
export type CardActions = {
  onReview: (canonical: string) => void
  onRepin: RepinFn
  onRetire: () => void
  onEditSet: (name: string) => void
}

/** The fallback, and still the only thing that always works.
 *
 *  Copy hands over the one thing that is tedious to retype — the node ID or the
 *  set file's path — for the cases where the fix is typed somewhere this
 *  console cannot reach. Open sits beside it when the reader's editor is known,
 *  and is absent rather than dead when it is not.
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
  ws,
  actions,
}: {
  d: Decision
  sets: SetView[]
  ws: Workspace | null
  actions: CardActions
}) {
  const href = editorHref(ws, d)
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
            <Button size="sm" onClick={() => actions.onReview(d.canonical ?? d.title)}>
              Review candidate
            </Button>
          ) : (
            <>
              <CopyLocator text={locator(d, sets)} />
              {href && (
                <a
                  href={href}
                  title={`${d.file}${d.line ? `:${d.line}` : ""}`}
                  className="inline-flex h-8 items-center rounded-md border border-hairline px-3.5 text-button whitespace-nowrap text-muted-strong transition-colors hover:border-hairline-strong hover:text-body"
                >
                  Open
                </a>
              )}
              {d.kind === "drift" && d.set && d.node_id && (
                <RepinEntry d={d} onRepin={actions.onRepin} />
              )}
              {d.kind === "glossary_full" && (
                <Button variant="outline" size="sm" className="px-3.5" onClick={actions.onRetire}>
                  Retire a term
                </Button>
              )}
              {d.kind === "unread_set" && d.set && (
                <Button
                  variant="outline"
                  size="sm"
                  className="px-3.5"
                  onClick={() => actions.onEditSet(d.set!)}
                >
                  Edit summary
                </Button>
              )}
            </>
          )}
        </div>
      </div>
    </article>
  )
}
