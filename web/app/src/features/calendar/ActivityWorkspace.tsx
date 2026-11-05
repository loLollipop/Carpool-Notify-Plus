import { useTranslation } from "react-i18next"
import { ChevronRight, History, ReceiptText, UserRoundPlus, UserRoundX } from "lucide-react"

import type { CalendarActivity } from "@/api/types"
import { Badge } from "@/components/ui/badge"
import { useAmountPrivacy } from "@/hooks/use-amount-privacy"
import { formatAccountLabel } from "@/lib/account-display"
import { maskAmount } from "@/lib/amount-privacy"
import { cn } from "@/lib/utils"

export function ActivityWorkspace({ items, onView }: {
  items: CalendarActivity[]
  onView: (item: CalendarActivity) => void
}) {
  const { t } = useTranslation()
  const { amountsHidden } = useAmountPrivacy()

  if (items.length === 0) {
    return (
      <div className="grid min-h-72 place-items-center rounded-lg border border-dashed text-center">
        <div>
          <History className="mx-auto size-8 text-muted-foreground" />
          <p className="mt-3 text-sm font-medium">{t("calendar.activityEmpty")}</p>
        </div>
      </div>
    )
  }

  return (
    <ol className="relative grid pl-1 before:absolute before:bottom-6 before:left-[15px] before:top-6 before:w-px before:bg-border" aria-label={t("calendar.viewActivity")}>
      {items.map((item) => {
        const occurrence = item.occurrence
        const created = item.kind === "created"
        const paid = item.kind === "paid"
        const Icon = created ? UserRoundPlus : paid ? ReceiptText : UserRoundX
        const label = t(created ? "calendar.activityCreated" : paid ? "calendar.activityPaid" : "calendar.activityArchived")
        return (
          <li key={item.id} className="relative py-2 pl-9 sm:pl-10">
            <span className={cn(
              "absolute left-0 top-7 z-10 grid size-6 place-items-center rounded-full ring-4 ring-background",
              created ? "bg-brand text-brand-foreground" : paid ? "bg-success text-success-foreground" : "bg-muted text-muted-foreground",
            )}>
              <Icon className="size-3.5" aria-hidden="true" />
            </span>
            <button
              type="button"
              onClick={() => onView(item)}
              className="group flex w-full min-w-0 items-center gap-3 rounded-lg border bg-card p-3 text-left outline-none transition-colors hover:bg-accent/20 focus-visible:ring-2 focus-visible:ring-ring sm:p-4"
            >
              <span className="min-w-0 flex-1">
                <span className="flex flex-wrap items-start justify-between gap-x-4 gap-y-1">
                  <strong className="min-w-0 break-all text-sm">{occurrence.customer_email || occurrence.customer_wechat || occurrence.name}</strong>
                  <time dateTime={item.occurred_at} className="shrink-0 text-xs tabular-nums text-muted-foreground">{item.occurred_at_label}</time>
                </span>
                <span className="mt-2 flex flex-wrap items-center gap-x-3 gap-y-2 text-xs text-muted-foreground">
                  <span className={cn("font-medium", created ? "text-brand" : paid ? "text-success" : "text-foreground")}>{label}</span>
                  <Badge variant={occurrence.business_type === "plus" ? "brand" : "secondary"} className="font-normal">{occurrence.business_type === "plus" ? "Plus" : "Team"}</Badge>
                  {item.amount_yuan ? (
                    <span className="font-medium tabular-nums text-foreground">
                      {created ? `${t("calendar.activityInitialPaid")} ` : ""}
                      {maskAmount(amountsHidden, `¥${item.amount_yuan}`)}
                    </span>
                  ) : null}
                  <span className="min-w-0 break-all">{formatAccountLabel(occurrence.account_serial, occurrence.account_display_email || occurrence.account_name, "—", occurrence.account_space_role)}</span>
                  {item.amount_yuan ? <span>{t("calendar.activityPeriod", { date: occurrence.due_date })}</span> : null}
                </span>
              </span>
              <ChevronRight className="hidden size-4 shrink-0 text-muted-foreground/60 transition-colors group-hover:text-foreground sm:block" aria-hidden="true" />
            </button>
          </li>
        )
      })}
    </ol>
  )
}
