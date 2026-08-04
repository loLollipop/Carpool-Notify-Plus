import { useTranslation } from "react-i18next"
import { Link } from "react-router-dom"
import { ExternalLink, Mail, Pencil, Receipt, Trash2, UserRoundMinus } from "lucide-react"

import type { CalendarOccurrence, SubscriptionView } from "@/api/types"
import { DueStatusBadge } from "@/components/due-status-badge"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Switch } from "@/components/ui/switch"
import { cn } from "@/lib/utils"

function MetaItem({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="min-w-0">
      <div className="text-[11px] font-medium tracking-wide text-muted-foreground">{label}</div>
      <div className="truncate text-[13px]">{children}</div>
    </div>
  )
}

export function AgendaOccurrenceRow({
  occurrence,
  onEdit,
  onSendReminder,
  onArchive,
  onTogglePaid,
  onView,
  paidToggleBusy,
}: {
  occurrence: CalendarOccurrence
  onEdit: (occurrence: CalendarOccurrence) => void
  onSendReminder: (occurrence: CalendarOccurrence) => void
  onArchive: (occurrence: CalendarOccurrence) => void
  onTogglePaid: (occurrence: CalendarOccurrence, paid: boolean) => void
  onView: (occurrence: CalendarOccurrence) => void
  paidToggleBusy: boolean
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
          <h3 className="truncate text-sm font-semibold">{occurrence.name}</h3>
          <div className="mt-1 flex flex-wrap items-center gap-1.5">
            <Badge variant="secondary" className="font-normal" asChild>
              <button
                type="button"
                onClick={() => onView(occurrence)}
                title={t("calendar.viewSeatHint")}
                className="cursor-pointer transition-colors hover:bg-accent hover:text-foreground"
              >
                {occurrence.account_name}
                {occurrence.seat_name ? ` · ${occurrence.seat_name}` : ""}
              </button>
            </Badge>
          </div>
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
        {occurrence.customer_wechat ? (
          <MetaItem label={t("subscriptionDialog.customerWechat")}>
            {occurrence.customer_wechat}
          </MetaItem>
        ) : null}
      </div>

      <p className="mt-2 text-xs text-muted-foreground">
        {occurrence.paid ? t("calendar.paidReminderPaused") : occurrence.reminder_label}
      </p>

      <div className="mt-3 flex flex-wrap items-center gap-1.5 border-t pt-3">
        <Button variant="outline" size="sm" onClick={() => onEdit(occurrence)}>
          <Pencil data-slot="icon" />
          {t("common.edit")}
        </Button>
        {occurrence.customer_email ? (
          <Button variant="outline" size="sm" onClick={() => onSendReminder(occurrence)}>
            <Mail data-slot="icon" />
            {t("cards.sendReminder")}
          </Button>
        ) : null}
        {occurrence.trade_url ? (
          <Button variant="ghost" size="sm" asChild>
            <a href={occurrence.trade_url} target="_blank" rel="noopener noreferrer">
              <ExternalLink data-slot="icon" />
              {t("common.openLink")}
            </a>
          </Button>
        ) : null}
        <Button
          variant="ghost"
          size="sm"
          className="text-destructive hover:text-destructive"
          onClick={() => onArchive(occurrence)}
        >
          <UserRoundMinus data-slot="icon" />
          {t("calendar.getOff")}
        </Button>

        <label className="ml-auto flex cursor-pointer items-center gap-2 text-xs text-muted-foreground select-none">
          {t("calendar.paidInline")}
          <Switch
            checked={occurrence.paid}
            disabled={paidToggleBusy}
            title={occurrence.paid ? t("calendar.switchTitleOff") : t("calendar.switchTitleOn")}
            aria-label={`${occurrence.name} ${occurrence.due_date} ${t("calendar.paidInline")}`}
            onCheckedChange={(checked) => onTogglePaid(occurrence, checked === true)}
          />
        </label>
      </div>
    </article>
  )
}

export function AgendaArchivedRow({
  view,
  onSoftDelete,
  onView,
}: {
  view: SubscriptionView
  onSoftDelete: (view: SubscriptionView) => void
  onView: (view: SubscriptionView) => void
}) {
  const { t } = useTranslation()
  const subscription = view.subscription

  return (
    <article className="rounded-lg border border-dashed bg-muted/30 p-3.5">
      <div className="flex flex-wrap items-center gap-1.5">
        <h3 className="text-sm font-semibold text-muted-foreground">{subscription.name}</h3>
        <Badge variant="secondary" asChild className="font-normal">
          <button
            type="button"
            title={t("calendar.viewSeatHint")}
            onClick={() => onView(view)}
            className="cursor-pointer transition-colors hover:bg-accent hover:text-accent-foreground"
          >
            {view.account_name}
            {view.seat_name ? ` · ${view.seat_name}` : ""}
          </button>
        </Badge>
        <Badge variant="outline" className="font-normal text-muted-foreground">
          {t("calendar.filterArchived")}
        </Badge>
      </div>

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
        {subscription.trade_url ? (
          <Button variant="ghost" size="sm" asChild>
            <a href={subscription.trade_url} target="_blank" rel="noopener noreferrer">
              <ExternalLink data-slot="icon" />
              {t("common.openLink")}
            </a>
          </Button>
        ) : null}
        <Button variant="ghost" size="sm" asChild>
          <Link to="/bills">
            <Receipt data-slot="icon" />
            {t("calendar.viewBills")}
          </Link>
        </Button>
        {view.can_soft_delete ? (
          <Button
            variant="ghost"
            size="sm"
            className="text-destructive hover:text-destructive"
            onClick={() => onSoftDelete(view)}
          >
            <Trash2 data-slot="icon" />
            {t("calendar.softDelete")}
          </Button>
        ) : (
          <Button
            variant="ghost"
            size="sm"
            disabled
            title={t("calendar.softDeleteBlocked", { count: view.bill_count })}
          >
            <Trash2 data-slot="icon" />
            {t("calendar.softDelete")}
          </Button>
        )}
      </div>
    </article>
  )
}
