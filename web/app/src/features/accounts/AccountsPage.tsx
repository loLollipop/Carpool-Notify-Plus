import * as React from "react"
import { useTranslation } from "react-i18next"
import {
  CheckCircle2,
  ChevronDown,
  ChevronLeft,
  ChevronRight,
  CircleParking,
  CircleParkingOff,
  KeyRound,
  Pencil,
  Plus,
  RefreshCw,
  Search,
  ShieldAlert,
  Snowflake,
  Trash2,
} from "lucide-react"

import { deleteAccount, markAccountRenewed } from "@/api/endpoints"
import { useAppMutation } from "@/api/mutations"
import { useAccounts } from "@/api/queries"
import type { AccountView } from "@/api/types"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { PageHeader } from "@/components/page-header"
import { OperationUnreadBadge } from "@/components/operation-unread-badge"
import {
  StatDetailDialog,
  type StatDetailState,
} from "@/components/stat-detail-dialog"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { cn } from "@/lib/utils"
import { useOperationNotifications } from "@/hooks/use-operation-notifications"
import { SubscriptionDialog } from "@/features/subscriptions/SubscriptionDialog"
import {
  prefillFromSeat,
  type SubscriptionPrefill,
} from "@/features/subscriptions/subscription-prefill"
import {
  getNextMonthlyRenewalDate,
} from "@/lib/account-renewal"
import { AccountDialog, type AccountPrefill } from "./AccountDialog"
import { AccountBanDialog, type AccountBanTarget } from "./AccountBanDialog"
import { SeatFreezeDialog, type SeatFreezeTarget } from "./SeatFreezeDialog"

type AccountStatsFilter = "all" | "renewal" | "banned" | "full" | "available"

interface AccountRenewalTarget {
  id: number
  name: string
  renewalDate: string
  amountYuan: string
}

const ACCOUNTS_PER_PAGE = 9

function formatCents(cents: number) {
  return (cents / 100).toFixed(2)
}

function formatOptionalCents(cents: number) {
  return cents > 0 ? formatCents(cents) : ""
}

function AccountSerial({ number, className }: { number: number; className?: string }) {
  const { t } = useTranslation()
  return (
    <span
      className={cn(
        "grid size-9 shrink-0 place-items-center rounded-md bg-brand/10 font-mono text-sm font-bold tracking-tight text-brand tabular-nums",
        className,
      )}
      title={t("accounts.serialTitle", { number })}
    >
      {number}
    </span>
  )
}

function AccountStatus({ view }: { view: AccountView }) {
  const { t } = useTranslation()
  if (view.account.banned_at) {
    return (
      <Badge variant="destructive" className="font-normal" title={t("accounts.statusBanned")}>
        <ShieldAlert />
        {t("accounts.statusBanned")}
      </Badge>
    )
  }
  if (view.is_full) {
    return (
      <Badge variant="destructive" className="font-normal" title={t("accounts.statusFull")}>
        <CircleParkingOff />
        {t("accounts.statusFull")}
      </Badge>
    )
  }
  if (view.seat_used === 0) {
    return (
      <Badge variant="secondary" className="font-normal" title={t("accounts.statusFree")}>
        <CircleParking />
        {t("accounts.statusFree")}
      </Badge>
    )
  }
  return (
    <Badge variant="success" className="font-normal" title={t("accounts.statusOpen")}>
      <CircleParking />
      {t("accounts.statusOpen")}
    </Badge>
  )
}

function AccountDateRows({
  view,
  onRenew,
}: {
  view: AccountView
  onRenew: () => void
}) {
  const { t } = useTranslation()
  const openedAt = view.account.opened_at
  const nextRenewalDate = view.next_renewal_date || getNextMonthlyRenewalDate(openedAt)
  return (
    <div className="grid min-w-0 grid-cols-[max-content_6.25rem] items-center gap-x-2.5 gap-y-1 text-xs text-muted-foreground">
      <span>{t("accounts.openedAt")}</span>
      <span className="truncate font-mono tabular-nums text-foreground" title={openedAt || "-"}>
        {openedAt || "-"}
      </span>
      <span>{t("accounts.nextRenewalAt")}</span>
      <span
        className="truncate font-mono font-medium text-brand tabular-nums"
        title={nextRenewalDate || "-"}
      >
        {nextRenewalDate || "-"}
      </span>
      {view.renewal_this_month && view.next_renewal_date ? (
        <>
          <span>{t("accounts.monthRenewalAt")}</span>
          <span
            className="truncate font-mono font-medium text-gold tabular-nums"
            title={view.next_renewal_date}
          >
            {view.next_renewal_date}
          </span>
        </>
      ) : null}
      {view.renewal_actionable && view.next_renewal_date ? (
        <div className="col-span-2 mt-1">
          <Button type="button" variant="outline" size="sm" onClick={onRenew}>
            <CheckCircle2 data-slot="icon" />
            {t("accounts.markRenewed")}
          </Button>
        </div>
      ) : null}
    </div>
  )
}

