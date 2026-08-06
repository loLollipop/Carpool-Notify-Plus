import { cn } from "@/lib/utils"

export function PageHeader({
  title,
  description,
  actions,
  className,
}: {
  title: string
  description?: string
  actions?: React.ReactNode
  className?: string
}) {
  return (
    <div
      className={cn(
        "mb-6 flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between animate-fade-up",
        className,
      )}
    >
      <div className="flex max-w-2xl items-start gap-3.5">
        <span className="mt-1 h-9 w-1 shrink-0 rounded-full bg-brand shadow-[0_0_0_4px_color-mix(in_oklab,var(--brand)_10%,transparent)]" />
        <div>
          <h1 className="text-2xl font-semibold leading-tight sm:text-[28px]">{title}</h1>
          {description ? (
            <p className="mt-1.5 text-sm leading-relaxed text-muted-foreground">{description}</p>
          ) : null}
        </div>
      </div>
      {actions ? (
        <div className="flex w-full flex-wrap items-center gap-2 lg:w-auto lg:justify-end">
          {actions}
        </div>
      ) : null}
    </div>
  )
}
