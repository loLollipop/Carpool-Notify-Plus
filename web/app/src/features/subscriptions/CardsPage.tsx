import * as React from "react"
import { useTranslation } from "react-i18next"
import { useNavigate, useSearchParams } from "react-router-dom"
import {
  ChevronLeft,
  ChevronRight,
  Clock3,
  Mail,
  MessageCircle,
  Pencil,
  Receipt,
  Search,
  Snowflake,
  UserRoundMinus,
} from "lucide-react"

import { archiveSubscription } from "@/api/endpoints"
import { useAppMutation } from "@/api/mutations"
import { useCalendar, useDashboard, useSubscriptions } from "@/api/queries"
import type { CalendarOccurrence, SubscriptionView } from "@/api/types"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { DueStatusBadge } from "@/components/due-status-badge"
import {
  KpiSection,
  KpiSectionSkeleton,
  type KpiDetailKey,
} from "@/components/kpi-section"
import { PageHeader } from "@/components/page-header"
import {
  StatDetailDialog,
  type StatDetailState,
} from "@/components/stat-detail-dialog"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Skeleton } from "@/components/ui/skeleton"
import { DuePaidDialog, type DuePaidTarget } from "@/features/calendar/DuePaidDialog"
import { cn } from "@/lib/utils"
import { ReminderPreviewDialog } from "./ReminderPreviewDialog"
import { SubscriptionDialog } from "./SubscriptionDialog"
import { prefillFromView, type SubscriptionPrefill } from "./subscription-prefill"

type CardsFilter = "all" | "due" | "pending" | "paid" | "archived"

const EMPTY_SUBSCRIPTION_VIEWS: SubscriptionView[] = []
const USERS_PER_PAGE = 9

function normalizeCardsFilter(value: string | null): CardsFilter {
  if (
    value === "due" ||
    value === "pending" ||
    value === "paid" ||
    value === "archived"
  ) {
    return value
  }
  return "all"
}

function isTeamRenewalOccurrence(occurrence: CalendarOccurrence) {
  return (
    occurrence.business_type !== "plus" &&
    (!occurrence.boarded_at || occurrence.due_date > occurrence.boarded_at)
  )
}

function MetaCell({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="min-w-0">
      <div className="text-[11px] font-medium text-muted-foreground">{label}</div>
      <div className="truncate text-[13px]">{children}</div>
    </div>
  )
}

function cardRemark(remark: string) {
  return remark
    .split(/\r?\n/)
    .filter((line) => !/^兑换码\s*[：:]/.test(line.trim()))
    .join("\n")
    .trim()
}

function CustomerContactRow({
  email,
  wechat,
  plusRental,
}: {
  email: string
  wechat: string
  plusRental: boolean
}) {
  const { t } = useTranslation()

  return (
    <div className="mt-2 grid min-w-0 gap-1.5">
      {!plusRental ? (
        <span
          className="flex h-6 max-w-full min-w-0 items-center gap-1.5 rounded-md border border-brand/10 bg-brand/[0.06] px-2 text-xs text-muted-foreground"
          title={`${t("subscriptionDialog.customerEmail")}: ${email || t("cards.contactMissing")}`}
        >
          <Mail className="size-3.5 shrink-0" aria-hidden="true" />
          <span className="truncate font-mono">{email || t("cards.contactMissing")}</span>
        </span>
      ) : null}
      <span
        className="flex h-6 max-w-full min-w-0 items-center gap-1.5 rounded-md border border-success/10 bg-success/[0.06] px-2 text-xs text-muted-foreground"
        title={`${t("subscriptionDialog.customerWechat")}: ${wechat || t("cards.contactMissing")}`}
      >
        <MessageCircle className="size-3.5 shrink-0" aria-hidden="true" />
        <span className="truncate">{wechat || t("cards.contactMissing")}</span>
      </span>
    </div>
  )
}

