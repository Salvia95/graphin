import type { ComponentProps } from "react"
import { cn } from "@/lib/utils"

// Inputs sit on canvas inside a surface panel — one step down, so the field
// reads as a hole in the card rather than a raised control.
const field =
  "w-full rounded-md border border-hairline bg-canvas px-3 text-body-md text-body placeholder:text-muted focus-visible:border-info focus-visible:outline-none disabled:text-muted-strong read-only:text-muted-strong"

export function Input({ className, ...props }: ComponentProps<"input">) {
  return <input className={cn(field, "h-10", className)} {...props} />
}

export function Textarea({ className, ...props }: ComponentProps<"textarea">) {
  return <textarea className={cn(field, "min-h-24 resize-y py-2.5", className)} {...props} />
}

export function Label({ className, ...props }: ComponentProps<"label">) {
  return (
    <label
      className={cn("block text-label tracking-eyebrow text-muted uppercase", className)}
      {...props}
    />
  )
}

/** The all-caps 11px label that titles every panel and column in this
 *  interface. One component so the tracking cannot drift between them. */
export function Eyebrow({ className, ...props }: ComponentProps<"span">) {
  return (
    <span
      className={cn("text-label tracking-eyebrow text-muted uppercase", className)}
      {...props}
    />
  )
}

/** Metadata chips: filled, not outlined, because they sit inside a card that is
 *  already a surface and a border there would read as a second card edge. */
export function Chip({ className, ...props }: ComponentProps<"span">) {
  return (
    <span
      className={cn("num rounded-sm bg-elevated px-2 py-[3px] text-label text-muted", className)}
      {...props}
    />
  )
}

/** Every number goes through here so columns align and the numeric voice stays
 *  separate from running text. */
export function Num({ className, ...props }: ComponentProps<"span">) {
  return <span className={cn("num", className)} {...props} />
}
