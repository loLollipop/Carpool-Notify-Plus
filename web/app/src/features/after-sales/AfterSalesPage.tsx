import * as React from "react"
import { useSearchParams } from "react-router-dom"
import { useTranslation } from "react-i18next"
import {
  AlertTriangle,
  ArrowRightLeft,
  BadgeDollarSign,
  CheckCircle2,
  ChevronLeft,
  ChevronRight,
  Mail,
  MessageCircle,
  Pencil,
  RotateCcw,
  Search,
  ShieldAlert,
  UserRoundMinus,
  Users,
} from "lucide-react"

import {
  reassignAfterSalesCase,
  setAfterSalesRefunded,
  updateAfterSalesCase,
} from "@/api/endpoints"
import { useAppMutation } from "@/api/mutations"
import { useAccountOptions, useAfterSales } from "@/api/queries"
import type { AfterSalesCaseView, AfterSalesStatus } from "@/api/types"
import { ConfirmDialog } from "@/components/confirm-dialog"
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
import { Label } from "@/components/ui/label"
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
import { Textarea } from "@/components/ui/textarea"
import { cn } from "@/lib/utils"

type AfterSalesFilter = "all" | AfterSalesStatus
type AfterSalesBusinessFilter = "all" | "team" | "plus"

const CASES_PER_PAGE = 5

function StatusBadge({ status }: { status: AfterSalesStatus }) {
  const { t } = useTranslation()
  if (status === "reassigned") {
    return (
      <Badge variant="secondary" className="font-normal text-brand">
        <ArrowRightLeft />
        {t("afterSales.statusReassigned")}
      </Badge>
    )
  }
  if (status === "refunded") {
    return (
      <Badge variant="success" className="font-normal">
        <CheckCircle2 />
        {t("afterSales.statusRefunded")}
      </Badge>
    )
  }
  if (status === "review") {
    return (
      <Badge variant="warning" className="font-normal">
        <AlertTriangle />
        {t("afterSales.statusReview")}
      </Badge>
    )
  }
  return (
    <Badge variant="destructive" className="font-normal">
      <BadgeDollarSign />
      {t("afterSales.statusPending")}
    </Badge>
  )
}

function ContactLines({ view }: { view: AfterSalesCaseView }) {
  const { t } = useTranslation()
  const plusRental = view.case.business_type === "plus"
  return (
    <div className="grid min-w-0 gap-1 text-xs text-muted-foreground">
      <div className="flex min-w-0 items-center gap-1.5">
        {plusRental ? (
          <UserRoundMinus className="size-3.5 shrink-0 text-brand" />
        ) : (
          <Mail className="size-3.5 shrink-0 text-brand" />
        )}
        <span
          className={cn("truncate", !plusRental && "font-mono")}
          title={(plusRental ? view.case.account_name : view.case.customer_email) || undefined}
        >
          {(plusRental ? view.case.account_name : view.case.customer_email) || t("afterSales.missingContact")}
        </span>
      </div>
      <div className="flex min-w-0 items-center gap-1.5">
        <MessageCircle className="size-3.5 shrink-0 text-success" />
        <span className="truncate" title={view.case.customer_wechat || undefined}>
          {view.case.customer_wechat || t("afterSales.missingContact")}
        </span>
      </div>
    </div>
  )
}

function SourceBadge({ view }: { view: AfterSalesCaseView }) {
  const { t } = useTranslation()
  const plusRental = view.case.business_type === "plus"
  const cancellation = view.case.source === "customer_cancellation"
  return (
    <Badge
      variant={cancellation ? "secondary" : "outline"}
      className="mb-1.5 w-fit font-normal"
    >
      {cancellation ? <UserRoundMinus /> : <ShieldAlert />}
      {plusRental
        ? t("afterSales.sourcePlus")
        : cancellation
          ? t("afterSales.sourceCancellation")
          : t("afterSales.sourceAccountBan")}
    </Badge>
  )
}

