import * as React from "react"
import { useTranslation } from "react-i18next"
import {
  ExternalLink,
  Mail,
  MessageCircle,
  Pencil,
  Plus,
  Search,
  UserRoundMinus,
} from "lucide-react"

import { archiveSubscription } from "@/api/endpoints"
import { useAppMutation } from "@/api/mutations"
import { useCalendar, useDashboard, useSubscriptions } from "@/api/queries"
import type { SubscriptionView } from "@/api/types"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { DueStatusBadge } from "@/components/due-status-badge"
import { KpiSection, KpiSectionSkeleton } from "@/components/kpi-section"
import { PageHeader } from "@/components/page-header"
import { ViewSwitcher } from "@/components/view-switcher"
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
import { ReminderPreviewDialog } from "./ReminderPreviewDialog"
import { SubscriptionDialog } from "./SubscriptionDialog"
import { prefillFromView, type SubscriptionPrefill } from "./subscription-prefill"

type CardsFilter = "all" | "pending" | "paid" | "archived" | "resale"

function MetaCell({ label, children }: { label: string; children: React.ReactNode }) {
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

function SubscriptionCard({
  view,
  index,
  onEdit,
  onSendReminder,
  onArchive,
}: {
  view: SubscriptionView
  index: number
  onEdit: (view: SubscriptionView) => void
  onSendReminder: (view: SubscriptionView) => void
  onArchive: (view: SubscriptionView) => void
}) {
  const { t } = useTranslation()
  const subscription = view.subscription

  return (
    <Card
      className="group gap-0 p-5 transition-colors duration-300 animate-fade-up hover:border-foreground/15"
      style={{ animationDelay: `${Math.min(index * 40, 320)}ms` }}
    >
      <div className="flex items-start justify-between gap-2">
        <div className="min-w-0">
          <h3 className="truncate text-[15px] font-semibold">{subscription.name}</h3>
          <div className="mt-1.5 flex flex-wrap items-center gap-1.5">
            <Badge variant="secondary" className="font-normal">
              {view.account_name}
            </Badge>
            {subscription.is_resale ? (
              <Badge variant="outline" className="font-normal">
                {t("cards.resale")}
              </Badge>
            ) : null}
            {view.seat_name ? (
              <span className="text-xs text-muted-foreground">{view.seat_name}</span>
            ) : null}
          </div>
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

      <div className="mt-4 grid grid-cols-3 gap-x-3 gap-y-2.5">
        <MetaCell label={t("cards.perPerson")}>
          <span className="tabular-nums">¥{view.price_yuan}</span>
        </MetaCell>
        <MetaCell label={t("cards.cost")}>
          <span className="tabular-nums">¥{view.cost_yuan}</span>
        </MetaCell>
        <MetaCell label={subscription.is_resale ? t("cards.agencyFee") : t("cards.profit")}>
          <span className="tabular-nums">
            ¥{subscription.is_resale ? view.agency_fee_yuan : view.profit_yuan}
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
          <span className="text-muted-foreground"> {t("common.global")}</span>
        </MetaCell>
      </div>

      {view.last_error ? (
        <p className="mt-3 text-xs text-destructive">
          {t("cards.lastError", { error: view.last_error })}
        </p>
      ) : null}

      <div className="mt-4 flex flex-wrap items-center gap-1.5 border-t pt-3.5">
        <Button variant="outline" size="sm" onClick={() => onEdit(view)}>
          <Pencil data-slot="icon" />
          {t("common.edit")}
        </Button>
        {subscription.customer_email ? (
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
        <Button
          variant="ghost"
          size="sm"
          className="ml-auto text-destructive hover:text-destructive"
          onClick={() => onArchive(view)}
        >
          <UserRoundMinus data-slot="icon" />
          {t("calendar.getOff")}
        </Button>
      </div>
    </Card>
  )
}

export function CardsPage() {
  const { t } = useTranslation()
  const subscriptionsQuery = useSubscriptions()
  const dashboardQuery = useDashboard()
  const calendarQuery = useCalendar()

  const [dialogOpen, setDialogOpen] = React.useState(false)
  const [editing, setEditing] = React.useState<SubscriptionPrefill | null>(null)
  const [reminderId, setReminderId] = React.useState<number | null>(null)
  const [archiveTarget, setArchiveTarget] = React.useState<{ id: number; name: string } | null>(null)
  const [search, setSearch] = React.useState("")
  const [filter, setFilter] = React.useState<CardsFilter>("all")

  const archiveMutation = useAppMutation((id: number) => archiveSubscription(id), {
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

  const activeViews = subscriptionsQuery.data?.subscriptions ?? []
  const archivedViews = subscriptionsQuery.data?.archived ?? []
  const calendar = calendarQuery.data

  const paidBySubscription = React.useMemo(() => {
    const map = new Map<number, boolean>()
    const month = calendar?.month_value
    for (const occurrence of calendar?.occurrences ?? []) {
      if (month && occurrence.due_date.slice(0, 7) !== month) continue
      // Focus row is the authoritative pending/paid status for the card list.
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
      pool = activeViews.filter((view) => paidBySubscription.get(view.subscription.id) === false)
    } else if (filter === "paid") {
      pool = activeViews.filter((view) => paidBySubscription.get(view.subscription.id) === true)
    } else {
      pool = activeViews
    }

    const query = search.trim().toLowerCase()
    if (!query) return pool
    return pool.filter((view) =>
      [
        view.subscription.name,
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
        view.agency_fee_yuan,
        ...(view.channel_labels ?? []),
      ].some((field) => field?.toLowerCase().includes(query)),
    )
  }, [activeViews, archivedViews, filter, paidBySubscription, search])

  return (
    <>
      <PageHeader
        title={t("cards.title")}
        description={t("cards.desc")}
        actions={
          <>
            <div className="relative">
              <Search className="absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                value={search}
                onChange={(event) => setSearch(event.target.value)}
                placeholder={t("cards.searchPlaceholder")}
                aria-label={t("cards.title")}
                className="h-9 w-56 pl-8 text-[13px] sm:w-72"
              />
            </div>
            <Select value={filter} onValueChange={(value) => setFilter(value as CardsFilter)}>
              <SelectTrigger className="h-9 w-28" aria-label={t("cards.filterLabel")}>
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
            <ViewSwitcher />
            <Button onClick={openCreate}>
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
        <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
          {filteredViews.map((view, index) => (
            <SubscriptionCard
              key={view.subscription.id}
              view={view}
              index={index}
              onEdit={openEdit}
              onSendReminder={(item) => setReminderId(item.subscription.id)}
              onArchive={(item) =>
                setArchiveTarget({ id: item.subscription.id, name: item.subscription.name })
              }
            />
          ))}
        </div>
      )}

      <SubscriptionDialog open={dialogOpen} onOpenChange={setDialogOpen} prefill={editing} />
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
