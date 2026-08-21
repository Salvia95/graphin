import { useState } from "react"
import { api, type Decision, type DecisionKind } from "@/api"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import { Badge, Input, Label, Num, Textarea } from "@/components/ui/field"
import { cn } from "@/lib/utils"

export const kindLabel: Record<DecisionKind, string> = {
  dangling: "Broken link",
  glossary_full: "Glossary full",
  expired: "Expired",
  drift: "Drift",
  approve: "Awaiting approval",
  stale_skill: "Stale skill",
  unread_set: "Unread set",
  uncovered: "Uncovered work",
}

// The stripe is the only thing scanned when there are thirty of these, and it
// carries the same claim the severity order does: alert costs a reader
// something now, watch costs it later, info is waiting on you, muted is a note.
// Yellow is absent on purpose — it belongs to actions, not to severity.
const stripe: Record<DecisionKind, string> = {
  dangling: "bg-status-alert",
  glossary_full: "bg-status-alert",
  expired: "bg-status-watch",
  drift: "bg-status-watch",
  stale_skill: "bg-status-watch",
  approve: "bg-info",
  unread_set: "bg-muted",
  uncovered: "bg-muted",
}

function Shell({ d, children }: { d: Decision; children?: React.ReactNode }) {
  return (
    <Card className="relative overflow-hidden">
      <span className={cn("absolute inset-y-0 left-0 w-1", stripe[d.kind])} />
      <CardContent className="space-y-3 pl-8">
        <div className="flex flex-wrap items-center gap-2">
          <Badge>{kindLabel[d.kind]}</Badge>
          {d.set && <Badge>{d.set}</Badge>}
          {d.role && <Badge>{d.role}</Badge>}
          {typeof d.count === "number" && d.count > 1 && (
            <Badge>
              <Num>{d.count}</Num>&nbsp;×
            </Badge>
          )}
        </div>
        <p className="num text-number-md break-all text-on-dark">{d.title}</p>
        <p className="text-body-md text-muted-strong">{d.detail}</p>
        {children ?? (
          <p className="text-body-sm text-muted">
            <span className="text-body">Next ·</span> {d.action}
          </p>
        )}
      </CardContent>
    </Card>
  )
}

const list = (s: string) =>
  s
    .split(",")
    .map((v) => v.trim())
    .filter(Boolean)

function ApproveForm({ d, onDone }: { d: Decision; onDone: (file?: string) => void }) {
  const [title, setTitle] = useState("")
  const [description, setDescription] = useState("")
  const [tags, setTags] = useState("")
  const [body, setBody] = useState("")
  const [open, setOpen] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const canonical = d.canonical ?? d.title

  async function run(fn: () => Promise<string | undefined>) {
    setBusy(true)
    setError(null)
    try {
      onDone(await fn())
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="space-y-4">
      {d.evidence && d.evidence.length > 0 && (
        <p className="num text-caption text-muted">{d.evidence.join("  ·  ")}</p>
      )}
      {open && (
        <div className="space-y-4 rounded-lg bg-elevated p-4">
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="space-y-1.5">
              <Label htmlFor={`${canonical}-t`}>title</Label>
              <Input
                id={`${canonical}-t`}
                value={title}
                placeholder="leave empty to keep as proposed"
                onChange={(e) => setTitle(e.target.value)}
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor={`${canonical}-g`}>tags (comma separated)</Label>
              <Input
                id={`${canonical}-g`}
                value={tags}
                placeholder="editorial, release"
                onChange={(e) => setTags(e.target.value)}
              />
            </div>
          </div>
          <div className="space-y-1.5">
            <Label htmlFor={`${canonical}-d`}>description</Label>
            <Input
              id={`${canonical}-d`}
              value={description}
              placeholder="one-line definition"
              onChange={(e) => setDescription(e.target.value)}
            />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor={`${canonical}-b`}>body</Label>
            <Textarea
              id={`${canonical}-b`}
              value={body}
              placeholder="leave empty to keep the proposed body"
              onChange={(e) => setBody(e.target.value)}
            />
          </div>
        </div>
      )}
      {error && (
        <p className="rounded-md border border-status-alert/40 px-4 py-2 text-body-sm text-status-alert">
          {error}
        </p>
      )}
      <div className="flex flex-wrap gap-3">
        <Button
          size="sm"
          disabled={busy}
          onClick={() =>
            run(async () => {
              const r = await api.approve(canonical, {
                title: title || undefined,
                description: description || undefined,
                body: body || undefined,
                tags: tags ? list(tags) : undefined,
              })
              return r.file
            })
          }
        >
          Approve
        </Button>
        <Button size="sm" variant="outline" onClick={() => setOpen((v) => !v)}>
          {open ? "Close" : "Edit"}
        </Button>
        <Button
          size="sm"
          variant="danger"
          disabled={busy}
          onClick={() =>
            run(async () => {
              await api.discard(canonical)
              return undefined
            })
          }
        >
          Discard
        </Button>
      </div>
    </div>
  )
}

export function DecisionCard({ d, onDone }: { d: Decision; onDone: (file?: string) => void }) {
  if (d.kind === "approve") {
    return (
      <Shell d={d}>
        <ApproveForm d={d} onDone={onDone} />
      </Shell>
    )
  }
  return <Shell d={d} />
}
