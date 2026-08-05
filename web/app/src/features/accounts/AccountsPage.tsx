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
    setPage(1)
  }, [search, filter])

  React.useEffect(() => {
    if (page !== safePage) {
      setPage(safePage)
    }
  }, [page, safePage])

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
            <div className="relative">
              <Search className="absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                value={search}
                onChange={(event) => setSearch(event.target.value)}
                placeholder={t("accounts.searchPlaceholder")}
                aria-label={t("accounts.title")}
                className="h-9 w-56 pl-8 text-[13px] sm:w-72"
              />
            </div>
            <Select
              value={filter}
              onValueChange={(value) => setFilter(value as AccountsFilter)}
            >
              <SelectTrigger className="h-9 w-28" aria-label={t("accounts.filterLabel")}>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">{t("calendar.filterAll")}</SelectItem>
                <SelectItem value="sale">{t("accounts.filterSale")}</SelectItem>
                <SelectItem value="resale">{t("calendar.filterResale")}</SelectItem>
              </SelectContent>
            </Select>
            <Button onClick={openCreate}>
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
        <Card className="gap-0 overflow-hidden p-0 animate-fade-up">
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
            <TableBody>
              {pagedAccounts.map((view) => {
                const isExpanded = expanded.has(view.account.id)
                const occupants = (view.seats ?? []).filter((seat) => seat.occupied)
                const accountName = view.account.name.trim()
                const accountEmail = view.account.email.trim()
                const showAccountEmail = accountEmail !== "" && accountEmail !== accountName
                const nextRenewalDate = getNextMonthlyRenewalDate(view.account.opened_at)
                return (
                  <TableRow key={view.account.id}>
                    <TableCell>
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
                    </TableCell>
                    <TableCell>
                      {view.seat_used > 0 ? (
                        <div>
                          <button
                            type="button"
                            aria-expanded={isExpanded}
                            title={t("accounts.occupancyTitle")}
                            onClick={() => toggleExpanded(view.account.id)}
                            className="inline-flex items-center gap-1 rounded-md px-1.5 py-0.5 font-mono text-[13px] tabular-nums transition-colors hover:bg-accent"
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
                                {occupants.map((seat) => (
                                  <button
                                    key={seat.seat.id}
                                    type="button"
                                    onClick={() => {
                                      setSubscriptionPrefill(prefillFromSeat(seat))
                                      setSubscriptionDialogOpen(true)
                                    }}
                                    className="max-w-full truncate rounded-md border bg-muted/50 px-2 py-0.5 text-xs transition-colors hover:border-brand/40 hover:text-brand"
                                  >
                                    {seat.active_subscription_name}
                                  </button>
                                ))}
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
                          <span className="font-mono tabular-nums">
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
          {pageCount > 1 ? (
            <div className="flex flex-col items-center justify-between gap-3 border-t px-4 py-3 text-xs text-muted-foreground sm:flex-row">
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
        </Card>
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
