import * as React from "react"
import { useTranslation } from "react-i18next"
import { useNavigate, useSearchParams } from "react-router-dom"
import {
  Archive,
  CalendarClock,
  CheckCircle2,
  ChevronLeft,
  ChevronRight,
  CircleDollarSign,
  Clock3,
  Mail,
  MessageCircle,
  Pencil,
  Receipt,
  Search,
  TimerReset,
  TrendingUp,
  UserRoundCheck,
  XCircle,
} from "lucide-react"

import { archiveSubscription, completeOneMonthRental } from "@/api/endpoints"
import { useAppMutation } from "@/api/mutations"
import { useSubscriptions } from "@/api/queries"
import type { SubscriptionView } from "@/api/types"
import { AmountPrivacyToggle } from "@/components/amount-privacy-toggle"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { DueStatusBadge } from "@/components/due-status-badge"
import { NumberTicker } from "@/components/number-ticker"
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
import { useAmountPrivacy } from "@/hooks/use-amount-privacy"
import { maskAmount, maskValue } from "@/lib/amount-privacy"
import { cn } from "@/lib/utils"
import { prefillFromView, type SubscriptionPrefill } from "@/features/subscriptions/subscription-prefill"
import { PlusRentalDialog } from "./PlusRentalDialog"
import { isOneMonthRentalCron } from "./rental-mode"

type RentalFilter = "active" | "due" | "archived" | "all"

const EMPTY_VIEWS: SubscriptionView[] = []
const RENTALS_PER_PAGE = 9

function formatYuan(cents: number) {
  return `¥${(cents / 100).toLocaleString("zh-CN", {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  })}`
}

function RentalProgress({ view }: { view: SubscriptionView }) {
  const { t } = useTranslation()
  const total = Math.max(view.cycle_days || 1, 1)
  const remaining = Math.max(view.days_remaining, 0)
  const percentage = view.days_remaining <= 0 ? 0 : Math.min(100, (remaining / total) * 100)

  return (
    <div className="mt-4">
      <div className="mb-1.5 flex items-center justify-between text-[11px] font-medium">
        <span className="text-muted-foreground">{t("plusRentals.rentalProgress")}</span>
        <span className={cn("tabular-nums", view.days_remaining <= 7 ? "text-brand" : "text-success")}>
          {view.days_remaining <= 0
            ? t("plusRentals.expired")
            : t("plusRentals.remainingDays", { count: view.days_remaining })}
        </span>
      </div>
      <div className="h-1.5 overflow-hidden rounded-full bg-foreground/[0.08]">
        <div
          className={cn(
            "h-full rounded-full transition-[width] duration-500",
            view.days_remaining <= 7 ? "bg-brand" : "bg-success",
          )}
          style={{ width: view.days_remaining <= 0 ? "4px" : `${Math.max(4, percentage)}%` }}
        />
      </div>
    </div>
  )
}

