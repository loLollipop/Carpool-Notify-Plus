import { cn } from "@/lib/utils"

export const APP_NAME = "Carpool Notify Plus"

export function BrandIcon({ className }: { className?: string }) {
  return (
    <img
      src="/carpool-notify-plus.svg"
      alt=""
      className={cn(
        "size-9 shrink-0 rounded-[7px] shadow-[0_5px_14px_rgba(15,118,110,0.22)] dark:shadow-none",
        className,
      )}
      aria-hidden="true"
    />
  )
}
