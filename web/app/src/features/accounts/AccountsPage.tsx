import * as React from "react"
import { useTranslation } from "react-i18next"
import {
  ChevronDown,
  ChevronLeft,
  ChevronRight,
  CircleParking,
  CircleParkingOff,
  Pencil,
  Plus,
  Search,
  Trash2,
} from "lucide-react"

import { deleteAccount } from "@/api/endpoints"
import { useAppMutation } from "@/api/mutations"
import { useAccounts } from "@/api/queries"
import type { AccountView } from "@/api/types"
import { ConfirmDialog } from "@/components/confirm-dialog"
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
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { cn } from "@/lib/utils"
import { SubscriptionDialog } from "@/features/subscriptions/SubscriptionDialog"
import {
  prefillFromSeat,
  type SubscriptionPrefill,
} from "@/features/subscriptions/subscription-prefill"
import { getNextMonthlyRenewalDate } from "@/lib/account-renewal"
import { AccountDialog, type AccountPrefill } from "./AccountDialog"

type AccountsFilter = "all" | "sale" | "resale"

const ACCOUNTS_PER_PAGE = 9

function formatCents(cents: number) {
  return (cents / 100).toFixed(2)
}

function formatOptionalCents(cents: number) {
  return cents > 0 ? formatCents(cents) : ""
}

