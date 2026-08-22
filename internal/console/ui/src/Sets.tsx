import type { EntryStatus, SetView, TermView, Trust } from "@/api"
import { LinkButton } from "@/components/ui/button"
import { Eyebrow, Num } from "@/components/ui/field"
import { cn } from "@/lib/utils"

/** The one thing worth colouring on a set row. A set with a broken entry is
 *  wrong now; one that is merely unopened is only expensive. */
function issueOf(s: SetView): { text: string; tone: string } | null {
  if (s.dangling > 0)
    return { text: `${s.dangling} broken`, tone: "text-status-alert" }
  if (s.drifted > 0) return { text: `${s.drifted} drifted`, tone: "text-status-watch" }
  if (s.expired) return { text: "expired", tone: "text-status-watch" }
  // Offered-and-never-opened is the catalogue paying rent. Three is where one
  // unlucky match stops being an explanation.
  if (s.offered >= 3 && s.opened === 0) return { text: "unread", tone: "text-status-watch" }
  return null
}

const trustGlyph: Record<Trust, { glyph: string; tone: string }> = {
  "human-reviewed": { glyph: "✓", tone: "text-status-good" },
  "machine-confirmed": { glyph: "◆", tone: "text-info" },
  unverified: { glyph: "○", tone: "text-muted" },
}

export const entryTone: Record<EntryStatus, string> = {
  ok: "text-muted",
  drift: "text-status-watch",
  dangling: "text-status-alert",
}

function SetRow({ s, onOpen }: { s: SetView; onOpen: () => void }) {
  const issue = issueOf(s)
  return (
    <button
      onClick={onOpen}
      className="rounded-lg border border-hairline p-3 text-left transition-colors hover:border-hairline-strong hover:bg-elevated"
    >
      <div className="flex items-baseline justify-between gap-2">
        <span className="num text-body-sm text-body">{s.name}</span>
        <span className="flex shrink-0 items-center gap-1.5">
          <span className="text-tag tracking-tag text-muted uppercase">{s.mode}</span>
          <span className="text-label text-muted">›</span>
        </span>
      </div>
      <div className="mt-0.5 text-pretty text-caption text-muted-strong">{s.title}</div>
      <div className="num mt-2 flex flex-wrap items-center gap-2.5 text-label text-muted">
        <span>{s.entries} entries</span>
        <span className={issue ? "text-status-watch" : undefined}>
          {s.opened} / {s.offered} read
        </span>
        {issue && <span className={issue.tone}>{issue.text}</span>}
      </div>
    </button>
  )
}

function SetCard({ s, onOpen }: { s: SetView; onOpen: () => void }) {
  const issue = issueOf(s)
  return (
    <button
      onClick={onOpen}
      className="rounded-lg border border-hairline bg-surface p-4 text-left transition-colors hover:border-hairline-strong hover:bg-elevated"
    >
      <div className="flex items-baseline justify-between gap-2">
        <span className="num text-body-sm text-body">{s.name}</span>
        <span className="shrink-0 text-label text-muted">›</span>
      </div>
      <div className="mt-1.5 min-h-9 text-pretty text-caption text-muted-strong">{s.title}</div>
      <div className="num mt-2.5 flex flex-wrap items-center gap-2.5 text-label text-muted">
        <span>{s.entries} entries</span>
        <span>
          {s.opened} / {s.offered} read
        </span>
        {issue && <span className={issue.tone}>{issue.text}</span>}
      </div>
    </button>
  )
}

/** The glossary as a pressure gauge. The cap is a decision point, not a limit
 *  to raise, so the bar is there to be seen filling up long before it blocks an
 *  approval. */
