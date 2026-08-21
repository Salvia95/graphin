import type { Overview, UsageReport } from "@/api"
import { Tile } from "@/components/ui/tile"

export function Tiles({ o, u }: { o: Overview; u: UsageReport | null }) {
  const h = o.health
  const broken = h.dangling > 0
  const soft = h.drifted + h.expired > 0
  const pressure = o.glossary.count / o.glossary.cap

  return (
    <div className="grid grid-cols-2 gap-6 lg:grid-cols-4">
      <Tile
        label="Wiki health"
        tone={broken ? "alert" : soft ? "watch" : "brand"}
        value={broken ? h.dangling : soft ? h.drifted + h.expired : "OK"}
        note={
          broken
            ? `broken links · ${h.drifted} drifted · ${h.expired} expired`
            : soft
              ? `${h.drifted} drifted · ${h.expired} expired · no broken links`
              : `${h.sets} sets · ${h.entries} entries`
        }
      />
      <Tile
        label="Decisions"
        tone={broken ? "alert" : h.decisions > 0 ? "brand" : "brand"}
        value={h.decisions}
        note={h.awaiting > 0 ? `includes ${h.awaiting} awaiting approval` : "nothing awaiting approval"}
      />
      <Tile
        label="Glossary"
        tone={pressure >= 1 ? "alert" : pressure >= 0.8 ? "watch" : "brand"}
        value={`${o.glossary.count}/${o.glossary.cap}`}
        note={
          pressure >= 1
            ? "full — displace an entry to approve"
            : `${o.glossary.cap - o.glossary.count} free`
        }
      />
      <Tile
        label="Adoption"
        tone={u ? "brand" : "watch"}
        value={u ? `${u.sessions_with_graphin}/${u.sessions}` : "—"}
        note={u ? `sessions using graphin · ${u.events.toLocaleString()} events` : "no usage log"}
      />
    </div>
  )
}
