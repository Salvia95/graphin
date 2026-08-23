import { useState } from "react"
import { api, type TermView } from "@/api"
import { Drawer, DrawerClose } from "@/components/Drawer"
import { Button } from "@/components/ui/button"
import { Num } from "@/components/ui/field"
import { TIER } from "@/lib/tiers"
import { trustGlyph } from "@/Sets"
import { cn } from "@/lib/utils"

/** The card said "remove or demote an existing entry" and the screen offered
 *  nothing to press. This is that button.
 *
 *  The cap is a decision point, not a limit to raise: which knowledge matters
 *  less is a judgement, and nothing here makes it. What the drawer does is put
 *  the evidence in one place — how often each term is cited, whether anyone
 *  vouched for it, whether it has gone stale — and then carry out the answer.
 */
export function RetireDrawer({
  open,
  terms,
  count,
  cap,
  onClose,
  onDone,
}: {
  open: boolean
  terms: TermView[]
  count: number
  cap: number
  onClose: () => void
  onDone: (file: string) => void
}) {
  const [confirming, setConfirming] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  // Expired first, then least-cited. The order is evidence, not advice: a term
  // past its own stale_after and one nobody quotes are the two that can be
  // argued for, and sorting says which those are without choosing between them.
  const ordered = [...terms].sort(
    (a, b) =>
      Number(b.expired) - Number(a.expired) ||
      a.evidence - b.evidence ||
      a.canonical.localeCompare(b.canonical),
  )
  const t = TIER.alert

  return (
    <Drawer open={open} onClose={onClose} width="w-[460px]">
      <div className="border-b border-hairline px-6 pt-6 pb-4">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            <span className={cn("text-body-md leading-none", t.text)} aria-hidden>
              {t.glyph}
            </span>
            <span className={cn("num text-tag tracking-tag uppercase", t.text)}>glossary full</span>
          </div>
          <DrawerClose onClose={onClose} />
        </div>
        <h2 className="mt-3 text-title-md text-on-dark">Retire a term</h2>
        <p className="mt-1 text-pretty text-caption text-muted-strong">
          <Num>
            {count}/{cap}
          </Num>{" "}
          — nothing new can be approved until one leaves. Retiring deletes the file in the working
          tree; <span className="num">git checkout</span> brings it back.
        </p>
      </div>

      <div className="flex-1 overflow-auto px-6 py-5">
        {ordered.length === 0 ? (
          <p className="text-body-sm text-muted">The glossary is empty.</p>
        ) : (
          <div className="flex flex-col gap-px overflow-hidden rounded-lg border border-hairline bg-elevated">
            {ordered.map((term) => {
              const g = trustGlyph[term.trust]
              const armed = confirming === term.canonical
              return (
                <div key={term.canonical} className="bg-canvas px-4 py-3">
                  <div className="flex items-baseline justify-between gap-3">
                    <span className="num text-body-sm break-words text-body">{term.canonical}</span>
                    <Button
                      variant={armed ? "danger" : "outline"}
                      size="xs"
                      disabled={busy}
                      onClick={() => {
                        if (!armed) {
                          setConfirming(term.canonical)
                          return
                        }
                        setBusy(true)
                        setError(null)
                        api
                          .retire(term.canonical)
                          .then((r) => {
                            onDone(r.file)
                            setConfirming(null)
                          })
                          .catch((e: Error) => setError(e.message))
                          .finally(() => setBusy(false))
                      }}
                    >
                      {armed ? "Confirm" : "Retire"}
                    </Button>
                  </div>
                  {term.description && (
                    <div className="mt-1 text-pretty text-caption text-muted-strong">
                      {term.description}
                    </div>
                  )}
                  <div className="mt-1.5 flex flex-wrap items-center gap-2.5 text-label text-muted">
                    <span className={cn("flex items-center gap-1.5", g.tone)}>
                      <span aria-hidden>{g.glyph}</span>
                      {term.trust}
                    </span>
                    <Num>{term.evidence} citations</Num>
                    {term.expired && <span className="text-status-watch">expired</span>}
                    {term.aliases && term.aliases.length > 0 && (
                      <span>aka {term.aliases.join(", ")}</span>
                    )}
                  </div>
                </div>
              )
            })}
          </div>
        )}
        {error && (
          <p className="mt-4 rounded-md border border-status-alert/40 px-3.5 py-2.5 text-body-sm text-status-alert">
            {error}
          </p>
        )}
        <p className="mt-4 text-pretty text-label text-muted">
          Merging two entries into one canonical also frees a slot, and often it is the better
          answer — that one is an edit, not a button.
        </p>
      </div>
    </Drawer>
  )
}
