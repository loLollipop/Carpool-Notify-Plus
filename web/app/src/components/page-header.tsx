import { cn } from "@/lib/utils"

export function PageHeader({
  title,
  titleAccessory,
  description,
  actions,
  className,
}: {
  title: string
  titleAccessory?: React.ReactNode
  description?: string
  actions?: React.ReactNode
  className?: string
}) {
  return (
    <div
      className={cn(
        "mb-5 flex flex-col gap-3 lg:flex-row lg:items-end lg:justify-between animate-fade-up",
        className,
      )}
    >
      <div className="max-w-2xl">
        <div>
          <div className="flex items-center gap-1.5">
            <h1 className="text-[22px] font-semibold leading-tight text-foreground sm:text-2xl">{title}</h1>
            {titleAccessory}
          </div>
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
