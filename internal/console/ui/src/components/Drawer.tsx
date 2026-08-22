import type { ReactNode } from "react"
import { useEscape } from "@/lib/useEscape"
import { cn } from "@/lib/utils"

/** Both drawers are the same object: a scrim over the page and a panel against
 *  the right edge. Editing a candidate and reading a set are different tasks,
 *  but neither is a place — leaving one puts you back exactly where you were,
 *  which is why neither is a route. */
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
  if (!open) return null
  return (
    <div className="fixed inset-0 z-40 flex justify-end">
      <div className="absolute inset-0 bg-canvas/70" onClick={onClose} />
      <div
        role="dialog"
        aria-modal="true"
        className={cn("relative flex h-full flex-col border-l border-hairline bg-surface", width)}
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
