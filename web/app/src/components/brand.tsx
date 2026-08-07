import { Waypoints } from "lucide-react"

import { cn } from "@/lib/utils"

export const APP_NAME = "SeatFlow"

export function BrandIcon({ className }: { className?: string }) {
  return (
    <span
      className={cn(
        "grid size-9 shrink-0 place-items-center rounded-[7px] bg-[#252c3a] text-white shadow-[0_5px_14px_rgba(37,44,58,0.18)] dark:bg-[#eef1f5] dark:text-[#252c3a] dark:shadow-none",
        className,
      )}
      aria-hidden="true"
    >
      <Waypoints className="size-5" strokeWidth={2.1} />
    </span>
  )
}
