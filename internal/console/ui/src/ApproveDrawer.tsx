import { useEffect, useState } from "react"
import { api, type Candidate, type Edits } from "@/api"
import { Drawer, DrawerClose } from "@/components/Drawer"
import { Button } from "@/components/ui/button"
import { Input, Label, Textarea } from "@/components/ui/field"
import { TIER } from "@/lib/tiers"
import { cn } from "@/lib/utils"

function Section({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div>
      <Label className="mb-2">{label}</Label>
      {children}
    </div>
  )
}

/** Aliases are the one list a reviewer really does edit: the proposer sees the
 *  word in code, the reviewer knows the two other things the team calls it. */
function Aliases({ value, onChange }: { value: string[]; onChange: (v: string[]) => void }) {
  const [draft, setDraft] = useState("")
  const [adding, setAdding] = useState(false)

  const add = () => {
    const v = draft.trim()
    if (v && !value.includes(v)) onChange([...value, v])
    setDraft("")
    setAdding(false)
  }

  return (
    <div className="flex flex-wrap items-center gap-1.5">
      {value.map((a) => (
        <span
          key={a}
          className="num flex items-center gap-2 rounded-sm bg-elevated px-2.5 py-1.5 text-caption text-body"
        >
          {a}
          <button
            aria-label={`Remove ${a}`}
            className="text-muted transition-colors hover:text-status-alert"
            onClick={() => onChange(value.filter((x) => x !== a))}
          >
            ✕
          </button>
        </span>
      ))}
      {adding ? (
        <input
          autoFocus
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          onBlur={add}
          onKeyDown={(e) => {
            if (e.key === "Enter") add()
            if (e.key === "Escape") {
              setDraft("")
              setAdding(false)
            }
          }}
          className="num h-8 rounded-sm border border-info bg-canvas px-2.5 text-caption text-body outline-none"
        />
      ) : (
        <Button variant="dashed" size="xs" onClick={() => setAdding(true)}>
          + add
        </Button>
      )}
    </div>
  )
}

/** The one card of the eight that has a form, moved out of the list.
 *
 *  It used to expand inline, which pushed every decision below it down the page
 *  while someone read a definition — the brief asked for a way to focus on the
 *  edit without breaking the queue (§6.4). A drawer is that: the list keeps its
 *  position, and closing returns to exactly the row you left.
 */
