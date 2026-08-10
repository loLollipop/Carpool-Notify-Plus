import * as React from "react"
import { useTranslation } from "react-i18next"
import { useSearchParams } from "react-router-dom"
import { ChevronLeft, ChevronRight } from "lucide-react"

import { useCalendar, useDashboard } from "@/api/queries"
import type {
  CalendarDay,
  CalendarMonth,
  CalendarOccurrence,
  Dashboard,
} from "@/api/types"
import { KpiSection, KpiSectionSkeleton } from "@/components/kpi-section"
import { PageHeader } from "@/components/page-header"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { Skeleton } from "@/components/ui/skeleton"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { cn } from "@/lib/utils"
import {
  SeatSubscriptionDialog,
  seatInfoFromOccurrence,
  type SeatSubscriptionInfo,
} from "./SeatSubscriptionDialog"

// ---- Month grid -------------------------------------------------------------------

function EventPill({
  occurrence,
  onView,
}: {
  occurrence: CalendarOccurrence
  onView: (occurrence: CalendarOccurrence) => void
}) {
  const { t } = useTranslation()
  const label =
    occurrence.customer_email ||
    occurrence.customer_wechat ||
    occurrence.account_name ||
    occurrence.name
  return (
    <Tooltip delayDuration={150}>
      <TooltipTrigger asChild>
        <button
          type="button"
          onClick={(event) => {
            event.stopPropagation()
            onView(occurrence)
          }}
          onKeyDown={(event) => event.stopPropagation()}
          className={cn(
            "block w-full truncate rounded-[4px] border px-1.5 py-0.5 text-left text-[11px] font-medium leading-4 transition-colors",
            occurrence.paid
              ? "border-success/35 bg-success/15 text-success line-through decoration-success/55 shadow-[inset_3px_0_0_var(--success),0_1px_1px_color-mix(in_oklab,var(--foreground)_4%,transparent)] hover:bg-success/20"
              : "border-gold/40 bg-gold/15 text-foreground shadow-[inset_3px_0_0_var(--gold),0_1px_1px_color-mix(in_oklab,var(--foreground)_4%,transparent)] hover:bg-gold/20",
          )}
        >
          {label}
        </button>
      </TooltipTrigger>
      <TooltipContent side="top" className="grid gap-0.5">
        <span className="font-semibold">{occurrence.account_name || occurrence.name}</span>
        {occurrence.customer_email ? <span>{occurrence.customer_email}</span> : null}
        {!occurrence.customer_email && occurrence.customer_wechat ? (
          <span>{occurrence.customer_wechat}</span>
        ) : null}
        <span className="tabular-nums">
          {occurrence.due_date} · ¥{occurrence.price_yuan}
        </span>
        <span>{occurrence.paid ? t("dueStatus.paid") : t("calendar.legendPending")}</span>
      </TooltipContent>
    </Tooltip>
  )
}

function MonthGrid({
  calendar,
  onSelectDay,
  onViewOccurrence,
}: {
  calendar: CalendarMonth
  onSelectDay: (day: CalendarDay) => void
  onViewOccurrence: (occurrence: CalendarOccurrence) => void
}) {
  const { t } = useTranslation()
  const weekdays = t("calendar.weekdays", { returnObjects: true }) as string[]

  return (
    <section aria-label={calendar.month_label}>
      <div className="grid grid-cols-7 border-b bg-muted/70" aria-hidden="true">
        {weekdays.map((weekday, index) => (
          <span
            key={weekday}
            className={cn(
              "px-2 py-2 text-center text-xs font-medium text-muted-foreground",
              index >= 5 && "text-muted-foreground/60",
            )}
          >
            {weekday}
          </span>
        ))}
      </div>
      <div className="grid grid-cols-7 gap-px bg-border/70">
        {calendar.days.map((day) => {
          const navigatesMonth = !day.in_month
          return (
            <div
              key={day.date}
              role={navigatesMonth ? "button" : undefined}
              tabIndex={navigatesMonth ? 0 : undefined}
              aria-label={
                navigatesMonth ? t("calendar.dateOnly", { date: day.date_label }) : undefined
              }
              onClick={() => {
                if (navigatesMonth) onSelectDay(day)
              }}
              onKeyDown={(event) => {
                if (!navigatesMonth) return
                if (event.key === "Enter" || event.key === " ") {
                  event.preventDefault()
                  onSelectDay(day)
                }
              }}
              className={cn(
                "relative flex flex-col gap-1 p-1.5 outline-none transition-colors",
                (day.occurrences?.length ?? 0) > 0
                  ? "min-h-[72px] sm:min-h-[88px]"
                  : "min-h-[44px] sm:min-h-[52px]",
                day.in_month
                  ? "bg-card"
                  : "cursor-pointer bg-muted/40 text-muted-foreground/70 hover:bg-accent",
              )}
            >
              <span className="flex items-center justify-between">
                <span
                  className={cn(
                    "text-xs tabular-nums",
                    day.is_today
                       ? "flex size-5 items-center justify-center rounded-full bg-brand font-semibold text-brand-foreground shadow-[0_0_0_3px_color-mix(in_oklab,var(--brand)_15%,transparent)]"
                      : day.in_month
                        ? day.is_weekend
                          ? "font-medium text-brand"
                          : "font-medium text-foreground"
                        : "text-muted-foreground/50",
                  )}
                >
                  {day.day_number}
                </span>
              </span>
              <span className="flex flex-col gap-0.5 overflow-hidden">
                {(day.occurrences ?? []).map((occurrence) => (
                  <EventPill
                    key={`${occurrence.subscription_id}:${occurrence.due_date}`}
                    occurrence={occurrence}
                    onView={onViewOccurrence}
                  />
                ))}
              </span>
            </div>
          )
        })}
      </div>
    </section>
  )
}

