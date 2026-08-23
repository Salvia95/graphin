import { useState } from "react"
import { Button } from "@/components/ui/button"

/** A short editable list — aliases on a term, roles on a set.
 *
 *  Both are lists of identifiers the reader types rarely and removes often, so
 *  the affordance is a chip with an ✕ and one dashed slot at the end. The
 *  dashed border is what says "empty", the same way it says it on a collapsed
 *  group. */
export function ChipEditor({
  value,
  onChange,
  placeholder,
}: {
  value: string[]
  onChange: (v: string[]) => void
  placeholder?: string
}) {
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
          placeholder={placeholder}
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
