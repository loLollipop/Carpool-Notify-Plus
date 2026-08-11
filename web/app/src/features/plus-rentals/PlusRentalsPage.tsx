import * as React from "react"
import { useTranslation } from "react-i18next"
import { useSearchParams } from "react-router-dom"
import {
  Archive,
  CalendarClock,
  CircleDollarSign,
  ExternalLink,
  Mail,
  MessageCircle,
  Pencil,
  Plus,
  Receipt,
  Search,
  TimerReset,
  TrendingUp,
  UserRoundCheck,
  XCircle,
} from "lucide-react"

import { archiveSubscription } from "@/api/endpoints"
import { useAppMutation } from "@/api/mutations"
import { useSubscriptions } from "@/api/queries"
import type { SubscriptionView } from "@/api/types"
import { AmountPrivacyToggle } from "@/components/amount-privacy-toggle"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { DueStatusBadge } from "@/components/due-status-badge"
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
import { useAmountPrivacy } from "@/hooks/use-amount-privacy"
import { maskAmount, maskValue } from "@/lib/amount-privacy"
import { cn } from "@/lib/utils"
import { prefillFromView, type SubscriptionPrefill } from "@/features/subscriptions/subscription-prefill"
import { PlusRentalDialog } from "./PlusRentalDialog"

type RentalFilter = "active" | "due" | "archived" | "all"