export function ApproveDrawer({
  canonical,
  glossaryFull,
  onClose,
  onDone,
}: {
  canonical: string | null
  glossaryFull: boolean
  onClose: () => void
  onDone: (file?: string) => void
}) {
  const [c, setC] = useState<Candidate | null>(null)
  const [body, setBody] = useState("")
  const [title, setTitle] = useState("")
  const [description, setDescription] = useState("")
  const [tags, setTags] = useState("")
  const [aliases, setAliases] = useState<string[]>([])
  const [more, setMore] = useState(false)
  const [confirmReject, setConfirmReject] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (!canonical) return
    setC(null)
    setError(null)
    setMore(false)
    setConfirmReject(false)
    let live = true
    api
      .candidate(canonical)
      .then((v) => {
        if (!live) return
        setC(v)
        setBody(v.body ?? "")
        setTitle(v.title ?? "")
        setDescription(v.description ?? "")
        setTags((v.tags ?? []).join(", "))
        setAliases(v.aliases ?? [])
      })
      .catch((e: Error) => live && setError(e.message))
    return () => {
      live = false
    }
  }, [canonical])

  async function run(fn: () => Promise<string | undefined>) {
    setBusy(true)
    setError(null)
    try {
      const file = await fn()
      onDone(file)
      onClose()
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(false)
    }
  }

  const t = TIER.info

  return (
    <Drawer open={canonical !== null} onClose={onClose} width="w-[440px]">
      <div className="border-b border-hairline px-6 pt-6 pb-4">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            <span className={cn("text-body-md leading-none", t.text)} aria-hidden>
              {t.glyph}
            </span>
            <span className={cn("num text-tag tracking-tag uppercase", t.text)}>approve</span>
          </div>
          <DrawerClose onClose={onClose} />
        </div>
        <h2 className="mt-3 text-title-md text-on-dark">Term candidate</h2>
        <p className="mt-1 text-caption text-muted-strong">
          Proposed by the indexer. Nothing is published until you approve it, and approving writes
          a file — it does not commit one.
        </p>
      </div>

      <div className="flex flex-1 flex-col gap-5 overflow-auto p-6">
        {!c && !error && <p className="text-body-sm text-muted">Loading candidate…</p>}

        {c && (
          <>
            <Section label="Canonical">
              <Input value={c.canonical} readOnly className="num" />
              <p className="mt-1.5 text-label text-muted">
                Identity, so it is fixed — a different word is a different term.
              </p>
            </Section>

            <Section label="Definition">
              <Textarea rows={5} value={body} onChange={(e) => setBody(e.target.value)} />
            </Section>

            <Section label="Aliases">
              <Aliases value={aliases} onChange={setAliases} />
            </Section>

            <Section label="Trust">
              <div className="flex items-center gap-2.5 rounded-md border border-hairline bg-canvas px-3.5 py-3">
                <span className="text-status-good" aria-hidden>
                  ✓
                </span>
                <div>
                  <div className="text-body-sm text-body">human-reviewed</div>
                  <div className="text-label text-muted">
                    Derived from this approval — not a field you set.
                  </div>
                </div>
              </div>
            </Section>

            <Section label={`Evidence · ${c.evidence?.length ?? 0} citations`}>
              {c.evidence && c.evidence.length > 0 ? (
                <div className="flex flex-col gap-px overflow-hidden rounded-md border border-hairline bg-elevated">
                  {c.evidence.map((e) => (
                    <div
                      key={e}
                      className="num bg-canvas px-3 py-2.5 text-caption break-words text-muted-strong"
                    >
                      {e}
                    </div>
                  ))}
                </div>
              ) : (
                <p className="text-caption text-muted">None recorded.</p>
              )}
            </Section>

            {/* The rest of the frontmatter, folded away. A reviewer's job here is
                to judge a definition, not to fill a form — but the fields exist
                in the file, and hiding them entirely would mean approving is the
                one moment they cannot be corrected. */}
            {more ? (
              <>
                <Section label="Title">
                  <Input
                    value={title}
                    placeholder="leave empty to keep as proposed"
                    onChange={(e) => setTitle(e.target.value)}
                  />
                </Section>
                <Section label="Summary">
                  <Input
                    value={description}
                    placeholder="one line, shown in catalogues"
                    onChange={(e) => setDescription(e.target.value)}
                  />
                </Section>
                <Section label="Tags">
                  <Input
                    value={tags}
                    placeholder="comma separated"
                    onChange={(e) => setTags(e.target.value)}
                  />
                </Section>
              </>
            ) : (
              <button
                onClick={() => setMore(true)}
                className="self-start text-caption text-muted transition-colors hover:text-body"
              >
                + title, summary, tags
              </button>
            )}

            {glossaryFull && (
              <div className="rounded-r-md border-l-[3px] border-l-status-alert bg-canvas px-3.5 py-3">
                <div className="text-caption text-body">Glossary is full</div>
                <div className="mt-1 text-pretty text-caption text-muted-strong">
                  Approving this requires retiring a term first — deleting its file under
                  <span className="num"> docs/wiki/glossary/</span>. Which one matters less is a
                  judgement, so nothing here makes it for you.
                </div>
              </div>
            )}
          </>
        )}

        {error && (
          <p className="rounded-md border border-status-alert/40 px-3.5 py-2.5 text-body-sm text-status-alert">
            {error}
          </p>
        )}
      </div>

      <div className="flex justify-end gap-2 border-t border-hairline px-6 py-4">
        <Button
          variant={confirmReject ? "danger" : "outline"}
          disabled={busy || !c}
          onClick={() => {
            if (!confirmReject) {
              setConfirmReject(true)
              return
            }
            run(async () => {
              await api.discard(c!.canonical)
              return undefined
            })
          }}
        >
          {confirmReject ? "Confirm reject" : "Reject"}
        </Button>
        <Button
          className="px-6"
          disabled={busy || !c || glossaryFull}
          onClick={() =>
            run(async () => {
              const edits: Edits = {
                body: body || undefined,
                title: title || undefined,
                description: description || undefined,
                tags: tags ? tags.split(",").map((v) => v.trim()).filter(Boolean) : undefined,
                aliases: aliases.length > 0 ? aliases : undefined,
              }
              const r = await api.approve(c!.canonical, edits)
              return r.file
            })
          }
        >
          Approve term
        </Button>
      </div>
    </Drawer>
  )
}
