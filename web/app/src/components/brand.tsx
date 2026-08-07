import { cn } from "@/lib/utils"

export const APP_NAME = "Carpool Notify Plus"

export function BrandIcon({ className }: { className?: string }) {
  return (
    <span
      className={cn(
        "grid size-9 shrink-0 place-items-center rounded-[7px] bg-[#315be8] text-white shadow-[0_5px_14px_rgba(49,91,232,0.24)] dark:bg-[#5578ee] dark:text-white dark:shadow-none",
        className,
      )}
      aria-hidden="true"
    >
      <svg viewBox="0 0 36 36" className="size-7" fill="none">
        <path
          d="M24 10A10 10 0 1 0 24 26"
          stroke="currentColor"
          strokeWidth="3.7"
          strokeLinecap="round"
        />
        <path
          d="M26 13.8V22.2M21.8 18H30.2"
          stroke="#c9f46c"
          strokeWidth="2.8"
          strokeLinecap="round"
        />
      </svg>
    </span>
  )
}