function GlossaryPanel({
  terms,
  count,
  cap,
  limit,
  onMore,
}: {
  terms: TermView[]
  count: number
  cap: number
  limit: number
  onMore: () => void
}) {
  const pressure = cap > 0 ? Math.min(1, count / cap) : 0
  const fill =
    pressure >= 1 ? "bg-status-alert" : pressure >= 0.8 ? "bg-status-watch" : "bg-muted-strong"
  const shown = terms.slice(0, limit)

  return (
    <div className="rounded-xl bg-surface p-5">
      <div className="mb-1.5 flex items-baseline justify-between">
        <Eyebrow>Glossary</Eyebrow>
        <Num className="text-caption text-muted">
          {count}/{cap}
        </Num>
      </div>
      <div className="my-2.5 mb-4 h-1 overflow-hidden rounded-xs bg-elevated">
        <div className={cn("h-full", fill)} style={{ width: `${pressure * 100}%` }} />
      </div>

      {terms.length === 0 ? (
        <p className="text-pretty text-caption text-muted">
          No terms yet. The glossary fills as candidates are approved — an empty one is not a gap,
          it just means nothing has needed a name.
        </p>
      ) : (
        <div className="flex flex-col gap-3">
          {shown.map((t) => {
            const g = trustGlyph[t.trust]
            return (
              <div key={t.canonical}>
                <div className="flex items-baseline justify-between gap-2">
                  <span className="num text-body-sm text-body">{t.canonical}</span>
                  <Num className="shrink-0 text-label text-muted">{t.evidence}×</Num>
                </div>
                <div className="mt-1.5 flex items-center gap-1.5">
                  <span className={cn("text-label", g.tone)} aria-hidden>
                    {g.glyph}
                  </span>
                  <span className="text-label text-muted-strong">{t.trust}</span>
                  {t.expired && <span className="text-label text-status-watch">· expired</span>}
                </div>
                {t.aliases && t.aliases.length > 0 && (
                  <div className="mt-1 text-label text-muted">aka {t.aliases.join(", ")}</div>
                )}
              </div>
            )
          })}
          {terms.length > shown.length && (
            <LinkButton className="text-left" onClick={onMore}>
              {terms.length - shown.length} more in the wiki map
            </LinkButton>
          )}
        </div>
      )}
    </div>
  )
}

/** The rail. Sets stop being a peer tab and become context beside the queue —
 *  the brief asked whether the two tabs were really equals (§6.5), and they are
 *  not: nobody opens this console to browse a catalogue. */
export function SetsRail({
  sets,
  terms,
  count,
  cap,
  onOpenSet,
  onOpenMap,
}: {
  sets: SetView[]
  terms: TermView[]
  count: number
  cap: number
  onOpenSet: (name: string) => void
  onOpenMap: () => void
}) {
  return (
    <div className="sticky top-22 flex max-h-[calc(100vh-6rem)] flex-col gap-4 overflow-y-auto">
      <div className="rounded-xl bg-surface p-5">
        <div className="mb-4 flex items-baseline justify-between">
          <Eyebrow>Knowledge sets</Eyebrow>
          <LinkButton onClick={onOpenMap}>Wiki map</LinkButton>
        </div>
        {sets.length === 0 ? (
          <p className="text-caption text-muted">No sets yet.</p>
        ) : (
          <div className="flex flex-col gap-2">
            {sets.map((s) => (
              <SetRow key={s.name} s={s} onOpen={() => onOpenSet(s.name)} />
            ))}
          </div>
        )}
      </div>

      <GlossaryPanel
        terms={terms}
        count={count}
        cap={cap}
        limit={6}
        onMore={onOpenMap}
      />
    </div>
  )
}

/** With no queue there is nothing for the rail to sit beside, so the catalogue
 *  takes the width it was never worth taking from a decision. */
export function SetsGrid({
  sets,
  glossaryEmpty,
  onOpenSet,
  onOpenMap,
}: {
  sets: SetView[]
  glossaryEmpty: boolean
  onOpenSet: (name: string) => void
  onOpenMap: () => void
}) {
  return (
    <div className="mt-2 border-t border-hairline pt-6">
      <div className="mb-4 flex items-baseline justify-between">
        <div className="flex items-baseline gap-3">
          <Eyebrow>Knowledge sets</Eyebrow>
          <Num className="text-label text-muted">{sets.length} sets</Num>
        </div>
        <LinkButton onClick={onOpenMap}>Wiki map</LinkButton>
      </div>
      <div className="grid grid-cols-4 gap-3">
        {sets.map((s) => (
          <SetCard key={s.name} s={s} onOpen={() => onOpenSet(s.name)} />
        ))}
      </div>
      {glossaryEmpty && (
        <p className="mt-3 text-caption text-muted">
          Glossary is empty — the wiki has not needed a named term yet.
        </p>
      )}
    </div>
  )
}

export { SetCard, issueOf, trustGlyph }
