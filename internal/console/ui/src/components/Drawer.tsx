import { useEffect, useRef, type ReactNode } from "react"
import { useEscape } from "@/lib/useEscape"
import { cn } from "@/lib/utils"

const FOCUSABLE =
  'a[href],button:not([disabled]),input:not([disabled]),textarea:not([disabled]),select:not([disabled]),[tabindex]:not([tabindex="-1"])'

/** Both drawers are the same object: a scrim over the page and a panel against
 *  the right edge. Editing a candidate and reading a set are different tasks,
 *  but neither is a place — leaving one puts you back exactly where you were,
 *  which is why neither is a route.
 *
 *  Focus is trapped and then handed back. A modal that lets Tab walk out into
 *  the list behind it is a modal only for the mouse, and this is a keyboard
 *  tool on a dense page: the row you opened is the row you want to be on when
 *  it closes.
 */
export function Drawer({
  open,
  onClose,
  width,
  children,
}: {
  open: boolean
  onClose: () => void
  width: string
  children: ReactNode
}) {
  useEscape(open, onClose)
  const panel = useRef<HTMLDivElement>(null)
  const opener = useRef<HTMLElement | null>(null)

  useEffect(() => {
    if (!open) return
    opener.current = document.activeElement as HTMLElement | null
    panel.current?.focus()
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== "Tab" || !panel.current) return
      const items = [...panel.current.querySelectorAll<HTMLElement>(FOCUSABLE)]
      if (items.length === 0) return
      const first = items[0]
      const last = items[items.length - 1]
      const here = document.activeElement
      if (e.shiftKey && (here === first || here === panel.current)) {
        e.preventDefault()
        last.focus()
      } else if (!e.shiftKey && here === last) {
        e.preventDefault()
        first.focus()
      }
    }
    document.addEventListener("keydown", onKey)
    return () => {
      document.removeEventListener("keydown", onKey)
      opener.current?.focus()
    }
  }, [open])

  if (!open) return null
  return (
    <div className="fixed inset-0 z-40 flex justify-end">
      <div className="absolute inset-0 bg-canvas/70" onClick={onClose} />
      <div
        ref={panel}
        tabIndex={-1}
        role="dialog"
        aria-modal="true"
        className={cn(
          "relative flex h-full flex-col border-l border-hairline bg-surface outline-none",
          width,
        )}
      >
        {children}
      </div>
    </div>
  )
}

export function DrawerClose({ onClose }: { onClose: () => void }) {
  return (
    <button
      onClick={onClose}
      aria-label="Close"
      className="p-1 text-title-md leading-none text-muted transition-colors hover:text-body"
    >
      ✕
    </button>
  )
}