function AccountSnapshot({ view }: { view: AfterSalesCaseView }) {
  const { t } = useTranslation()
  const plusRental = view.case.business_type === "plus"
  const cancellation = view.case.source === "customer_cancellation"
  return (
    <div className="min-w-0">
      <SourceBadge view={view} />
      <div className="truncate font-medium" title={view.case.account_email || view.case.account_name}>
        {view.case.account_email || view.case.account_name || "-"}
      </div>
      <div className="mt-1 truncate text-xs text-muted-foreground" title={view.case.account_space_name}>
        {plusRental ? t("afterSales.plusAccount") : view.case.account_space_name || "-"}
      </div>
      <div className={cn("mt-1 font-mono text-xs tabular-nums", cancellation ? "text-muted-foreground" : "text-destructive")}>
        {cancellation ? t("afterSales.requestedAt") : t("afterSales.bannedAt")} · {view.case.banned_date}
      </div>
      {cancellation && view.expires_at_label ? (
        <div className="mt-1 text-xs text-muted-foreground">
          {t("afterSales.autoRestoreAt", { time: view.expires_at_label })}
        </div>
      ) : null}
    </div>
  )
}

function ReplacementSnapshot({ view }: { view: AfterSalesCaseView }) {
  const { t } = useTranslation()
  if (view.case.status !== "reassigned") return null
  return (
    <div className="mt-2 border-l-2 border-success/50 pl-2 text-xs">
      <div className="font-medium text-success">{t("afterSales.replacementAccount")}</div>
      <div className="mt-0.5 truncate" title={view.case.replacement_account_email || view.case.replacement_account_name}>
        {view.case.replacement_account_email || view.case.replacement_account_name}
      </div>
      <div className="mt-0.5 truncate text-muted-foreground">
        {[view.case.replacement_space_name, view.case.replacement_seat_name].filter(Boolean).join(" · ") || "-"}
      </div>
    </div>
  )
}

function customerLabel(view: AfterSalesCaseView | null) {
  if (!view) return ""
  if (view.case.business_type === "plus") {
    return view.case.account_name || view.case.customer_wechat || view.case.account_email
  }
  return view.case.customer_email || view.case.customer_wechat
}

function PeriodDetail({ view }: { view: AfterSalesCaseView }) {
  const { t } = useTranslation()
  if (!view.case.period_start) {
    return <span className="text-xs text-warning-foreground">{t("afterSales.periodReview")}</span>
  }
  return (
    <div className="text-xs">
      <div className="font-mono tabular-nums">
        {view.case.period_start} - {view.case.period_end}
      </div>
      <div className="mt-1 text-muted-foreground">
        {t("afterSales.usage", {
          used: view.case.used_days,
          remaining: view.case.remaining_days,
        })}
      </div>
    </div>
  )
}

function RefundActions({
  view,
  onEdit,
  onReassign,
  onToggleRefunded,
}: {
  view: AfterSalesCaseView
  onEdit: () => void
  onReassign: () => void
  onToggleRefunded: () => void
}) {
  const { t } = useTranslation()
  const refunded = view.case.status === "refunded"
  const reassigned = view.case.status === "reassigned"
  const cancellation = view.case.source === "customer_cancellation"
  const completedCancellation = cancellation && refunded
  return (
    <div className="flex flex-wrap items-center justify-end gap-1">
      {!reassigned && !completedCancellation ? (
        <Button variant="outline" size="sm" onClick={onEdit}>
          <Pencil data-slot="icon" />
          {t("common.edit")}
        </Button>
      ) : null}
      {!refunded && !reassigned && !cancellation ? (
        <Button variant="outline" size="sm" onClick={onReassign}>
          <ArrowRightLeft data-slot="icon" />
          {t("afterSales.reassign")}
        </Button>
      ) : null}
      {!reassigned && !completedCancellation ? (
        <Button
          variant={refunded ? "ghost" : "default"}
          size="sm"
          className={cn(refunded && "text-muted-foreground")}
          onClick={onToggleRefunded}
        >
          {refunded ? <RotateCcw data-slot="icon" /> : <CheckCircle2 data-slot="icon" />}
          {refunded ? t("afterSales.undoRefunded") : t("afterSales.markRefunded")}
        </Button>
      ) : null}
    </div>
  )
}

