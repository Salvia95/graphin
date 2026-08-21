import { cva, type VariantProps } from "class-variance-authority"
import type { ComponentProps } from "react"
import { cn } from "@/lib/utils"

const button = cva(
  "inline-flex items-center justify-center gap-2 rounded-md text-sm font-medium transition-colors disabled:pointer-events-none disabled:opacity-50 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-sky-600",
  {
    variants: {
      variant: {
        default:
          "bg-slate-900 text-white hover:bg-slate-700 dark:bg-slate-100 dark:text-slate-900 dark:hover:bg-white",
        outline:
          "border border-slate-300 hover:bg-slate-100 dark:border-slate-700 dark:hover:bg-slate-800",
        ghost: "hover:bg-slate-100 dark:hover:bg-slate-800",
        danger:
          "border border-red-300 text-red-700 hover:bg-red-50 dark:border-red-900 dark:text-red-400 dark:hover:bg-red-950",
      },
      size: { default: "h-9 px-4 py-2", sm: "h-8 px-3 text-xs" },
    },
    defaultVariants: { variant: "default", size: "default" },
  },
)

export function Button({
  className,
  variant,
  size,
  ...props
}: ComponentProps<"button"> & VariantProps<typeof button>) {
  return <button className={cn(button({ variant, size }), className)} {...props} />
}
