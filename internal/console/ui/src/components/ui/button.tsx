import { cva, type VariantProps } from "class-variance-authority"
import type { ComponentProps } from "react"
import { cn } from "@/lib/utils"

// Black on yellow is the system's signature and is never inverted. `danger`
// deliberately is not a red fill: the alert colour is a signal, carried as text,
// and a filled red button would read as the primary action of the card.
//
// Hover is a real state here — this is a desktop-only local tool (brief §3.4) —
// and all it ever does is step a border or a fill up by one token.
const button = cva(
  "inline-flex items-center justify-center gap-2 whitespace-nowrap text-button transition-colors disabled:pointer-events-none disabled:opacity-60 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-info",
  {
    variants: {
      variant: {
        primary:
          "bg-primary text-on-primary rounded-md hover:bg-primary-active disabled:bg-primary-disabled disabled:text-muted",
        secondary: "bg-surface text-on-dark rounded-md hover:bg-elevated",
        outline:
          "border border-hairline text-muted-strong rounded-md hover:text-body hover:border-hairline-strong",
        danger:
          "border border-hairline text-status-alert rounded-md hover:border-status-alert/60",
        ghost: "text-muted rounded-md hover:text-body hover:bg-elevated",
        /** For "+ add" affordances: dashed, so it reads as an empty slot rather
         *  than a control competing with the ones that are filled. */
        dashed:
          "border border-dashed border-hairline text-muted rounded-sm hover:text-body hover:border-hairline-strong",
      },
      size: {
        md: "h-10 px-4",
        sm: "h-8 px-4",
        xs: "h-7 px-2.5 text-label",
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

/** Links are yellow, which is the one place the brand colour appears that is
 *  not an action or a number. They are rare enough that it holds. */
export function LinkButton({ className, ...props }: ComponentProps<"button">) {
  return (
    <button
      className={cn(
        "text-caption text-primary transition-colors hover:text-primary-active",
        className,
      )}
      {...props}
    />
  )
}
