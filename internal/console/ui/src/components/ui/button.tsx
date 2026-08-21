import { cva, type VariantProps } from "class-variance-authority"
import type { ComponentProps } from "react"
import { cn } from "@/lib/utils"

// Black on yellow is the system's signature and is never inverted. `danger`
// deliberately is not a red fill: the alert colour is a signal, carried as text,
// and a filled red button would read as the primary action of the card.
const button = cva(
  "inline-flex items-center justify-center gap-2 text-button transition-colors disabled:pointer-events-none disabled:opacity-60 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-info",
  {
    variants: {
      variant: {
        primary:
          "bg-primary text-on-primary rounded-md active:bg-primary-active disabled:bg-primary-disabled disabled:text-muted",
        secondary: "bg-surface text-on-dark rounded-md active:bg-elevated",
        outline:
          "border border-hairline text-body rounded-md active:bg-surface",
        danger:
          "border border-hairline text-status-alert rounded-md active:bg-surface",
        ghost: "text-muted rounded-md active:bg-surface",
      },
      size: {
        md: "h-10 px-6",
        sm: "h-8 px-4",
        xs: "h-7 px-3 text-caption",
      },
    },
    defaultVariants: { variant: "primary", size: "md" },
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
