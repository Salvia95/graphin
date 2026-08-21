import type { ComponentProps } from "react"
import { cn } from "@/lib/utils"

const field =
  "w-full rounded-md border border-hairline bg-elevated px-4 text-body-md text-body placeholder:text-muted focus-visible:outline-2 focus-visible:outline-offset-0 focus-visible:outline-info"

export function Input({ className, ...props }: ComponentProps<"input">) {
  return <input className={cn(field, "h-10", className)} {...props} />
}

export function Textarea({ className, ...props }: ComponentProps<"textarea">) {
  return <textarea className={cn(field, "min-h-24 py-3", className)} {...props} />
}

export function Label({ className, ...props }: ComponentProps<"label">) {
  return <label className={cn("text-caption text-muted", className)} {...props} />
}

/** Badges carry hairline borders rather than fills — a filled badge would
 *  compete with the one thing that is allowed a solid colour, the primary CTA. */
export function Badge({ className, ...props }: ComponentProps<"span">) {
  return (
    <span
      className={cn(
        "inline-flex items-center rounded-sm border border-hairline px-2 py-0.5 text-caption text-muted",
        className,
      )}
      {...props}
    />
  )
}

/** Every number goes through here so columns align and the numeric voice stays
 *  separate from running text. */
export function Num({ className, ...props }: ComponentProps<"span">) {
  return <span className={cn("num", className)} {...props} />
}
