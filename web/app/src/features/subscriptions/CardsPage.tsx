import * as React from "react"
import { useTranslation } from "react-i18next"
import { useSearchParams } from "react-router-dom"
import {
  ChevronLeft,
  ChevronRight,
  ExternalLink,
  Mail,
  MessageCircle,
  Pencil,
  Plus,
  Receipt,
  Search,
  UserRoundMinus,
} from "lucide-react"

import { archiveSubscription } from "@/api/endpoints"
import { useAppMutation } from "@/api/mutations"
import { useCalendar, useDashboard, useSubscriptions } from "@/api/queries"
import type { CalendarOccurrence, SubscriptionView } from "@/api/types"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { DueStatusBadge } from "@/components/due-status-badge"
import { KpiSection, KpiSectionSkeleton } from "@/components/kpi-section"
import { PageHeader } from "@/components/page-header"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
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

type CardsFilter = "all" | "pending" | "paid" | "archived" | "resale"

const EMPTY_SUBSCRIPTION_VIEWS: SubscriptionView[] = []
const USERS_PER_PAGE = 9

function normalizeCardsFilter(value: string | null): CardsFilter {
  if (
    value === "pending" ||
    value === "paid" ||
    value === "archived" ||
    value === "resale"
  ) {
    return value
  }
  return "all"
}

function isPaymentDueOccurrence(occurrence: { paid: boolean; days_remaining: number }) {
  return !occurrence.paid && occurrence.days_remaining <= 0
}

function shouldPreferRenewOccurrence(current: CalendarOccurrence, candidate: CalendarOccurrence) {
  const currentActionable = isPaymentDueOccurrence(current)
  const candidateActionable = isPaymentDueOccurrence(candidate)
  if (currentActionable !== candidateActionable) return candidateActionable
  return candidate.due_date < current.due_date
}

function MetaCell({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="min-w-0">
      <div className="text-[11px] font-medium text-muted-foreground">{label}</div>
      <div className="truncate text-[13px]">{children}</div>
    </div>
  )
}