function AccountStatsSection({
  accounts,
  renewalDates,
  activeFilter,
  onSelect,
}: {
  accounts: AccountView[]
  renewalDates: Map<number, string>
  activeFilter: AccountStatsFilter
  onSelect: (filter: AccountStatsFilter) => void
}) {
  const { t } = useTranslation()
  const stats = [
    {
      key: "all" as const,
      label: t("accounts.statsTotal"),
      value: accounts.length,
      hint: t("accounts.statsTotalHint"),
      icon: KeyRound,
      tone: "bg-brand/10 text-brand",
    },
    {
      key: "renewal" as const,
      label: t("accounts.statsRenewal"),
      value: accounts.filter((view) => renewalDates.has(view.account.id)).length,
      hint: t("accounts.statsRenewalHint"),
      icon: RefreshCw,
      tone: "bg-gold/12 text-gold",
    },
    {
      key: "banned" as const,
      label: t("accounts.statusBanned"),
      value: accounts.filter((view) => Boolean(view.account.banned_at)).length,
      hint: t("accounts.statsBannedHint"),
      icon: ShieldAlert,
      tone: "bg-destructive/10 text-destructive",
    },
    {
      key: "full" as const,
      label: t("accounts.statusFull"),
      value: accounts.filter((view) => !view.account.banned_at && view.is_full).length,
      hint: t("accounts.statsFullHint"),
      icon: CircleParkingOff,
      tone: "bg-muted text-muted-foreground",
    },
    {
      key: "available" as const,
      label: t("accounts.statusFree"),
      value: accounts.filter(
        (view) => !view.account.banned_at && view.seat_used < view.seat_total,
      ).length,
      hint: t("accounts.statsAvailableHint"),
      icon: CircleParking,
      tone: "bg-success/10 text-success",
    },
  ]

  return (
    <section
      aria-label={t("accounts.statsLabel")}
      className="mb-5 grid grid-cols-2 gap-3 lg:grid-cols-5"
    >
      {stats.map((stat, index) => {
        const Icon = stat.icon
        const active = activeFilter === stat.key
        return (
          <Card
            key={stat.key}
            className={cn(
              "group relative gap-0 overflow-hidden p-0 transition-[border-color,background-color,box-shadow] duration-200 animate-fade-up",
              active
                ? "border-brand/45 bg-brand/[0.035] shadow-[0_0_0_1px_var(--color-brand)]"
                : "hover:border-brand/25 hover:bg-accent/25",
            )}
            style={{ animationDelay: `${index * 45}ms` }}
          >
            <button
              type="button"
              aria-pressed={active}
              onClick={() => onSelect(stat.key)}
              className="flex w-full flex-col items-start p-4 text-left outline-none focus-visible:ring-2 focus-visible:ring-brand/45 focus-visible:ring-inset"
            >
              <div className="flex w-full items-start justify-between gap-3">
                <span className="text-xs font-medium text-muted-foreground">{stat.label}</span>
                <span className={cn("grid size-8 shrink-0 place-items-center rounded-md", stat.tone)}>
                  <Icon className="size-4" />
                </span>
              </div>
              <strong className="display-numeral mt-3 text-[28px] leading-none tabular-nums">
                {stat.value}
              </strong>
              <span className={cn("mt-2 text-[11px] text-muted-foreground", active && "text-brand")}>
                {active ? t("accounts.statsViewing") : stat.hint}
              </span>
              <span
                className={cn(
                  "absolute inset-x-0 bottom-0 h-0.5 origin-left bg-brand transition-transform duration-300",
                  active ? "scale-x-100" : "scale-x-0 group-hover:scale-x-100",
                )}
              />
            </button>
          </Card>
        )
      })}
    </section>
  )
}

function AccountStatsSkeleton() {
  return (
    <div className="mb-5 grid grid-cols-2 gap-3 lg:grid-cols-5">
      {Array.from({ length: 5 }).map((_, index) => (
        <Skeleton key={index} className="h-[116px] rounded-xl" />
      ))}
    </div>
  )
}