// ---- Workspace (keyed by month so filters reset on month change) -------------------

function CalendarWorkspace({
  calendar,
  dashboard,
  onNavigateMonth,
  onViewOccurrence,
}: {
  calendar: CalendarMonth
  dashboard: Dashboard | undefined
  onNavigateMonth: (month: string) => void
  onViewOccurrence: (occurrence: CalendarOccurrence) => void
}) {
  const { t } = useTranslation()

  const handleSelectDay = (day: CalendarDay) => {
    if (!day.in_month) {
      onNavigateMonth(day.date.slice(0, 7))
    }
  }

  return (
    <>
      {dashboard ? (
        <KpiSection
          dashboard={dashboard}
          pendingCount={calendar.pending_month_count}
          pendingMode="monthDue"
        />
      ) : (
        <KpiSectionSkeleton />
      )}

      <div className="grid items-start gap-4">
        <Card className="gap-0 overflow-hidden p-0 animate-fade-up">
          <div className="flex flex-wrap items-center justify-between gap-3 border-b bg-muted/35 px-4 py-3 text-foreground">
            <div className="flex items-center gap-1">
              <Button
                variant="ghost"
                size="icon-sm"
                className="text-muted-foreground hover:bg-accent hover:text-foreground"
                aria-label={t("calendar.prevMonth")}
                onClick={() => onNavigateMonth(calendar.previous_month)}
              >
                <ChevronLeft />
              </Button>
              <h2 className="display-numeral min-w-28 text-center text-base font-semibold text-foreground">
                {calendar.month_label}
              </h2>
              <Button
                variant="ghost"
                size="icon-sm"
                className="text-muted-foreground hover:bg-accent hover:text-foreground"
                aria-label={t("calendar.nextMonth")}
                onClick={() => onNavigateMonth(calendar.next_month)}
              >
                <ChevronRight />
              </Button>
            </div>
            <div className="flex items-center gap-3">
              <div className="hidden items-center gap-3 text-xs text-muted-foreground sm:flex">
                <span className="flex items-center gap-1.5">
                  <i className="size-2 rounded-full bg-gold ring-2 ring-gold/15" />
                  {t("calendar.legendPending")}
                </span>
                <span className="flex items-center gap-1.5">
                  <i className="size-2 rounded-full bg-success ring-2 ring-success/15" />
                  {t("calendar.legendPaid")}
                </span>
              </div>
              <Button
                variant="outline"
                size="sm"
                onClick={() => onNavigateMonth(calendar.current_month)}
              >
                {t("calendar.today")}
              </Button>
            </div>
          </div>
          <MonthGrid
            calendar={calendar}
            onSelectDay={handleSelectDay}
            onViewOccurrence={onViewOccurrence}
          />
        </Card>
      </div>
    </>
  )
}

// ---- Page -------------------------------------------------------------------------

export function CalendarPage() {
  const { t } = useTranslation()
  const [searchParams, setSearchParams] = useSearchParams()
  const month = searchParams.get("month") ?? ""

  const calendarQuery = useCalendar(month || undefined)
  const dashboardQuery = useDashboard()

  const [seatInfo, setSeatInfo] = React.useState<SeatSubscriptionInfo | null>(null)

  const calendar = calendarQuery.data

  const goToMonth = (value: string) => {
    setSearchParams(value ? { month: value } : {})
  }

  return (
    <>
      <PageHeader
        title={t("calendar.title")}
        description={t("calendar.desc")}
      />

      {calendarQuery.isPending ? (
        <>
          <KpiSectionSkeleton />
          <div className="grid gap-4">
            <Skeleton className="h-[640px] rounded-xl" />
          </div>
        </>
      ) : calendarQuery.isError ? (
        <Card className="items-center gap-3 py-16 text-center">
          <p className="text-sm text-muted-foreground">{t("common.loadFailed")}</p>
          <Button variant="outline" onClick={() => calendarQuery.refetch()}>
            {t("common.retry")}
          </Button>
        </Card>
      ) : calendar ? (
        <CalendarWorkspace
          key={calendar.month_value}
          calendar={calendar}
          dashboard={dashboardQuery.data}
          onNavigateMonth={goToMonth}
          onViewOccurrence={(occurrence) => setSeatInfo(seatInfoFromOccurrence(occurrence, t))}
        />
      ) : null}

      <SeatSubscriptionDialog
        open={seatInfo !== null}
        onOpenChange={(open) => {
          if (!open) setSeatInfo(null)
        }}
        info={seatInfo}
      />
    </>
  )
}
