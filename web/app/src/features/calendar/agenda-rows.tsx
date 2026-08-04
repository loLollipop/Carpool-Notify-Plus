import { useTranslation } from "react-i18next"
import { Eye, Mail, MessageCircle } from "lucide-react"

import type { CalendarOccurrence, SubscriptionView } from "@/api/types"
import { DueStatusBadge } from "@/components/due-status-badge"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { cn } from "@/lib/utils"

function MetaItem({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="min-w-0">
      <div className="text-[11px] font-medium tracking-wide text-muted-foreground">{label}</div>
      <div className="truncate text-[13px]">{children}</div>
    </div>
  )
}

function CustomerContactRow({ email, wechat }: { email: string; wechat: string }) {
  const { t } = useTranslation()
  if (!email && !wechat) return null

  return (
    <div className="mt-2 flex min-w-0 flex-wrap items-center gap-1.5">
      {email ? (
        <span
          className="inline-flex max-w-full min-w-0 items-center gap-1 rounded-md bg-muted px-2 py-0.5 text-xs text-muted-foreground"
          title={`${t("subscriptionDialog.customerEmail")}: ${email}`}
        >
          <Mail className="size-3.5 shrink-0" aria-hidden="true" />
          <span className="truncate font-mono">{email}</span>
        </span>
      ) : null}
      {wechat ? (
        <span
          className="inline-flex max-w-full min-w-0 items-center gap-1 rounded-md bg-muted px-2 py-0.5 text-xs text-muted-foreground"
          title={`${t("subscriptionDialog.customerWechat")}: ${wechat}`}
        >
          <MessageCircle className="size-3.5 shrink-0" aria-hidden="true" />
          <span className="truncate">{wechat}</span>
        </span>
      ) : null}
    </div>
  )
}

export function AgendaOccurrenceRow({
  occurrence,
  onView,
}: {
  occurrence: CalendarOccurrence
  onView: (occurrence: CalendarOccurrence) => void
}) {
  const { t } = useTranslation()

  return (
    <article
      className={cn(
        "group rounded-lg border bg-card p-3.5 transition-colors",
        occurrence.paid ? "border-success/25 bg-success/[0.04]" : "hover:border-ring/40",
      )}
    >
      <div className="flex items-start justify-between gap-2">
        <div className="min-w-0">
          <h3 className="truncate text-sm font-semibold">
            <button
              type="button"
              onClick={() => onView(occurrence)}
              title={t("calendar.viewSeatHint")}
              className="max-w-full cursor-pointer truncate text-left transition-colors hover:text-brand"
            >
              {occurrence.account_name || occurrence.name}
            </button>
          </h3>
          <CustomerContactRow
            email={occurrence.customer_email}
            wechat={occurrence.customer_wechat}
          />
        </div>
        <DueStatusBadge paid={occurrence.paid} daysRemaining={occurrence.days_remaining} />
      </div>

      <div className="mt-3 grid grid-cols-2 gap-x-4 gap-y-2 sm:grid-cols-3">
        <MetaItem label={t("calendar.dueDate")}>
          <span className="tabular-nums">{occurrence.due_date}</span> · {occurrence.weekday_label}
        </MetaItem>
        <MetaItem label={t("calendar.cycle")}>{occurrence.cycle_desc}</MetaItem>
        {occurrence.boarded_at ? (
          <MetaItem label={t("calendar.boardedAt")}>
            <span className="tabular-nums">{occurrence.boarded_at}</span>
          </MetaItem>
        ) : null}
      </div>

      <p className="mt-2 text-xs text-muted-foreground">
        {occurrence.paid ? t("calendar.paidReminderPaused") : occurrence.reminder_label}
      </p>

      <div className="mt-3 flex flex-wrap items-center gap-1.5 border-t pt-3">
        <Button variant="outline" size="sm" onClick={() => onView(occurrence)}>
          <Eye data-slot="icon" />
          {t("calendar.viewUser")}
        </Button>
      </div>
    </article>
  )
}

export function AgendaArchivedRow({
  view,
  onView,
}: {
  view: SubscriptionView
  onView: (view: SubscriptionView) => void
}) {
  const { t } = useTranslation()
  const subscription = view.subscription

  return (
    <article className="rounded-lg border border-dashed bg-muted/30 p-3.5">
      <div className="flex flex-wrap items-center gap-1.5">
        <h3 className="min-w-0 flex-1 truncate text-sm font-semibold text-muted-foreground">
          <button
            type="button"
            title={t("calendar.viewSeatHint")}
            onClick={() => onView(view)}
            className="max-w-full cursor-pointer truncate text-left transition-colors hover:text-foreground"
          >
            {view.account_name || subscription.name}
          </button>
        </h3>
        <Badge variant="outline" className="font-normal text-muted-foreground">
          {t("calendar.filterArchived")}
        </Badge>
      </div>
      <CustomerContactRow
        email={subscription.customer_email}
        wechat={subscription.customer_wechat}
      />

      <div className="mt-3 grid grid-cols-2 gap-x-4 gap-y-2 sm:grid-cols-3">
        <MetaItem label={t("calendar.cycle")}>{view.cycle_desc}</MetaItem>
        {view.boarded_at ? (
          <MetaItem label={t("calendar.boardedAt")}>
            <span className="tabular-nums">{view.boarded_at}</span>
          </MetaItem>
        ) : null}
        {view.archived_at_label ? (
          <MetaItem label={t("calendar.archivedAt")}>
            <span className="tabular-nums">{view.archived_at_label}</span>
          </MetaItem>
        ) : null}
      </div>

      <p className="mt-2 text-xs text-muted-foreground">
        {t("calendar.archivedNote")}{" "}
        {view.bill_count > 0
          ? t("calendar.archivedBillCount", { count: view.bill_count })
          : t("calendar.archivedNoBills")}
      </p>

      <div className="mt-3 flex flex-wrap items-center gap-1.5 border-t pt-3">
        <Button variant="outline" size="sm" onClick={() => onView(view)}>
          <Eye data-slot="icon" />
          {t("calendar.viewUser")}
        </Button>
      </div>
    </article>
  )
}
