import type { Overview } from "@/api"
import { Num } from "@/components/ui/field"

/** Zero decisions is the wiki working, not the screen failing to load.
 *
 *  The brief called this out directly (§6.3): an empty queue read as a blank
 *  page. So the empty state is the loudest thing on it — a green edge, a claim
 *  in plain words, and the four checks that were run to earn it. Showing the
 *  zeros matters more than the sentence: "0 broken links" is evidence, "nothing
 *  to do" is a shrug.
 */
export function HealthyPanel({ o, backlog }: { o: Overview; backlog: number }) {
  const h = o.health
  return (
    <div className="mb-6 rounded-xl border-l-4 border-l-status-good bg-surface px-6 pt-6 pb-5">
      <div className="mb-1.5 flex items-center gap-2.5">
        <span className="text-body-md text-status-good" aria-hidden>
          ●
        </span>
        <h2 className="text-title-sm text-on-dark">The wiki is healthy</h2>
      </div>
      <p className="mb-5 max-w-[60ch] text-pretty text-body-md text-muted-strong">
        Nothing is broken, drifting or waiting on you.{" "}
        {backlog > 0
          ? `${backlog} unanswered ${backlog === 1 ? "question sits" : "questions sit"} in the backlog — they cost nothing until someone hits them again.`
          : "The backlog is empty too."}
      </p>

      <div className="grid grid-cols-[repeat(4,max-content)] gap-8">
        {(
          [
            [h.dangling, "broken links"],
            [h.drifted, "drifted pins"],
            [h.expired, "expired"],
            [h.awaiting, "awaiting you"],
          ] as [number, string][]
        ).map(([n, label]) => (
          <div key={label}>
            <div className="num text-number-lg text-body">{n}</div>
            <div className="mt-1 text-label text-muted">{label}</div>
          </div>
        ))}
      </div>

      <div className="mt-5 border-t border-hairline pt-4 text-caption text-muted">
        <Num>{h.sets}</Num> sets · <Num>{h.entries}</Num> entries · glossary{" "}
        <Num>
          {o.glossary.count}/{o.glossary.cap}
        </Num>
      </div>
    </div>
  )
}
