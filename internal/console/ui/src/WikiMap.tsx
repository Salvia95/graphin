import type { Overview, TermView } from "@/api"
import { Card, CardContent } from "@/components/ui/card"
import { Badge, Num } from "@/components/ui/field"
import { cn } from "@/lib/utils"

// Trust is derived from who reviewed an entry, never declared, so it is the one
// column worth colouring — everything else on a term is what its author typed.
const trustTone: Record<TermView["trust"], string> = {
  "human-reviewed": "border-status-good/40 text-status-good",
  "machine-confirmed": "border-info/40 text-info",
  unverified: "border-hairline text-muted",
}

function Head({ children }: { children: React.ReactNode }) {
  return <h2 className="text-caption tracking-wide text-muted uppercase">{children}</h2>
}

export function WikiMap({ o }: { o: Overview }) {
  return (
    <div className="space-y-10">
      <section className="space-y-4">
        <Head>
          세트 <Num>{o.sets.length}</Num> · 엔트리 <Num>{o.health.entries}</Num>
        </Head>
        <div className="space-y-3">
          {o.sets.map((s) => {
            // Offered-and-never-opened is the catalogue paying rent. Showing the
            // ratio rather than a flag is what lets someone see it coming.
            const ignored = s.offered >= 3 && s.opened === 0
            return (
              <Card key={s.name}>
                <CardContent className="space-y-2">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="num text-number-md text-on-dark">{s.name}</span>
                    <span className="text-body-md text-muted-strong">{s.title}</span>
                    <Badge>
                      <Num>{s.entries}</Num>&nbsp;엔트리
                    </Badge>
                    {s.mode !== "live" && <Badge>{s.mode}</Badge>}
                    {s.roles.map((r) => (
                      <Badge key={r}>{r}</Badge>
                    ))}
                    {s.dangling > 0 && (
                      <Badge className="border-status-alert/40 text-status-alert">
                        끊어짐&nbsp;<Num>{s.dangling}</Num>
                      </Badge>
                    )}
                    {s.drifted > 0 && (
                      <Badge className="border-status-watch/40 text-status-watch">
                        드리프트&nbsp;<Num>{s.drifted}</Num>
                      </Badge>
                    )}
                    {s.expired && (
                      <Badge className="border-status-watch/40 text-status-watch">만료</Badge>
                    )}
                  </div>
                  <p className="text-body-md text-muted-strong">{s.summary}</p>
                  <p className={cn("text-body-sm", ignored ? "text-status-watch" : "text-muted")}>
                    제시 <Num>{s.offered}</Num>회 · 열람 <Num>{s.opened}</Num>회
                    {ignored && " — 비용만 물리고 있다"}
                    {s.prerequisites.length > 0 && ` · 선행 ${s.prerequisites.join(", ")}`}
                  </p>
                </CardContent>
              </Card>
            )
          })}
        </div>
      </section>

      <section className="space-y-4">
        <Head>
          용어집 <Num>{o.terms.length}</Num> / <Num>{o.glossary.cap}</Num>
        </Head>
        {o.terms.length === 0 ? (
          <p className="text-body-md text-muted">
            아직 없음. 용어는 실작업에서 태어난다 — 위키가 답 못 한 작업 목록이 쓸 거리다.
          </p>
        ) : (
          <Card>
            <CardContent className="overflow-x-auto p-0">
              <table className="w-full text-body-md">
                <thead>
                  <tr className="text-left text-caption text-muted">
                    <th className="px-6 py-3">용어</th>
                    <th className="px-6 py-3">신뢰</th>
                    <th className="px-6 py-3">상태</th>
                    <th className="px-6 py-3">인용</th>
                    <th className="px-6 py-3">별칭</th>
                  </tr>
                </thead>
                <tbody>
                  {o.terms.map((t) => (
                    <tr key={t.canonical} className="border-t border-hairline">
                      <td className="num px-6 py-3 text-on-dark">{t.canonical}</td>
                      <td className="px-6 py-3">
                        <Badge className={trustTone[t.trust]}>{t.trust}</Badge>
                      </td>
                      <td className="px-6 py-3 text-muted">
                        {t.status}
                        {t.expired && <span className="text-status-watch"> · 만료</span>}
                      </td>
                      <td className="num px-6 py-3 text-muted">{t.evidence}</td>
                      <td className="px-6 py-3 text-muted">{(t.aliases ?? []).join(", ")}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </CardContent>
          </Card>
        )}
      </section>
    </div>
  )
}