function AccountMobileCard({
  view,
  isExpanded,
  onToggle,
  onOpenSeat,
  onEditFreeze,
  onRenew,
  onEdit,
  onBan,
  onDelete,
  unreadCount,
  onAcknowledge,
}: {
  view: AccountView
  isExpanded: boolean
  onToggle: () => void
  onOpenSeat: (seat: NonNullable<AccountView["seats"]>[number]) => void
  onEditFreeze: (seat: NonNullable<AccountView["seats"]>[number]) => void
  onRenew: () => void
  onEdit: () => void
  onBan: () => void
  onDelete: () => void
  unreadCount: number
  onAcknowledge: () => void
}) {
  const { t } = useTranslation()
  const occupants = (view.seats ?? []).filter((seat) => seat.occupied || seat.frozen)
  const accountName = view.account.name.trim()
  const accountEmail = view.account.email.trim()
  const showAccountEmail = accountEmail !== "" && accountEmail !== accountName

  return (
    <Card
      onClick={unreadCount > 0 ? onAcknowledge : undefined}
      className={cn(
        "relative gap-0 overflow-hidden p-0 animate-fade-up",
        unreadCount > 0 && "cursor-pointer ring-1 ring-destructive/15",
      )}
    >
      <span
        className={cn(
          "absolute inset-y-0 left-0 w-1",
          view.account.banned_at
            ? "bg-destructive"
            : view.is_full
              ? "bg-destructive"
              : view.seat_used === 0
                ? "bg-brand"
                : "bg-success",
        )}
      />
      <div className="p-4">
        <div className="flex min-w-0 items-start justify-between gap-3">
          <div className="flex min-w-0 items-start gap-3">
            <AccountSerial number={view.account.id} />
            <div className="min-w-0">
              <div className="flex min-w-0 flex-wrap items-center gap-1.5">
                <h2 className="min-w-0 break-all text-sm font-semibold leading-5">
                  {view.account.name}
                </h2>
              </div>
              {showAccountEmail ? (
                <p className="mt-1 break-all text-xs text-muted-foreground">{accountEmail}</p>
              ) : null}
              {view.account.space_name ? (
                <p className="mt-1 truncate text-xs text-muted-foreground">
                  {view.account.space_name}
                </p>
              ) : null}
            </div>
          </div>
          <div className="flex shrink-0 items-center gap-1.5">
            <OperationUnreadBadge count={unreadCount} />
            <AccountStatus view={view} />
          </div>
        </div>

        <div className="mt-4 grid grid-cols-2 gap-x-4 gap-y-3 rounded-md border border-foreground/[0.06] bg-muted/45 p-3 text-xs">
          <div>
            <div className="text-muted-foreground">{t("accounts.colOccupancy")}</div>
            <div className="mt-1 font-semibold text-brand tabular-nums">
              {view.seat_used} / {view.seat_total}
            </div>
          </div>
          <div>
            <div className="text-muted-foreground">{t("accounts.totalCostShort")}</div>
            <div className="mt-1 font-semibold text-gold tabular-nums">
              ¥{formatCents(view.account.total_cost_cents)}
            </div>
          </div>
          <div className="col-span-2 border-t border-foreground/[0.06] pt-2.5">
            <AccountDateRows view={view} onRenew={onRenew} />
          </div>
          <div className="col-span-2">
            <div className="text-muted-foreground">{t("accounts.colPaymentMethod")}</div>
            <div className="mt-1 flex flex-wrap items-center gap-2">
              <span>{view.account.payment_method || "-"}</span>
              {view.account.zero_renewal_next_month ? (
                <Badge variant="outline" className="font-normal">
                  {t("accounts.zeroRenewalBadge")}
                </Badge>
              ) : null}
            </div>
            <div className="mt-1 text-muted-foreground">
              {t("accounts.costShort")} ¥{formatCents(view.account.cost_cents)}
            </div>
          </div>
          {view.account.remark ? (
            <div className="col-span-2">
              <div className="text-muted-foreground">{t("accounts.colRemark")}</div>
              <div className="mt-1 break-words leading-5">{view.account.remark}</div>
            </div>
          ) : null}
        </div>

        {occupants.length > 0 ? (
          <div className="mt-3">
            <button
              type="button"
              aria-expanded={isExpanded}
              onClick={onToggle}
              className="flex w-full items-center justify-between rounded-md px-2 py-2 text-left text-xs font-medium transition-colors hover:bg-accent"
            >
              <span>{t("accounts.occupancyTitle")}</span>
              <ChevronDown
                className={cn(
                  "size-4 text-muted-foreground transition-transform",
                  isExpanded && "rotate-180",
                )}
              />
            </button>
            <div
              className={cn(
                "grid transition-[grid-template-rows] duration-300 ease-out",
                isExpanded ? "grid-rows-[1fr]" : "grid-rows-[0fr]",
              )}
            >
              <div className="overflow-hidden">
                <div className="grid gap-1.5 pt-1">
                  {occupants.map((seat) => {
                    const occupantLabel =
                      (seat.frozen
                        ? seat.frozen_customer_email.trim() || seat.frozen_subscription_name.trim()
                        : seat.active_customer_email.trim() || seat.active_subscription_name.trim()) ||
                      seat.seat.name
                    if (seat.frozen) {
                      return (
                        <button
                          key={seat.seat.id}
                          type="button"
                          onClick={() => onEditFreeze(seat)}
                          className="flex items-center justify-between gap-2 rounded-md border border-dashed border-sky-500/25 bg-sky-500/[0.06] px-3 py-2 text-left text-xs transition-colors hover:border-sky-500/45 hover:bg-sky-500/10"
                        >
                          <span className="min-w-0 truncate">{occupantLabel}</span>
                          <Badge
                            variant="outline"
                            className="shrink-0 border-sky-500/30 text-sky-700 dark:text-sky-300"
                          >
                            <Snowflake />
                            {t("accounts.frozenUntil", { time: seat.frozen_until_label })}
                          </Badge>
                        </button>
                      )
                    }
                    return (
                      <button
                        key={seat.seat.id}
                        type="button"
                        onClick={() => onOpenSeat(seat)}
                        className="truncate rounded-md border bg-card px-3 py-2 text-left text-xs transition-colors hover:border-brand/40 hover:text-brand"
                      >
                        {occupantLabel}
                      </button>
                    )
                  })}
                </div>
              </div>
            </div>
          </div>
        ) : null}
      </div>

      <div className="flex flex-wrap items-center gap-2 border-t bg-muted/35 px-4 py-3">
        <Button variant="outline" size="sm" onClick={onEdit}>
          <Pencil data-slot="icon" />
          {t("common.edit")}
        </Button>
        {!view.account.banned_at ? (
          <Button
            variant="ghost"
            size="sm"
            className="text-destructive hover:text-destructive"
            onClick={onBan}
          >
            <ShieldAlert data-slot="icon" />
            {t("accounts.markBanned")}
          </Button>
        ) : null}
        {view.can_delete ? (
          <Button
            variant="ghost"
            size="sm"
            className="ml-auto text-destructive hover:text-destructive"
            onClick={onDelete}
          >
            <Trash2 data-slot="icon" />
            {t("common.delete")}
          </Button>
        ) : (
          <span className="ml-auto text-xs text-muted-foreground">
            {t("accounts.occupiedLock")}
          </span>
        )}
      </div>
    </Card>
  )
}

