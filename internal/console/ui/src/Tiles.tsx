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
        label="위키 건강"
        tone={broken ? "alert" : soft ? "watch" : "brand"}
        value={broken ? h.dangling : soft ? h.drifted + h.expired : "정상"}
        note={
          broken
            ? `끊어진 링크 · 드리프트 ${h.drifted} · 만료 ${h.expired}`
            : soft
              ? `드리프트 ${h.drifted} · 만료 ${h.expired} · 끊어짐 없음`
              : `세트 ${h.sets} · 엔트리 ${h.entries}`
        }
      />
      <Tile
        label="결정 대기"
        tone={broken ? "alert" : h.decisions > 0 ? "brand" : "brand"}
        value={h.decisions}
        note={h.awaiting > 0 ? `승인 대기 ${h.awaiting}건 포함` : "승인 대기 없음"}
      />
      <Tile
        label="용어집"
        tone={pressure >= 1 ? "alert" : pressure >= 0.8 ? "watch" : "brand"}
        value={`${o.glossary.count}/${o.glossary.cap}`}
        note={
          pressure >= 1
            ? "찼다 — 밀어낼 것을 정해야 승인이 된다"
            : `여유 ${o.glossary.cap - o.glossary.count}`
        }
      />
      <Tile
        label="채택"
        tone={u ? "brand" : "watch"}
        value={u ? `${u.sessions_with_graphin}/${u.sessions}` : "—"}
        note={u ? `세션에서 사용 · 이벤트 ${u.events.toLocaleString()}` : "usage 로그 없음"}
      />
    </div>
  )
}
