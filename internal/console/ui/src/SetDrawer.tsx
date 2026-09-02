import type { SetView } from "@/api"
import { Drawer, DrawerClose } from "@/components/Drawer"
import { Eyebrow } from "@/components/ui/field"
import { entryTone } from "@/Sets"
import { cn } from "@/lib/utils"

function Stat({ value, label }: { value: string | number; label: string }) {
  return (
    <div>
      <div className="num text-number-md text-body">{value}</div>
      <div className="mt-1 text-label text-muted">{label}</div>
    </div>
  )
}

/** A set opened up. The rail can only say "3 issues"; this is where a person
 *  finds out which three lines, which is the difference between knowing a set
 *  is unhealthy and being able to fix it. */
export function SetDrawer({ s, onClose }: { s: SetView | null; onClose: () => void }) {
  return (
    <Drawer open={s !== null} onClose={onClose} width="w-[520px]">
      {s && (
        <>
          <div className="border-b border-hairline px-6 pt-6 pb-5">
            <div className="flex items-center justify-between">
              <Eyebrow>Knowledge set</Eyebrow>
              <DrawerClose onClose={onClose} />
            </div>
            <div className="mt-3 flex flex-wrap items-baseline gap-2.5">
              <span className="num text-title-lg text-on-dark">{s.name}</span>
              <span className="rounded-sm border border-hairline px-1.5 py-0.5 text-tag tracking-tag text-muted uppercase">
                {s.mode}
              </span>
              {s.expired && (
                <span className="rounded-sm border border-status-watch/40 px-1.5 py-0.5 text-tag tracking-tag text-status-watch uppercase">
                  expired
                </span>
              )}
              {s.unreviewed && (
                // Same watch tone as drift: an agent's change is being served
                // and a person has not looked. Not an error, not a pass.
                <span className="rounded-sm border border-status-watch/40 px-1.5 py-0.5 text-tag tracking-tag text-status-watch uppercase">
                  unreviewed
                </span>
              )}
            </div>
            <div className="mt-1.5 text-pretty text-body-sm text-muted-strong">{s.title}</div>
            <div className="mt-5 grid grid-cols-[repeat(3,max-content)] gap-8">
              <Stat value={s.entries} label="entries" />
              <Stat value={`${s.opened} / ${s.offered}`} label="opened / presented" />
              <Stat value={s.roles.length > 0 ? s.roles.join(", ") : "—"} label="role" />
            </div>
            {s.prerequisites.length > 0 && (
              <div className="mt-4 text-label text-muted">
                requires {s.prerequisites.join(", ")}
              </div>
            )}
          </div>

          <div className="flex-1 overflow-auto px-6 pt-5 pb-6">
            <Eyebrow className="mb-3 block">Entries</Eyebrow>
            {s.items.length === 0 ? (
              <p className="text-caption text-muted">This set lists nothing yet.</p>
            ) : (
              // Hairlines are gaps in a filled stack rather than borders on each
              // row: one rule between two rows, never two stacked.
              <div className="flex flex-col gap-px overflow-hidden rounded-lg border border-hairline bg-elevated">
                {s.items.map((e, i) => (
                  <div key={`${e.node_id}-${i}`} className="bg-canvas px-3.5 py-3">
                    <div className="flex items-baseline justify-between gap-3">
                      <span className="num break-words text-caption text-body">{e.node_id}</span>
                      <span
                        className={cn(
                          "num shrink-0 text-tag tracking-tag whitespace-nowrap uppercase",
                          entryTone[e.status],
                        )}
                      >
                        {e.status}
                      </span>
                    </div>
                    <div className="mt-1 text-pretty text-caption text-muted-strong">
                      {e.summary || e.title}
                    </div>
                  </div>
                ))}
              </div>
            )}
            <p className="mt-3 text-label text-muted">
              <span className="num">{s.rel_path}</span>
            </p>
          </div>
        </>
      )}
    </Drawer>
  )
}
