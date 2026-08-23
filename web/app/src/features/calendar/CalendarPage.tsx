import * as React from "react"
import { Link, useSearchParams } from "react-router-dom"
import { useTranslation } from "react-i18next"
import {
  ArrowRight,
  CalendarDays,
  CheckCircle2,
  ChevronLeft,
  ChevronRight,
  CircleDollarSign,
  Clock3,
  History,
  ListTodo,
  ReceiptText,
  UserRoundX,
} from "lucide-react"

import { useCalendar } from "@/api/queries"
import type { CalendarDay, CalendarMonth, CalendarOccurrence, SubscriptionView } from "@/api/types"
import { PageHeader } from "@/components/page-header"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { Skeleton } from "@/components/ui/skeleton"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { useAmountPrivacy } from "@/hooks/use-amount-privacy"
import { maskAmount } from "@/lib/amount-privacy"
import { cn } from "@/lib/utils"
import { isOneMonthRentalCron } from "@/features/plus-rentals/rental-mode"
import { DuePaidDialog, type DuePaidTarget } from "./DuePaidDialog"
import {
  seatInfoFromOccurrence,
  type SeatSubscriptionInfo,
} from "./seat-subscription-info"
import { SeatSubscriptionDialog } from "./SeatSubscriptionDialog"

type CalendarView = "tasks" | "calendar" | "activity"
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
              ? "border-success/35 bg-success/15 text-success line-through decoration-success/55 shadow-[inset_3px_0_0_var(--success)] hover:bg-success/20"
              : "border-gold/40 bg-gold/15 text-foreground shadow-[inset_3px_0_0_var(--gold)] hover:bg-gold/20",
          )}
        >
          <span className="flex min-w-0 items-center gap-1">
            <span className="min-w-0 truncate">{label}</span>
          </span>
        </button>
      </TooltipTrigger>
      <TooltipContent side="top" className="grid gap-0.5">
        <span className="font-semibold">{occurrence.account_name || occurrence.name}</span>
        {occurrence.customer_email ? <span>{occurrence.customer_email}</span> : null}
        {!occurrence.customer_email && occurrence.customer_wechat ? <span>{occurrence.customer_wechat}</span> : null}
        <span className="tabular-nums">{occurrence.due_date} · ¥{occurrence.price_yuan}</span>
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
              aria-label={navigatesMonth ? t("calendar.dateOnly", { date: day.date_label }) : undefined}
              onClick={() => navigatesMonth && onSelectDay(day)}
              onKeyDown={(event) => {
                if (!navigatesMonth) return
                if (event.key === "Enter" || event.key === " ") {
                  event.preventDefault()
                  onSelectDay(day)
                }
              }}
              className={cn(
                "relative flex flex-col gap-1 p-1.5 outline-none transition-colors",
                (day.occurrences?.length ?? 0) > 0 ? "min-h-[72px] sm:min-h-[88px]" : "min-h-[44px] sm:min-h-[52px]",
                day.in_month ? "bg-card" : "cursor-pointer bg-muted/40 text-muted-foreground/70 hover:bg-accent",
              )}
            >
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

function MonthToolbar({
  calendar,
  onNavigateMonth,
}: {
  calendar: CalendarMonth
  onNavigateMonth: (month: string) => void
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
          <span className="flex items-center gap-1.5"><i className="size-2 rounded-full bg-gold" />{t("calendar.legendPending")}</span>
          <span className="flex items-center gap-1.5"><i className="size-2 rounded-full bg-success" />{t("calendar.legendPaid")}</span>
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
  const pending = (calendar.occurrences ?? []).filter((item) => !item.paid)
  const overdue = pending.filter((item) => item.days_remaining < 0).length
  const soon = pending.filter((item) => item.days_remaining >= 0 && item.days_remaining <= 7).length
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
  const route = plus
    ? `/plus-rentals?q=${encodeURIComponent(occurrence.customer_email || occurrence.customer_wechat || occurrence.name)}`
    : `/users?subscription=${occurrence.subscription_id}`
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
          <span className="truncate">{[occurrence.account_name, occurrence.seat_name].filter(Boolean).join(" · ")}</span>
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
      <div className="ml-auto flex items-center gap-1.5 sm:ml-0">
        {!occurrence.paid && !oneMonthRental ? (
          <Button size="sm" className="h-8" onClick={() => onPaid(occurrence)}>
            <ReceiptText />
            {t(plus ? "calendar.work.recordRenewal" : "calendar.work.recordPaid")}
          </Button>
        ) : null}
        <Button asChild variant="outline" size="sm" className="h-8">
          <Link to={route}>
            {plus ? t("calendar.work.contact") : t("calendar.manageUser")}
            <ArrowRight />
          </Link>
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
    const filtered = filter === "overdue"
      ? pending.filter((item) => item.days_remaining < 0)
      : filter === "soon"
        ? pending.filter((item) => item.days_remaining >= 0 && item.days_remaining <= 7)
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
  }, [calendar.occurrences, calendar.paid_in_month_occurrences, filter])
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

