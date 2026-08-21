import type { ComponentProps } from "react"
import { cn } from "@/lib/utils"

const base =
  "w-full rounded-md border border-slate-300 bg-transparent px-3 py-1.5 text-sm placeholder:text-slate-400 focus-visible:outline-2 focus-visible:outline-offset-0 focus-visible:outline-sky-600 dark:border-slate-700"

export function Input({ className, ...props }: ComponentProps<"input">) {
  return <input className={cn(base, "h-9", className)} {...props} />
}

export function Textarea({ className, ...props }: ComponentProps<"textarea">) {
  return <textarea className={cn(base, "min-h-20", className)} {...props} />
}

export function Label({ className, ...props }: ComponentProps<"label">) {
  return (
    <label
      className={cn("text-xs font-medium text-slate-500 dark:text-slate-400", className)}
      {...props}
    />
  )
}

export function Badge({ className, ...props }: ComponentProps<"span">) {
  return (
    <span
      className={cn(
        "inline-flex items-center rounded-full border border-slate-200 px-2 py-0.5 text-xs text-slate-600 dark:border-slate-700 dark:text-slate-400",
        className,
      )}
      {...props}
    />
  )
}
