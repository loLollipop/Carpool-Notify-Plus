import * as React from "react"
import { useTranslation } from "react-i18next"
import { useSearchParams } from "react-router-dom"
import { CalendarRange, ChevronLeft, ChevronRight, Plus, X } from "lucide-react"

import { archiveSubscription, setDuePaid, softDeleteSubscription } from "@/api/endpoints"
import { useAppMutation } from "@/api/mutations"
import { useCalendar, useDashboard } from "@/api/queries"
import type {
  CalendarDay,
  CalendarMonth,
  CalendarOccurrence,
  Dashboard,
  SubscriptionView,
} from "@/api/types"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { KpiSection, KpiSectionSkeleton } from "@/components/kpi-section"
import { PageHeader } from "@/components/page-header"
import { ViewSwitcher } from "@/components/view-switcher"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { Skeleton } from "@/components/ui/skeleton"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { cn } from "@/lib/utils"
import { ReminderPreviewDialog } from "@/features/subscriptions/ReminderPreviewDialog"
import { SubscriptionDialog } from "@/features/subscriptions/SubscriptionDialog"
import {
  prefillFromOccurrence,
  type SubscriptionPrefill,
} from "@/features/subscriptions/subscription-prefill"
import { AgendaArchivedRow, AgendaOccurrenceRow } from "./agenda-rows"
import { DuePaidDialog, type DuePaidTarget } from "./DuePaidDialog"
import {
  SeatSubscriptionDialog,
  seatInfoFromArchived,
  seatInfoFromOccurrence,
  type SeatSubscriptionInfo,
} from "./SeatSubscriptionDialog"

type AgendaFilter = "all" | "pending" | "paid" | "archived" | "resale"

function isPaymentDue(occurrence: CalendarOccurrence) {
  return !occurrence.paid && occurrence.days_remaining <= 0
}

// ---- Month grid -------------------------------------------------------------------