function SubscriptionProgress({ view }: { view: SubscriptionView }) {
  const { t } = useTranslation()
  const cycleDays = Math.max(view.cycle_days || view.days_remaining || 1, 1)
  const remainingDays = Math.max(view.days_remaining, 0)
  const progress = Math.min(100, (remainingDays / cycleDays) * 100)
  const progressPercent = Math.round(progress)
  const progressColor = `hsl(${Math.round(progress * 1.2)} 72% 42%)`
  const progressWidth = view.days_remaining <= 0 ? "4px" : `${progress}%`
  const progressLabel =
    view.days_remaining <= 0
      ? t("cards.progressExpired")
      : t("cards.progressRemaining", { count: view.days_remaining })

  return (
    <div className="col-span-3 min-w-0 pt-0.5">
      <div className="mb-1.5 flex items-center justify-between gap-3 text-[11px] font-medium">
        <span className="text-muted-foreground">{t("cards.progress")}</span>
        <span className="shrink-0 tabular-nums" style={{ color: progressColor }}>
          {progressLabel} · {progressPercent}%
        </span>
      </div>
      <div
        className="h-2 overflow-hidden rounded-full bg-foreground/[0.08]"
        role="progressbar"
        aria-label={t("cards.progress")}
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={progressPercent}
        aria-valuetext={`${progressLabel}, ${progressPercent}%`}
      >
        <div
          className="h-full rounded-full transition-[width,background-color] duration-500"
          style={{ width: progressWidth, backgroundColor: progressColor }}
        />
      </div>
    </div>
  )
}