type ActivityItem =
  | { id: string; kind: "paid"; date: string; occurrence: CalendarOccurrence }
  | { id: string; kind: "archived"; date: string; subscription: SubscriptionView }

function ActivityWorkspace({ calendar, onView }: { calendar: CalendarMonth; onView: (occurrence: CalendarOccurrence) => void }) {
  const { t } = useTranslation()
  const { amountsHidden } = useAmountPrivacy()
  const items: ActivityItem[] = [
    ...(calendar.paid_in_month_occurrences ?? []).map((occurrence) => ({
      id: `paid:${occurrence.subscription_id}:${occurrence.due_date}`,
      kind: "paid" as const,
      date: occurrence.due_date,
      occurrence,
    })),
    ...(calendar.archived_subscriptions ?? []).map((subscription) => ({
      id: `archived:${subscription.subscription.id}`,
      kind: "archived" as const,
      date: subscription.archived_at_label,
      subscription,
    })),
  ].sort((left, right) => right.date.localeCompare(left.date))

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
    <div className="relative grid gap-0 pl-4 before:absolute before:bottom-3 before:left-[25px] before:top-3 before:w-px before:bg-border">
      {items.map((item) => {
        const paid = item.kind === "paid"
        const occurrence = paid ? item.occurrence : null
        const subscription = !paid ? item.subscription : null
        const identifier = paid
          ? occurrence?.customer_email || occurrence?.customer_wechat || occurrence?.name
          : subscription?.subscription.customer_email || subscription?.subscription.name
        return (
          <button
            key={item.id}
            type="button"
            onClick={() => occurrence && onView(occurrence)}
            className="group relative flex min-w-0 items-center gap-3 py-3 pl-7 text-left"
          >
            <span className={cn("absolute left-0 z-10 grid size-5 place-items-center rounded-full ring-4 ring-background", paid ? "bg-success text-success-foreground" : "bg-muted text-muted-foreground")}>
              {paid ? <CheckCircle2 className="size-3" /> : <UserRoundX className="size-3" />}
            </span>
            <span className="min-w-0 flex-1 rounded-lg border bg-card px-3 py-3 transition-colors group-hover:bg-accent/20">
              <span className="flex flex-wrap items-center justify-between gap-2">
                <strong className="truncate text-sm">{identifier}</strong>
                <span className="text-xs text-muted-foreground">{item.date}</span>
              </span>
              <span className="mt-1 flex flex-wrap items-center gap-3 text-[11px] text-muted-foreground">
                <span className={paid ? "text-success" : "text-foreground"}>{paid ? t("calendar.activityPaid") : t("calendar.activityArchived")}</span>
                {occurrence ? <span>{maskAmount(amountsHidden, `¥${occurrence.price_yuan}`)}</span> : null}
                <span>{occurrence?.account_name || subscription?.account_name}</span>
              </span>
            </span>
          </button>
        )
      })}
    </div>
  )
}

export function CalendarPage() {
  const { t } = useTranslation()
  const [searchParams, setSearchParams] = useSearchParams()
  const month = searchParams.get("month") ?? ""
  const requestedView = searchParams.get("view")
  const view: CalendarView = requestedView === "calendar" || requestedView === "activity" ? requestedView : "tasks"
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
          <MonthToolbar calendar={calendar} onNavigateMonth={(value) => updateParams({ month: value })} />
          <Tabs value={view} onValueChange={(value) => updateParams({ view: value as CalendarView })}>
            <TabsList aria-label={t("calendar.workspaceLabel")}>
              <TabsTrigger value="tasks"><ListTodo />{t("calendar.viewTasks")}</TabsTrigger>
              <TabsTrigger value="calendar"><CalendarDays />{t("calendar.viewCalendar")}</TabsTrigger>
              <TabsTrigger value="activity"><History />{t("calendar.viewActivity")}</TabsTrigger>
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
              <Card className="gap-0 overflow-hidden p-0">
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
                calendar={calendar}
                onView={(occurrence) => setSeatInfo(seatInfoFromOccurrence(occurrence, t))}
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
