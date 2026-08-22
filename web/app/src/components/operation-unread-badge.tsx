import { cn } from "@/lib/utils"

export function OperationUnreadBadge({
  count,
  className,
}: {
  count: number
  className?: string
}) {
  if (count <= 0) return null
  return (
    <span
      aria-label={`${count} new notifications`}
      className={cn(
        "inline-flex h-5 min-w-5 shrink-0 items-center justify-center rounded-full bg-destructive px-1.5 text-[10px] font-bold leading-none text-destructive-foreground shadow-sm tabular-nums",
        className,
      )}
    >
      {count > 99 ? "99+" : count}
    </span>
  )
}