function SubscriptionCard({
  view,
  index,
  onEdit,
  onRenew,
  onSendReminder,
  onArchive,
  onGoAfterSales,
}: {
  view: SubscriptionView
  index: number
  onEdit: (view: SubscriptionView) => void
  onRenew: (view: SubscriptionView) => void
  onSendReminder: (view: SubscriptionView) => void
  onArchive: (view: SubscriptionView) => void
  onGoAfterSales: (caseId: number) => void
}) {
  const { t } = useTranslation()
  const subscription = view.subscription
  const plusRental = subscription.business_type === "plus"
  const visibleRemark = cardRemark(subscription.remark)
  const archived = subscription.archived_at !== null
  const cancellationPending = view.cancellation_pending
  const displayedCostYuan = view.allocated_cost_yuan || view.cost_yuan
  const displayedProfitYuan = view.allocated_profit_yuan || view.profit_yuan
  const accentClass = archived || cancellationPending
    ? "bg-muted-foreground/35"
    : view.days_remaining <= 0
      ? "bg-destructive"
      : view.days_remaining <= 7
        ? "bg-gold"
        : "bg-brand"

  return (
    <Card
      className={cn(
        "group relative gap-0 overflow-hidden p-5 transition-[border-color,background-color] duration-200 animate-fade-up hover:border-input hover:bg-accent/25",
        cancellationPending && "border-dashed bg-muted/55 text-muted-foreground hover:bg-muted/55",
      )}
      style={{ animationDelay: `${Math.min(index * 40, 320)}ms` }}
    >
      <span className={cn("absolute inset-y-0 left-0 w-1", accentClass)} />
      <div className="flex items-start justify-between gap-2">
        <div className="min-w-0">
          <h3 className="truncate text-[15px] font-semibold">
            {view.account_name || subscription.name}
          </h3>
          {plusRental || view.next_price_yuan ? (
            <div className="mt-1.5 flex flex-wrap items-center gap-1.5">
              {plusRental ? (
                <Badge className="border-brand/25 bg-brand/10 font-normal text-brand">
                  {t("cards.plusRental")}
                </Badge>
              ) : null}
              {view.next_price_yuan ? (
                <Badge variant="outline" className="border-gold/25 bg-gold/[0.07] font-normal text-gold">
                  {t("cards.nextPrice", { price: `¥${view.next_price_yuan}` })} · {view.next_price_effective_due_date}
                </Badge>
              ) : null}
            </div>
          ) : null}
          {visibleRemark ? (
            <p className="mt-1.5 line-clamp-2 text-xs text-muted-foreground">
              {visibleRemark}
            </p>
          ) : null}
          <CustomerContactRow
            email={subscription.customer_email}
            wechat={subscription.customer_wechat}
            plusRental={plusRental}
          />
        </div>
        {cancellationPending ? (
          <Badge variant="secondary" className="shrink-0 font-normal">
            <Clock3 />
            {t("cards.cancellationPending")}
          </Badge>
        ) : archived && view.seat_frozen_until_label ? (
          <Badge variant="outline" className="shrink-0 border-gold/30 bg-gold/[0.06] font-normal text-gold">
            <Snowflake />
            {t("cards.seatFrozen")}
          </Badge>
        ) : (
          <DueStatusBadge paid={false} daysRemaining={view.days_remaining} />
        )}
      </div>

      {cancellationPending ? (
        <div className="mt-3 flex items-start gap-2 rounded-md border border-dashed bg-background/55 px-3 py-2.5 text-xs leading-5">
          <Clock3 className="mt-0.5 size-3.5 shrink-0" />
          <span>
            {t("cards.cancellationPendingHint", {
              time: view.cancellation_expires_at_label,
            })}
          </span>
        </div>
      ) : null}

      {archived && view.seat_frozen_until_label ? (
        <div className="mt-3 flex items-start gap-2 rounded-md border border-dashed border-gold/25 bg-gold/[0.05] px-3 py-2.5 text-xs leading-5 text-gold">
          <Snowflake className="mt-0.5 size-3.5 shrink-0" />
          <span>{t("cards.seatFrozenUntil", { time: view.seat_frozen_until_label })}</span>
        </div>
      ) : null}

      <div className="mt-4 grid grid-cols-3 gap-x-3 gap-y-2.5 rounded-md border border-foreground/[0.07] bg-muted/40 p-3">
        <MetaCell label={t("cards.perPerson")}>
          <span className="tabular-nums">¥{view.price_yuan}</span>
        </MetaCell>
        <MetaCell label={t("cards.cost")}>
          <span className="font-medium text-gold tabular-nums">¥{displayedCostYuan}</span>
        </MetaCell>
        <MetaCell label={t("cards.profit")}>
          <span className="font-medium text-success tabular-nums">
            ¥{displayedProfitYuan}
          </span>
        </MetaCell>
        <MetaCell label={t("cards.cycle")}>{view.cycle_desc}</MetaCell>
        <MetaCell label={t("cards.nextDue")}>
          <span className="tabular-nums">{view.next_due_date}</span>
        </MetaCell>
        {view.boarded_at ? (
          <MetaCell label={t("cards.boardedAt")}>
            <span className="tabular-nums">{view.boarded_at}</span>
          </MetaCell>
        ) : null}
        <SubscriptionProgress view={view} />
      </div>

      {view.last_error ? (
        <p className="mt-3 text-xs text-destructive">
          {t("cards.lastError", { error: view.last_error })}
        </p>
      ) : null}

      <div className="-mx-5 -mb-5 mt-4 flex flex-wrap items-center gap-1.5 border-t bg-muted/20 px-5 py-3.5">
        {!cancellationPending ? (
          <Button variant="outline" size="sm" onClick={() => onEdit(view)}>
            <Pencil data-slot="icon" />
            {t("common.edit")}
          </Button>
        ) : null}
        {!archived && !cancellationPending ? (
          <Button variant="outline" size="sm" onClick={() => onRenew(view)}>
            <Receipt data-slot="icon" />
            {t("cards.renew")}
          </Button>
        ) : null}
        {!plusRental && !archived && !cancellationPending && subscription.customer_email ? (
          <Button variant="outline" size="sm" onClick={() => onSendReminder(view)}>
            <Mail data-slot="icon" />
            {t("cards.sendReminder")}
          </Button>
        ) : null}
        {!archived && !cancellationPending ? (
          <Button
            variant="ghost"
            size="sm"
            className="ml-auto text-destructive hover:text-destructive"
            onClick={() => onArchive(view)}
          >
            <UserRoundMinus data-slot="icon" />
            {plusRental ? t("cards.endRental") : t("calendar.getOff")}
          </Button>
        ) : null}
        {cancellationPending ? (
          <Button
            variant="outline"
            size="sm"
            className="ml-auto"
            onClick={() => onGoAfterSales(view.cancellation_case_id)}
          >
            <Clock3 data-slot="icon" />
            {t("cards.goAfterSales")}
          </Button>
        ) : null}
      </div>
    </Card>
  )
}

