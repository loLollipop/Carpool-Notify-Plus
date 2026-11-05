import * as React from "react"
import { useSearchParams } from "react-router-dom"
import { useTranslation } from "react-i18next"
import {
  CalendarDays,
  CheckCircle2,
  ChevronLeft,
  ChevronRight,
  CircleDollarSign,
  Clock3,
  Eye,
  History,
  ListTodo,
  Pause,
  ReceiptText,
} from "lucide-react"

import { useCalendar } from "@/api/queries"
import type { CalendarDay, CalendarMonth, CalendarOccurrence } from "@/api/types"
import { PageHeader } from "@/components/page-header"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Skeleton } from "@/components/ui/skeleton"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { useAmountPrivacy } from "@/hooks/use-amount-privacy"
import { formatAccountLabel, formatAccountName } from "@/lib/account-display"
import { maskAmount } from "@/lib/amount-privacy"
import { cn } from "@/lib/utils"
import { isOneMonthRentalCron } from "@/features/plus-rentals/rental-mode"
import { DuePaidDialog, type DuePaidTarget } from "./DuePaidDialog"
import {
  seatInfoFromOccurrence,
  seatInfoFromActivity,
  type SeatSubscriptionInfo,
} from "./seat-subscription-info"
import { SeatSubscriptionDialog } from "./SeatSubscriptionDialog"
import { DeferredWorkspace } from "./DeferredWorkspace"
import { ActivityWorkspace } from "./ActivityWorkspace"

type CalendarView = "tasks" | "calendar" | "activity" | "deferred"
type TaskFilter = "all" | "overdue" | "soon" | "paid"
const TASKS_PER_PAGE = 12