const EMPTY_VIEWS: SubscriptionView[] = []

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
}: {
  view: SubscriptionView
  archived: boolean
  amountsHidden: boolean
  onEdit: (view: SubscriptionView) => void
  onRenew: (view: SubscriptionView) => void
  onArchive: (view: SubscriptionView) => void
}) {
  const { t } = useTranslation()
  const subscription = view.subscription
  const profitCents = subscription.price_per_person_cents - subscription.cost_cents

  return (
    <Card className="group relative gap-0 overflow-hidden p-0 transition-[border-color,box-shadow] hover:border-brand/25 hover:shadow-sm">
      <div className="absolute inset-x-0 top-0 h-px bg-gradient-to-r from-transparent via-brand/45 to-transparent opacity-0 transition-opacity group-hover:opacity-100" />
      <div className="p-5">
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0">
            <div className="flex items-center gap-2">
              <h2 className="truncate text-base font-semibold">{subscription.name}</h2>
              <span className="shrink-0 font-mono text-[10px] text-muted-foreground/60">
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
          ) : (
            <DueStatusBadge paid={false} daysRemaining={view.days_remaining} />
          )}
        </div>

        <div className="mt-4 grid gap-2">
          <div className="flex min-w-0 items-center gap-2 rounded-lg border border-border/60 bg-muted/25 px-3 py-2 text-xs">
            <Mail className="size-3.5 shrink-0 text-brand" />
            <span className="shrink-0 text-muted-foreground">{t("plusRentals.accountEmailShort")}</span>
            <span className="ml-auto truncate font-mono" title={subscription.customer_email}>
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

        {!archived ? (
          <div className="mt-4 rounded-lg border border-brand/10 bg-brand/[0.035] px-3 py-3">
            <div className="flex items-center justify-between gap-3">
              <span className="flex items-center gap-1.5 text-xs text-muted-foreground">
                <CalendarClock className="size-3.5" />
                {t("plusRentals.nextDue")}
              </span>
              <span className="font-mono text-sm font-semibold tabular-nums">{view.next_due_date}</span>
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
        {!archived ? (
          <>
            <Button variant="outline" size="sm" onClick={() => onEdit(view)}>
              <Pencil data-slot="icon" />
              {t("common.edit")}
            </Button>
            <Button variant="outline" size="sm" onClick={() => onRenew(view)}>
              <Receipt data-slot="icon" />
              {t("plusRentals.recordRenewal")}
            </Button>
          </>
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
            <XCircle data-slot="icon" />
            {t("plusRentals.endRental")}
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
}: {
  label: string
  value: string | number
  hint: string
  icon: React.ReactNode
  tone: "brand" | "success" | "gold" | "neutral"
}) {
  const toneClass = {
    brand: "bg-brand/10 text-brand",
    success: "bg-success/10 text-success",
    gold: "bg-gold/12 text-gold",
    neutral: "bg-muted text-muted-foreground",
  }[tone]
  return (
    <Card className="gap-0 p-4">
      <div className="flex items-center justify-between text-xs font-medium text-muted-foreground">
        {label}
        <span className={cn("grid size-8 place-items-center rounded-lg", toneClass)}>{icon}</span>
      </div>
      <div className="mt-3 text-2xl font-semibold tabular-nums">{value}</div>
      <p className="mt-1 text-[11px] text-muted-foreground">{hint}</p>
    </Card>
  )
}

export function PlusRentalsPage() {
  const { t } = useTranslation()
  const [searchParams] = useSearchParams()
  const subscriptionsQuery = useSubscriptions()
  const { amountsHidden, toggleAmounts } = useAmountPrivacy()
  const [dialogOpen, setDialogOpen] = React.useState(false)
  const [editing, setEditing] = React.useState<SubscriptionPrefill | null>(null)
  const [filter, setFilter] = React.useState<RentalFilter>(() => {
    const initial = searchParams.get("filter")
    return initial === "archived" || initial === "due" || initial === "all" ? initial : "active"
  })
  const [search, setSearch] = React.useState(() => searchParams.get("q") ?? "")
  const [renewTarget, setRenewTarget] = React.useState<DuePaidTarget | null>(null)
  const [archiveTarget, setArchiveTarget] = React.useState<SubscriptionView | null>(null)

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

  const dueSoon = active.filter((view) => view.days_remaining <= 7)
  const rentCents = active.reduce((sum, view) => sum + view.subscription.price_per_person_cents, 0)
  const costCents = active.reduce((sum, view) => sum + view.subscription.cost_cents, 0)
  const profitCents = rentCents - costCents

  const filtered = React.useMemo(() => {
    let pool = filter === "archived" ? archived : filter === "all" ? [...active, ...archived] : active
    if (filter === "due") pool = active.filter((view) => view.days_remaining <= 7)
    const query = search.trim().toLowerCase()
    if (!query) return pool
    return pool.filter((view) =>
      [
        view.subscription.name,
        view.subscription.customer_email,
        view.subscription.customer_wechat,
        view.subscription.remark,
        view.subscription.trade_url,
        view.next_due_date,
      ].some((value) => value?.toLowerCase().includes(query)),
    )
  }, [active, archived, filter, search])

  const archiveMutation = useAppMutation((id: number) => archiveSubscription(id), {
    successMessage: t("plusRentals.ended"),
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

  return (
    <>
      <PageHeader
        title={t("plusRentals.title")}
        description={t("plusRentals.desc")}
        titleAccessory={<AmountPrivacyToggle amountsHidden={amountsHidden} onToggle={toggleAmounts} />}
        actions={
          <Button onClick={openCreate}>
            <Plus data-slot="icon" />
            {t("plusRentals.newRental")}
          </Button>
        }
      />

      <div className="mb-6 grid grid-cols-2 gap-3 lg:grid-cols-4">
        <SummaryCard
          label={t("plusRentals.activeCount")}
          value={maskValue(amountsHidden, active.length)}
          hint={t("plusRentals.activeHint")}
          icon={<UserRoundCheck className="size-4" />}
          tone="brand"
        />
        <SummaryCard
          label={t("plusRentals.dueSoonCount")}
          value={maskValue(amountsHidden, dueSoon.length)}
          hint={t("plusRentals.dueSoonHint")}
          icon={<TimerReset className="size-4" />}
          tone="gold"
        />
        <SummaryCard
          label={t("plusRentals.currentRent")}
          value={maskAmount(amountsHidden, formatYuan(rentCents))}
          hint={t("plusRentals.currentRentHint")}
          icon={<CircleDollarSign className="size-4" />}
          tone="neutral"
        />
        <SummaryCard
          label={t("plusRentals.currentProfit")}
          value={maskAmount(amountsHidden, formatYuan(profitCents))}
          hint={t("plusRentals.currentProfitHint", { cost: maskAmount(amountsHidden, formatYuan(costCents)) })}
          icon={<TrendingUp className="size-4" />}
          tone="success"
        />
      </div>

      <div className="mb-4 flex flex-col gap-2 sm:flex-row sm:items-center">
        <div className="relative flex-1">
          <Search className="absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            value={search}
            onChange={(event) => setSearch(event.target.value)}
            className="pl-9"
            placeholder={t("plusRentals.searchPlaceholder")}
          />
        </div>
        <Select value={filter} onValueChange={(value) => setFilter(value as RentalFilter)}>
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

      <div className="mb-4 flex items-center gap-2 rounded-lg border border-brand/15 bg-brand/[0.045] px-3.5 py-2.5 text-xs leading-5 text-muted-foreground">
        <MessageCircle className="size-4 shrink-0 text-brand" />
        {t("plusRentals.manualReminder")}
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
          <p className="max-w-sm text-xs leading-5 text-muted-foreground">{t("plusRentals.emptyHint")}</p>
          {active.length === 0 && archived.length === 0 ? (
            <Button size="sm" onClick={openCreate}>
              <Plus data-slot="icon" />
              {t("plusRentals.newRental")}
            </Button>
          ) : null}
        </Card>
      ) : (
        <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
          {filtered.map((view) => (
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
            />
          ))}
        </div>
      )}

      <PlusRentalDialog open={dialogOpen} onOpenChange={setDialogOpen} prefill={editing} />
      <DuePaidDialog
        open={renewTarget !== null}
        onOpenChange={(open) => {
          if (!open) setRenewTarget(null)
        }}
        target={renewTarget}
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
    </>
  )
}
