import { useEffect, useState } from "react"
import { api, type SetView } from "@/api"
import { Drawer, DrawerClose } from "@/components/Drawer"
import { Button } from "@/components/ui/button"
import { ChipEditor } from "@/components/ui/chips"
import { Label, Num, Textarea } from "@/components/ui/field"
import { TIER } from "@/lib/tiers"
import { cn } from "@/lib/utils"

/** "Demote it or delete it" was an instruction with nothing to press.
 *
 *  Two fields, because the unread-set decision has exactly two honest answers.
 *  The summary is the catalogue line — the one sentence that decides whether
 *  anyone opens the set, and the only thing that can make an ignored set worth
 *  its place. The roles are the push list; emptying it is what "demote" means
 *  here, and it does not hide the set, it stops charging every delegation for
 *  a line nobody reads.
 *
 *  Deleting is not offered. It is the one answer that cannot be undone from a
 *  diff on an uncommitted file, and the card already hands over the path.
 */
export function SetEditDrawer({
  set,
  onClose,
  onDone,
}: {
  set: SetView | null
  onClose: () => void
  onDone: (file: string) => void
}) {
  const [summary, setSummary] = useState("")
  const [roles, setRoles] = useState<string[]>([])
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (!set) return
    setSummary(set.summary)
    setRoles(set.roles)
    setError(null)
  }, [set])

  const t = TIER.neutral

  return (
    <Drawer open={set !== null} onClose={onClose} width="w-[460px]">
      {set && (
        <>
          <div className="border-b border-hairline px-6 pt-6 pb-4">
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-2">
                <span className={cn("text-body-md leading-none", t.text)} aria-hidden>
                  {t.glyph}
                </span>
                <span className={cn("num text-tag tracking-tag uppercase", t.text)}>
                  unread set
                </span>
              </div>
              <DrawerClose onClose={onClose} />
            </div>
            <h2 className="num mt-3 text-title-md text-on-dark">{set.name}</h2>
            <p className="mt-1 text-pretty text-caption text-muted-strong">
              Offered <Num>{set.offered}</Num> times, opened <Num>{set.opened}</Num>. A catalogue
              line costs every delegation whether or not anyone follows it.
            </p>
          </div>

          <div className="flex flex-1 flex-col gap-5 overflow-auto p-6">
            <div>
              <Label className="mb-2">Summary</Label>
              <Textarea
                rows={4}
                value={summary}
                onChange={(e) => setSummary(e.target.value)}
              />
              <p className="mt-1.5 text-pretty text-label text-muted">
                Saved as <span className="num">description:</span> in the set's frontmatter. This is
                the whole of what a reader sees before deciding to open it.
              </p>
            </div>

            <div>
              <Label className="mb-2">Roles</Label>
              <ChipEditor value={roles} onChange={setRoles} placeholder="backend" />
              <p className="mt-1.5 text-pretty text-label text-muted">
                Every role here gets this set pushed into every delegation, unasked. Emptying it
                demotes the set to task matching — still reachable, no longer charged for.
              </p>
            </div>

            <p className="text-pretty text-label text-muted">
              Only these two lines are rewritten. Everything else in{" "}
              <span className="num">{set.rel_path}</span> is left byte for byte, so the diff stays
              reviewable.
            </p>

            {error && (
              <p className="rounded-md border border-status-alert/40 px-3.5 py-2.5 text-body-sm text-status-alert">
                {error}
              </p>
            )}
          </div>

          <div className="flex justify-end gap-2 border-t border-hairline px-6 py-4">
            <Button variant="outline" onClick={onClose}>
              Cancel
            </Button>
            <Button
              className="px-6"
              disabled={busy}
              onClick={() => {
                setBusy(true)
                setError(null)
                api
                  .editSet(set.name, { description: summary, roles })
                  .then((r) => {
                    onDone(r.file)
                    onClose()
                  })
                  .catch((e: Error) => setError(e.message))
                  .finally(() => setBusy(false))
              }}
            >
              Save
            </Button>
          </div>
        </>
      )}
    </Drawer>
  )
}
