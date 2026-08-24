import type * as React from "react"

import { Card } from "@/components/ui/card"
import { cn } from "@/lib/utils"

export function EmptyState({
  icon,
  title,
  description,
  action,
  className,
}: {
  icon: React.ReactNode
  title: string
  description?: string
  action?: React.ReactNode
  className?: string
}) {
  return (
    <Card
      className={cn(
        "relative min-h-64 items-center justify-center overflow-hidden px-6 py-14 text-center animate-fade-up",
        className,
      )}
    >
      <div
        aria-hidden="true"
        className="pointer-events-none absolute inset-x-[18%] top-0 h-px bg-gradient-to-r from-transparent via-brand/35 to-transparent"
      />
      <span className="grid size-12 place-items-center rounded-xl border border-brand/15 bg-brand/[0.07] text-brand shadow-sm">
        {icon}
      </span>
      <div className="max-w-md">
        <h3 className="text-sm font-semibold text-foreground">{title}</h3>
        {description ? (
          <p className="mt-1.5 text-xs leading-5 text-muted-foreground">{description}</p>
        ) : null}
      </div>
      {action ? <div className="mt-1">{action}</div> : null}
    </Card>
  )
}