function RentalCard({
  view,
  archived,
  amountsHidden,
  onEdit,
  onRenew,
  onArchive,
  onComplete,
  onGoAfterSales,
}: {
  view: SubscriptionView
  archived: boolean
  amountsHidden: boolean
  onEdit: (view: SubscriptionView) => void
  onRenew: (view: SubscriptionView) => void
  onArchive: (view: SubscriptionView) => void
  onComplete: (view: SubscriptionView) => void
  onGoAfterSales: (caseId: number) => void
}) {
  const { t } = useTranslation()
  const subscription = view.subscription
  const profitCents = subscription.price_per_person_cents - subscription.cost_cents
  const cancellationPending = view.cancellation_pending
  const oneMonthRental = isOneMonthRentalCron(subscription.cron_expr)
  const oneMonthExpired = oneMonthRental && view.days_remaining <= 0

  return (
    <Card
      className={cn(
        "group relative gap-0 overflow-hidden p-0 transition-[border-color,box-shadow] hover:border-brand/25 hover:shadow-sm",
        cancellationPending && "border-dashed bg-muted/35",
      )}
    >
      <div className="absolute inset-x-0 top-0 h-px bg-gradient-to-r from-transparent via-brand/45 to-transparent opacity-0 transition-opacity group-hover:opacity-100" />
      <div className="p-5">
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0">
            <div className="flex items-center gap-2">
              <h2 className="truncate text-base font-semibold">{subscription.name}</h2>
              {oneMonthRental ? (
                <Badge variant="outline" className="shrink-0 border-brand/20 bg-brand/[0.06] text-brand">
                  {t("plusRentals.oneMonthBadge")}
                </Badge>
              ) : null}
              {view.next_price_yuan ? (
                <Badge variant="outline" className="shrink-0 border-gold/25 bg-gold/[0.07] text-gold">
                  {t("cards.nextPrice", {
                    price: maskAmount(amountsHidden, `¥${view.next_price_yuan}`),
                  })}
                </Badge>
              ) : null}
              <span className="shrink-0 text-[10px] text-muted-foreground/60 tabular-nums">
                #{subscription.id}
              </span>
            </div>
            <p className="mt-1 text-xs text-muted-foreground">
              {t("plusRentals.startedLabel", { date: view.boarded_at })}
            </p>
          </div>
          {archived ? (
            <Badge variant="outline" className="shrink-0 text-muted-foreground">
              <Archive className="size-3" />
              {t("plusRentals.archived")}
            </Badge>
          ) : cancellationPending ? (
            <Badge variant="warning" className="shrink-0">
              <Clock3 className="size-3" />
              {t("plusRentals.afterSalesPending")}
            </Badge>
          ) : (
            <DueStatusBadge
              paid={oneMonthRental ? false : view.current_period_paid}
              daysRemaining={view.days_remaining}
              showPaidLabel
            />
          )}
        </div>

        <div className="mt-4 grid gap-2">
          <div className="flex min-w-0 items-center gap-2 rounded-lg border border-border/60 bg-muted/25 px-3 py-2 text-xs">
            <Mail className="size-3.5 shrink-0 text-brand" />
            <span className="shrink-0 text-muted-foreground">{t("plusRentals.accountEmailShort")}</span>
            <span className="ml-auto truncate" title={subscription.customer_email}>
              {subscription.customer_email || t("cards.contactMissing")}
            </span>
          </div>
          <div className="flex min-w-0 items-center gap-2 rounded-lg border border-border/60 bg-muted/25 px-3 py-2 text-xs">
            <MessageCircle className="size-3.5 shrink-0 text-success" />
            <span className="shrink-0 text-muted-foreground">{t("plusRentals.contactShort")}</span>
            <span className="ml-auto truncate" title={subscription.customer_wechat}>
              {subscription.customer_wechat || t("cards.contactMissing")}
            </span>
          </div>
        </div>

        <div className="mt-4 grid grid-cols-3 divide-x divide-border/60 rounded-lg border border-border/60 bg-background/70 py-3 text-center">
          <div className="px-2">
            <div className="text-[10px] font-medium text-muted-foreground">{t("plusRentals.rentShort")}</div>
            <div className="mt-1 text-sm font-semibold tabular-nums">
              {maskAmount(amountsHidden, formatYuan(subscription.price_per_person_cents))}
            </div>
          </div>
          <div className="px-2">
            <div className="text-[10px] font-medium text-muted-foreground">{t("plusRentals.costShort")}</div>
            <div className="mt-1 text-sm font-semibold text-gold tabular-nums">
              {maskAmount(amountsHidden, formatYuan(subscription.cost_cents))}
            </div>
          </div>
          <div className="px-2">
            <div className="text-[10px] font-medium text-muted-foreground">{t("plusRentals.profitShort")}</div>
            <div className={cn("mt-1 text-sm font-semibold tabular-nums", profitCents >= 0 ? "text-success" : "text-destructive")}>
              {maskAmount(amountsHidden, formatYuan(profitCents))}
            </div>
          </div>
        </div>

        {!archived && cancellationPending ? (
          <div className="mt-4 rounded-lg border border-gold/20 bg-gold/[0.06] px-3 py-3 text-xs leading-5 text-muted-foreground">
            {t("plusRentals.afterSalesPendingHint", {
              time: view.cancellation_expires_at_label,
            })}
          </div>
        ) : !archived ? (
          <div className="mt-4 rounded-lg border border-brand/10 bg-brand/[0.035] px-3 py-3">
            <div className="flex items-center justify-between gap-3">
              <span className="flex items-center gap-1.5 text-xs text-muted-foreground">
                <CalendarClock className="size-3.5" />
                {t(oneMonthRental ? "plusRentals.oneMonthEnd" : "plusRentals.nextDue")}
              </span>
              <span className="text-sm font-semibold tabular-nums">{view.next_due_date}</span>
            </div>
            <RentalProgress view={view} />
          </div>
        ) : (
          <p className="mt-4 text-xs leading-5 text-muted-foreground">
            {t("plusRentals.archivedAt", { time: view.archived_at_label })}
          </p>
        )}

        {subscription.remark ? (
          <p className="mt-3 line-clamp-2 text-xs leading-5 text-muted-foreground" title={subscription.remark}>
            {subscription.remark}
          </p>
        ) : null}
      </div>

      <div className="flex flex-wrap items-center gap-1.5 border-t bg-muted/20 px-5 py-3.5">
        {!archived && !cancellationPending ? (
          <>
            <Button variant="outline" size="sm" onClick={() => onEdit(view)}>
              <Pencil data-slot="icon" />
              {t("common.edit")}
            </Button>
            {!oneMonthRental ? (
              <Button variant="outline" size="sm" onClick={() => onRenew(view)}>
                <Receipt data-slot="icon" />
                {t("plusRentals.recordRenewal")}
              </Button>
            ) : null}
          </>
        ) : null}
        {!archived && !cancellationPending && oneMonthExpired ? (
          <Button size="sm" className="ml-auto" onClick={() => onComplete(view)}>
            <CheckCircle2 data-slot="icon" />
            {t("plusRentals.completeRental")}
          </Button>
        ) : !archived && !cancellationPending ? (
          <Button
            variant="ghost"
            size="sm"
            className="ml-auto text-destructive hover:text-destructive"
            onClick={() => onArchive(view)}
          >
            <XCircle data-slot="icon" />
            {t("plusRentals.endRental")}
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

function SummaryCard({
  label,
  value,
  hint,
  icon,
  tone,
  onClick,
}: {
  label: string
  value: string | number
  hint: string
  icon: React.ReactNode
  tone: "brand" | "success" | "gold" | "neutral"
  onClick: () => void
}) {
  const toneClass = {
    brand: "bg-brand/10 text-brand",
    success: "bg-success/10 text-success",
    gold: "bg-gold/12 text-gold",
    neutral: "bg-muted text-muted-foreground",
  }[tone]
  return (
    <Card className="group relative gap-0 overflow-hidden p-0 transition-[border-color,background-color,box-shadow] hover:border-input hover:bg-accent/25 hover:shadow-lift">
      <button
        type="button"
        onClick={onClick}
        aria-label={label}
        className="w-full p-4 text-left outline-none focus-visible:ring-2 focus-visible:ring-brand/45 focus-visible:ring-inset"
      >
        <div className="flex items-center justify-between text-xs font-medium text-muted-foreground">
          {label}
          <span className={cn("grid size-8 place-items-center rounded-lg", toneClass)}>{icon}</span>
        </div>
        <div className="display-numeral mt-3 text-[30px] leading-none">
          <NumberTicker value={value} />
        </div>
        <p className="mt-1 text-[11px] text-muted-foreground">{hint}</p>
        <span className="absolute inset-x-0 bottom-0 h-0.5 origin-left scale-x-0 bg-brand transition-transform duration-300 group-hover:scale-x-100 group-focus-within:scale-x-100" />
      </button>
    </Card>
  )
}

export function PlusRentalsPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [searchParams, setSearchParams] = useSearchParams()
  const subscriptionsQuery = useSubscriptions()
  const { refetch: refetchSubscriptions } = subscriptionsQuery
  const { amountsHidden, toggleAmounts } = useAmountPrivacy()
  const [dialogOpen, setDialogOpen] = React.useState(() => searchParams.get("create") === "1")
  const [editing, setEditing] = React.useState<SubscriptionPrefill | null>(null)
  const [filter, setFilter] = React.useState<RentalFilter>(() => {
    const initial = searchParams.get("filter")
    return initial === "archived" || initial === "due" || initial === "all" ? initial : "active"
  })
  const [search, setSearch] = React.useState(() => searchParams.get("q") ?? "")
  const [page, setPage] = React.useState(1)
  const [renewTarget, setRenewTarget] = React.useState<DuePaidTarget | null>(null)
  const [archiveTarget, setArchiveTarget] = React.useState<SubscriptionView | null>(null)
  const [completeTarget, setCompleteTarget] = React.useState<SubscriptionView | null>(null)
  const [statDetail, setStatDetail] = React.useState<StatDetailState | null>(null)
  const [cancellationResult, setCancellationResult] = React.useState<{
    caseId: number
    expiresAtLabel: string
  } | null>(null)

  const active = React.useMemo(
    () =>
      (subscriptionsQuery.data?.subscriptions ?? EMPTY_VIEWS).filter(
        (view) => view.subscription.business_type === "plus",
      ),
    [subscriptionsQuery.data?.subscriptions],
  )
  const archived = React.useMemo(
    () =>
      (subscriptionsQuery.data?.archived ?? EMPTY_VIEWS).filter(
        (view) => view.subscription.business_type === "plus",
      ),
    [subscriptionsQuery.data?.archived],
  )

  const isActionableDue = React.useCallback(
    (view: SubscriptionView) => !view.cancellation_pending && view.days_remaining <= 7,
    [],
  )
  const dueSoon = active.filter(isActionableDue)
  const rentCents = active.reduce((sum, view) => sum + view.subscription.price_per_person_cents, 0)
  const costCents = active.reduce((sum, view) => sum + view.subscription.cost_cents, 0)
  const profitCents = rentCents - costCents

  const openStatDetail = (key: "active" | "due" | "rent" | "profit") => {
    const source = key === "due" ? dueSoon : active
    const title = key === "active"
      ? t("plusRentals.activeCount")
      : key === "due"
        ? t("plusRentals.dueSoonCount")
        : key === "rent"
          ? t("plusRentals.currentRent")
          : t("plusRentals.currentProfit")
    setStatDetail({
      title,
      items: source.map((view) => ({
        id: view.subscription.id,
        title: view.subscription.customer_email || view.subscription.name,
        subtitle: view.subscription.customer_wechat || view.subscription.name,
        meta: [view.next_due_date, view.cycle_desc, view.subscription.remark],
        value: key === "profit"
          ? maskAmount(amountsHidden, `¥${view.profit_yuan}`)
          : maskAmount(amountsHidden, `¥${view.price_yuan}`),
        valueTone: key === "profit" ? "success" : key === "due" ? "warning" : "default",
      })),
    })
  }

  const nextCancellationExpiry = React.useMemo(() => {
    const expiries = active
      .filter((view) => view.cancellation_pending && view.subscription.cancellation_expires_at)
      .map((view) => Date.parse(view.subscription.cancellation_expires_at ?? ""))
      .filter(Number.isFinite)
    return expiries.length > 0 ? Math.min(...expiries) : 0
  }, [active])

  React.useEffect(() => {
    if (nextCancellationExpiry <= 0) return
    const delay = Math.max(1_000, nextCancellationExpiry - Date.now() + 1_000)
    const timer = window.setTimeout(() => {
      void refetchSubscriptions()
    }, delay)
    return () => window.clearTimeout(timer)
  }, [nextCancellationExpiry, refetchSubscriptions])

  const filtered = React.useMemo(() => {
    let pool = filter === "archived" ? archived : filter === "all" ? [...active, ...archived] : active
    if (filter === "due") pool = active.filter(isActionableDue)
    const query = search.trim().toLowerCase()
    if (!query) return pool
    return pool.filter((view) =>
      [
        view.subscription.name,
        view.subscription.customer_email,
        view.subscription.customer_wechat,
        view.subscription.remark,
        view.next_due_date,
      ].some((value) => value?.toLowerCase().includes(query)),
    )
  }, [active, archived, filter, isActionableDue, search])

  const pageCount = Math.max(1, Math.ceil(filtered.length / RENTALS_PER_PAGE))
  const safePage = Math.min(page, pageCount)
  const pageStart = (safePage - 1) * RENTALS_PER_PAGE
  const paged = filtered.slice(pageStart, pageStart + RENTALS_PER_PAGE)

  const setDialogVisibility = (open: boolean) => {
    setDialogOpen(open)
    if (!open && searchParams.get("create") === "1") {
      const nextParams = new URLSearchParams(searchParams)
      nextParams.delete("create")
      setSearchParams(nextParams, { replace: true })
    }
  }

  const archiveMutation = useAppMutation((id: number) => archiveSubscription(id), {
    successMessage: t("plusRentals.ended"),
    onSuccess: (data) => {
      setArchiveTarget(null)
      setCancellationResult({
        caseId: data.case_id ?? 0,
        expiresAtLabel: data.expires_at_label ?? "",
      })
    },
  })

  const completeMutation = useAppMutation((id: number) => completeOneMonthRental(id), {
    successMessage: t("plusRentals.completed"),
    onSuccess: () => setCompleteTarget(null),
  })

  const openEdit = (view: SubscriptionView) => {
    setEditing(prefillFromView(view))
    setDialogOpen(true)
  }

  return (
    <>
      <PageHeader
        title={t("plusRentals.title")}
        titleAccessory={<AmountPrivacyToggle amountsHidden={amountsHidden} onToggle={toggleAmounts} />}
      />

      <div className="mb-6 grid grid-cols-2 gap-3 lg:grid-cols-4">
        <SummaryCard
          label={t("plusRentals.activeCount")}
          value={maskValue(amountsHidden, active.length)}
          hint={t("plusRentals.activeHint")}
          icon={<UserRoundCheck className="size-4" />}
          tone="brand"
          onClick={() => openStatDetail("active")}
        />
        <SummaryCard
          label={t("plusRentals.dueSoonCount")}
          value={maskValue(amountsHidden, dueSoon.length)}
          hint={t("plusRentals.dueSoonHint")}
          icon={<TimerReset className="size-4" />}
          tone="gold"
          onClick={() => openStatDetail("due")}
        />
        <SummaryCard
          label={t("plusRentals.currentRent")}
          value={maskAmount(amountsHidden, formatYuan(rentCents))}
          hint={t("plusRentals.currentRentHint")}
          icon={<CircleDollarSign className="size-4" />}
          tone="neutral"
          onClick={() => openStatDetail("rent")}
        />
        <SummaryCard
          label={t("plusRentals.currentProfit")}
          value={maskAmount(amountsHidden, formatYuan(profitCents))}
          hint={t("plusRentals.currentProfitHint", { cost: maskAmount(amountsHidden, formatYuan(costCents)) })}
          icon={<TrendingUp className="size-4" />}
          tone="success"
          onClick={() => openStatDetail("profit")}
        />
      </div>

      <div className="mb-4 flex flex-col gap-2 sm:flex-row sm:items-center">
        <div className="relative flex-1">
          <Search className="absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            value={search}
            onChange={(event) => {
              setPage(1)
              setSearch(event.target.value)
            }}
            className="pl-9"
            placeholder={t("plusRentals.searchPlaceholder")}
          />
        </div>
        <Select
          value={filter}
          onValueChange={(value) => {
            setPage(1)
            setFilter(value as RentalFilter)
          }}
        >
          <SelectTrigger className="w-full sm:w-36">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="active">{t("plusRentals.filterActive")}</SelectItem>
            <SelectItem value="due">{t("plusRentals.filterDue")}</SelectItem>
            <SelectItem value="archived">{t("plusRentals.filterArchived")}</SelectItem>
            <SelectItem value="all">{t("plusRentals.filterAll")}</SelectItem>
          </SelectContent>
        </Select>
      </div>

      {subscriptionsQuery.isPending ? (
        <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
          {Array.from({ length: 6 }).map((_, index) => (
            <Skeleton key={index} className="h-[390px] rounded-xl" />
          ))}
        </div>
      ) : subscriptionsQuery.isError ? (
        <Card className="items-center gap-3 py-16 text-center">
          <p className="text-sm text-muted-foreground">{t("common.loadFailed")}</p>
          <Button variant="outline" onClick={() => subscriptionsQuery.refetch()}>
            {t("common.retry")}
          </Button>
        </Card>
      ) : filtered.length === 0 ? (
        <Card className="items-center gap-3 py-16 text-center">
          <span className="grid size-11 place-items-center rounded-xl bg-muted text-muted-foreground">
            <Archive className="size-5" />
          </span>
          <p className="text-sm font-medium">{t("plusRentals.empty")}</p>
        </Card>
      ) : (
        <div className="space-y-4">
          <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
            {paged.map((view) => (
              <RentalCard
                key={view.subscription.id}
                view={view}
                archived={view.subscription.archived_at !== null}
                amountsHidden={amountsHidden}
                onEdit={openEdit}
                onRenew={(item) =>
                  setRenewTarget({
                    subscriptionId: item.subscription.id,
                    name: item.subscription.name,
                    priceYuan: item.price_yuan,
                    cycleDesc: item.cycle_desc,
                    dueDate: item.next_due_date,
                    kind: "plus",
                  })
                }
                onArchive={setArchiveTarget}
                onComplete={setCompleteTarget}
                onGoAfterSales={(caseId) => navigate(`/after-sales?case=${caseId}`)}
              />
            ))}
          </div>
          {pageCount > 1 ? (
            <div className="flex flex-col items-center justify-between gap-3 border-t pt-4 text-xs text-muted-foreground sm:flex-row">
              <span>
                {t("cards.pageStatus", {
                  page: maskValue(amountsHidden, safePage),
                  pageCount: maskValue(amountsHidden, pageCount),
                })}
              </span>
              <div className="flex items-center gap-1.5">
                <Button
                  variant="outline"
                  size="icon-sm"
                  aria-label={t("cards.prevPage")}
                  disabled={safePage <= 1}
                  onClick={() => setPage(safePage - 1)}
                >
                  <ChevronLeft />
                </Button>
                <Button
                  variant="outline"
                  size="icon-sm"
                  aria-label={t("cards.nextPage")}
                  disabled={safePage >= pageCount}
                  onClick={() => setPage(safePage + 1)}
                >
                  <ChevronRight />
                </Button>
              </div>
            </div>
          ) : null}
        </div>
      )}

      <PlusRentalDialog open={dialogOpen} onOpenChange={setDialogVisibility} prefill={editing} />
      <StatDetailDialog
        open={statDetail !== null}
        onOpenChange={(open) => {
          if (!open) setStatDetail(null)
        }}
        detail={statDetail}
      />
      <DuePaidDialog
        open={renewTarget !== null}
        onOpenChange={(open) => {
          if (!open) setRenewTarget(null)
        }}
        target={renewTarget}
      />
      <ConfirmDialog
        open={completeTarget !== null}
        onOpenChange={(open) => {
          if (!open) setCompleteTarget(null)
        }}
        title={t("plusRentals.completeTitle")}
        description={t("plusRentals.completeDesc", {
          name: completeTarget?.subscription.name ?? "",
        })}
        actionLabel={t("plusRentals.completeAction")}
        pending={completeMutation.isPending}
        onConfirm={() => {
          if (completeTarget) completeMutation.mutate(completeTarget.subscription.id)
        }}
      />
      <ConfirmDialog
        open={archiveTarget !== null}
        onOpenChange={(open) => {
          if (!open) setArchiveTarget(null)
        }}
        title={t("plusRentals.endTitle")}
        description={t("plusRentals.endDesc", { name: archiveTarget?.subscription.name ?? "" })}
        actionLabel={t("plusRentals.endAction")}
        destructive
        pending={archiveMutation.isPending}
        onConfirm={() => {
          if (archiveTarget) archiveMutation.mutate(archiveTarget.subscription.id)
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
            <DialogTitle>{t("plusRentals.afterSalesQueuedTitle")}</DialogTitle>
            <DialogDescription>
              {t("plusRentals.afterSalesQueuedDesc", {
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
