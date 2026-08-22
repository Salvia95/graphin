import type { Overview } from "@/api"
import { LinkButton } from "@/components/ui/button"
import { Eyebrow, Num } from "@/components/ui/field"
import { SetCard, trustGlyph } from "@/Sets"
import { cn } from "@/lib/utils"

/** Everything the rail shows a slice of.
 *
 *  The map stopped being a peer of the queue and became the place you go when
 *  the slice is not enough — every set rather than the ones with problems, every
 *  term rather than the first six. Nothing here is a decision, which is exactly
 *  why it is one click away instead of one tab away.
 */
export function WikiMap({
  o,
  onOpenSet,
  onBack,
}: {
  o: Overview
  onOpenSet: (name: string) => void
  onBack: () => void
}) {
  return (
    <div>
      <div className="mb-5 flex items-baseline justify-between border-b border-hairline pb-3">
        <div className="flex items-baseline gap-3">
          <h2 className="text-title-sm text-on-dark">Wiki map</h2>
          <Num className="text-caption text-muted">
            {o.sets.length} sets · {o.health.entries} entries · {o.terms.length} terms
          </Num>
        </div>
        <LinkButton onClick={onBack}>← Decisions</LinkButton>
      </div>

      <Eyebrow className="mb-3 block">Knowledge sets</Eyebrow>
      {o.sets.length === 0 ? (
        <p className="text-body-md text-muted">No sets yet.</p>
      ) : (
        <div className="grid grid-cols-4 gap-3">
          {o.sets.map((s) => (
            <SetCard key={s.name} s={s} onOpen={() => onOpenSet(s.name)} />
          ))}
        </div>
      )}

      <div className="mt-8 mb-3 flex items-baseline gap-3">
        <Eyebrow>Glossary</Eyebrow>
        <Num className="text-label text-muted">
          {o.glossary.count} / {o.glossary.cap}
        </Num>
      </div>
      {o.terms.length === 0 ? (
        <p className="max-w-[70ch] text-pretty text-body-md text-muted">
          Empty. Terms are born from real work — the backlog is what to write them from, and an
          empty glossary means nothing has yet needed a name of its own.
        </p>
      ) : (
        <div className="flex flex-col gap-px overflow-hidden rounded-lg border border-hairline bg-elevated">
          {o.terms.map((t) => {
            const g = trustGlyph[t.trust]
            return (
              <div key={t.canonical} className="bg-canvas px-4 py-3">
                <div className="flex items-baseline justify-between gap-4">
                  <span className="num text-body-sm text-body">{t.canonical}</span>
                  <span className="flex shrink-0 items-center gap-3">
                    <span className={cn("flex items-center gap-1.5 text-label", g.tone)}>
                      <span aria-hidden>{g.glyph}</span>
                      {t.trust}
                    </span>
                    <Num className="text-label text-muted">{t.evidence}×</Num>
                  </span>
                </div>
                {t.description && (
                  <div className="mt-1 text-pretty text-caption text-muted-strong">
                    {t.description}
                  </div>
                )}
                <div className="mt-1.5 flex flex-wrap items-center gap-2.5 text-label text-muted">
                  <span>{t.status}</span>
                  {t.expired && <span className="text-status-watch">expired</span>}
                  {t.aliases && t.aliases.length > 0 && <span>aka {t.aliases.join(", ")}</span>}
                  <span className="num">{t.rel_path}</span>
                </div>
              </div>
            )
          })}
        </div>
      )}
    </div>
  )
}