function ReassignCaseDialog({
  view,
  onOpenChange,
}: {
  view: AfterSalesCaseView | null
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation()
  const optionsQuery = useAccountOptions(0, view !== null)
  const options = React.useMemo(
    () => (optionsQuery.data ?? []).filter((option) => (option.seats ?? []).length > 0),
    [optionsQuery.data],
  )
  const [accountID, setAccountID] = React.useState("")
  const [seatID, setSeatID] = React.useState("")
  const selectedAccount = options.find((option) => String(option.id) === accountID)
  const mutation = useAppMutation(
    (input: { id: number; accountId: number; seatId: number }) =>
      reassignAfterSalesCase(input.id, input.accountId, input.seatId),
    { onSuccess: () => onOpenChange(false) },
  )

  return (
    <Dialog open={view !== null} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{t("afterSales.reassignTitle")}</DialogTitle>
          <DialogDescription>
            {t("afterSales.reassignDesc", { email: view?.case.customer_email || "-" })}
          </DialogDescription>
        </DialogHeader>
        <div className="grid gap-4 py-1">
          <div className="grid gap-2">
            <Label>{t("afterSales.replacementAccount")}</Label>
            <Select
              value={accountID}
              onValueChange={(value) => {
                setAccountID(value)
                const account = options.find((option) => String(option.id) === value)
                setSeatID(String(account?.seats?.[0]?.id ?? ""))
              }}
            >
              <SelectTrigger>
                <SelectValue placeholder={t("afterSales.replacementAccountPlaceholder")} />
              </SelectTrigger>
              <SelectContent>
                {options.map((option) => (
                  <SelectItem key={option.id} value={String(option.id)}>
                    {option.email || option.name} · {option.space_name || t("afterSales.spaceUnnamed")}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="grid gap-2">
            <Label>{t("afterSales.replacementSeat")}</Label>
            <Select value={seatID} disabled={!selectedAccount} onValueChange={setSeatID}>
              <SelectTrigger>
                <SelectValue placeholder={t("afterSales.replacementSeatPlaceholder")} />
              </SelectTrigger>
              <SelectContent>
                {(selectedAccount?.seats ?? []).map((seat) => (
                  <SelectItem key={seat.id} value={String(seat.id)}>{seat.name}</SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          {optionsQuery.isSuccess && options.length === 0 ? (
            <p className="text-sm text-destructive">{t("afterSales.noReplacementSeat")}</p>
          ) : null}
          <p className="text-xs leading-5 text-muted-foreground">{t("afterSales.reassignHint")}</p>
        </div>
        <DialogFooter>
          <Button variant="outline" disabled={mutation.isPending} onClick={() => onOpenChange(false)}>
            {t("common.cancel")}
          </Button>
          <Button
            disabled={mutation.isPending || !view || !accountID || !seatID}
            onClick={() => {
              if (!view) return
              mutation.mutate({ id: view.case.id, accountId: Number(accountID), seatId: Number(seatID) })
            }}
          >
            <ArrowRightLeft data-slot="icon" />
            {t("afterSales.confirmReassign")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function EditCaseDialog({
  view,
  onOpenChange,
}: {
  view: AfterSalesCaseView | null
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation()
  const [amount, setAmount] = React.useState(view?.refund_amount_yuan ?? "")
  const [note, setNote] = React.useState(view?.case.note ?? "")
  const mutation = useAppMutation(
    (input: { id: number; amount: string; note: string }) =>
      updateAfterSalesCase(input.id, { refund_amount_yuan: input.amount, note: input.note }),
    { onSuccess: () => onOpenChange(false) },
  )

  return (
    <Dialog open={view !== null} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("afterSales.editTitle")}</DialogTitle>
          <DialogDescription>{t("afterSales.editDesc")}</DialogDescription>
        </DialogHeader>
        <div className="grid gap-4 py-1">
          <div className="grid gap-2">
            <Label htmlFor="after-sales-refund">{t("afterSales.refundAmount")}</Label>
            <Input
              id="after-sales-refund"
              inputMode="decimal"
              value={amount}
              onChange={(event) => setAmount(event.target.value)}
            />
          </div>
          <div className="grid gap-2">
            <Label htmlFor="after-sales-note">{t("afterSales.note")}</Label>
            <Textarea
              id="after-sales-note"
              rows={4}
              value={note}
              placeholder={t("afterSales.notePlaceholder")}
              onChange={(event) => setNote(event.target.value)}
            />
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" disabled={mutation.isPending} onClick={() => onOpenChange(false)}>
            {t("common.cancel")}
          </Button>
          <Button
            disabled={mutation.isPending || !amount.trim()}
            onClick={() => {
              if (view) mutation.mutate({ id: view.case.id, amount, note })
            }}
          >
            {t("common.save")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

export function AfterSalesPage() {
  const { t } = useTranslation()
  const query = useAfterSales()
  const [searchParams, setSearchParams] = useSearchParams()
  const accountFilter = Number(searchParams.get("account") || 0)
  const caseFilter = Number(searchParams.get("case") || 0)
  const [search, setSearch] = React.useState("")
  const [filter, setFilter] = React.useState<AfterSalesFilter>("all")
  const [businessFilter, setBusinessFilter] = React.useState<AfterSalesBusinessFilter>("all")
  const [page, setPage] = React.useState(1)
  const [editTarget, setEditTarget] = React.useState<AfterSalesCaseView | null>(null)
  const [reassignTarget, setReassignTarget] = React.useState<AfterSalesCaseView | null>(null)
  const [refundTarget, setRefundTarget] = React.useState<AfterSalesCaseView | null>(null)
  const [statDetail, setStatDetail] = React.useState<StatDetailState | null>(null)

  const toggleRefundMutation = useAppMutation(
    (input: { id: number; refunded: boolean }) => setAfterSalesRefunded(input.id, input.refunded),
    { onSuccess: () => setRefundTarget(null) },
  )

  const allCases = React.useMemo(() => query.data?.cases ?? [], [query.data?.cases])
  const summaryCases = React.useMemo(
    () => query.data?.summary_cases ?? allCases,
    [allCases, query.data?.summary_cases],
  )
  const filteredCases = React.useMemo(() => {
    const needle = search.trim().toLowerCase()
    return allCases.filter((view) => {
      if (accountFilter > 0 && view.case.account_id !== accountFilter) return false
      if (caseFilter > 0 && view.case.id !== caseFilter) return false
      if (businessFilter !== "all" && view.case.business_type !== businessFilter) return false
      if (filter !== "all" && view.case.status !== filter) return false
      if (!needle) return true
      return [
        view.case.customer_email,
        view.case.customer_wechat,
        view.case.account_name,
        view.case.account_email,
        view.case.account_space_name,
        view.case.replacement_account_name,
        view.case.replacement_account_email,
        view.case.replacement_space_name,
        view.case.replacement_seat_name,
        view.case.banned_date,
        view.case.note,
        view.refund_amount_yuan,
      ].some((field) => field.toLowerCase().includes(needle))
    })
  }, [accountFilter, allCases, businessFilter, caseFilter, filter, search])

  const pageCount = Math.max(1, Math.ceil(filteredCases.length / CASES_PER_PAGE))
  const safePage = Math.min(page, pageCount)
  const start = (safePage - 1) * CASES_PER_PAGE
  const pagedCases = filteredCases.slice(start, start + CASES_PER_PAGE)
  const summary = query.data?.summary

  const openStatDetail = (key: "affected" | "pending" | "reassigned" | "refunded") => {
    const source = key === "affected"
      ? summaryCases
      : key === "pending"
        ? summaryCases.filter((view) => view.case.status === "pending" || view.case.status === "review")
        : summaryCases.filter((view) => view.case.status === key)
    const title = key === "affected"
      ? t("afterSales.kpiAffected")
      : key === "pending"
        ? t("afterSales.kpiPending")
        : key === "reassigned"
          ? t("afterSales.kpiReassigned")
          : t("afterSales.kpiRefunded")
    setStatDetail({
      title,
      items: source.map((view) => ({
        id: view.case.id,
        title: view.case.customer_email || view.case.customer_wechat || `#${view.case.id}`,
        subtitle: view.case.customer_wechat || view.case.account_name,
        meta: [view.case.account_email, view.case.period_end, view.status_label],
        value: key === "refunded" ? `¥${view.refund_amount_yuan}` : `¥${view.paid_amount_yuan}`,
        valueTone: key === "refunded" ? "danger" : key === "reassigned" ? "success" : "default",
        searchText: `${view.case.replacement_account_name} ${view.case.note}`,
      })),
    })
  }

  return (
    <div className="flex flex-col xl:h-[calc(100dvh-7rem)] xl:min-h-0 xl:overflow-hidden">
      <PageHeader
        title={t("afterSales.title")}
        actions={
          accountFilter > 0 || caseFilter > 0 ? (
            <Button
              variant="outline"
              onClick={() => {
                setPage(1)
                setSearchParams({})
              }}
            >
              {t("afterSales.showAllAccounts")}
            </Button>
          ) : null
        }
      />

      {query.isPending ? (
        <Skeleton className="h-28 rounded-lg" />
      ) : (
        <div className="mb-5 grid grid-cols-2 gap-3 lg:grid-cols-4">
          {[
            {
              key: "affected" as const,
              label: t("afterSales.kpiAffected"),
              value: summary?.total_count ?? 0,
              hint: t("afterSales.kpiAffectedHint"),
              icon: Users,
              tone: "bg-brand/10 text-brand",
            },
            {
              key: "pending" as const,
              label: t("afterSales.kpiPending"),
              value: (summary?.pending_count ?? 0) + (summary?.review_count ?? 0),
              hint: t("afterSales.kpiPendingHint", { review: summary?.review_count ?? 0 }),
              icon: ShieldAlert,
              tone: "bg-destructive/10 text-destructive",
            },
            {
              key: "reassigned" as const,
              label: t("afterSales.kpiReassigned"),
              value: summary?.reassigned_count ?? 0,
              hint: t("afterSales.kpiReassignedHint"),
              icon: ArrowRightLeft,
              tone: "bg-brand/10 text-brand",
            },
            {
              key: "refunded" as const,
              label: t("afterSales.kpiRefunded"),
              value: `¥${summary?.refunded_amount_yuan ?? "0.00"}`,
              hint: t("afterSales.kpiRefundedHint", { count: summary?.refunded_count ?? 0 }),
              icon: CheckCircle2,
              tone: "bg-success/10 text-success",
            },
          ].map((item) => (
            <Card key={item.label} className="group relative gap-0 overflow-hidden p-0 transition-[border-color,background-color,box-shadow] hover:border-input hover:bg-accent/25 hover:shadow-lift">
              <button
                type="button"
                onClick={() => openStatDetail(item.key)}
                aria-label={item.label}
                className="w-full p-4 text-left outline-none focus-visible:ring-2 focus-visible:ring-brand/45 focus-visible:ring-inset"
              >
                <div className="flex items-start justify-between gap-3">
                <div className="min-w-0">
                  <div className="text-xs text-muted-foreground">{item.label}</div>
                  <div className="mt-3 truncate text-2xl font-semibold tabular-nums">{item.value}</div>
                  <div className="mt-1.5 truncate text-xs text-muted-foreground">{item.hint}</div>
                </div>
                <span className={cn("grid size-9 shrink-0 place-items-center rounded-md", item.tone)}>
                  <item.icon className="size-4" />
                </span>
                </div>
                <span className="absolute inset-x-0 bottom-0 h-0.5 origin-left scale-x-0 bg-brand transition-transform duration-300 group-hover:scale-x-100 group-focus-within:scale-x-100" />
              </button>
            </Card>
          ))}
        </div>
      )}

      <div className="mb-4 flex flex-col gap-2 sm:flex-row">
        <div className="relative min-w-0 flex-1">
          <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            className="pl-9"
            value={search}
            placeholder={t("afterSales.searchPlaceholder")}
            onChange={(event) => {
              setPage(1)
              setSearch(event.target.value)
            }}
          />
        </div>
        <Select
          value={filter}
          onValueChange={(value) => {
            setPage(1)
            setFilter(value as AfterSalesFilter)
          }}
        >
          <SelectTrigger className="w-full sm:w-40" aria-label={t("afterSales.filterLabel")}>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">{t("afterSales.filterAll")}</SelectItem>
            <SelectItem value="pending">{t("afterSales.statusPending")}</SelectItem>
            <SelectItem value="review">{t("afterSales.statusReview")}</SelectItem>
            <SelectItem value="refunded">{t("afterSales.statusRefunded")}</SelectItem>
            <SelectItem value="reassigned">{t("afterSales.statusReassigned")}</SelectItem>
          </SelectContent>
        </Select>
        <Select
          value={businessFilter}
          onValueChange={(value) => {
            setPage(1)
            setBusinessFilter(value as AfterSalesBusinessFilter)
          }}
        >
          <SelectTrigger className="w-full sm:w-36" aria-label={t("afterSales.businessFilterLabel")}>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">{t("afterSales.businessFilterAll")}</SelectItem>
            <SelectItem value="team">{t("afterSales.businessFilterTeam")}</SelectItem>
            <SelectItem value="plus">{t("afterSales.businessFilterPlus")}</SelectItem>
          </SelectContent>
        </Select>
      </div>

      {query.isPending ? (
        <Skeleton className="min-h-96 flex-1 rounded-lg" />
      ) : query.isError ? (
        <Card className="flex-1 items-center justify-center gap-3 py-16 text-center">
          <p className="text-sm text-muted-foreground">{t("common.loadFailed")}</p>
          <Button variant="outline" onClick={() => query.refetch()}>{t("common.retry")}</Button>
        </Card>
      ) : filteredCases.length === 0 ? (
        <Card className="flex-1 items-center justify-center py-16 text-center">
          <ShieldAlert className="size-8 text-muted-foreground/50" />
          <p className="text-sm text-muted-foreground">{t("afterSales.empty")}</p>
        </Card>
      ) : (
        <div className="flex min-h-0 flex-1 flex-col gap-3">
          <div className="grid gap-3 md:hidden">
            {pagedCases.map((view) => (
              <Card key={view.case.id} className="gap-0 overflow-hidden p-0">
                <div className="flex items-start justify-between gap-3 border-b px-4 py-3.5">
                  <ContactLines view={view} />
                  <StatusBadge status={view.case.status} />
                </div>
                <div className="grid grid-cols-2 gap-x-4 gap-y-3 px-4 py-3.5 text-sm">
                  <div className="col-span-2">
                    <div className="mb-1 text-xs text-muted-foreground">{t("afterSales.ownerAccount")}</div>
                    <AccountSnapshot view={view} />
                    <ReplacementSnapshot view={view} />
                  </div>
                  <div className="col-span-2 border-t pt-3">
                    <PeriodDetail view={view} />
                  </div>
                  <div>
                    <div className="text-xs text-muted-foreground">{t("afterSales.paidAmount")}</div>
                    <div className="mt-1 font-mono tabular-nums">¥{view.paid_amount_yuan}</div>
                  </div>
                  <div>
                    <div className="text-xs text-muted-foreground">{t("afterSales.refundAmount")}</div>
                    <div className={cn("mt-1 font-mono font-semibold tabular-nums", view.case.status === "reassigned" ? "text-muted-foreground" : "text-destructive")}>
                      {view.case.status === "reassigned" ? t("afterSales.noRefundRequired") : `¥${view.refund_amount_yuan}`}
                    </div>
                  </div>
                  {view.case.note ? (
                    <div className="col-span-2 rounded-md bg-muted/50 px-3 py-2 text-xs leading-5 text-muted-foreground">
                      {view.case.note}
                    </div>
                  ) : null}
                </div>
                <div className="border-t bg-muted/25 px-4 py-3">
                  <RefundActions
                    view={view}
                    onEdit={() => setEditTarget(view)}
                    onReassign={() => setReassignTarget(view)}
                    onToggleRefunded={() => setRefundTarget(view)}
                  />
                </div>
              </Card>
            ))}
          </div>

          <Card className="hidden min-h-0 flex-1 gap-0 overflow-auto p-0 md:flex">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("afterSales.customer")}</TableHead>
                  <TableHead>{t("afterSales.ownerAccount")}</TableHead>
                  <TableHead>{t("afterSales.period")}</TableHead>
                  <TableHead className="text-right">{t("afterSales.paidAmount")}</TableHead>
                  <TableHead className="text-right">{t("afterSales.refundAmount")}</TableHead>
                  <TableHead>{t("afterSales.status")}</TableHead>
                  <TableHead className="w-64 text-right">{t("afterSales.actions")}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {pagedCases.map((view) => (
                  <TableRow key={view.case.id}>
                    <TableCell className="max-w-56 whitespace-normal"><ContactLines view={view} /></TableCell>
                    <TableCell className="max-w-52 whitespace-normal">
                      <AccountSnapshot view={view} />
                      <ReplacementSnapshot view={view} />
                    </TableCell>
                    <TableCell className="whitespace-normal"><PeriodDetail view={view} /></TableCell>
                    <TableCell className="text-right font-mono tabular-nums">¥{view.paid_amount_yuan}</TableCell>
                    <TableCell className={cn("text-right font-mono font-semibold tabular-nums", view.case.status === "reassigned" ? "text-muted-foreground" : "text-destructive")}>
                      {view.case.status === "reassigned" ? t("afterSales.noRefundRequired") : `¥${view.refund_amount_yuan}`}
                    </TableCell>
                    <TableCell><StatusBadge status={view.case.status} /></TableCell>
                    <TableCell className="text-right">
                      <RefundActions
                        view={view}
                        onEdit={() => setEditTarget(view)}
                        onReassign={() => setReassignTarget(view)}
                        onToggleRefunded={() => setRefundTarget(view)}
                      />
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </Card>

          <div className="flex items-center justify-between border-t pt-3 text-xs text-muted-foreground">
            <span>{t("afterSales.pageStatus", { page: safePage, pages: pageCount })}</span>
            <div className="flex items-center gap-1.5">
              <Button variant="outline" size="icon-sm" disabled={safePage <= 1} onClick={() => setPage(safePage - 1)}>
                <ChevronLeft />
              </Button>
              <Button variant="outline" size="icon-sm" disabled={safePage >= pageCount} onClick={() => setPage(safePage + 1)}>
                <ChevronRight />
              </Button>
            </div>
          </div>
        </div>
      )}

      <EditCaseDialog
        key={editTarget?.case.id ?? "closed"}
        view={editTarget}
        onOpenChange={(open) => {
          if (!open) setEditTarget(null)
        }}
      />
      <ReassignCaseDialog
        key={reassignTarget?.case.id ?? "closed"}
        view={reassignTarget}
        onOpenChange={(open) => {
          if (!open) setReassignTarget(null)
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
        open={refundTarget !== null}
        onOpenChange={(open) => {
          if (!open) setRefundTarget(null)
        }}
        title={refundTarget?.case.status === "refunded" ? t("afterSales.undoTitle") : t("afterSales.refundTitle")}
        description={
          refundTarget?.case.status === "refunded"
            ? t("afterSales.undoDesc", { email: customerLabel(refundTarget) })
            : t(
                refundTarget?.case.business_type === "plus"
                  ? "afterSales.plusRefundDesc"
                  : refundTarget?.case.source === "customer_cancellation"
                    ? "afterSales.cancellationRefundDesc"
                    : "afterSales.refundDesc",
                {
                  email: customerLabel(refundTarget),
                  customer: customerLabel(refundTarget),
                  amount: refundTarget?.refund_amount_yuan ?? "0.00",
                },
              )
        }
        actionLabel={refundTarget?.case.status === "refunded" ? t("afterSales.undoRefunded") : t("afterSales.markRefunded")}
        pending={toggleRefundMutation.isPending}
        onConfirm={() => {
          if (!refundTarget) return
          toggleRefundMutation.mutate({
            id: refundTarget.case.id,
            refunded: refundTarget.case.status !== "refunded",
          })
        }}
      />
    </div>
  )
}
