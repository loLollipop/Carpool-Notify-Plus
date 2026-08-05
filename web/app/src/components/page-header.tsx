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
      <div className="max-w-2xl">
        <h1 className="text-2xl font-semibold sm:text-[28px]">{title}</h1>
        {description ? (
          <p className="mt-1.5 text-sm leading-relaxed text-muted-foreground">{description}</p>
        ) : null}
      </div>
      {actions ? (
        <div className="flex w-full flex-wrap items-center gap-2 lg:w-auto lg:justify-end">
          {actions}
        </div>
      ) : null}
    </div>
  )
}