function EventPill({ occurrence }: { occurrence: CalendarOccurrence }) {
  const { t } = useTranslation()
  return (
    <Tooltip delayDuration={150}>
      <TooltipTrigger asChild>
        <span
          className={cn(
            "block w-full truncate rounded-[4px] border-l-2 px-1.5 py-0.5 text-left text-[11px] leading-4",
            occurrence.paid
              ? "border-success/50 bg-success/10 text-muted-foreground line-through decoration-muted-foreground/40 dark:bg-success/15"
              : "border-brand bg-brand/10 text-foreground dark:bg-brand/15",
          )}
        >
          {occurrence.name}
        </span>
      </TooltipTrigger>
      <TooltipContent side="top" className="grid gap-0.5">
        <span className="font-semibold">{occurrence.name}</span>
        <span>
          {occurrence.account_name}
          {occurrence.seat_name ? ` / ${occurrence.seat_name}` : ""}
        </span>
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
  selectedDate,
  onSelectDay,
}: {
  calendar: CalendarMonth
  selectedDate: string
  onSelectDay: (day: CalendarDay) => void
}) {
  const { t } = useTranslation()
  const weekdays = t("calendar.weekdays", { returnObjects: true }) as string[]

  return (
    <section aria-label={calendar.month_label}>
      <div className="grid grid-cols-7 border-b" aria-hidden="true">
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
          const selected = selectedDate !== "" && day.date === selectedDate
          return (
            <div
              key={day.date}
              role="button"
              tabIndex={0}
              aria-pressed={selected}
              aria-label={t("calendar.dateOnly", { date: day.date_label })}
              onClick={() => onSelectDay(day)}
              onKeyDown={(event) => {
                if (event.key === "Enter" || event.key === " ") {
                  event.preventDefault()
                  onSelectDay(day)
                }
              }}
              className={cn(
                "relative flex cursor-pointer flex-col gap-1 p-1.5 outline-none transition-colors",
                (day.occurrences?.length ?? 0) > 0
                  ? "min-h-[72px] sm:min-h-[88px]"
                  : "min-h-[44px] sm:min-h-[52px]",
                day.in_month ? "bg-card hover:bg-accent" : "bg-muted/40 text-muted-foreground/70 hover:bg-accent",
                selected && "ring-2 ring-brand ring-inset",
              )}
            >
              <span className="flex items-center justify-between">
                <span
                  className={cn(
                    "text-xs tabular-nums",
                    day.is_today
                      ? "flex size-5 items-center justify-center rounded-full bg-brand font-semibold text-brand-foreground"
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
  onEdit,
  onSendReminder,
  onArchive,
  onTogglePaid,
  onSoftDelete,
  onViewOccurrence,
  onViewArchived,
  paidToggleBusy,
}: {
  calendar: CalendarMonth
  dashboard: Dashboard | undefined
  onNavigateMonth: (month: string) => void
  onEdit: (occurrence: CalendarOccurrence) => void
  onSendReminder: (occurrence: CalendarOccurrence) => void
  onArchive: (occurrence: CalendarOccurrence) => void
  onTogglePaid: (occurrence: CalendarOccurrence, paid: boolean) => void
  onSoftDelete: (view: SubscriptionView) => void
  onViewOccurrence: (occurrence: CalendarOccurrence) => void
  onViewArchived: (view: SubscriptionView) => void
  paidToggleBusy: boolean
}) {
  const { t } = useTranslation()
  const [filter, setFilter] = React.useState<AgendaFilter>("all")
  const [selectedDate, setSelectedDate] = React.useState("")
  const [selectedDateLabel, setSelectedDateLabel] = React.useState("")
  const agendaRef = React.useRef<HTMLDivElement | null>(null)

  const scrollToAgenda = () => {
    agendaRef.current?.scrollIntoView({ behavior: "smooth", block: "start" })
  }

  const handleSelectDay = (day: CalendarDay) => {
    if (!day.in_month) {
      onNavigateMonth(day.date.slice(0, 7))
      return
    }
    const next = selectedDate === day.date ? "" : day.date
    setSelectedDate(next)
    setSelectedDateLabel(next === "" ? "" : day.date_label)
  }

  const focusRows = calendar.occurrences ?? []
  const paidTabRows = calendar.paid_in_month_occurrences ?? []
  const archivedRows = calendar.archived_subscriptions ?? []

  // 「全部」展示所有种类（到期事项 + 已下车）；具体种类各自收敛。
  let visibleOccurrences: CalendarOccurrence[]
  let showArchived: boolean
  if (filter === "archived") {
    visibleOccurrences = []
    showArchived = true
  } else if (filter === "pending") {
    // Keep this aligned with calendar.pending_count: due today or overdue, not future cycles.
    visibleOccurrences = focusRows.filter(
      (occurrence) =>
        isPaymentDue(occurrence) && occurrence.due_date.slice(0, 7) === calendar.month_value,
    )
    showArchived = false
  } else if (filter === "paid") {
    visibleOccurrences = paidTabRows
    showArchived = false
  } else if (filter === "resale") {
    visibleOccurrences = focusRows.filter((occurrence) => occurrence.is_resale)
    showArchived = true
  } else {
    // 「全部」= 焦点行（未交 / 已交清时的最新已交）+ 已下车。
    visibleOccurrences = focusRows
    showArchived = true
  }
  if (selectedDate !== "") {
    visibleOccurrences = visibleOccurrences.filter(
      (occurrence) => occurrence.due_date === selectedDate,
    )
  }
  // 已下车按下车时间（archived_at_label 的日期部分）参与日期筛选；串货筛选时只留串货。
  const visibleArchived: SubscriptionView[] = showArchived
    ? archivedRows.filter((view) => {
        if (filter === "resale" && !view.subscription.is_resale) return false
        return selectedDate === "" || view.archived_at_label.slice(0, 10) === selectedDate
      })
    : []

  const emptyText = (() => {
    if (focusRows.length === 0 && archivedRows.length === 0) return t("calendar.emptyMonth")
    if (filter === "archived") return t("calendar.emptyArchived")
    if (filter === "pending") return t("calendar.emptyPending")
    if (filter === "resale") return t("calendar.emptyResale")
    return t("calendar.emptyFiltered")
  })()

  const filters: { value: AgendaFilter; label: string }[] = [
    { value: "all", label: t("calendar.filterAll") },
    { value: "pending", label: t("calendar.filterPending") },
    { value: "paid", label: t("calendar.filterPaid") },
    { value: "archived", label: t("calendar.filterArchived") },
    { value: "resale", label: t("calendar.filterResale") },
  ]

  // 标题与计数跟随当前筛选口径（含已选日期）。
  const titleByFilter: Record<AgendaFilter, string> = {
    all: t("calendar.filterAll"),
    pending: t("calendar.filterPending"),
    paid: t("calendar.filterPaid"),
    archived: t("calendar.filterArchived"),
    resale: t("calendar.filterResale"),
  }
  const visibleCount = visibleOccurrences.length + visibleArchived.length

  return (
    <>
      {dashboard ? (
        <KpiSection
          dashboard={dashboard}
          pendingCount={calendar.pending_count}
          onFilterAll={() => {
            setFilter("all")
            scrollToAgenda()
          }}
          onFilterPending={() => {
            setFilter("pending")
            scrollToAgenda()
          }}
        />
      ) : (
        <KpiSectionSkeleton />
      )}

      <div className="grid items-start gap-4 lg:grid-cols-[minmax(0,1fr)_360px]">
        <Card className="gap-0 overflow-hidden p-0 animate-fade-up">
          <div className="flex flex-wrap items-center justify-between gap-3 border-b px-4 py-3">
            <div className="flex items-center gap-1">
              <Button
                variant="ghost"
                size="icon-sm"
                aria-label={t("calendar.prevMonth")}
                onClick={() => onNavigateMonth(calendar.previous_month)}
              >
                <ChevronLeft />
              </Button>
              <h2 className="display-numeral min-w-28 text-center text-base font-semibold">
                {calendar.month_label}
              </h2>
              <Button
                variant="ghost"
                size="icon-sm"
                aria-label={t("calendar.nextMonth")}
                onClick={() => onNavigateMonth(calendar.next_month)}
              >
                <ChevronRight />
              </Button>
            </div>
            <div className="flex items-center gap-3">
              <div className="hidden items-center gap-3 text-xs text-muted-foreground sm:flex">
                <span className="flex items-center gap-1.5">
                  <i className="size-2 rounded-full bg-brand" />
                  {t("calendar.legendPending")}
                </span>
                <span className="flex items-center gap-1.5">
                  <i className="size-2 rounded-full bg-success/70" />
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
            selectedDate={selectedDate}
            onSelectDay={handleSelectDay}
          />
        </Card>

        <Card
          ref={agendaRef}
          className="h-fit w-full gap-0 self-start scroll-mt-20 p-0 animate-fade-up lg:sticky lg:top-20 lg:max-h-[calc(100dvh-5.5rem)] lg:overflow-y-auto"
          style={{ animationDelay: "80ms" }}
          aria-label={t("calendar.agendaTitle")}
        >
          <div className="sticky top-0 z-10 border-b bg-card px-4 py-3.5">
            <div className="flex items-baseline justify-between">
              <h2 className="flex items-center gap-2 text-base font-semibold">
                <CalendarRange className="size-4 text-brand" />
                {titleByFilter[filter]}
              </h2>
              <span className="text-xs text-muted-foreground">
                <strong className="display-numeral text-sm text-foreground">
                  {visibleCount}
                </strong>{" "}
                {t("calendar.timesSuffix")}
              </span>
            </div>
            <div className="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-xs text-muted-foreground">
              <span>
                {t("calendar.summaryPaid")}{" "}
                <strong className="tabular-nums text-foreground">{calendar.paid_count}</strong>
              </span>
              <span>
                {t("calendar.summaryPending")}{" "}
                <strong className="tabular-nums text-foreground">{calendar.pending_count}</strong>
              </span>
              <span>
                {t("calendar.summaryArchived")}{" "}
                <strong className="tabular-nums text-foreground">
                  {calendar.archived_count}
                </strong>
              </span>
            </div>
            <div
              role="group"
              aria-label={t("calendar.agendaTitle")}
              className="mt-3 flex flex-wrap gap-1"
            >
              {filters.map((item) => (
                <button
                  key={item.value}
                  type="button"
                  onClick={() => setFilter(item.value)}
                  className={cn(
                    "rounded-full border px-2.5 py-1 text-xs font-medium transition-colors",
                    filter === item.value
                      ? "border-primary bg-primary text-primary-foreground"
                      : "text-muted-foreground hover:bg-accent hover:text-foreground",
                  )}
                >
                  {item.label}
                </button>
              ))}
            </div>
            {selectedDate !== "" ? (
              <div className="mt-2 inline-flex items-center gap-1.5 rounded-full bg-brand/10 py-1 pr-1 pl-2.5 text-xs font-medium text-brand">
                {t("calendar.dateOnly", { date: selectedDateLabel })}
                <button
                  type="button"
                  aria-label={t("calendar.clearDate")}
                  className="rounded-full p-0.5 transition-colors hover:bg-brand/15"
                  onClick={() => {
                    setSelectedDate("")
                    setSelectedDateLabel("")
                  }}
                >
                  <X className="size-3" />
                </button>
              </div>
            ) : null}
          </div>

          <div className="flex flex-col gap-2.5 p-3">
            {visibleOccurrences.map((occurrence) => (
              <AgendaOccurrenceRow
                key={`${occurrence.subscription_id}:${occurrence.due_date}`}
                occurrence={occurrence}
                onEdit={onEdit}
                onSendReminder={onSendReminder}
                onArchive={onArchive}
                onTogglePaid={onTogglePaid}
                onView={onViewOccurrence}
                paidToggleBusy={paidToggleBusy}
              />
            ))}
            {visibleArchived.map((view) => (
              <AgendaArchivedRow
                key={view.subscription.id}
                view={view}
                onSoftDelete={onSoftDelete}
                onView={onViewArchived}
              />
            ))}
            {visibleOccurrences.length === 0 && visibleArchived.length === 0 ? (
              <div className="py-8 text-center text-sm text-muted-foreground">{emptyText}</div>
            ) : null}
          </div>
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

  const [dialogOpen, setDialogOpen] = React.useState(false)
  const [editing, setEditing] = React.useState<SubscriptionPrefill | null>(null)
  const [reminderId, setReminderId] = React.useState<number | null>(null)
  const [archiveTarget, setArchiveTarget] = React.useState<{ id: number; name: string } | null>(null)
  const [softDeleteTarget, setSoftDeleteTarget] = React.useState<{ id: number; name: string } | null>(null)
  const [duePaidTarget, setDuePaidTarget] = React.useState<DuePaidTarget | null>(null)
  const [seatInfo, setSeatInfo] = React.useState<SeatSubscriptionInfo | null>(null)

  const unmarkMutation = useAppMutation(
    ({ id, due }: { id: number; due: string }) => setDuePaid(id, due, false),
    { successMessage: t("duePaid.unmarked") },
  )
  const archiveMutation = useAppMutation((id: number) => archiveSubscription(id), {
    onSuccess: () => setArchiveTarget(null),
  })
  const softDeleteMutation = useAppMutation((id: number) => softDeleteSubscription(id), {
    onSuccess: () => setSoftDeleteTarget(null),
  })

  const calendar = calendarQuery.data

  const goToMonth = (value: string) => {
    setSearchParams(value ? { month: value } : {})
  }

  const openCreate = () => {
    setEditing(null)
    setDialogOpen(true)
  }
  const openEdit = (occurrence: CalendarOccurrence) => {
    setEditing(prefillFromOccurrence(occurrence))
    setDialogOpen(true)
  }
  const handleTogglePaid = (occurrence: CalendarOccurrence, paid: boolean) => {
    if (paid) {
      setDuePaidTarget({
        subscriptionId: occurrence.subscription_id,
        name: occurrence.name,
        priceYuan: occurrence.price_yuan,
        cycleDesc: occurrence.cycle_desc,
        dueDate: occurrence.due_date,
      })
    } else {
      unmarkMutation.mutate({ id: occurrence.subscription_id, due: occurrence.due_date })
    }
  }

  return (
    <>
      <PageHeader
        title={t("calendar.title")}
        description={t("calendar.desc")}
        actions={
          <>
            <ViewSwitcher />
            <Button onClick={openCreate}>
              <Plus data-slot="icon" />
              {t("nav.newSubscription")}
            </Button>
          </>
        }
      />

      {calendarQuery.isPending ? (
        <>
          <KpiSectionSkeleton />
          <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_360px]">
            <Skeleton className="h-[560px] rounded-xl" />
            <Skeleton className="h-[560px] rounded-xl" />
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
          onEdit={openEdit}
          onSendReminder={(occurrence) => setReminderId(occurrence.subscription_id)}
          onArchive={(occurrence) =>
            setArchiveTarget({ id: occurrence.subscription_id, name: occurrence.name })
          }
          onTogglePaid={handleTogglePaid}
          onSoftDelete={(view) =>
            setSoftDeleteTarget({ id: view.subscription.id, name: view.subscription.name })
          }
          onViewOccurrence={(occurrence) => setSeatInfo(seatInfoFromOccurrence(occurrence, t))}
          onViewArchived={(view) => setSeatInfo(seatInfoFromArchived(view, t))}
          paidToggleBusy={unmarkMutation.isPending}
        />
      ) : null}

      <SubscriptionDialog open={dialogOpen} onOpenChange={setDialogOpen} prefill={editing} />
      <ReminderPreviewDialog
        open={reminderId !== null}
        onOpenChange={(open) => {
          if (!open) setReminderId(null)
        }}
        subscriptionId={reminderId}
      />
      <DuePaidDialog
        open={duePaidTarget !== null}
        onOpenChange={(open) => {
          if (!open) setDuePaidTarget(null)
        }}
        target={duePaidTarget}
      />
      <SeatSubscriptionDialog
        open={seatInfo !== null}
        onOpenChange={(open) => {
          if (!open) setSeatInfo(null)
        }}
        info={seatInfo}
      />
      <ConfirmDialog
        open={archiveTarget !== null}
        onOpenChange={(open) => {
          if (!open) setArchiveTarget(null)
        }}
        title={t("confirms.archiveTitle")}
        description={t("confirms.archiveDesc", { name: archiveTarget?.name ?? "" })}
        actionLabel={t("confirms.archiveAction")}
        destructive
        pending={archiveMutation.isPending}
        onConfirm={() => {
          if (archiveTarget) archiveMutation.mutate(archiveTarget.id)
        }}
      />
      <ConfirmDialog
        open={softDeleteTarget !== null}
        onOpenChange={(open) => {
          if (!open) setSoftDeleteTarget(null)
        }}
        title={t("confirms.softDeleteTitle")}
        description={t("confirms.softDeleteDesc", { name: softDeleteTarget?.name ?? "" })}
        actionLabel={t("confirms.softDeleteAction")}
        destructive
        pending={softDeleteMutation.isPending}
        onConfirm={() => {
          if (softDeleteTarget) softDeleteMutation.mutate(softDeleteTarget.id)
        }}
      />
    </>
  )
}
