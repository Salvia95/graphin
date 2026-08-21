import { useState } from "react"
import { api, type Decision, type DecisionKind } from "@/api"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import { Badge, Input, Label, Num, Textarea } from "@/components/ui/field"
import { cn } from "@/lib/utils"

export const kindLabel: Record<DecisionKind, string> = {
  dangling: "끊어진 링크",
  glossary_full: "용어집 포화",
  expired: "만료",
  drift: "드리프트",
  approve: "승인 대기",
  stale_skill: "낡은 스킬",
  unread_set: "안 읽히는 세트",
  uncovered: "답 없던 작업",
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
              <Num>{d.count}</Num>회
            </Badge>
          )}
        </div>
        <p className="num text-number-md break-all text-on-dark">{d.title}</p>
        <p className="text-body-md text-muted-strong">{d.detail}</p>
        {children ?? (
          <p className="text-body-sm text-muted">
            <span className="text-body">할 일 ·</span> {d.action}
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
                placeholder="비우면 제안된 값 그대로"
                onChange={(e) => setTitle(e.target.value)}
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor={`${canonical}-g`}>tags (쉼표)</Label>
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
              placeholder="한 줄 정의"
              onChange={(e) => setDescription(e.target.value)}
            />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor={`${canonical}-b`}>본문</Label>
            <Textarea
              id={`${canonical}-b`}
              value={body}
              placeholder="비우면 제안된 본문 그대로"
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
          승인
        </Button>
        <Button size="sm" variant="outline" onClick={() => setOpen((v) => !v)}>
          {open ? "편집 닫기" : "편집"}
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
          거절
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