function CustomerContactRow({ email, wechat }: { email: string; wechat: string }) {
  const { t } = useTranslation()

  return (
    <div className="mt-2 grid min-w-0 gap-1.5">
      <span
        className="flex h-6 max-w-full min-w-0 items-center gap-1.5 rounded-md border border-brand/10 bg-brand/[0.06] px-2 text-xs text-muted-foreground"
        title={`${t("subscriptionDialog.customerEmail")}: ${email || t("cards.contactMissing")}`}
      >
        <Mail className="size-3.5 shrink-0" aria-hidden="true" />
        <span className="truncate font-mono">{email || t("cards.contactMissing")}</span>
      </span>
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

function SubscriptionCard({
  view,
  index,
  onEdit,
  onRenew,
  onSendReminder,
  onArchive,
}: {
  view: SubscriptionView
  index: number
  onEdit: (view: SubscriptionView) => void
  onRenew: (view: SubscriptionView) => void
  onSendReminder: (view: SubscriptionView) => void
  onArchive: (view: SubscriptionView) => void
}) {
  const { t } = useTranslation()
  const subscription = view.subscription
  const archived = subscription.archived_at !== null
  const displayedCostYuan = view.allocated_cost_yuan || view.cost_yuan
  const displayedProfitYuan = view.allocated_profit_yuan || view.profit_yuan
  const accentClass = archived
    ? "bg-muted-foreground/35"
    : view.days_remaining <= 0
      ? "bg-destructive"
      : view.days_remaining <= 7
        ? "bg-gold"
        : "bg-brand"

  return (
    <Card
      className="group relative gap-0 overflow-hidden p-5 transition-[border-color,background-color] duration-200 animate-fade-up hover:border-input hover:bg-accent/25"
      style={{ animationDelay: `${Math.min(index * 40, 320)}ms` }}
    >
      <span className={cn("absolute inset-y-0 left-0 w-1", accentClass)} />
      <div className="flex items-start justify-between gap-2">
        <div className="min-w-0">
          <h3 className="truncate text-[15px] font-semibold">
            {view.account_name || subscription.name}
          </h3>
          {subscription.is_resale ? (
            <div className="mt-1.5 flex flex-wrap items-center gap-1.5">
              <Badge variant="outline" className="font-normal">
                {t("cards.resale")}
              </Badge>
            </div>
          ) : null}
          {subscription.remark ? (
            <p className="mt-1.5 line-clamp-2 text-xs text-muted-foreground">
              {subscription.remark}
            </p>
          ) : null}
          <CustomerContactRow
            email={subscription.customer_email}
            wechat={subscription.customer_wechat}
          />
        </div>
        <DueStatusBadge paid={false} daysRemaining={view.days_remaining} />
      </div>

      <div className="mt-4 grid grid-cols-3 gap-x-3 gap-y-2.5 rounded-md border border-foreground/[0.07] bg-muted/40 p-3">
        <MetaCell label={t("cards.perPerson")}>
          <span className="tabular-nums">¥{view.price_yuan}</span>
        </MetaCell>
        <MetaCell label={t("cards.cost")}>
          <span className="font-medium text-gold tabular-nums">¥{displayedCostYuan}</span>
        </MetaCell>
        <MetaCell label={subscription.is_resale ? t("cards.agencyFee") : t("cards.profit")}>
          <span className="font-medium text-success tabular-nums">
            ¥{subscription.is_resale ? view.agency_fee_yuan : displayedProfitYuan}
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
        <MetaCell label={t("cards.channels")}>
          {(view.channel_labels ?? []).join(" · ")}
        </MetaCell>
      </div>

      {view.last_error ? (
        <p className="mt-3 text-xs text-destructive">
          {t("cards.lastError", { error: view.last_error })}
        </p>
      ) : null}

      <div className="-mx-5 -mb-5 mt-4 flex flex-wrap items-center gap-1.5 border-t bg-muted/20 px-5 py-3.5">
        <Button variant="outline" size="sm" onClick={() => onEdit(view)}>
          <Pencil data-slot="icon" />
          {t("common.edit")}
        </Button>
        {!archived ? (
          <Button variant="outline" size="sm" onClick={() => onRenew(view)}>
            <Receipt data-slot="icon" />
            {t("cards.renew")}
          </Button>
        ) : null}
        {!archived && subscription.customer_email ? (
          <Button variant="outline" size="sm" onClick={() => onSendReminder(view)}>
            <Mail data-slot="icon" />
            {t("cards.sendReminder")}
          </Button>
        ) : null}
        {subscription.trade_url ? (
          <Button variant="ghost" size="sm" asChild>
            <a href={subscription.trade_url} target="_blank" rel="noopener noreferrer">
              <ExternalLink data-slot="icon" />
              {t("common.openLink")}
            </a>
          </Button>
        ) : null}
        {!archived ? (
          <Button
            variant="ghost"
            size="sm"
            className="ml-auto text-destructive hover:text-destructive"
            onClick={() => onArchive(view)}
          >
            <UserRoundMinus data-slot="icon" />
            {t("calendar.getOff")}
          </Button>
        ) : null}
      </div>
    </Card>
  )
}

export function CardsPage() {
  const { t } = useTranslation()
  const [searchParams, setSearchParams] = useSearchParams()
  const routeSearch = searchParams.get("q") ?? ""
  const routeFilter = normalizeCardsFilter(searchParams.get("filter"))
  const routePage = Number.parseInt(searchParams.get("page") ?? "1", 10)
  const currentPage = Number.isFinite(routePage) && routePage > 0 ? routePage : 1
  const routeFocusId = Number.parseInt(searchParams.get("subscription") ?? "", 10)
  const focusedSubscriptionId = Number.isFinite(routeFocusId) ? routeFocusId : 0

  const subscriptionsQuery = useSubscriptions()
  const dashboardQuery = useDashboard()
  const calendarQuery = useCalendar()

  const [dialogOpen, setDialogOpen] = React.useState(false)
  const [editing, setEditing] = React.useState<SubscriptionPrefill | null>(null)
  const [duePaidTarget, setDuePaidTarget] = React.useState<DuePaidTarget | null>(null)
  const [reminderId, setReminderId] = React.useState<number | null>(null)
  const [archiveTarget, setArchiveTarget] = React.useState<{ id: number; name: string } | null>(null)
  const search = routeSearch
  const filter = routeFilter

  const archiveMutation = useAppMutation((id: number) => archiveSubscription(id), {
    successMessage: t("confirms.archiveSuccess"),
    onSuccess: () => setArchiveTarget(null),
  })

  const openCreate = () => {
    setEditing(null)
    setDialogOpen(true)
  }
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

  const activeViews = subscriptionsQuery.data?.subscriptions ?? EMPTY_SUBSCRIPTION_VIEWS
  const archivedViews = subscriptionsQuery.data?.archived ?? EMPTY_SUBSCRIPTION_VIEWS
  const calendar = calendarQuery.data

  const renewOccurrenceBySubscription = React.useMemo(() => {
    const map = new Map<number, CalendarOccurrence>()
    const month = calendar?.month_value
    for (const occurrence of calendar?.occurrences ?? []) {
      if (month && occurrence.due_date.slice(0, 7) !== month) continue
      if (occurrence.paid) continue
      const current = map.get(occurrence.subscription_id)
      if (!current || shouldPreferRenewOccurrence(current, occurrence)) {
        map.set(occurrence.subscription_id, occurrence)
      }
    }
    return map
  }, [calendar])

  const openRenew = (view: SubscriptionView) => {
    const occurrence = renewOccurrenceBySubscription.get(view.subscription.id)
    setDuePaidTarget({
      subscriptionId: view.subscription.id,
      name: occurrence?.account_name || view.account_name || view.subscription.name,
      priceYuan: occurrence?.price_yuan || view.price_yuan,
      cycleDesc: occurrence?.cycle_desc || view.cycle_desc,
      dueDate: occurrence?.due_date || view.next_due_date,
    })
  }

  const paymentDueBySubscription = React.useMemo(() => {
    const map = new Map<number, boolean>()
    const month = calendar?.month_value
    for (const occurrence of calendar?.occurrences ?? []) {
      if (month && occurrence.due_date.slice(0, 7) !== month) continue
      if (isPaymentDueOccurrence(occurrence)) {
        map.set(occurrence.subscription_id, true)
      }
    }
    return map
  }, [calendar])

  const paidBySubscription = React.useMemo(() => {
    const map = new Map<number, boolean>()
    const month = calendar?.month_value
    for (const occurrence of calendar?.occurrences ?? []) {
      if (month && occurrence.due_date.slice(0, 7) !== month) continue
      // Paid filter follows the calendar focus row; future unpaid rows are handled separately.
      map.set(occurrence.subscription_id, occurrence.paid)
    }
    for (const occurrence of calendar?.paid_in_month_occurrences ?? []) {
      if (!map.has(occurrence.subscription_id)) {
        map.set(occurrence.subscription_id, true)
      }
    }
    return map
  }, [calendar])

  const filteredViews = React.useMemo(() => {
    let pool: SubscriptionView[]
    if (filter === "archived") {
      pool = archivedViews
    } else if (filter === "resale") {
      pool = [...activeViews, ...archivedViews].filter((view) => view.subscription.is_resale)
    } else if (filter === "pending") {
      pool = activeViews.filter(
        (view) => paymentDueBySubscription.get(view.subscription.id) === true,
      )
    } else if (filter === "paid") {
      pool = activeViews.filter((view) => paidBySubscription.get(view.subscription.id) === true)
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
        String(view.subscription.id),
        view.account_name,
        view.seat_name,
        view.subscription.remark,
        view.subscription.customer_email,
        view.subscription.customer_wechat,
        view.subscription.trade_url,
        view.cycle_desc,
        view.next_due_date,
        view.price_yuan,
        view.cost_yuan,
        view.allocated_cost_yuan,
        view.allocated_profit_yuan,
        view.agency_fee_yuan,
        ...(view.channel_labels ?? []),
      ].some((field) => field?.toLowerCase().includes(query)),
    )
  }, [
    activeViews,
    archivedViews,
    filter,
    focusedSubscriptionId,
    paidBySubscription,
    paymentDueBySubscription,
    search,
  ])

  const pageCount = Math.max(1, Math.ceil(filteredViews.length / USERS_PER_PAGE))
  const safePage = Math.min(currentPage, pageCount)
  const pageStartIndex = (safePage - 1) * USERS_PER_PAGE
  const pagedViews = filteredViews.slice(pageStartIndex, pageStartIndex + USERS_PER_PAGE)
  const pageEndIndex = pageStartIndex + pagedViews.length

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

  return (
    <>
      <PageHeader
        title={t("cards.title")}
        description={t("cards.desc")}
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
                <SelectItem value="pending">{t("calendar.filterPending")}</SelectItem>
                <SelectItem value="paid">{t("calendar.filterPaid")}</SelectItem>
                <SelectItem value="archived">{t("calendar.filterArchived")}</SelectItem>
                <SelectItem value="resale">{t("calendar.filterResale")}</SelectItem>
              </SelectContent>
            </Select>
            <Button className="flex-1 sm:flex-none" onClick={openCreate}>
              <Plus data-slot="icon" />
              {t("nav.newSubscription")}
            </Button>
          </>
        }
      />

      {dashboardQuery.data ? (
        <KpiSection
          dashboard={dashboardQuery.data}
          pendingCount={calendarQuery.data?.pending_count ?? 0}
        />
      ) : (
        <KpiSectionSkeleton />
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
        <Card className="items-center gap-4 py-20 text-center animate-fade-up">
          <p className="text-sm text-muted-foreground">{t("cards.empty")}</p>
          <Button onClick={openCreate}>
            <Plus data-slot="icon" />
            {t("cards.createFirst")}
          </Button>
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
                  setArchiveTarget({ id: item.subscription.id, name: item.subscription.name })
                }
              />
            ))}
          </div>
          {pageCount > 1 ? (
            <div className="flex flex-col items-center justify-between gap-3 border-t pt-4 text-xs text-muted-foreground sm:flex-row">
              <span>
                {t("cards.pageStatus", {
                  page: safePage,
                  pageCount,
                  start: pageStartIndex + 1,
                  end: pageEndIndex,
                  total: filteredViews.length,
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
    </>
  )
}