export function CardsPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [searchParams, setSearchParams] = useSearchParams()
  const routeSearch = searchParams.get("q") ?? ""
  const routeFilter = normalizeCardsFilter(searchParams.get("filter"))
  const routePage = Number.parseInt(searchParams.get("page") ?? "1", 10)
  const currentPage = Number.isFinite(routePage) && routePage > 0 ? routePage : 1
  const routeFocusId = Number.parseInt(searchParams.get("subscription") ?? "", 10)
  const focusedSubscriptionId = Number.isFinite(routeFocusId) ? routeFocusId : 0

  const subscriptionsQuery = useSubscriptions()
  const { refetch: refetchSubscriptions } = subscriptionsQuery
  const dashboardQuery = useDashboard()
  const calendarQuery = useCalendar()

  const [dialogOpen, setDialogOpen] = React.useState(false)
  const [editing, setEditing] = React.useState<SubscriptionPrefill | null>(null)
  const [duePaidTarget, setDuePaidTarget] = React.useState<DuePaidTarget | null>(null)
  const [reminderId, setReminderId] = React.useState<number | null>(null)
  const [statDetail, setStatDetail] = React.useState<StatDetailState | null>(null)
  const [archiveTarget, setArchiveTarget] = React.useState<{
    id: number
    name: string
    plusRental: boolean
  } | null>(null)
  const [cancellationResult, setCancellationResult] = React.useState<{
    caseId: number
    expiresAtLabel: string
  } | null>(null)
  const search = routeSearch
  const filter = routeFilter

  const archiveMutation = useAppMutation((id: number) => archiveSubscription(id), {
    onSuccess: (data) => {
      setArchiveTarget(null)
      if (data.archived) return
      setCancellationResult({
        caseId: data.case_id ?? 0,
        expiresAtLabel: data.expires_at_label ?? "",
      })
    },
  })

  const openEdit = (view: SubscriptionView) => {
    setEditing(prefillFromView(view))
    setDialogOpen(true)
  }
  const updateSearch = (value: string) => {
    const next = new URLSearchParams(searchParams)
    if (value.trim()) {
      next.set("q", value)
    } else {
      next.delete("q")
    }
    next.delete("page")
    next.delete("subscription")
    setSearchParams(next, { replace: true })
  }

  const updateFilter = (value: CardsFilter) => {
    const next = new URLSearchParams(searchParams)
    if (value === "all") {
      next.delete("filter")
    } else {
      next.set("filter", value)
    }
    next.delete("page")
    setSearchParams(next, { replace: true })
  }

  const activeViews = React.useMemo(
    () =>
      (subscriptionsQuery.data?.subscriptions ?? EMPTY_SUBSCRIPTION_VIEWS).filter(
        (view) => view.subscription.business_type !== "plus",
      ),
    [subscriptionsQuery.data?.subscriptions],
  )
  const archivedViews = React.useMemo(
    () =>
      (subscriptionsQuery.data?.archived ?? EMPTY_SUBSCRIPTION_VIEWS).filter(
        (view) => view.subscription.business_type !== "plus",
      ),
    [subscriptionsQuery.data?.archived],
  )
  const calendar = calendarQuery.data

  const nextCancellationExpiry = React.useMemo(() => {
    const expiries = activeViews
      .filter((view) => view.cancellation_pending && view.subscription.cancellation_expires_at)
      .map((view) => Date.parse(view.subscription.cancellation_expires_at ?? ""))
      .filter(Number.isFinite)
    return expiries.length > 0 ? Math.min(...expiries) : 0
  }, [activeViews])

  React.useEffect(() => {
    if (nextCancellationExpiry <= 0) return
    const delay = Math.max(1_000, nextCancellationExpiry - Date.now() + 1_000)
    const timer = window.setTimeout(() => {
      void refetchSubscriptions()
    }, delay)
    return () => window.clearTimeout(timer)
  }, [nextCancellationExpiry, refetchSubscriptions])

  const openRenew = (view: SubscriptionView) => {
    setDuePaidTarget({
      subscriptionId: view.subscription.id,
      name: view.account_name || view.subscription.name,
      priceYuan: view.price_yuan,
      cycleDesc: view.cycle_desc,
      dueDate: view.next_due_date,
    })
  }

  const teamRenewalStats = React.useMemo(() => {
    const pendingSubscriptionIds = new Set<number>()
    const paidSubscriptionIds = new Set<number>()
    for (const occurrence of calendar?.occurrences ?? []) {
      if (!isTeamRenewalOccurrence(occurrence)) continue
      if (occurrence.paid) {
        paidSubscriptionIds.add(occurrence.subscription_id)
      } else {
        pendingSubscriptionIds.add(occurrence.subscription_id)
      }
    }
    for (const occurrence of calendar?.paid_in_month_occurrences ?? []) {
      if (!isTeamRenewalOccurrence(occurrence)) continue
      paidSubscriptionIds.add(occurrence.subscription_id)
    }
    const renewedSubscriptionIds = new Set(
      [...paidSubscriptionIds].filter(
        (subscriptionId) => !pendingSubscriptionIds.has(subscriptionId),
      ),
    )
    return {
      pendingCount: pendingSubscriptionIds.size,
      renewedCount: renewedSubscriptionIds.size,
      pendingSubscriptionIds,
      renewedSubscriptionIds,
    }
  }, [calendar])

  const filteredViews = React.useMemo(() => {
    let pool: SubscriptionView[]
    if (filter === "archived") {
      pool = archivedViews
    } else if (filter === "due") {
      pool = activeViews.filter(
        (view) =>
          !view.cancellation_pending &&
          view.days_remaining >= 0 &&
          view.days_remaining <= 7,
      )
    } else if (filter === "pending") {
      pool = activeViews.filter(
        (view) => !view.cancellation_pending && view.days_remaining <= 0,
      )
    } else if (filter === "paid") {
      pool = activeViews.filter((view) =>
        teamRenewalStats.renewedSubscriptionIds.has(view.subscription.id),
      )
    } else {
      pool = activeViews
    }

    if (focusedSubscriptionId > 0) {
      const focused = [...activeViews, ...archivedViews].filter(
        (view) => view.subscription.id === focusedSubscriptionId,
      )
      if (focused.length > 0) pool = focused
    }

    const query = search.trim().toLowerCase()
    if (!query) return pool
    return pool.filter((view) =>
      [
        view.subscription.name,
        view.subscription.business_type,
        String(view.subscription.id),
        view.account_name,
        view.seat_name,
        view.subscription.remark,
        view.subscription.customer_email,
        view.subscription.customer_wechat,
        view.cycle_desc,
        view.next_due_date,
        view.price_yuan,
        view.cost_yuan,
        view.allocated_cost_yuan,
        view.allocated_profit_yuan,
        ...(view.channel_labels ?? []),
      ].some((field) => field?.toLowerCase().includes(query)),
    )
  }, [
    activeViews,
    archivedViews,
    filter,
    focusedSubscriptionId,
    search,
    teamRenewalStats.renewedSubscriptionIds,
  ])

  const pageCount = Math.max(1, Math.ceil(filteredViews.length / USERS_PER_PAGE))
  const safePage = Math.min(currentPage, pageCount)
  const pageStartIndex = (safePage - 1) * USERS_PER_PAGE
  const pagedViews = filteredViews.slice(pageStartIndex, pageStartIndex + USERS_PER_PAGE)

  const teamSubscriptionIDs = new Set(activeViews.map((view) => view.subscription.id))
  const teamNotificationActivity = (dashboardQuery.data?.notification_activity_30d ?? [])
    .filter((item) => teamSubscriptionIDs.has(item.subscription_id))

  const teamDashboard = dashboardQuery.data
    ? {
        ...dashboardQuery.data,
        subscription_count: activeViews.length,
        active_count: activeViews.length,
        archived_count: archivedViews.length,
        notify_success_30d: teamNotificationActivity.filter((item) => item.status === "success").length,
        notify_failed_30d: teamNotificationActivity.filter((item) => item.status === "failed").length,
        notification_activity_30d: teamNotificationActivity,
        accounts: (dashboardQuery.data.accounts ?? []).filter(
          (account) => account.account_name !== "Plus 出租",
        ),
      }
    : null

  const updatePage = (value: number) => {
    const nextPage = Math.min(Math.max(value, 1), pageCount)
    const next = new URLSearchParams(searchParams)
    if (nextPage <= 1) {
      next.delete("page")
    } else {
      next.set("page", String(nextPage))
    }
    setSearchParams(next, { replace: true })
  }

  const openStatDetail = (key: KpiDetailKey) => {
    if (!teamDashboard) return
    if (key === "notifications") {
      setStatDetail({
        title: t("dashboard.notifyActivity"),
        items: (teamDashboard.notification_activity_30d ?? []).map((item) => ({
          id: item.id,
          title: item.customer_email || item.subscription_name,
          subtitle: item.customer_wechat || item.subscription_name,
          meta: [item.channel, item.due_date, item.updated_at_label, item.last_error],
          value: item.status === "success" ? t("common.success") : t("common.failed"),
          valueTone: item.status === "success" ? "success" : "danger",
        })),
      })
      return
    }
    const source = key === "subscriptions"
      ? activeViews
      : key === "pending"
        ? activeViews.filter((view) => teamRenewalStats.pendingSubscriptionIds.has(view.subscription.id))
        : key === "renewed"
          ? activeViews.filter((view) => teamRenewalStats.renewedSubscriptionIds.has(view.subscription.id))
          : archivedViews
    const title = key === "subscriptions"
      ? t("dashboard.subscriptions")
      : key === "pending"
        ? t("dashboard.monthDue")
        : key === "renewed"
          ? t("dashboard.monthRenewed")
          : t("dashboard.archived")
    setStatDetail({
      title,
      items: source.map((view) => ({
        id: view.subscription.id,
        title: view.subscription.customer_email || view.subscription.name,
        subtitle: view.subscription.customer_wechat || view.account_name,
        meta: [view.account_name, view.seat_name, view.next_due_date, view.cycle_desc],
        value: `¥${view.price_yuan}`,
        valueTone: key === "pending" ? "warning" : key === "renewed" ? "success" : "default",
        searchText: view.subscription.remark,
      })),
    })
  }

  return (
    <>
      <PageHeader
        title={t("cards.title")}
        actions={
          <>
            <div className="relative w-full sm:w-auto">
              <Search className="absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                value={search}
                onChange={(event) => updateSearch(event.target.value)}
                placeholder={t("cards.searchPlaceholder")}
                aria-label={t("cards.title")}
                className="h-9 w-full pl-8 text-[13px] sm:w-72"
              />
            </div>
            <Select value={filter} onValueChange={(value) => updateFilter(value as CardsFilter)}>
              <SelectTrigger className="h-9 flex-1 sm:w-28 sm:flex-none" aria-label={t("cards.filterLabel")}>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">{t("calendar.filterAll")}</SelectItem>
                <SelectItem value="due">{t("calendar.filterDueSoon")}</SelectItem>
                <SelectItem value="pending">{t("calendar.filterPending")}</SelectItem>
                <SelectItem value="paid">{t("calendar.filterPaid")}</SelectItem>
                <SelectItem value="archived">{t("calendar.filterArchived")}</SelectItem>
              </SelectContent>
            </Select>
          </>
        }
      />

      {teamDashboard ? (
        <KpiSection
          dashboard={teamDashboard}
          pendingCount={teamRenewalStats.pendingCount}
          pendingMode="monthDue"
          renewedCount={teamRenewalStats.renewedCount}
          onFilterRenewed={() => updateFilter("paid")}
          onOpenDetail={openStatDetail}
        />
      ) : (
        <KpiSectionSkeleton count={5} />
      )}

      {subscriptionsQuery.isPending ? (
        <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
          {Array.from({ length: 6 }).map((_, index) => (
            <Skeleton key={index} className="h-72 rounded-xl" />
          ))}
        </div>
      ) : subscriptionsQuery.isError ? (
        <Card className="items-center gap-3 py-16 text-center">
          <p className="text-sm text-muted-foreground">{t("common.loadFailed")}</p>
          <Button variant="outline" onClick={() => subscriptionsQuery.refetch()}>
            {t("common.retry")}
          </Button>
        </Card>
      ) : activeViews.length === 0 && archivedViews.length === 0 ? (
        <Card className="items-center py-20 text-center animate-fade-up">
          <p className="text-sm text-muted-foreground">{t("cards.empty")}</p>
        </Card>
      ) : filteredViews.length === 0 ? (
        <Card className="items-center py-16 text-center animate-fade-up">
          <p className="text-sm text-muted-foreground">{t("cards.searchEmpty")}</p>
        </Card>
      ) : (
        <div className="space-y-4">
          <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
            {pagedViews.map((view, index) => (
              <SubscriptionCard
                key={view.subscription.id}
                view={view}
                index={index}
                onEdit={openEdit}
                onRenew={openRenew}
                onSendReminder={(item) => setReminderId(item.subscription.id)}
                onArchive={(item) =>
                  setArchiveTarget({
                    id: item.subscription.id,
                    name: item.subscription.name,
                    plusRental: item.subscription.business_type === "plus",
                  })
                }
                onGoAfterSales={(caseId) => navigate(`/after-sales?case=${caseId}`)}
              />
            ))}
          </div>
          {pageCount > 1 ? (
            <div className="flex flex-col items-center justify-between gap-3 border-t pt-4 text-xs text-muted-foreground sm:flex-row">
              <span>
                {t("cards.pageStatus", {
                  page: safePage,
                  pageCount,
                })}
              </span>
              <div className="flex items-center gap-1.5">
                <Button
                  variant="outline"
                  size="icon-sm"
                  aria-label={t("cards.prevPage")}
                  disabled={safePage <= 1}
                  onClick={() => updatePage(safePage - 1)}
                >
                  <ChevronLeft />
                </Button>
                <Button
                  variant="outline"
                  size="icon-sm"
                  aria-label={t("cards.nextPage")}
                  disabled={safePage >= pageCount}
                  onClick={() => updatePage(safePage + 1)}
                >
                  <ChevronRight />
                </Button>
              </div>
            </div>
          ) : null}
        </div>
      )}

      <SubscriptionDialog open={dialogOpen} onOpenChange={setDialogOpen} prefill={editing} />
      <DuePaidDialog
        open={duePaidTarget !== null}
        onOpenChange={(open) => {
          if (!open) setDuePaidTarget(null)
        }}
        target={duePaidTarget}
      />
      <ReminderPreviewDialog
        open={reminderId !== null}
        onOpenChange={(open) => {
          if (!open) setReminderId(null)
        }}
        subscriptionId={reminderId}
      />
      <StatDetailDialog
        open={statDetail !== null}
        onOpenChange={(open) => {
          if (!open) setStatDetail(null)
        }}
        detail={statDetail}
      />
      <ConfirmDialog
        open={archiveTarget !== null}
        onOpenChange={(open) => {
          if (!open) setArchiveTarget(null)
        }}
        title={
          archiveTarget?.plusRental
            ? t("confirms.endRentalTitle")
            : t("confirms.archiveTitle")
        }
        description={
          archiveTarget?.plusRental
            ? t("confirms.endRentalDesc", { name: archiveTarget.name })
            : t("confirms.archiveDesc", { name: archiveTarget?.name ?? "" })
        }
        actionLabel={
          archiveTarget?.plusRental
            ? t("confirms.endRentalAction")
            : t("confirms.archiveAction")
        }
        destructive
        pending={archiveMutation.isPending}
        onConfirm={() => {
          if (archiveTarget) archiveMutation.mutate(archiveTarget.id)
        }}
      />
      <Dialog
        open={cancellationResult !== null}
        onOpenChange={(open) => {
          if (!open) setCancellationResult(null)
        }}
      >
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>{t("confirms.archiveQueuedTitle")}</DialogTitle>
            <DialogDescription>
              {t("confirms.archiveQueuedDesc", {
                time: cancellationResult?.expiresAtLabel ?? "",
              })}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setCancellationResult(null)}>
              {t("confirms.archiveLater")}
            </Button>
            <Button
              onClick={() => {
                const caseId = cancellationResult?.caseId
                setCancellationResult(null)
                if (caseId) navigate(`/after-sales?case=${caseId}`)
              }}
            >
              <Clock3 data-slot="icon" />
              {t("cards.goAfterSales")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}