export function AccountsPage() {
  const { t } = useTranslation()
  const accountsQuery = useAccounts()
  const { notifications, acknowledge } = useOperationNotifications()

  const [expanded, setExpanded] = React.useState<Set<number>>(new Set())
  const [accountDialogOpen, setAccountDialogOpen] = React.useState(false)
  const [editingAccount, setEditingAccount] = React.useState<AccountPrefill | null>(null)
  const [deleteTarget, setDeleteTarget] = React.useState<{ id: number; name: string } | null>(null)
  const [banTarget, setBanTarget] = React.useState<AccountBanTarget | null>(null)
  const [freezeTarget, setFreezeTarget] = React.useState<SeatFreezeTarget | null>(null)
  const [subscriptionDialogOpen, setSubscriptionDialogOpen] = React.useState(false)
  const [subscriptionPrefill, setSubscriptionPrefill] = React.useState<SubscriptionPrefill | null>(null)
  const [search, setSearch] = React.useState("")
  const [statsFilter, setStatsFilter] = React.useState<AccountStatsFilter>("all")
  const [statDetail, setStatDetail] = React.useState<StatDetailState | null>(null)
  const [renewalTarget, setRenewalTarget] = React.useState<AccountRenewalTarget | null>(null)
  const [page, setPage] = React.useState(1)
  const unreadTasksByAccount = React.useMemo(() => {
    const tasks = new Map<number, typeof notifications>()
    for (const task of notifications) {
      if (!task.account_id || (task.kind !== "account_renewal" && task.kind !== "seat_release")) continue
      const current = tasks.get(task.account_id) ?? []
      current.push(task)
      tasks.set(task.account_id, current)
    }
    return tasks
  }, [notifications])

  const deleteMutation = useAppMutation((id: number) => deleteAccount(id), {
    onSuccess: () => setDeleteTarget(null),
  })
  const renewalMutation = useAppMutation(
    (target: AccountRenewalTarget) => markAccountRenewed(target.id, target.renewalDate),
    { onSuccess: () => setRenewalTarget(null) },
  )

  const toggleExpanded = (accountId: number) => {
    setExpanded((previous) => {
      const next = new Set(previous)
      if (next.has(accountId)) {
        next.delete(accountId)
      } else {
        next.add(accountId)
      }
      return next
    })
  }

  const openCreate = () => {
    setEditingAccount(null)
    setAccountDialogOpen(true)
  }
  const openEdit = (view: AccountView) => {
    setEditingAccount({
      id: view.account.id,
      name: view.account.email || view.account.name,
      remark: view.account.remark,
      paymentMethod: view.account.payment_method,
      email: view.account.email,
      spaceName: view.account.space_name,
      openedAt: view.account.opened_at,
      costYuan: formatOptionalCents(view.account.cost_cents),
      zeroRenewalNextMonth: view.account.zero_renewal_next_month,
      seatCount: view.seat_total,
    })
    setAccountDialogOpen(true)
  }

  const openFreezeEditor = (seat: NonNullable<AccountView["seats"]>[number]) => {
    const customer =
      seat.frozen_customer_email.trim() ||
      seat.frozen_subscription_name.trim() ||
      seat.seat.name
    setFreezeTarget({
      seatId: seat.seat.id,
      seatName: seat.seat.name,
      customer,
      frozenUntil: seat.frozen_until,
      frozenUntilLabel: seat.frozen_until_label,
    })
  }

  const accounts = React.useMemo(() => accountsQuery.data ?? [], [accountsQuery.data])
  const openRenewal = (view: AccountView) => {
    if (!view.next_renewal_date || !view.renewal_actionable) return
    setStatDetail(null)
    setRenewalTarget({
      id: view.account.id,
      name: view.account.email || view.account.name,
      renewalDate: view.next_renewal_date,
      amountYuan: formatCents(view.account.zero_renewal_next_month ? 0 : view.account.cost_cents),
    })
  }
  const renewalDates = React.useMemo(() => {
    const dates = new Map<number, string>()
    for (const view of accounts) {
      if (view.account.banned_at) continue
      if (view.renewal_this_month && view.next_renewal_date) {
        dates.set(view.account.id, view.next_renewal_date)
      }
    }
    return dates
  }, [accounts])

  const selectStatsFilter = (nextFilter: AccountStatsFilter) => {
    setStatsFilter((current) =>
      current === nextFilter && nextFilter !== "all" ? "all" : nextFilter,
    )
    setSearch("")
    setPage(1)
    const selectedAccounts = accounts.filter((view) => {
      if (nextFilter === "all") return true
      if (nextFilter === "renewal") return renewalDates.has(view.account.id)
      if (nextFilter === "banned") return Boolean(view.account.banned_at)
      if (nextFilter === "full") return !view.account.banned_at && view.is_full
      return !view.account.banned_at && view.seat_used < view.seat_total
    })
    const title = nextFilter === "all"
      ? t("accounts.statsTotal")
      : nextFilter === "renewal"
        ? t("accounts.statsRenewal")
        : nextFilter === "banned"
          ? t("accounts.statusBanned")
          : nextFilter === "full"
            ? t("accounts.statusFull")
            : t("accounts.statusFree")
    setStatDetail({
      title,
      items: selectedAccounts.map((view) => ({
        id: view.account.id,
        title: `${view.account.id} · ${view.account.email || view.account.name}`,
        subtitle: view.account.email && view.account.email !== view.account.name
          ? view.account.name
          : view.account.space_name,
        meta: [
          view.account.space_name,
          renewalDates.get(view.account.id),
          view.account.banned_at ? t("accounts.statusBanned") : view.is_full
            ? t("accounts.statusFull")
            : t("accounts.statusOpen"),
        ],
        value: nextFilter === "renewal" ? (
          <Button type="button" size="sm" onClick={() => openRenewal(view)}>
            <CheckCircle2 data-slot="icon" />
            {t("accounts.markRenewed")}
          </Button>
        ) : `${view.seat_used}/${view.seat_total}`,
        valueTone: nextFilter === "renewal"
          ? undefined
          : view.account.banned_at || view.is_full ? "danger" : "success",
        searchText: view.account.remark,
      })),
    })
  }

  const filteredAccounts = React.useMemo(() => {
    const matchesStatsFilter = (view: AccountView) => {
      if (statsFilter === "all") return true
      if (statsFilter === "renewal") return renewalDates.has(view.account.id)
      if (statsFilter === "banned") return Boolean(view.account.banned_at)
      if (statsFilter === "full") return !view.account.banned_at && view.is_full
      return !view.account.banned_at && view.seat_used < view.seat_total
    }

    const query = search.trim().toLowerCase()
    const matches = accounts.filter((view) => {
      if (!matchesStatsFilter(view)) return false
      if (!query) return true
      const seatFields = (view.seats ?? []).flatMap((seat) => [
        seat.seat.name,
        seat.active_subscription_name,
        seat.active_customer_email,
        seat.active_customer_wechat,
        seat.active_trade_url,
        seat.active_remark,
        seat.frozen_subscription_name,
        seat.frozen_customer_email,
        seat.frozen_until_label,
      ])
      const nextRenewalDate = getNextMonthlyRenewalDate(view.account.opened_at)
      return [
        view.account.name,
        String(view.account.id),
        `#${view.account.id}`,
        `${view.account.id}号`,
        `no.${view.account.id}`,
        view.account.remark,
        view.account.payment_method,
        view.account.email,
        view.account.space_name,
        view.account.opened_at,
        nextRenewalDate,
        formatCents(view.account.cost_cents),
        ...seatFields,
      ].some((field) => field?.toLowerCase().includes(query))
    })
    return matches.sort((left, right) =>
      (unreadTasksByAccount.get(right.account.id)?.length ?? 0) -
      (unreadTasksByAccount.get(left.account.id)?.length ?? 0),
    )
  }, [accounts, renewalDates, search, statsFilter, unreadTasksByAccount])

  const pageCount = Math.max(1, Math.ceil(filteredAccounts.length / ACCOUNTS_PER_PAGE))
  const safePage = Math.min(page, pageCount)
  const pageStartIndex = (safePage - 1) * ACCOUNTS_PER_PAGE
  const pagedAccounts = filteredAccounts.slice(pageStartIndex, pageStartIndex + ACCOUNTS_PER_PAGE)

  React.useEffect(() => {
    const query = search.trim().toLowerCase()
    if (!query) return
    const timer = window.setTimeout(() => {
      setExpanded((previous) => {
        const next = new Set(previous)
        for (const view of filteredAccounts) {
          const seatHit = (view.seats ?? []).some((seat) => {
            return [
              seat.seat.name,
              seat.active_subscription_name,
              seat.active_customer_email,
              seat.active_customer_wechat,
              seat.active_trade_url,
              seat.active_remark,
              seat.frozen_subscription_name,
              seat.frozen_customer_email,
              seat.frozen_until_label,
            ].some((field) => field?.toLowerCase().includes(query))
          })
          if (seatHit) next.add(view.account.id)
        }
        return next
      })
    }, 0)
    return () => window.clearTimeout(timer)
  }, [filteredAccounts, search])

  return (
    <>
      <PageHeader
        title={t("accounts.title")}
        actions={
          <>
            <div className="relative w-full sm:w-auto">
              <Search className="absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                value={search}
                onChange={(event) => {
                  setSearch(event.target.value)
                  setPage(1)
                }}
                placeholder={t("accounts.searchPlaceholder")}
                aria-label={t("accounts.title")}
                className="h-9 w-full pl-8 text-[13px] sm:w-72"
              />
            </div>
            <Button className="flex-1 sm:flex-none" onClick={openCreate}>
              <Plus data-slot="icon" />
              {t("accounts.newAccount")}
            </Button>
          </>
        }
      />

      {accountsQuery.isPending ? (
        <AccountStatsSkeleton />
      ) : accountsQuery.isError || accounts.length === 0 ? null : (
        <AccountStatsSection
          accounts={accounts}
          renewalDates={renewalDates}
          activeFilter={statsFilter}
          onSelect={selectStatsFilter}
        />
      )}

      {accountsQuery.isPending ? (
        <Skeleton className="h-96 rounded-xl" />
      ) : accountsQuery.isError ? (
        <Card className="items-center gap-3 py-16 text-center">
          <p className="text-sm text-muted-foreground">{t("common.loadFailed")}</p>
          <Button variant="outline" onClick={() => accountsQuery.refetch()}>
            {t("common.retry")}
          </Button>
        </Card>
      ) : accounts.length === 0 ? (
        <Card className="items-center gap-4 py-20 text-center animate-fade-up">
          <p className="text-sm text-muted-foreground">{t("accounts.empty")}</p>
          <Button onClick={openCreate}>
            <Plus data-slot="icon" />
            {t("accounts.newAccount")}
          </Button>
        </Card>
      ) : filteredAccounts.length === 0 ? (
        <Card className="items-center py-16 text-center animate-fade-up">
          <p className="text-sm text-muted-foreground">{t("accounts.searchEmpty")}</p>
        </Card>
      ) : (
        <div className="space-y-3">
          <div className="grid gap-3 sm:hidden">
            {pagedAccounts.map((view) => (
              <AccountMobileCard
                key={view.account.id}
                view={view}
                isExpanded={expanded.has(view.account.id)}
                onToggle={() => toggleExpanded(view.account.id)}
                onOpenSeat={(seat) => {
                  setSubscriptionPrefill(prefillFromSeat(seat))
                  setSubscriptionDialogOpen(true)
                }}
                onEditFreeze={openFreezeEditor}
                onRenew={() => openRenewal(view)}
                onEdit={() => openEdit(view)}
                onBan={() =>
                  setBanTarget({
                    id: view.account.id,
                    name: view.account.name,
                    activeCount: view.seat_used,
                  })
                }
                onDelete={() =>
                  setDeleteTarget({ id: view.account.id, name: view.account.name })
                }
                unreadCount={unreadTasksByAccount.get(view.account.id)?.length ?? 0}
                onAcknowledge={() => acknowledge(unreadTasksByAccount.get(view.account.id) ?? [])}
              />
            ))}
          </div>

          <Card className="hidden gap-0 overflow-hidden p-0 animate-fade-up sm:flex">
          <Table className="table-fixed">
            <TableHeader>
              <TableRow>
                <TableHead>{t("accounts.colAccount")}</TableHead>
                <TableHead className="w-44">{t("accounts.colOccupancy")}</TableHead>
                <TableHead className="w-24">{t("accounts.colStatus")}</TableHead>
                <TableHead className="hidden lg:table-cell lg:w-40">
                  {t("accounts.colPaymentMethod")}
                </TableHead>
                <TableHead className="hidden md:table-cell md:w-52">
                  {t("accounts.colRemark")}
                </TableHead>
                <TableHead className="w-64 text-right">{t("accounts.colActions")}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody className="[&_tr:last-child]:border-b [&_tr:last-child]:border-b-border">
              {pagedAccounts.map((view) => {
                const isExpanded = expanded.has(view.account.id)
                const occupants = (view.seats ?? []).filter((seat) => seat.occupied || seat.frozen)
                const accountName = view.account.name.trim()
                const accountEmail = view.account.email.trim()
                const showAccountEmail = accountEmail !== "" && accountEmail !== accountName
                return (
                  <TableRow
                    key={view.account.id}
                    onClick={
                      unreadTasksByAccount.has(view.account.id)
                        ? () => acknowledge(unreadTasksByAccount.get(view.account.id) ?? [])
                        : undefined
                    }
                    className={cn(
                      "border-l-2",
                      view.account.banned_at
                        ? "border-l-destructive"
                        : view.is_full
                          ? "border-l-destructive"
                          : view.seat_used === 0
                            ? "border-l-brand"
                            : "border-l-success",
                    )}
                  >
                    <TableCell>
                      <div className="flex min-w-0 items-start gap-2.5">
                        <AccountSerial number={view.account.id} className="mt-0.5 size-8 text-xs" />
                        <div className="min-w-0">
                          <div className="flex min-w-0 items-center gap-1.5">
                            <div className="min-w-0 truncate font-medium" title={view.account.name}>
                              {view.account.name}
                            </div>
                            <OperationUnreadBadge
                              count={unreadTasksByAccount.get(view.account.id)?.length ?? 0}
                            />
                          </div>
                          {showAccountEmail ? (
                            <div className="mt-1 truncate text-xs text-muted-foreground" title={accountEmail}>
                              {accountEmail}
                            </div>
                          ) : null}
                          {view.account.space_name ? (
                            <div
                              className="mt-0.5 truncate text-xs text-muted-foreground"
                              title={view.account.space_name}
                            >
                              {view.account.space_name}
                            </div>
                          ) : null}
                          <div className="mt-2">
                            <AccountDateRows view={view} onRenew={() => openRenewal(view)} />
                          </div>
                        </div>
                      </div>
                    </TableCell>
                    <TableCell>
                      {view.seat_used > 0 ? (
                        <div>
                          <button
                            type="button"
                            aria-expanded={isExpanded}
                            title={t("accounts.occupancyTitle")}
                            onClick={() => toggleExpanded(view.account.id)}
                            className="inline-flex items-center gap-1 rounded-md bg-brand/[0.06] px-2 py-1 font-mono text-[13px] text-brand tabular-nums transition-colors hover:bg-brand/10"
                          >
                            {view.seat_used} / {view.seat_total}
                            <ChevronDown
                              className={cn(
                                "size-3.5 text-muted-foreground transition-transform",
                                isExpanded && "rotate-180",
                              )}
                            />
                          </button>
                          <div
                            className={cn(
                              "grid transition-[grid-template-rows] duration-300 ease-out",
                              isExpanded ? "grid-rows-[1fr]" : "grid-rows-[0fr]",
                            )}
                          >
                            <div className="overflow-hidden">
                              <div className="flex flex-wrap gap-1 pt-1.5">
                                {occupants.map((seat) => {
                                  const occupantLabel =
                                    (seat.frozen
                                      ? seat.frozen_customer_email.trim() || seat.frozen_subscription_name.trim()
                                      : seat.active_customer_email.trim() || seat.active_subscription_name.trim()) ||
                                    seat.seat.name
                                  if (seat.frozen) {
                                    return (
                                      <button
                                        key={seat.seat.id}
                                        type="button"
                                        title={t("accounts.frozenUntil", { time: seat.frozen_until_label })}
                                        aria-label={`${occupantLabel} · ${t("accounts.frozenUntil", { time: seat.frozen_until_label })}`}
                                        onClick={() => openFreezeEditor(seat)}
                                        className="inline-flex max-w-full items-center gap-1 truncate rounded-md border border-dashed border-sky-500/30 bg-sky-500/[0.06] px-2 py-0.5 text-xs text-sky-700 transition-colors hover:border-sky-500/50 hover:bg-sky-500/10 dark:text-sky-300"
                                      >
                                        <Snowflake className="size-3 shrink-0" />
                                        <span className="truncate">{occupantLabel}</span>
                                      </button>
                                    )
                                  }
                                  return (
                                    <button
                                      key={seat.seat.id}
                                      type="button"
                                      title={occupantLabel}
                                      onClick={() => {
                                        setSubscriptionPrefill(prefillFromSeat(seat))
                                        setSubscriptionDialogOpen(true)
                                      }}
                                      className="max-w-full truncate rounded-md border bg-muted/50 px-2 py-0.5 text-xs transition-colors hover:border-brand/40 hover:text-brand"
                                    >
                                      {occupantLabel}
                                    </button>
                                  )
                                })}
                              </div>
                            </div>
                          </div>
                        </div>
                      ) : (
                        <span className="font-mono text-[13px] text-muted-foreground tabular-nums">
                          {view.seat_used} / {view.seat_total}
                        </span>
                      )}
                    </TableCell>
                    <TableCell>
                      <AccountStatus view={view} />
                    </TableCell>
                    <TableCell className="hidden lg:table-cell">
                      <div className="min-w-0 space-y-1">
                        {view.account.payment_method ? (
                          <span className="block truncate" title={view.account.payment_method}>
                            {view.account.payment_method}
                          </span>
                        ) : (
                          <span className="text-muted-foreground/50">—</span>
                        )}
                        <div className="text-xs text-muted-foreground">
                          {t("accounts.totalCostShort")}{" "}
                          <span className="font-mono font-semibold text-gold tabular-nums">
                            ¥{formatCents(view.account.total_cost_cents)}
                          </span>
                        </div>
                        <div className="text-xs text-muted-foreground">
                          {t("accounts.costShort")} ¥{formatCents(view.account.cost_cents)}
                        </div>
                        {view.account.zero_renewal_next_month ? (
                          <Badge variant="outline" className="font-normal">
                            {t("accounts.zeroRenewalBadge")}
                          </Badge>
                        ) : null}
                      </div>
                    </TableCell>
                    <TableCell className="hidden md:table-cell">
                      {view.account.remark ? (
                        <span className="block truncate text-muted-foreground">
                          {view.account.remark}
                        </span>
                      ) : (
                        <span className="text-muted-foreground/50">—</span>
                      )}
                    </TableCell>
                    <TableCell className="text-right">
                      <div className="inline-flex items-center gap-1">
                        <Button variant="outline" size="sm" onClick={() => openEdit(view)}>
                          <Pencil data-slot="icon" />
                          {t("common.edit")}
                        </Button>
                        {!view.account.banned_at ? (
                          <Button
                            variant="ghost"
                            size="sm"
                            className="text-destructive hover:text-destructive"
                            onClick={() =>
                              setBanTarget({
                                id: view.account.id,
                                name: view.account.name,
                                activeCount: view.seat_used,
                              })
                            }
                          >
                            <ShieldAlert data-slot="icon" />
                            {t("accounts.markBanned")}
                          </Button>
                        ) : null}
                        {view.can_delete ? (
                          <Button
                            variant="ghost"
                            size="sm"
                            className="text-destructive hover:text-destructive"
                            onClick={() =>
                              setDeleteTarget({ id: view.account.id, name: view.account.name })
                            }
                          >
                            <Trash2 data-slot="icon" />
                            {t("common.delete")}
                          </Button>
                        ) : (
                          <span
                            className="px-2 text-xs text-muted-foreground"
                            title={t("accounts.occupiedLockTitle")}
                          >
                            {t("accounts.occupiedLock")}
                          </span>
                        )}
                      </div>
                    </TableCell>
                  </TableRow>
                )
              })}
            </TableBody>
          </Table>
          <div className="flex min-h-12 flex-col items-center justify-between gap-3 bg-muted/25 px-4 py-3 text-xs text-muted-foreground sm:flex-row">
            <span>
              {t("accounts.pageStatus", {
                page: safePage,
                pageCount,
              })}
            </span>
            {pageCount > 1 ? (
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
            ) : null}
          </div>
          </Card>

          {pageCount > 1 ? (
            <div className="flex items-center justify-between border-t pt-3 text-xs text-muted-foreground sm:hidden">
              <span>
                {t("accounts.pageStatus", {
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

      <AccountDialog
        open={accountDialogOpen}
        onOpenChange={setAccountDialogOpen}
        prefill={editingAccount}
      />
      <AccountBanDialog
        target={banTarget}
        onOpenChange={(open) => {
          if (!open) setBanTarget(null)
        }}
      />
      <SubscriptionDialog
        open={subscriptionDialogOpen}
        onOpenChange={setSubscriptionDialogOpen}
        prefill={subscriptionPrefill}
      />
      <SeatFreezeDialog
        target={freezeTarget}
        onOpenChange={(open) => {
          if (!open) setFreezeTarget(null)
        }}
      />
      <StatDetailDialog
        open={statDetail !== null}
        onOpenChange={(open) => {
          if (!open) setStatDetail(null)
        }}
        detail={statDetail}
      />
      <ConfirmDialog
        open={renewalTarget !== null}
        onOpenChange={(open) => {
          if (!open && !renewalMutation.isPending) setRenewalTarget(null)
        }}
        title={t("accounts.renewalConfirmTitle")}
        description={t("accounts.renewalConfirmDesc", {
          name: renewalTarget?.name ?? "",
          date: renewalTarget?.renewalDate ?? "",
          amount: renewalTarget?.amountYuan ?? "0.00",
        })}
        actionLabel={t("accounts.markRenewed")}
        pending={renewalMutation.isPending}
        onConfirm={() => {
          if (renewalTarget) renewalMutation.mutate(renewalTarget)
        }}
      />
      <ConfirmDialog
        open={deleteTarget !== null}
        onOpenChange={(open) => {
          if (!open) setDeleteTarget(null)
        }}
        title={t("confirms.deleteAccountTitle")}
        description={t("confirms.deleteAccountDesc", { name: deleteTarget?.name ?? "" })}
        actionLabel={t("confirms.deleteAccountAction")}
        destructive
        pending={deleteMutation.isPending}
        onConfirm={() => {
          if (deleteTarget) deleteMutation.mutate(deleteTarget.id)
        }}
      />
    </>
  )
}
