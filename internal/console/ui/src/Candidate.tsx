import { useState } from "react"
import { api, type Approved, type QueuedProposal } from "@/api"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Badge, Input, Label, Textarea } from "@/components/ui/field"

const list = (s: string) =>
  s
    .split(",")
    .map((v) => v.trim())
    .filter(Boolean)

export function Candidate({
  p,
  onDone,
}: {
  p: QueuedProposal
  onDone: (result: Approved | null) => void
}) {
  const [title, setTitle] = useState("")
  const [description, setDescription] = useState("")
  const [tags, setTags] = useState("")
  const [body, setBody] = useState("")
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  async function run(fn: () => Promise<Approved | null>) {
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
    <Card>
      <CardHeader>
        <div className="flex items-center gap-2">
          <CardTitle className="font-mono">{p.canonical}</CardTitle>
          <Badge>{p.seen}× 제안됨</Badge>
          <Badge>{p.evidence.length} 인용</Badge>
        </div>
        <p className="font-mono text-xs text-slate-500">{p.evidence.join("  ·  ")}</p>
      </CardHeader>
      <CardContent className="space-y-3">
        <div className="grid gap-3 sm:grid-cols-2">
          <div className="space-y-1">
            <Label htmlFor={`${p.canonical}-title`}>title</Label>
            <Input
              id={`${p.canonical}-title`}
              value={title}
              placeholder="비우면 제안된 값 그대로"
              onChange={(e) => setTitle(e.target.value)}
            />
          </div>
          <div className="space-y-1">
            <Label htmlFor={`${p.canonical}-tags`}>tags (쉼표로 구분)</Label>
            <Input
              id={`${p.canonical}-tags`}
              value={tags}
              placeholder="editorial, release"
              onChange={(e) => setTags(e.target.value)}
            />
          </div>
        </div>
        <div className="space-y-1">
          <Label htmlFor={`${p.canonical}-desc`}>description</Label>
          <Input
            id={`${p.canonical}-desc`}
            value={description}
            placeholder="한 줄 정의"
            onChange={(e) => setDescription(e.target.value)}
          />
        </div>
        <div className="space-y-1">
          <Label htmlFor={`${p.canonical}-body`}>본문</Label>
          <Textarea
            id={`${p.canonical}-body`}
            value={body}
            placeholder="비우면 제안된 본문 그대로"
            onChange={(e) => setBody(e.target.value)}
          />
        </div>

        {error && (
          <p className="rounded-md border border-red-300 bg-red-50 px-3 py-2 text-sm text-red-700 dark:border-red-900 dark:bg-red-950 dark:text-red-400">
            {error}
          </p>
        )}

        <div className="flex gap-2">
          <Button
            disabled={busy}
            onClick={() =>
              run(() =>
                api.approve(p.canonical, {
                  title: title || undefined,
                  description: description || undefined,
                  body: body || undefined,
                  tags: tags ? list(tags) : undefined,
                }),
              )
            }
          >
            승인
          </Button>
          <Button
            variant="danger"
            disabled={busy}
            onClick={() =>
              run(async () => {
                await api.discard(p.canonical)
                return null
              })
            }
          >
            거절
          </Button>
        </div>
      </CardContent>
    </Card>
  )
}