function AccountStatus({ view }: { view: AccountView }) {
  const { t } = useTranslation()
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

function AccountMobileCard({
  view,
  isExpanded,
  onToggle,
  onOpenSeat,
  onEdit,
  onDelete,
}: {
  view: AccountView
  isExpanded: boolean
  onToggle: () => void
  onOpenSeat: (seat: NonNullable<AccountView["seats"]>[number]) => void
  onEdit: () => void
  onDelete: () => void
}) {
  const { t } = useTranslation()
  const occupants = (view.seats ?? []).filter((seat) => seat.occupied)
  const accountName = view.account.name.trim()
  const accountEmail = view.account.email.trim()
  const showAccountEmail = accountEmail !== "" && accountEmail !== accountName
  const nextRenewalDate = getNextMonthlyRenewalDate(view.account.opened_at)

  return (
    <Card className="relative gap-0 overflow-hidden p-0 animate-fade-up">
      <span
        className={cn(
          "absolute inset-y-0 left-0 w-1",
          view.is_full ? "bg-destructive" : view.seat_used === 0 ? "bg-brand" : "bg-success",
        )}
      />
      <div className="p-4">
        <div className="flex min-w-0 items-start justify-between gap-3">
          <div className="flex min-w-0 items-start gap-3">
            <span className="grid size-9 shrink-0 place-items-center rounded-md bg-brand/10 text-brand">
              <CircleParking className="size-[18px]" />
            </span>
            <div className="min-w-0">
              <h2 className="break-all text-sm font-semibold leading-5">{view.account.name}</h2>
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
          <AccountStatus view={view} />
        </div>

        <div className="mt-4 grid grid-cols-2 gap-x-4 gap-y-3 rounded-md border border-foreground/[0.06] bg-muted/45 p-3 text-xs">
          <div>
            <div className="text-muted-foreground">{t("accounts.colOccupancy")}</div>
            <div className="mt-1 font-semibold text-brand tabular-nums">
              {view.seat_used} / {view.seat_total}
            </div>
          </div>
          <div>
            <div className="text-muted-foreground">{t("accounts.costShort")}</div>
            <div className="mt-1 font-semibold text-gold tabular-nums">
              ¥{formatCents(view.account.cost_cents)}
            </div>
          </div>
          <div>
            <div className="text-muted-foreground">{t("accounts.openedAt")}</div>
            <div className="mt-1 tabular-nums">{view.account.opened_at || "-"}</div>
          </div>
          <div>
            <div className="text-muted-foreground">{t("accounts.nextRenewalAt")}</div>
            <div className="mt-1 font-medium text-brand tabular-nums">
              {nextRenewalDate || "-"}
            </div>
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
                      seat.active_customer_email.trim() ||
                      seat.active_subscription_name.trim() ||
                      seat.seat.name
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

      <div className="flex items-center gap-2 border-t bg-muted/35 px-4 py-3">
        <Button variant="outline" size="sm" onClick={onEdit}>
          <Pencil data-slot="icon" />
          {t("common.edit")}
        </Button>
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

  const [expanded, setExpanded] = React.useState<Set<number>>(new Set())
  const [accountDialogOpen, setAccountDialogOpen] = React.useState(false)
  const [editingAccount, setEditingAccount] = React.useState<AccountPrefill | null>(null)
  const [deleteTarget, setDeleteTarget] = React.useState<{ id: number; name: string } | null>(null)
  const [subscriptionDialogOpen, setSubscriptionDialogOpen] = React.useState(false)
  const [subscriptionPrefill, setSubscriptionPrefill] = React.useState<SubscriptionPrefill | null>(null)
  const [search, setSearch] = React.useState("")
  const [filter, setFilter] = React.useState<AccountsFilter>("all")
  const [page, setPage] = React.useState(1)

  const deleteMutation = useAppMutation((id: number) => deleteAccount(id), {
    onSuccess: () => setDeleteTarget(null),
  })

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

  const accounts = React.useMemo(() => accountsQuery.data ?? [], [accountsQuery.data])

  const filteredAccounts = React.useMemo(() => {
    const matchesFilter = (view: AccountView) => {
      const seats = view.seats ?? []
      if (filter === "all") return true
      if (filter === "sale") {
        return seats.some((seat) => seat.occupied && !seat.active_is_resale)
      }
      if (filter === "resale") {
        return seats.some((seat) => seat.occupied && seat.active_is_resale)
      }
      return true
    }

    const query = search.trim().toLowerCase()
    return accounts.filter((view) => {
      if (!matchesFilter(view)) return false
      if (!query) return true
      const seatFields = (view.seats ?? []).flatMap((seat) => [
        seat.seat.name,
        seat.active_subscription_name,
        seat.active_customer_email,
        seat.active_customer_wechat,
        seat.active_trade_url,
        seat.active_remark,
      ])
      const nextRenewalDate = getNextMonthlyRenewalDate(view.account.opened_at)
      return [
        view.account.name,
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
  }, [accounts, filter, search])

  const pageCount = Math.max(1, Math.ceil(filteredAccounts.length / ACCOUNTS_PER_PAGE))
  const safePage = Math.min(page, pageCount)
  const pageStartIndex = (safePage - 1) * ACCOUNTS_PER_PAGE
  const pagedAccounts = filteredAccounts.slice(pageStartIndex, pageStartIndex + ACCOUNTS_PER_PAGE)
  const pageEndIndex = pageStartIndex + pagedAccounts.length

  React.useEffect(() => {
    const query = search.trim().toLowerCase()
    if (!query && filter === "all") return
    const timer = window.setTimeout(() => {
      setExpanded((previous) => {
        const next = new Set(previous)
        for (const view of filteredAccounts) {
          const seatHit = (view.seats ?? []).some((seat) => {
            if (filter === "sale" && seat.occupied && !seat.active_is_resale) return true
            if (filter === "resale" && seat.occupied && seat.active_is_resale) return true
            if (!query) return false
            return [
              seat.seat.name,
              seat.active_subscription_name,
              seat.active_customer_email,
              seat.active_customer_wechat,
              seat.active_trade_url,
              seat.active_remark,
            ].some((field) => field?.toLowerCase().includes(query))
          })
          if (seatHit) next.add(view.account.id)
        }
        return next
      })
    }, 0)
    return () => window.clearTimeout(timer)
  }, [filteredAccounts, filter, search])

  return (
    <>
      <PageHeader
        title={t("accounts.title")}
        description={t("accounts.desc")}
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
            <Select
              value={filter}
              onValueChange={(value) => {
                setFilter(value as AccountsFilter)
                setPage(1)
              }}
            >
              <SelectTrigger className="h-9 flex-1 sm:w-28 sm:flex-none" aria-label={t("accounts.filterLabel")}>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">{t("calendar.filterAll")}</SelectItem>
                <SelectItem value="sale">{t("accounts.filterSale")}</SelectItem>
                <SelectItem value="resale">{t("calendar.filterResale")}</SelectItem>
              </SelectContent>
            </Select>
            <Button className="flex-1 sm:flex-none" onClick={openCreate}>
              <Plus data-slot="icon" />
              {t("accounts.newAccount")}
            </Button>
          </>
        }
      />

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
                onEdit={() => openEdit(view)}
                onDelete={() =>
                  setDeleteTarget({ id: view.account.id, name: view.account.name })
                }
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
                <TableHead className="w-52 text-right">{t("accounts.colActions")}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody className="[&_tr:last-child]:border-b [&_tr:last-child]:border-b-border">
              {pagedAccounts.map((view) => {
                const isExpanded = expanded.has(view.account.id)
                const occupants = (view.seats ?? []).filter((seat) => seat.occupied)
                const accountName = view.account.name.trim()
                const accountEmail = view.account.email.trim()
                const showAccountEmail = accountEmail !== "" && accountEmail !== accountName
                const nextRenewalDate = getNextMonthlyRenewalDate(view.account.opened_at)
                return (
                  <TableRow
                    key={view.account.id}
                    className={cn(
                      "border-l-2",
                      view.is_full
                        ? "border-l-destructive"
                        : view.seat_used === 0
                          ? "border-l-brand"
                          : "border-l-success",
                    )}
                  >
                    <TableCell>
                      <div className="flex min-w-0 items-start gap-2.5">
                        <span className="mt-0.5 grid size-8 shrink-0 place-items-center rounded-md bg-brand/10 text-brand">
                          <CircleParking className="size-4" />
                        </span>
                        <div className="min-w-0">
                          <div className="truncate font-medium" title={view.account.name}>
                            {view.account.name}
                          </div>
                          <div className="mt-1 flex min-w-0 flex-wrap gap-x-2 gap-y-0.5 text-xs text-muted-foreground">
                        {showAccountEmail ? (
                          <span className="max-w-full truncate" title={accountEmail}>
                            {accountEmail}
                          </span>
                        ) : null}
                        {view.account.space_name ? (
                          <span className="max-w-full truncate" title={view.account.space_name}>
                            {view.account.space_name}
                          </span>
                        ) : null}
                        {view.account.opened_at ? (
                          <span className="inline-flex items-center gap-1">
                            <span>{t("accounts.openedAt")}</span>
                            <span className="font-mono tabular-nums">
                              {view.account.opened_at}
                            </span>
                          </span>
                        ) : null}
                        {nextRenewalDate ? (
                          <span className="inline-flex items-center gap-1 text-brand">
                            <span>{t("accounts.nextRenewalAt")}</span>
                            <span className="font-mono tabular-nums">{nextRenewalDate}</span>
                          </span>
                        ) : null}
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
                                    seat.active_customer_email.trim() ||
                                    seat.active_subscription_name.trim() ||
                                    seat.seat.name
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
                          {t("accounts.costShort")}{" "}
                          <span className="font-mono font-semibold text-gold tabular-nums">
                            ¥{formatCents(view.account.cost_cents)}
                          </span>
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
                start: pageStartIndex + 1,
                end: pageEndIndex,
                total: filteredAccounts.length,
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
                  start: pageStartIndex + 1,
                  end: pageEndIndex,
                  total: filteredAccounts.length,
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
      <SubscriptionDialog
        open={subscriptionDialogOpen}
        onOpenChange={setSubscriptionDialogOpen}
        prefill={subscriptionPrefill}
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
