import { useTranslation } from "react-i18next"
import { CheckCircle2, Clock3 } from "lucide-react"

import { cn } from "@/lib/utils"

function dueMetaLabel(t: (key: string, options?: Record<string, unknown>) => string, days: number) {
  if (days === 0) return t("dueStatus.today")
  if (days < 0) return t("dueStatus.overdueDays", { count: -days })
  return t("dueStatus.days", { count: days })
}

export function DueStatusBadge({
  paid,
  daysRemaining,
  showPaidLabel = false,
  className,
}: {
  paid: boolean
  daysRemaining: number
  showPaidLabel?: boolean
  className?: string
}) {
  const { t } = useTranslation()

  if (paid) {
    return (
      <span
        title={t("dueStatus.paid")}
        className={cn(
          "inline-flex h-6 min-w-max shrink-0 flex-nowrap items-center gap-1 whitespace-nowrap rounded-full bg-success/12 px-2 text-xs font-medium text-success dark:bg-success/18",
          className,
        )}
      >
        <CheckCircle2 className="size-3.5 shrink-0" />
        <span className={showPaidLabel ? "whitespace-nowrap" : "sr-only"}>
          {t(showPaidLabel ? "dueStatus.paidInline" : "dueStatus.paid")}
        </span>
      </span>
    )
  }

  const soon = daysRemaining <= 7
  const title =
    daysRemaining === 0
      ? t("dueStatus.pendingToday")
      : daysRemaining < 0
        ? t("dueStatus.pendingOverdue", { count: -daysRemaining })
        : t("dueStatus.pendingDays", { count: daysRemaining })

  return (
    <span
      title={title}
      className={cn(
        "inline-flex h-6 min-w-max shrink-0 flex-nowrap items-center gap-1 whitespace-nowrap rounded-full px-2 text-xs font-medium tabular-nums",
        soon
          ? "bg-brand/12 text-brand dark:bg-brand/18"
          : "bg-muted text-muted-foreground",
        className,
      )}
    >
      <Clock3 className="size-3.5 shrink-0" />
      <span className="whitespace-nowrap">{dueMetaLabel(t, daysRemaining)}</span>
    </span>
  )
}
