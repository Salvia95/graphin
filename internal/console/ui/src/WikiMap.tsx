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
          <Num>{o.sets.length}</Num> sets · <Num>{o.health.entries}</Num> entries
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
                      <Num>{s.entries}</Num>&nbsp;entries
                    </Badge>
                    {s.mode !== "live" && <Badge>{s.mode}</Badge>}
                    {s.roles.map((r) => (
                      <Badge key={r}>{r}</Badge>
                    ))}
                    {s.dangling > 0 && (
                      <Badge className="border-status-alert/40 text-status-alert">
                        <Num>{s.dangling}</Num>&nbsp;broken
                      </Badge>
                    )}
                    {s.drifted > 0 && (
                      <Badge className="border-status-watch/40 text-status-watch">
                        <Num>{s.drifted}</Num>&nbsp;drifted
                      </Badge>
                    )}
                    {s.expired && (
                      <Badge className="border-status-watch/40 text-status-watch">expired</Badge>
                    )}
                  </div>
                  <p className="text-body-md text-muted-strong">{s.summary}</p>
                  <p className={cn("text-body-sm", ignored ? "text-status-watch" : "text-muted")}>
                    offered <Num>{s.offered}</Num> · opened <Num>{s.opened}</Num>
                    {ignored && " — paying rent"}
                    {s.prerequisites.length > 0 && ` · requires ${s.prerequisites.join(", ")}`}
                  </p>
                </CardContent>
              </Card>
            )
          })}
        </div>
      </section>

      <section className="space-y-4">
        <Head>
          Glossary <Num>{o.terms.length}</Num> / <Num>{o.glossary.cap}</Num>
        </Head>
        {o.terms.length === 0 ? (
          <p className="text-body-md text-muted">
            Empty. Terms are born from real work — the uncovered list is what to write from.
          </p>
        ) : (
          <Card>
            <CardContent className="overflow-x-auto p-0">
              <table className="w-full text-body-md">
                <thead>
                  <tr className="text-left text-caption text-muted">
                    <th className="px-6 py-3">term</th>
                    <th className="px-6 py-3">trust</th>
                    <th className="px-6 py-3">status</th>
                    <th className="px-6 py-3">citations</th>
                    <th className="px-6 py-3">aliases</th>
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
                        {t.expired && <span className="text-status-watch"> · expired</span>}
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
