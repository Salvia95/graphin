import { useEffect } from "react"

/** Escape closes whatever is on top. Both drawers are modal and neither is a
 *  route, so the keyboard is the only way out that does not require finding the
 *  ✕ with the mouse. */
export function useEscape(active: boolean, close: () => void) {
  useEffect(() => {
    if (!active) return
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") close()
    }
    window.addEventListener("keydown", onKey)
    return () => window.removeEventListener("keydown", onKey)
  }, [active, close])
}