function EventPill({
  occurrence,
  onView,
}: {
  occurrence: CalendarOccurrence
  onView: (occurrence: CalendarOccurrence) => void
}) {
  const { t } = useTranslation()
  const { amountsHidden } = useAmountPrivacy()
  const label =
    occurrence.customer_email ||
    occurrence.customer_wechat ||
    formatAccountName(
      occurrence.account_display_email || occurrence.account_name || occurrence.name,
      occurrence.account_space_role,
    )
  return (
    <button
      type="button"
      aria-label={label}
      onClick={() => onView(occurrence)}
      className={cn(
        "grid w-full gap-1 rounded-lg border px-3 py-2.5 text-left text-sm leading-5 outline-none transition-colors focus-visible:ring-2 focus-visible:ring-ring",
        occurrence.paid
          ? "border-success/25 bg-success/5 hover:bg-success/10"
          : "border-gold/25 bg-gold/5 hover:bg-gold/10",
      )}
    >
      <span className="min-w-0 break-all font-medium">{label}</span>
      <span className="flex flex-wrap items-center justify-between gap-x-3 gap-y-1 text-xs">
        <span className={occurrence.paid ? "text-success" : "text-gold"}>{occurrence.paid ? t("dueStatus.paid") : t("calendar.legendPending")}</span>
        <span className="tabular-nums text-muted-foreground">{maskAmount(amountsHidden, `¥${occurrence.price_yuan}`)}</span>
      </span>
    </button>
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
  const [expandedDay, setExpandedDay] = React.useState<CalendarDay | null>(null)

  return (
    <>
      <section aria-label={calendar.month_label}>
        <div className="grid grid-cols-7 border-b bg-muted/25" aria-hidden="true">
          {weekdays.map((weekday, index) => (
            <span
              key={weekday}
              className={cn(
                "px-2 py-2 text-left text-xs font-medium text-muted-foreground sm:px-3",
                index >= 5 && "text-muted-foreground/60",
              )}
            >
              {weekday}
            </span>
          ))}
        </div>
        <div className="grid grid-cols-7 gap-px bg-border/70">
          {calendar.days.map((day) => {
            const occurrences = day.occurrences ?? []
            const allPaid = occurrences.every((occurrence) => occurrence.paid)
            const navigatesMonth = !day.in_month
            const interactive = occurrences.length > 0 || navigatesMonth
            const Cell = interactive ? "button" : "div"
            return (
              <Cell
                key={day.date}
                type={interactive ? "button" : undefined}
                aria-label={occurrences.length > 0
                  ? t("calendar.dayEventCount", { date: day.date_label, count: occurrences.length })
                  : navigatesMonth ? t("calendar.dateOnly", { date: day.date_label }) : undefined}
                onClick={() => {
                  if (occurrences.length > 0) setExpandedDay(day)
                  else if (navigatesMonth) onSelectDay(day)
                }}
                className={cn(
                  "relative flex h-16 min-w-0 flex-col items-start justify-between px-2 py-1.5 text-left outline-none transition-colors sm:h-[clamp(3.5rem,calc((100dvh_-_21.25rem)/6),5rem)] sm:px-3 sm:py-2",
                  day.in_month ? "bg-card" : "bg-muted/15",
                  interactive && "cursor-pointer hover:bg-brand/[0.045] focus-visible:z-10 focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring",
                  day.is_today && "bg-brand/[0.035] ring-1 ring-inset ring-brand/25",
                )}
              >
                <span
                  className={cn(
                    "grid size-5 place-items-center text-xs font-medium tabular-nums sm:text-[13px]",
                    day.is_today
                      ? "rounded-full bg-brand font-semibold text-brand-foreground"
                      : day.in_month ? "text-foreground" : "text-muted-foreground/45",
                  )}
                >
                  {day.day_number}
                </span>
                {occurrences.length > 0 ? (
                  <span
                    className={cn(
                      "inline-flex min-w-6 items-center justify-center gap-1.5 self-end rounded-md px-1.5 py-0.5 text-xs font-semibold leading-5 tabular-nums sm:min-w-7 sm:px-2",
                      !day.in_month ? "text-muted-foreground/45"
                        : allPaid ? "bg-success/[0.07] text-success" : "bg-gold/[0.07] text-gold",
                    )}
                  >
                    {day.in_month ? <span className="hidden size-1 rounded-full bg-current sm:block" aria-hidden="true" /> : null}
                    {occurrences.length}
                  </span>
                ) : null}
              </Cell>
            )
          })}
        </div>
      </section>

      <Dialog
        open={expandedDay !== null}
        onOpenChange={(open) => {
          if (!open) setExpandedDay(null)
        }}
      >
        <DialogContent className="max-h-[85dvh] grid-rows-[auto_minmax(0,1fr)] overflow-hidden sm:max-w-md">
          <DialogHeader>
            <DialogTitle>{t("calendar.dayEventsTitle", { date: expandedDay?.date_label ?? "" })}</DialogTitle>
            <DialogDescription>
              {t("calendar.dayEventsDesc", { count: expandedDay?.occurrences?.length ?? 0 })}
            </DialogDescription>
          </DialogHeader>
          <div className="grid min-h-0 gap-2 overflow-y-auto overscroll-contain">
            {(expandedDay?.occurrences ?? []).map((occurrence) => (
              <EventPill
                key={`${occurrence.subscription_id}:${occurrence.due_date}`}
                occurrence={occurrence}
                onView={(selectedOccurrence) => {
                  setExpandedDay(null)
                  onViewOccurrence(selectedOccurrence)
                }}
              />
            ))}
          </div>
        </DialogContent>
      </Dialog>
    </>
  )
}

function MonthToolbar({
  calendar,
  onNavigateMonth,
  activity = false,
}: {
  calendar: CalendarMonth
  onNavigateMonth: (month: string) => void
  activity?: boolean
}) {
  const { t } = useTranslation()
  return (
    <div className="flex flex-wrap items-center justify-between gap-3 rounded-lg border bg-card px-3 py-2.5">
      <div className="flex items-center gap-1">
        <Button variant="ghost" size="icon-sm" aria-label={t("calendar.prevMonth")} onClick={() => onNavigateMonth(calendar.previous_month)}>
          <ChevronLeft />
        </Button>
        <h2 className="display-numeral min-w-28 text-center text-base font-semibold">{calendar.month_label}</h2>
        <Button variant="ghost" size="icon-sm" aria-label={t("calendar.nextMonth")} onClick={() => onNavigateMonth(calendar.next_month)}>
          <ChevronRight />
        </Button>
      </div>
      <div className="flex items-center gap-3">
        <div className="hidden items-center gap-3 text-xs text-muted-foreground sm:flex">
          <span className="flex items-center gap-1.5"><i className={cn("size-2 rounded-full", activity ? "bg-brand" : "bg-gold")} />{t(activity ? "calendar.activityCreated" : "calendar.legendPending")}</span>
          <span className="flex items-center gap-1.5"><i className="size-2 rounded-full bg-success" />{t(activity ? "calendar.activityPaid" : "calendar.legendPaid")}</span>
          {activity ? <span className="flex items-center gap-1.5"><i className="size-2 rounded-full bg-muted-foreground" />{t("calendar.activityArchived")}</span> : null}
        </div>
        <Button variant="outline" size="sm" onClick={() => onNavigateMonth(calendar.current_month)}>{t("calendar.today")}</Button>
      </div>
    </div>
  )
}

function TaskSummary({
  calendar,
  activeFilter,
  onFilter,
}: {
  calendar: CalendarMonth
  activeFilter: TaskFilter
  onFilter: (filter: TaskFilter) => void
}) {
  const { t } = useTranslation()
  const { amountsHidden } = useAmountPrivacy()
  const actionable = calendar.actionable_occurrences ?? []
  const overdue = actionable.filter((item) => item.days_remaining < 0).length
  const soon = actionable.filter((item) => item.days_remaining >= 0 && item.days_remaining <= 7).length
  const paid = calendar.paid_in_month_occurrences?.length ?? 0
  const items = [
    {
      filter: "all" as const,
      icon: CircleDollarSign,
      label: t("calendar.work.pendingAmount"),
      value: maskAmount(amountsHidden, `¥${calendar.pending_month_amount_yuan}`),
      hint: t("calendar.work.pendingCount", { count: calendar.pending_month_count }),
    },
    {
      filter: "overdue" as const,
      icon: Clock3,
      label: t("calendar.work.overdue"),
      value: String(overdue),
      hint: t("calendar.work.handleFirst"),
    },
    {
      filter: "soon" as const,
      icon: CalendarDays,
      label: t("calendar.work.dueSoon"),
      value: String(soon),
      hint: t("calendar.work.nextSevenDays"),
    },
    {
      filter: "paid" as const,
      icon: CheckCircle2,
      label: t("calendar.work.paidMonth"),
      value: String(paid),
      hint: t("calendar.work.recordedBills"),
    },
  ]

  return (
    <div className="grid grid-cols-2 gap-2 xl:grid-cols-4">
      {items.map((item) => {
        const Icon = item.icon
        const selected = activeFilter === item.filter
        return (
          <button
            key={item.filter}
            type="button"
            onClick={() => onFilter(item.filter)}
            className={cn(
              "rounded-lg border bg-card p-3 text-left outline-none transition-colors hover:bg-accent/25 focus-visible:ring-2 focus-visible:ring-brand/45",
              selected && "border-brand/35 bg-brand/[0.04] ring-1 ring-inset ring-brand/15",
            )}
          >
            <span className="flex items-center justify-between gap-2">
              <span className="text-xs font-medium text-muted-foreground">{item.label}</span>
              <Icon className={cn("size-4", item.filter === "overdue" ? "text-destructive" : "text-brand")} />
            </span>
            <span className="display-numeral mt-2 block text-xl font-semibold">{item.value}</span>
            <span className="mt-1 block truncate text-[10px] text-muted-foreground">{item.hint}</span>
          </button>
        )
      })}
    </div>
  )
}

function TaskRow({
  occurrence,
  onView,
  onPaid,
}: {
  occurrence: CalendarOccurrence
  onView: (occurrence: CalendarOccurrence) => void
  onPaid: (occurrence: CalendarOccurrence) => void
}) {
  const { t } = useTranslation()
  const { amountsHidden } = useAmountPrivacy()
  const plus = occurrence.business_type === "plus"
  const oneMonthRental = plus && isOneMonthRentalCron(occurrence.cron_expr)
  const identifier = occurrence.customer_email || occurrence.customer_wechat || occurrence.name
  return (
    <div className="flex min-w-0 flex-wrap items-center gap-3 rounded-lg border bg-card px-3 py-3 sm:flex-nowrap">
      <span
        className={cn(
          "grid size-9 shrink-0 place-items-center rounded-md",
          occurrence.paid
            ? "bg-success/10 text-success"
            : occurrence.days_remaining < 0
              ? "bg-destructive/10 text-destructive"
              : "bg-warning/15 text-warning-foreground dark:text-warning",
        )}
      >
        {occurrence.paid ? <CheckCircle2 className="size-4" /> : <Clock3 className="size-4" />}
      </span>
      <button type="button" className="min-w-0 flex-1 text-left" onClick={() => onView(occurrence)}>
        <span className="flex min-w-0 flex-wrap items-center gap-2">
          <strong className="truncate text-sm">{identifier}</strong>
          <Badge variant={plus ? "brand" : "secondary"}>{plus ? "Plus" : "Team"}</Badge>
          {occurrence.paid ? <Badge variant="success">{t("dueStatus.paid")}</Badge> : null}
        </span>
        <span className="mt-1 flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1 text-[11px] text-muted-foreground">
          <span>{occurrence.due_date}</span>
          <span className="truncate">
            {[formatAccountLabel(occurrence.account_serial, occurrence.account_display_email || occurrence.account_name, "", occurrence.account_space_role), occurrence.seat_name].filter(Boolean).join(" · ")}
          </span>
          <span>{occurrence.cycle_desc}</span>
        </span>
      </button>
      <span className="ml-12 shrink-0 text-sm font-semibold tabular-nums sm:ml-0">
        {maskAmount(amountsHidden, `¥${occurrence.price_yuan}`)}
      </span>
      <span
        className={cn(
          "w-20 shrink-0 text-right text-xs font-medium",
          occurrence.paid ? "text-success" : occurrence.days_remaining < 0 ? "text-destructive" : "text-muted-foreground",
        )}
      >
        {occurrence.paid
          ? t("dueStatus.paid")
          : occurrence.days_remaining < 0
            ? t("dueStatus.overdueDays", { count: Math.abs(occurrence.days_remaining) })
            : occurrence.days_remaining === 0
              ? t("dueStatus.today")
              : t("dueStatus.days", { count: occurrence.days_remaining })}
      </span>
      <div className="ml-auto flex shrink-0 items-center gap-2 sm:ml-0">
        {!occurrence.paid && !oneMonthRental ? (
          <Button size="sm" className="h-8 shadow-none" onClick={() => onPaid(occurrence)}>
            <ReceiptText />
            {t(plus ? "calendar.work.recordRenewal" : "calendar.work.recordPaid")}
          </Button>
        ) : null}
        <Button
          type="button"
          variant="outline"
          size="sm"
          className="h-8 text-muted-foreground hover:text-foreground"
          onClick={() => onView(occurrence)}
        >
          <Eye />
          {t("calendar.viewInfo")}
        </Button>
      </div>
    </div>
  )
}

function TaskWorkspace({
  calendar,
  onView,
  onPaid,
}: {
  calendar: CalendarMonth
  onView: (occurrence: CalendarOccurrence) => void
  onPaid: (occurrence: CalendarOccurrence) => void
}) {
  const { t } = useTranslation()
  const [filter, setFilter] = React.useState<TaskFilter>("all")
  const [page, setPage] = React.useState(1)
  const occurrences = React.useMemo(() => {
    const pending = (calendar.occurrences ?? []).filter((item) => !item.paid)
    const actionable = calendar.actionable_occurrences ?? []
    const filtered = filter === "overdue"
      ? actionable.filter((item) => item.days_remaining < 0)
      : filter === "soon"
        ? actionable.filter((item) => item.days_remaining >= 0 && item.days_remaining <= 7)
        : filter === "paid"
          ? calendar.paid_in_month_occurrences ?? []
          : pending
    return [...filtered].sort((left, right) => {
      const leftActionable = !left.paid && left.days_remaining <= 7
      const rightActionable = !right.paid && right.days_remaining <= 7
      const actionDelta = Number(rightActionable) - Number(leftActionable)
      if (actionDelta !== 0) return actionDelta
      return left.days_remaining - right.days_remaining
    })
  }, [calendar.actionable_occurrences, calendar.occurrences, calendar.paid_in_month_occurrences, filter])
  const pageCount = Math.max(1, Math.ceil(occurrences.length / TASKS_PER_PAGE))
  const currentPage = Math.min(page, pageCount)
  const pageOccurrences = occurrences.slice(
    (currentPage - 1) * TASKS_PER_PAGE,
    currentPage * TASKS_PER_PAGE,
  )
  const groups = [
    { key: "overdue", label: t("calendar.work.groupOverdue"), items: pageOccurrences.filter((item) => !item.paid && item.days_remaining < 0) },
    { key: "today", label: t("calendar.work.groupToday"), items: pageOccurrences.filter((item) => !item.paid && item.days_remaining === 0) },
    { key: "soon", label: t("calendar.work.groupSoon"), items: pageOccurrences.filter((item) => !item.paid && item.days_remaining > 0 && item.days_remaining <= 7) },
    { key: "later", label: t("calendar.work.groupLater"), items: pageOccurrences.filter((item) => !item.paid && item.days_remaining > 7) },
    { key: "paid", label: t("calendar.work.groupPaid"), items: pageOccurrences.filter((item) => item.paid) },
  ].filter((group) => group.items.length > 0)

  return (
    <div className="grid gap-4">
      <TaskSummary
        calendar={calendar}
        activeFilter={filter}
        onFilter={(nextFilter) => {
          setFilter(nextFilter)
          setPage(1)
        }}
      />
      {groups.length === 0 ? (
        <div className="grid min-h-72 place-items-center rounded-lg border border-dashed text-center">
          <div>
            <CheckCircle2 className="mx-auto size-8 text-success" />
            <p className="mt-3 text-sm font-medium">{t("calendar.work.empty")}</p>
          </div>
        </div>
      ) : (
        <div className="grid gap-4">
          {groups.map((group) => (
            <section key={group.key} aria-labelledby={`calendar-task-${group.key}`}>
              <div className="mb-2 flex items-center gap-2">
                <h3 id={`calendar-task-${group.key}`} className="text-xs font-semibold">{group.label}</h3>
                <Badge variant={group.key === "overdue" ? "destructive" : group.key === "paid" ? "success" : "secondary"}>{group.items.length}</Badge>
              </div>
              <div className="grid gap-2">
                {group.items.map((occurrence) => (
                  <TaskRow
                    key={`${occurrence.subscription_id}:${occurrence.due_date}`}
                    occurrence={occurrence}
                    onView={onView}
                    onPaid={onPaid}
                  />
                ))}
              </div>
            </section>
          ))}
          {pageCount > 1 ? (
            <div className="flex items-center justify-between gap-3 border-t pt-3">
              <p className="text-xs text-muted-foreground">
                {t("statDetails.pageStatus", { page: currentPage, pageCount })}
              </p>
              <div className="flex items-center gap-2">
                <Button
                  variant="outline"
                  size="sm"
                  disabled={currentPage <= 1}
                  onClick={() => setPage((value) => Math.max(1, value - 1))}
                >
                  <ChevronLeft />
                  {t("statDetails.prevPage")}
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  disabled={currentPage >= pageCount}
                  onClick={() => setPage((value) => Math.min(pageCount, value + 1))}
                >
                  {t("statDetails.nextPage")}
                  <ChevronRight />
                </Button>
              </div>
            </div>
          ) : null}
        </div>
      )}
    </div>
  )
}

export function CalendarPage() {
  const { t } = useTranslation()
  const [searchParams, setSearchParams] = useSearchParams()
  const month = searchParams.get("month") ?? ""
  const requestedView = searchParams.get("view")
  const view: CalendarView = requestedView === "calendar" || requestedView === "activity" || requestedView === "deferred" ? requestedView : "tasks"
  const calendarQuery = useCalendar(month || undefined)
  const [seatInfo, setSeatInfo] = React.useState<SeatSubscriptionInfo | null>(null)
  const [duePaidTarget, setDuePaidTarget] = React.useState<DuePaidTarget | null>(null)
  const calendar = calendarQuery.data

  const openOccurrence = (occurrence: CalendarOccurrence) => {
    setSeatInfo(seatInfoFromOccurrence(occurrence, t))
  }

  const updateParams = (updates: { month?: string; view?: CalendarView }) => {
    const next = new URLSearchParams(searchParams)
    if (updates.month !== undefined) {
      if (updates.month) next.set("month", updates.month)
      else next.delete("month")
    }
    if (updates.view !== undefined) {
      if (updates.view === "tasks") next.delete("view")
      else next.set("view", updates.view)
    }
    setSearchParams(next)
  }

  const openPaid = (occurrence: CalendarOccurrence) => {
    setDuePaidTarget({
      subscriptionId: occurrence.subscription_id,
      name: occurrence.customer_email || occurrence.customer_wechat || occurrence.name,
      priceYuan: occurrence.price_yuan,
      cycleDesc: occurrence.cycle_desc,
      dueDate: occurrence.due_date,
      kind: occurrence.business_type === "plus" ? "plus" : "team",
    })
  }

  return (
    <>
      <PageHeader title={t("calendar.title")} />
      {calendarQuery.isPending ? (
        <div className="grid gap-3">
          <Skeleton className="h-12 rounded-lg" />
          <Skeleton className="h-[600px] rounded-lg" />
        </div>
      ) : calendarQuery.isError ? (
        <Card className="items-center gap-3 py-16 text-center">
          <p className="text-sm text-muted-foreground">{t("common.loadFailed")}</p>
          <Button variant="outline" onClick={() => void calendarQuery.refetch()}>{t("common.retry")}</Button>
        </Card>
      ) : calendar ? (
        <div className="grid gap-3">
          {view !== "deferred" ? <MonthToolbar calendar={calendar} activity={view === "activity"} onNavigateMonth={(value) => updateParams({ month: value })} /> : null}
          <Tabs value={view} onValueChange={(value) => updateParams({ view: value as CalendarView })}>
            <TabsList className="h-auto min-h-8 max-w-full flex-wrap justify-start" aria-label={t("calendar.workspaceLabel")}>
              <TabsTrigger value="tasks"><ListTodo />{t("calendar.viewTasks")}</TabsTrigger>
              <TabsTrigger value="calendar"><CalendarDays />{t("calendar.viewCalendar")}</TabsTrigger>
              <TabsTrigger value="activity"><History />{t("calendar.viewActivity")}</TabsTrigger>
              <TabsTrigger value="deferred"><Pause />{t("calendar.deferred.tab")}<span className="tabular-nums">{calendar.deferred_subscriptions?.length ?? 0}</span></TabsTrigger>
            </TabsList>
            <TabsContent value="tasks" className="mt-3">
              <TaskWorkspace
                key={calendar.month_value}
                calendar={calendar}
                onView={openOccurrence}
                onPaid={openPaid}
              />
            </TabsContent>
            <TabsContent value="calendar" className="mt-3">
              <Card className="gap-0 overflow-x-auto overflow-y-hidden p-0">
                <MonthGrid
                  calendar={calendar}
                  onSelectDay={(day) => {
                    if (!day.in_month) updateParams({ month: day.date.slice(0, 7) })
                  }}
                  onViewOccurrence={openOccurrence}
                />
              </Card>
            </TabsContent>
            <TabsContent value="activity" className="mt-3">
              <ActivityWorkspace
                items={calendar.activity ?? []}
                onView={(item) => setSeatInfo(seatInfoFromActivity(item, t))}
              />
            </TabsContent>
            <TabsContent value="deferred" className="mt-3">
              <DeferredWorkspace
                subscriptions={calendar.deferred_subscriptions ?? []}
                onView={(selected) => setSeatInfo({
                  subscriptionId: selected.subscription.id,
                  businessType: "team",
                  name: selected.subscription.name,
                  accountName: formatAccountLabel(selected.account_serial, selected.account_display_email || selected.account_name, "—", selected.account_space_role),
                  seatName: selected.seat_name,
                  statusLabel: t("calendar.deferred.status"),
                  statusTone: "warning",
                  priceYuan: selected.price_yuan,
                  costYuan: selected.allocated_cost_yuan || selected.cost_yuan,
                  profitYuan: selected.allocated_profit_yuan || selected.profit_yuan,
                  cycleDesc: selected.cycle_desc,
                  cronExpr: selected.subscription.cron_expr,
                  offsetsText: selected.offsets_text,
                  channelLabels: selected.channel_labels?.join(" / ") || "—",
                  boardedAt: selected.boarded_at,
                  extraDateLabel: t("calendar.dueDate"),
                  extraDate: selected.subscription.renewal_deferred_due_date,
                  customerEmail: selected.subscription.customer_email,
                  customerWechat: selected.subscription.customer_wechat,
                  remark: selected.subscription.remark,
                  tradeUrl: selected.subscription.trade_url,
                  archived: false,
                })}
                onPaid={(selected) => setDuePaidTarget({
                  subscriptionId: selected.subscription.id,
                  name: selected.subscription.customer_email || selected.subscription.name,
                  priceYuan: selected.price_yuan,
                  cycleDesc: selected.cycle_desc,
                  dueDate: selected.subscription.renewal_deferred_due_date,
                  kind: "team",
                })}
              />
            </TabsContent>
          </Tabs>
        </div>
      ) : null}

      <SeatSubscriptionDialog
        open={seatInfo !== null}
        onOpenChange={(open) => {
          if (!open) setSeatInfo(null)
        }}
        info={seatInfo}
      />
      <DuePaidDialog
        open={duePaidTarget !== null}
        onOpenChange={(open) => {
          if (!open) setDuePaidTarget(null)
        }}
        target={duePaidTarget}
      />
    </>
  )
}
