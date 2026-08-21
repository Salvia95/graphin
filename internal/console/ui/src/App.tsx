import { useCallback, useEffect, useMemo, useState } from "react"
import { api, type DecisionKind, type Overview, type RepinResult, type UsageReport } from "@/api"
import { DecisionCard, kindLabel } from "@/DecisionCard"
import { Tiles } from "@/Tiles"
import { WikiMap } from "@/WikiMap"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import { Num } from "@/components/ui/field"
import { cn } from "@/lib/utils"

type Tab = "decisions" | "map"

function Chip({
  on,
  children,
  ...props
}: React.ComponentProps<"button"> & { on: boolean }) {
  return (
    <button
      className={cn(
        "rounded-sm border px-3 py-1.5 text-caption",
        on
          ? "border-hairline bg-elevated text-on-dark"
          : "border-hairline text-muted active:bg-surface",
      )}
      {...props}
    >
      {children}
    </button>
  )
}

export default function App() {
  const [o, setO] = useState<Overview | null>(null)
  const [u, setU] = useState<UsageReport | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [tab, setTab] = useState<Tab>("decisions")
  const [filter, setFilter] = useState<DecisionKind | null>(null)
  const [written, setWritten] = useState<string[]>([])
  const [repin, setRepin] = useState<RepinResult | null>(null)
  const [busy, setBusy] = useState(false)

  const reload = useCallback(() => {
    api.wiki().then(setO).catch((e: Error) => setError(e.message))
    // Adoption is context, not the work. A workspace with no usage log yet is an
    // ordinary state and must not blank the page the decisions are on.
    api.usage().then(setU).catch(() => setU(null))
  }, [])
  useEffect(reload, [reload])

  const counts = useMemo(() => {
    const c = new Map<DecisionKind, number>()
    for (const d of o?.decisions ?? []) c.set(d.kind, (c.get(d.kind) ?? 0) + 1)
    return c
  }, [o])

  if (error) {
    return (
      <main className="mx-auto max-w-6xl p-8">
        <p className="rounded-lg border border-status-alert/40 px-6 py-4 text-body-md text-status-alert">
          {error}
        </p>
      </main>
    )
  }
  if (!o) return <main className="p-8 text-body-md text-muted">읽는 중…</main>

  if (!o.present) {
    return (
      <main className="mx-auto max-w-2xl space-y-4 p-8">
        <h1 className="text-display-sm text-on-dark">
          <span className="text-primary">graphin</span> console
        </h1>
        <p className="text-body-md text-muted-strong">
          이 워크스페이스에는 <span className="num text-body">docs/wiki</span>가 없습니다. 지식
          계층이 없으면 결정할 것도 없고, 지식 게이트도 무장하지 않습니다.
        </p>
      </main>
    )
  }

  const shown = o.decisions.filter((d) => !filter || d.kind === filter)
  const drifted = counts.get("drift") ?? 0

  return (
    <div className="min-h-screen">
      {/* 64px bar, flat, no border — the canvas/surface step below does the
          separating. */}
      <header className="flex h-16 items-center justify-between gap-4 px-6 sm:px-10">
        <h1 className="text-title-md">
          <span className="text-primary">graphin</span>{" "}
          <span className="text-muted-strong">console</span>
        </h1>
        <nav className="flex gap-6">
          {(
            [
              ["decisions", "결정"],
              ["map", "위키 지도"],
            ] as [Tab, string][]
          ).map(([id, label]) => (
            <button
              key={id}
              onClick={() => setTab(id)}
              className={cn(
                "-mb-px border-b-2 py-2 text-body-md",
                tab === id
                  ? "border-primary text-on-dark"
                  : "border-transparent text-muted active:text-body",
              )}
            >
              {label}
              {id === "decisions" && o.health.decisions > 0 && (
                <>
                  {" "}
                  <Num className="text-muted">{o.health.decisions}</Num>
                </>
              )}
            </button>
          ))}
        </nav>
      </header>

      <main className="mx-auto max-w-6xl space-y-10 px-6 pb-20 sm:px-10">
        <Tiles o={o} u={u} />

        {written.length > 0 && (
          <Card>
            <CardContent className="space-y-2">
              <p className="text-title-sm text-on-dark">
                <Num>{written.length}</Num>개 파일이 워킹 트리에 쓰였습니다 — 아직 커밋되지
                않았습니다
              </p>
              {written.map((f) => (
                <p key={f} className="num text-body-sm text-muted">
                  {f}
                </p>
              ))}
              <p className="text-body-sm text-muted">
                <span className="num text-body">git diff</span>로 확인하고 직접 커밋하십시오.
              </p>
            </CardContent>
          </Card>
        )}

        {tab === "map" ? (
          <WikiMap o={o} />
        ) : (
          <div className="space-y-6">
            <div className="flex flex-wrap gap-2">
              <Chip on={filter === null} onClick={() => setFilter(null)}>
                전체 <Num>{o.decisions.length}</Num>
              </Chip>
              {[...counts.entries()].map(([k, n]) => (
                <Chip key={k} on={filter === k} onClick={() => setFilter(filter === k ? null : k)}>
                  {kindLabel[k]} <Num>{n}</Num>
                </Chip>
              ))}
            </div>

            {drifted > 0 && (!filter || filter === "drift") && (
              <Card>
                <CardContent className="space-y-4">
                  <p className="text-body-md text-body">
                    드리프트 <Num className="text-status-watch">{drifted}</Num>건.{" "}
                    <span className="text-on-dark">repin은 전부를 한꺼번에 다시 고정합니다</span> —
                    아래 절들을 먼저 읽고 각 요약이 여전히 맞는지 확인한 뒤에 누르십시오.
                  </p>
                  {repin && (
                    <p className="text-body-sm text-muted">
                      <Num>{repin.added}</Num> 추가 · <Num>{repin.updated}</Num> 갱신 ·{" "}
                      <Num>{repin.dropped}</Num> 삭제 →{" "}
                      <span className="num text-body">{repin.path}</span> (커밋 안 됨)
                    </p>
                  )}
                  <Button
                    size="sm"
                    variant="outline"
                    disabled={busy}
                    onClick={async () => {
                      setBusy(true)
                      try {
                        const r = await api.repin()
                        setRepin(r)
                        setWritten((w) => [...new Set([...w, r.path])])
                        reload()
                      } catch (e) {
                        setError(e instanceof Error ? e.message : String(e))
                      } finally {
                        setBusy(false)
                      }
                    }}
                  >
                    전체 repin
                  </Button>
                </CardContent>
              </Card>
            )}

            {shown.length === 0 ? (
              <p className="text-body-md text-muted">
                {filter ? "이 종류는 없습니다." : "결정할 것이 없습니다."}
              </p>
            ) : (
              <div className="space-y-3">
                {shown.map((d, i) => (
                  <DecisionCard
                    key={`${d.kind}-${d.title}-${i}`}
                    d={d}
                    onDone={(file) => {
                      if (file) setWritten((w) => [...new Set([...w, file])])
                      reload()
                    }}
                  />
                ))}
              </div>
            )}
          </div>
        )}
      </main>
    </div>
  )
}
