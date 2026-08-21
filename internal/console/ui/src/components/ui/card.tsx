import type { ComponentProps } from "react"
import { cn } from "@/lib/utils"

// No shadow, no border. Depth is the lightness step from canvas to surface —
// the reference is explicit that adding drop shadows or glass muddies it.
export function Card({ className, ...props }: ComponentProps<"div">) {
  return <div className={cn("rounded-xl bg-surface", className)} {...props} />
}

export function CardContent({ className, ...props }: ComponentProps<"div">) {
  return <div className={cn("p-6", className)} {...props} />
}
