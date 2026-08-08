import * as React from "react"
import { useTranslation } from "react-i18next"
import {
  Bar,
  BarChart,
  CartesianGrid,
  Cell,
  Pie,
  PieChart,
  ResponsiveContainer,
  Tooltip as RechartsTooltip,
  XAxis,
  YAxis,
} from "recharts"
import {
  BadgeDollarSign,
  CalendarDays,
  ChevronLeft,
  ChevronRight,
  CircleDot,
  Eye,
  HandCoins,
  Pencil,
  Search,
  Trash2,
  Wallet,
} from "lucide-react"

import { deleteBill } from "@/api/endpoints"
import { useAppMutation } from "@/api/mutations"
import { useBills } from "@/api/queries"
import type { BillView, BillsSummary } from "@/api/types"
import {
  AmountPrivacyToggle,
} from "@/components/amount-privacy-toggle"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { NumberTicker } from "@/components/number-ticker"
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
import { AMOUNT_MASK, VALUE_MASK, maskAmount } from "@/lib/amount-privacy"
import { useAmountPrivacy } from "@/hooks/use-amount-privacy"
import { BillEditDialog } from "./BillEditDialog"
import { SubscriptionViewDialog } from "./SubscriptionViewDialog"

const CHART_COLORS = [
  "var(--chart-1)",
  "var(--chart-2)",
  "var(--chart-3)",
  "var(--chart-4)",
  "var(--chart-5)",
]

const BILLS_PER_PAGE = 9
const AMOUNT_ITEMS_PER_PAGE = 6

function formatCents(cents: number) {
  return `¥${(cents / 100).toLocaleString("zh-CN", {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  })}`
}

function formatAxisCents(cents: number) {
  const yuan = cents / 100
  return `¥${yuan.toLocaleString("zh-CN", {
    notation: Math.abs(yuan) >= 10_000 ? "compact" : "standard",
    minimumFractionDigits: 0,
    maximumFractionDigits: Math.abs(yuan) < 10 ? 2 : 1,
  })}`
}

function getChartScale(values: number[]) {
  const minValue = Math.min(0, ...values)
  const maxValue = Math.max(0, ...values)
  const range = maxValue - minValue

  if (range === 0) {
    return { domain: [0, 10_000] as [number, number], ticks: [0, 2_500, 5_000, 7_500, 10_000] }
  }

  const roughStep = range / 5
  const magnitude = 10 ** Math.floor(Math.log10(roughStep))
  const normalizedStep = roughStep / magnitude
  const niceFactor = normalizedStep <= 1
    ? 1
    : normalizedStep <= 2
      ? 2
      : normalizedStep <= 2.5
        ? 2.5
        : normalizedStep <= 5
          ? 5
          : 10
  const step = Math.max(1, niceFactor * magnitude)
  const domainMin = Math.floor(minValue / step) * step
  const domainMax = Math.ceil(maxValue / step) * step
  const ticks = Array.from(
    { length: Math.round((domainMax - domainMin) / step) + 1 },
    (_, index) => domainMin + index * step,
  )

  return { domain: [domainMin, domainMax] as [number, number], ticks }
}

function ChartTooltip({
  active,
  payload,
  amountsHidden = false,
}: {
  active?: boolean
  amountsHidden?: boolean
  payload?: { payload: {
    name: string
    cents: number
    grossCents?: number
    refundCents?: number
    count?: number
  } }[]
}) {
  const { t } = useTranslation()
  if (!active || !payload || payload.length === 0) return null
  const item = payload[0].payload
  return (
    <div className="rounded-lg border bg-popover px-3 py-2 text-xs text-popover-foreground animate-fade-in">
      <div className="font-medium">{item.name}</div>
      {amountsHidden ? (
        <div className="mt-1 text-muted-foreground">{t("privacy.amountHidden")}</div>
      ) : item.grossCents !== undefined ? (
        <div className="mt-1.5 grid gap-1 tabular-nums text-muted-foreground">
          <div>{t("bills.colGross")} <span className="float-right ml-4 text-foreground">{formatCents(item.grossCents)}</span></div>
          <div>{t("bills.colRefund")} <span className="float-right ml-4 text-destructive">-{formatCents(item.refundCents ?? 0)}</span></div>
          <div>{t("bills.colNet")} <span className="float-right ml-4 font-medium text-success">{formatCents(item.cents)}</span></div>
        </div>
      ) : (
        <div className="mt-0.5 tabular-nums text-muted-foreground">
          {formatCents(item.cents)}
          {item.count !== undefined ? ` · ${t("bills.countSuffix", { count: item.count })}` : ""}
        </div>
      )}
    </div>
  )
}

function KpiCard({
  label,
  value,
  hint,
  icon,
  delay,
  tone,
}: {
  label: string
  value: string | number
  hint: string
  icon: React.ReactNode
  delay: number
  tone: "brand" | "success" | "gold"
}) {
  const toneClass = {
    brand: "bg-brand/10 text-brand",
    success: "bg-success/10 text-success",
    gold: "bg-gold/12 text-gold",
  }[tone]
  return (
    <Card
      className="group relative gap-0 overflow-hidden p-5 transition-[border-color,background-color] duration-200 animate-fade-up hover:border-input hover:bg-accent/25"
      style={{ animationDelay: `${delay}ms` }}
    >
      <div className="flex items-center justify-between text-xs font-medium text-muted-foreground">
        <span>{label}</span>
        <span className={cn("grid size-8 place-items-center rounded-md", toneClass)}>{icon}</span>
      </div>
      <div className="display-numeral mt-4 text-[27px] leading-none">
        <NumberTicker value={value} />
      </div>
      <div className="mt-2.5 text-xs text-muted-foreground">{hint}</div>
    </Card>
  )
}

function BillMobileCard({
  bill,
  amountsHidden,
  onView,
  onEdit,
  onDelete,
}: {
  bill: BillView
  amountsHidden: boolean
  onView: () => void
  onEdit: () => void
  onDelete: () => void
}) {
  const { t } = useTranslation()
  const primaryName = bill.account_name || bill.subscription_name
  const customerLine = bill.customer_email || bill.subscription_name

  return (
    <Card className="relative gap-0 overflow-hidden p-0 animate-fade-up">
      <span
        className={cn(
          "absolute inset-y-0 left-0 w-1",
          bill.archived ? "bg-muted-foreground/35" : bill.is_resale ? "bg-violet" : "bg-success",
        )}
      />
      <div className="p-4">
        <div className="flex min-w-0 items-start justify-between gap-3">
          <div className="min-w-0">
            <h3 className="truncate text-sm font-semibold" title={primaryName}>
              {primaryName}
            </h3>
            <p className="mt-1 break-all text-xs text-muted-foreground">
              {customerLine}
              {bill.seat_name ? ` · ${bill.seat_name}` : ""}
            </p>
          </div>
          <div className="flex shrink-0 flex-col items-end gap-1.5">
            <Badge variant={bill.archived ? "secondary" : "success"} className="font-normal">
              {bill.status_label}
            </Badge>
            {bill.is_resale ? (
              <Badge variant="outline" className="font-normal">
                {t("cards.resale")}
              </Badge>
            ) : null}
          </div>
        </div>

        <div className="mt-4 grid grid-cols-2 gap-x-4 gap-y-3 rounded-md border border-foreground/[0.06] bg-muted/45 p-3 text-xs">
          <div>
            <div className="text-muted-foreground">{t("bills.colGross")}</div>
            <div className="mt-1 font-medium tabular-nums">
              {maskAmount(amountsHidden, `¥${bill.amount_yuan}`)}
            </div>
          </div>
          <div>
            <div className="text-muted-foreground">{t("bills.colRefund")}</div>
            <div className={cn("mt-1 font-medium tabular-nums", bill.refund_cents > 0 && "text-destructive")}>
              {amountsHidden
                ? AMOUNT_MASK
                : `${bill.refund_cents > 0 ? "-" : ""}¥${bill.refund_yuan}`}
            </div>
          </div>
          <div>
            <div className="text-muted-foreground">{t("bills.colNet")}</div>
            <div className="mt-1 text-base font-semibold text-success tabular-nums">
              {maskAmount(amountsHidden, `¥${bill.net_amount_yuan}`)}
            </div>
          </div>
          <div>
            <div className="text-muted-foreground">{t("bills.colStartDate")}</div>
            <div className="mt-1 font-medium tabular-nums">{bill.due_date}</div>
          </div>
          <div className="col-span-2">
            <div className="text-muted-foreground">{t("bills.colPaidAt")}</div>
            <div className="mt-1 tabular-nums">{bill.paid_at_label || "-"}</div>
          </div>
          {bill.note ? (
            <div className="col-span-2">
              <div className="text-muted-foreground">{t("bills.colNote")}</div>
              <div className="mt-1 break-words leading-5">{bill.note}</div>
            </div>
          ) : null}
        </div>
      </div>

      <div className="grid grid-cols-3 gap-2 border-t bg-muted/35 px-3 py-3">
        <Button variant="outline" size="sm" onClick={onView}>
          <Eye data-slot="icon" />
          <span className="truncate">{t("bills.viewSubscription")}</span>
        </Button>
        <Button variant="outline" size="sm" onClick={onEdit}>
          <Pencil data-slot="icon" />
          <span className="truncate">{t("bills.editBill")}</span>
        </Button>
        <Button
          variant="ghost"
          size="sm"
          className="text-destructive hover:text-destructive"
          onClick={onDelete}
        >
          <Trash2 data-slot="icon" />
          {t("common.delete")}
        </Button>
      </div>
    </Card>
  )
}

// ---- 金额分布 (bar list, 按订阅 / 按账号) ----------------------------------------

function AmountDistributionCard({
  summary,
  amountsHidden,
}: {
  summary: BillsSummary
  amountsHidden: boolean
}) {
  const { t } = useTranslation()
  const [mode, setMode] = React.useState<"subscription" | "account">("subscription")
  const [page, setPage] = React.useState(1)

  const data = React.useMemo(() => {
    if (mode === "subscription") {
      return (summary.amount_by_subscription ?? []).map((bar) => ({
        name: bar.name,
        cents: bar.amount_cents,
        yuan: bar.amount_yuan,
      }))
    }
    return (summary.accounts ?? []).map((bar) => ({
      name: bar.account_name,
      cents: bar.amount_cents,
      yuan: bar.amount_yuan,
      count: bar.count,
    }))
  }, [summary, mode])

  const pageCount = Math.max(1, Math.ceil(data.length / AMOUNT_ITEMS_PER_PAGE))
  const safePage = Math.min(page, pageCount)
  const pageStartIndex = (safePage - 1) * AMOUNT_ITEMS_PER_PAGE
  const pagedData = data.slice(pageStartIndex, pageStartIndex + AMOUNT_ITEMS_PER_PAGE)
  const pageEndIndex = pageStartIndex + pagedData.length

  return (
    <Card className="gap-4 overflow-hidden p-5 animate-fade-up" style={{ animationDelay: "120ms" }}>
      <div className="flex flex-wrap items-start justify-between gap-2">
        <div>
          <h2 className="panel-heading text-sm font-semibold">{t("bills.chartAmountTitle")}</h2>
          <p className="mt-0.5 text-xs text-muted-foreground">
            {mode === "subscription" ? t("bills.chartAmountBySub") : t("bills.chartAmountByAccount")}
            {amountsHidden ? ` · ${t("privacy.relativeOnly")}` : ""}
          </p>
        </div>
        <div role="tablist" className="inline-flex items-center rounded-md border bg-muted/50 p-0.5">
          {(
            [
              { value: "subscription", label: t("bills.chartModeSub") },
              { value: "account", label: t("bills.chartModeAccount") },
            ] as const
          ).map((item) => (
            <button
              key={item.value}
              type="button"
              role="tab"
              aria-selected={mode === item.value}
              onClick={() => {
                setMode(item.value)
                setPage(1)
              }}
              className={cn(
                "rounded-md border px-2.5 py-1 text-xs font-medium transition-colors",
                mode === item.value
                  ? "border-border bg-background text-foreground"
                  : "border-transparent text-muted-foreground hover:text-foreground",
              )}
            >
              {item.label}
            </button>
          ))}
        </div>
      </div>

      {data.length === 0 ? (
        <p className="py-10 text-center text-sm text-muted-foreground">{t("bills.chartEmpty")}</p>
      ) : (
        <div className="grid gap-3">
          <div className="h-[240px]">
            <ResponsiveContainer width="100%" height="100%">
              <BarChart data={pagedData} layout="vertical" margin={{ top: 0, right: 56, bottom: 0, left: 0 }}>
                <XAxis type="number" hide />
                <YAxis
                  type="category"
                  dataKey="name"
                  width={104}
                  axisLine={false}
                  tickLine={false}
                  tick={{ fontSize: 11, fill: "var(--muted-foreground)" }}
                  tickFormatter={(value: string) =>
                    value.length > 8 ? `${value.slice(0, 8)}…` : value
                  }
                />
                <RechartsTooltip
                  cursor={{ fill: "var(--accent)" }}
                  content={<ChartTooltip amountsHidden={amountsHidden} />}
                />
                <Bar
                  dataKey="cents"
                  radius={[3, 3, 3, 3]}
                  barSize={14}
                  label={{
                    position: "right",
                    fontSize: 11,
                    fill: "var(--muted-foreground)",
                    formatter: (value: unknown) =>
                      amountsHidden ? AMOUNT_MASK : formatCents(Number(value)),
                  }}
                >
                  {pagedData.map((_, index) => (
                    <Cell
                      key={index}
                      fill={CHART_COLORS[(pageStartIndex + index) % CHART_COLORS.length]}
                    />
                  ))}
                </Bar>
              </BarChart>
            </ResponsiveContainer>
          </div>

          {pageCount > 1 ? (
            <div className="flex items-center justify-between border-t pt-3 text-xs text-muted-foreground">
              <span className="tabular-nums">
                {pageStartIndex + 1}-{pageEndIndex} / {data.length}
              </span>
              <div className="flex items-center gap-1.5">
                <Button
                  type="button"
                  variant="outline"
                  size="icon-sm"
                  aria-label={t("cards.prevPage")}
                  disabled={safePage <= 1}
                  onClick={() => setPage(safePage - 1)}
                >
                  <ChevronLeft />
                </Button>
                <span className="min-w-12 text-center tabular-nums">
                  {safePage} / {pageCount}
                </span>
                <Button
                  type="button"
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
    </Card>
  )
}

// ---- 所属账号 donut ----------------------------------------------------------------

function AccountDonutCard({
  summary,
  amountsHidden,
}: {
  summary: BillsSummary
  amountsHidden: boolean
}) {
  const { t } = useTranslation()
  const [reduceMotion, setReduceMotion] = React.useState(() =>
    window.matchMedia("(prefers-reduced-motion: reduce)").matches,
  )
  const data = (summary.accounts ?? []).map((item) => ({
    name: item.account_name,
    cents: item.amount_cents,
    yuan: item.amount_yuan,
    count: item.count,
  }))

  React.useEffect(() => {
    const media = window.matchMedia("(prefers-reduced-motion: reduce)")
    const handleChange = (event: MediaQueryListEvent) => setReduceMotion(event.matches)
    media.addEventListener("change", handleChange)
    return () => media.removeEventListener("change", handleChange)
  }, [])

  return (
    <Card className="gap-4 p-5 animate-fade-up" style={{ animationDelay: "180ms" }}>
      <div>
        <h2 className="panel-heading text-sm font-semibold">{t("bills.chartAccountTitle")}</h2>
        <p className="mt-0.5 text-xs text-muted-foreground">{t("bills.chartAccountDesc")}</p>
      </div>

      {data.length === 0 ? (
        <p className="py-10 text-center text-sm text-muted-foreground">
          {t("bills.chartEmptyAccounts")}
        </p>
      ) : (
        <div className="flex flex-col items-stretch gap-4 sm:flex-row sm:items-center sm:gap-5">
          <div className="mx-auto h-[170px] w-[170px] shrink-0 sm:mx-0">
            <PieChart width={170} height={170}>
              <RechartsTooltip content={<ChartTooltip amountsHidden={amountsHidden} />} />
              <Pie
                data={data}
                dataKey="cents"
                nameKey="name"
                innerRadius={52}
                outerRadius={80}
                startAngle={90}
                endAngle={-270}
                paddingAngle={3}
                strokeWidth={0}
                isAnimationActive={!reduceMotion}
                animationBegin={120}
                animationDuration={1000}
                animationEasing="ease-out"
              >
                {data.map((_, index) => (
                  <Cell key={index} fill={CHART_COLORS[index % CHART_COLORS.length]} />
                ))}
              </Pie>
            </PieChart>
          </div>
          <ul className="grid min-w-0 flex-1 gap-2">
            {data.map((item, index) => (
              <li key={item.name} className="grid min-w-0 grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-2 text-xs">
                <i
                  className="size-2.5 shrink-0 rounded-[3px]"
                  style={{ background: CHART_COLORS[index % CHART_COLORS.length] }}
                />
                <span className="truncate">{item.name}</span>
                <span className="shrink-0 tabular-nums text-muted-foreground">
                  {maskAmount(amountsHidden, `¥${item.yuan}`)} · {t("bills.countSuffix", { count: item.count })}
                </span>
              </li>
            ))}
          </ul>
        </div>
      )}
    </Card>
  )
}

// ---- 近 6 个月实收 -----------------------------------------------------------------

function MonthlyTrendCard({
  summary,
  amountsHidden,
}: {
  summary: BillsSummary
  amountsHidden: boolean
}) {
  const { t } = useTranslation()
  const data = (summary.monthly_trend ?? []).map((item) => ({
    name: item.month,
    label: item.label,
    cents: item.amount_cents,
    grossCents: item.gross_amount_cents,
    refundCents: item.refund_cents,
    count: item.count,
  }))
  const chartScale = getChartScale(data.map((item) => item.cents))

  return (
    <Card className="h-[280px] gap-4 overflow-hidden p-5 animate-fade-up" style={{ animationDelay: "240ms" }}>
      <div>
        <h2 className="panel-heading text-sm font-semibold">{t("bills.chartTrendTitle")}</h2>
        <p className="mt-0.5 text-xs text-muted-foreground">{t("bills.chartTrendDesc")}</p>
      </div>

      {data.every((item) => item.grossCents === 0 && item.refundCents === 0) ? (
        <p className="py-10 text-center text-sm text-muted-foreground">
          {t("bills.chartEmptyTrend")}
        </p>
      ) : (
        <div className="min-h-0 flex-1">
          <ResponsiveContainer width="100%" height="100%">
            <BarChart data={data} margin={{ top: 22, right: 8, bottom: 0, left: 0 }}>
              <CartesianGrid
                vertical={false}
                stroke="var(--border)"
                strokeDasharray="3 5"
              />
              <XAxis
                dataKey="label"
                axisLine={false}
                tickLine={false}
                tick={{ fontSize: 11, fill: "var(--muted-foreground)" }}
              />
              <YAxis
                axisLine={false}
                domain={chartScale.domain}
                tickLine={false}
                ticks={chartScale.ticks}
                tick={{ fontSize: 10, fill: "var(--muted-foreground)" }}
                tickFormatter={(value) => (amountsHidden ? "¥••" : formatAxisCents(value))}
                tickMargin={8}
                width={58}
              />
              <RechartsTooltip
                cursor={{ fill: "var(--accent)" }}
                content={<ChartTooltip amountsHidden={amountsHidden} />}
              />
              <Bar
                dataKey="cents"
                fill="var(--brand)"
                radius={[4, 4, 0, 0]}
                barSize={36}
                label={{
                  position: "top",
                  fontSize: 10,
                  fill: "var(--muted-foreground)",
                  formatter: (value: unknown) =>
                    amountsHidden ? AMOUNT_MASK : formatCents(Number(value)),
                }}
              />
            </BarChart>
          </ResponsiveContainer>
        </div>
      )}
    </Card>
  )
}

// ---- Page --------------------------------------------------------------------------

export function BillsPage() {
  const { t } = useTranslation()
  const { amountsHidden, toggleAmounts } = useAmountPrivacy()
  const billsQuery = useBills()

  const [editingBill, setEditingBill] = React.useState<BillView | null>(null)
  const [viewingBill, setViewingBill] = React.useState<BillView | null>(null)
  const [deleteTarget, setDeleteTarget] = React.useState<BillView | null>(null)
  const [search, setSearch] = React.useState("")
  const [typeFilter, setTypeFilter] = React.useState<"all" | "sale" | "resale">("all")
  const [page, setPage] = React.useState(1)

  const deleteMutation = useAppMutation((id: number) => deleteBill(id), {
    onSuccess: () => setDeleteTarget(null),
  })

  const billsData = billsQuery.data?.bills
  const bills = React.useMemo(() => billsData ?? [], [billsData])
  const summary = billsQuery.data?.summary

  const filteredBills = React.useMemo(() => {
    const byType = bills.filter((bill) => {
      if (typeFilter === "sale") return !bill.is_resale
      if (typeFilter === "resale") return bill.is_resale
      return true
    })
    const query = search.trim().toLowerCase()
    if (!query) return byType
    return byType.filter((bill) =>
      [
        bill.subscription_name,
        bill.account_name,
        bill.account_email,
        bill.account_space_name,
        bill.seat_name,
        bill.customer_email,
        bill.customer_wechat,
        bill.note,
        bill.due_date,
        bill.amount_yuan,
        bill.refund_yuan,
        bill.net_amount_yuan,
        bill.profit_yuan,
        bill.agency_fee_yuan,
        bill.status_label,
      ].some((field) => field?.toLowerCase().includes(query)),
    )
  }, [bills, search, typeFilter])

  const pageCount = Math.max(1, Math.ceil(filteredBills.length / BILLS_PER_PAGE))
  const safePage = Math.min(page, pageCount)
  const pageStartIndex = (safePage - 1) * BILLS_PER_PAGE
  const pagedBills = filteredBills.slice(pageStartIndex, pageStartIndex + BILLS_PER_PAGE)
  const pageEndIndex = pageStartIndex + pagedBills.length

  return (
    <>
      <PageHeader
        title={t("bills.title")}
        titleAccessory={
          <AmountPrivacyToggle amountsHidden={amountsHidden} onToggle={toggleAmounts} />
        }
        description={t("bills.desc")}
      />

      {billsQuery.isPending ? (
        <div className="grid gap-4">
          <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-6">
            {Array.from({ length: 6 }).map((_, index) => (
              <Skeleton key={index} className="h-28 rounded-xl" />
            ))}
          </div>
          <Skeleton className="h-72 rounded-xl" />
          <Skeleton className="h-96 rounded-xl" />
        </div>
      ) : billsQuery.isError ? (
        <Card className="items-center gap-3 py-16 text-center">
          <p className="text-sm text-muted-foreground">{t("common.loadFailed")}</p>
          <Button variant="outline" onClick={() => billsQuery.refetch()}>
            {t("common.retry")}
          </Button>
        </Card>
      ) : summary ? (
        <>
          <section aria-label={t("bills.title")} className="mb-4 grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-6">
            <KpiCard
              label={t("bills.kpiTotal")}
              value={maskAmount(amountsHidden, `¥${summary.total_amount_yuan}`)}
              hint={t("bills.kpiTotalHint")}
              icon={<Wallet className="size-4" />}
              delay={0}
              tone="brand"
            />
            <KpiCard
              label={t("bills.kpiRefund")}
              value={maskAmount(amountsHidden, `¥${summary.total_refund_yuan}`)}
              hint={t("bills.kpiRefundHint")}
              icon={<HandCoins className="size-4" />}
              delay={40}
              tone="gold"
            />
            <KpiCard
              label={t("bills.kpiNet")}
              value={maskAmount(amountsHidden, `¥${summary.net_amount_yuan}`)}
              hint={t("bills.kpiNetHint")}
              icon={<BadgeDollarSign className="size-4" />}
              delay={80}
              tone="success"
            />
            <KpiCard
              label={t("bills.kpiThisMonth")}
              value={maskAmount(amountsHidden, `¥${summary.this_month_net_amount_yuan}`)}
              hint={t("bills.kpiThisMonthHint", {
                gross: amountsHidden ? VALUE_MASK : summary.this_month_amount_yuan,
                refund: amountsHidden ? VALUE_MASK : summary.this_month_refund_yuan,
              })}
              icon={<CalendarDays className="size-4" />}
              delay={120}
              tone="brand"
            />
            <KpiCard
              label={t("bills.kpiAgencyFee")}
              value={maskAmount(amountsHidden, `¥${summary.total_agency_fee_yuan}`)}
              hint={t("bills.kpiAgencyFeeHint", {
                count: summary.resale_bill_count,
                month: amountsHidden ? VALUE_MASK : summary.this_month_agency_fee_yuan,
              })}
              icon={<HandCoins className="size-4" />}
              delay={160}
              tone="gold"
            />
            <KpiCard
              label={t("bills.kpiActive")}
              value={summary.active_count}
              hint={t("bills.kpiActiveHint")}
              icon={<CircleDot className="size-4" />}
              delay={200}
              tone="brand"
            />
          </section>

          <div className="mb-4 grid gap-4 lg:grid-cols-2">
            <AmountDistributionCard summary={summary} amountsHidden={amountsHidden} />
            <AccountDonutCard summary={summary} amountsHidden={amountsHidden} />
          </div>
          <MonthlyTrendCard summary={summary} amountsHidden={amountsHidden} />

          <div className="mt-8 mb-4 flex flex-col gap-3 border-t pt-6 sm:flex-row sm:items-center sm:justify-between animate-fade-up">
            <h2 className="panel-heading text-lg font-semibold">{t("bills.listTitle")}</h2>
            <div className="flex flex-wrap items-center gap-3">
              <div className="relative">
                <Search className="absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground" />
                <Input
                  value={search}
                  onChange={(event) => {
                    setSearch(event.target.value)
                    setPage(1)
                  }}
                  placeholder={t("bills.searchPlaceholder")}
                  aria-label={t("bills.listTitle")}
                  className="h-8 w-52 pl-8 text-[13px] sm:w-64"
                />
              </div>
              <Select
                value={typeFilter}
                onValueChange={(value) => {
                  setTypeFilter(value as "all" | "sale" | "resale")
                  setPage(1)
                }}
              >
                <SelectTrigger className="h-8 w-28" aria-label={t("bills.filterLabel")}>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">{t("bills.filterAll")}</SelectItem>
                  <SelectItem value="sale">{t("bills.filterSale")}</SelectItem>
                  <SelectItem value="resale">{t("bills.filterResale")}</SelectItem>
                </SelectContent>
              </Select>
              <p className="shrink-0 text-xs text-muted-foreground">
                {t("bills.listMeta", { count: filteredBills.length })}
              </p>
            </div>
          </div>

          {bills.length === 0 ? (
            <Card className="items-center py-16 text-center animate-fade-up">
              <p className="max-w-md text-sm leading-relaxed text-muted-foreground">
                {t("bills.empty")}
              </p>
            </Card>
          ) : (
            <div className="space-y-3">
              <div className="grid gap-3 md:hidden">
                {pagedBills.map((bill) => (
                  <BillMobileCard
                    key={bill.id}
                    bill={bill}
                    amountsHidden={amountsHidden}
                    onView={() => setViewingBill(bill)}
                    onEdit={() => setEditingBill(bill)}
                    onDelete={() => setDeleteTarget(bill)}
                  />
                ))}
              </div>

              <Card className="hidden gap-0 overflow-hidden p-0 animate-fade-up md:flex">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t("bills.colSubscription")}</TableHead>
                    <TableHead>{t("bills.colStartDate")}</TableHead>
                    <TableHead>{t("bills.colGross")}</TableHead>
                    <TableHead className="hidden lg:table-cell">{t("bills.colRefund")}</TableHead>
                    <TableHead className="hidden lg:table-cell">{t("bills.colNet")}</TableHead>
                    <TableHead className="hidden lg:table-cell">{t("bills.colAgencyFee")}</TableHead>
                    <TableHead className="hidden md:table-cell">{t("bills.colNote")}</TableHead>
                    <TableHead className="hidden sm:table-cell">{t("bills.colPaidAt")}</TableHead>
                    <TableHead>{t("bills.colStatus")}</TableHead>
                    <TableHead className="text-right">{t("bills.colActions")}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {filteredBills.length === 0 ? (
                    <TableRow className="hover:bg-transparent">
                      <TableCell colSpan={10} className="py-12 text-center text-muted-foreground">
                        {t("bills.searchEmpty")}
                      </TableCell>
                    </TableRow>
                  ) : null}
                  {pagedBills.map((bill) => {
                    const primaryName = bill.account_name || bill.subscription_name
                    const customerLine = bill.customer_email || bill.subscription_name
                    return (
                    <TableRow key={bill.id}>
                      <TableCell className="max-w-48">
                        <div className="flex items-center gap-1.5 truncate">
                          <span className="truncate font-medium" title={primaryName}>
                            {primaryName}
                          </span>
                          {bill.is_resale ? (
                            <Badge variant="outline" className="shrink-0 font-normal">
                              {t("cards.resale")}
                            </Badge>
                          ) : null}
                        </div>
                        <div className="truncate text-xs text-muted-foreground" title={customerLine}>
                          {customerLine}
                          {bill.seat_name ? ` · ${bill.seat_name}` : ""}
                        </div>
                      </TableCell>
                      <TableCell className="tabular-nums">{bill.due_date}</TableCell>
                      <TableCell className="font-medium tabular-nums">
                        {maskAmount(amountsHidden, `¥${bill.amount_yuan}`)}
                      </TableCell>
                      <TableCell className={cn("hidden tabular-nums lg:table-cell", bill.refund_cents > 0 && "text-destructive")}>
                        {amountsHidden
                          ? AMOUNT_MASK
                          : `${bill.refund_cents > 0 ? "-" : ""}¥${bill.refund_yuan}`}
                      </TableCell>
                      <TableCell className="hidden font-medium text-success tabular-nums lg:table-cell">
                        {maskAmount(amountsHidden, `¥${bill.net_amount_yuan}`)}
                      </TableCell>
                      <TableCell className="hidden tabular-nums lg:table-cell">
                        {bill.is_resale
                          ? maskAmount(amountsHidden, `¥${bill.agency_fee_yuan}`)
                          : "—"}
                      </TableCell>
                      <TableCell className="hidden max-w-52 md:table-cell">
                        {bill.note ? (
                          <span className="block truncate text-muted-foreground">{bill.note}</span>
                        ) : (
                          <span className="text-muted-foreground/50">—</span>
                        )}
                      </TableCell>
                      <TableCell className="hidden tabular-nums text-muted-foreground sm:table-cell">
                        {bill.paid_at_label}
                      </TableCell>
                      <TableCell>
                        <Badge variant={bill.archived ? "secondary" : "success"} className="font-normal">
                          {bill.status_label}
                        </Badge>
                      </TableCell>
                      <TableCell className="text-right">
                        <div className="inline-flex items-center gap-1">
                          <Button variant="outline" size="sm" onClick={() => setViewingBill(bill)}>
                            <Eye data-slot="icon" />
                            <span className="hidden lg:inline">{t("bills.viewSubscription")}</span>
                          </Button>
                          <Button variant="outline" size="sm" onClick={() => setEditingBill(bill)}>
                            <Pencil data-slot="icon" />
                            <span className="hidden lg:inline">{t("bills.editBill")}</span>
                          </Button>
                          <Button
                            variant="ghost"
                            size="sm"
                            className="text-destructive hover:text-destructive"
                            onClick={() => setDeleteTarget(bill)}
                          >
                            <Trash2 data-slot="icon" />
                            <span className="hidden lg:inline">{t("common.delete")}</span>
                          </Button>
                        </div>
                      </TableCell>
                    </TableRow>
                  )})}
                </TableBody>
              </Table>
              {pageCount > 1 ? (
                <div className="flex flex-col items-center justify-between gap-3 border-t px-4 py-3 text-xs text-muted-foreground sm:flex-row">
                  <span>
                    {t("bills.pageStatus", {
                      page: safePage,
                      pageCount,
                      start: pageStartIndex + 1,
                      end: pageEndIndex,
                      total: filteredBills.length,
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

              {pageCount > 1 ? (
                <div className="flex items-center justify-between border-t pt-3 text-xs text-muted-foreground md:hidden">
                  <span>
                    {t("bills.pageStatus", {
                      page: safePage,
                      pageCount,
                      start: pageStartIndex + 1,
                      end: pageEndIndex,
                      total: filteredBills.length,
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
        </>
      ) : null}

      <BillEditDialog
        open={editingBill !== null}
        onOpenChange={(open) => {
          if (!open) setEditingBill(null)
        }}
        bill={editingBill}
      />
      <SubscriptionViewDialog
        open={viewingBill !== null}
        onOpenChange={(open) => {
          if (!open) setViewingBill(null)
        }}
        bill={viewingBill}
        amountsHidden={amountsHidden}
      />
      <ConfirmDialog
        open={deleteTarget !== null}
        onOpenChange={(open) => {
          if (!open) setDeleteTarget(null)
        }}
        title={t("confirms.deleteBillTitle")}
        description={t("confirms.deleteBillDesc")}
        actionLabel={t("confirms.deleteBillAction")}
        destructive
        pending={deleteMutation.isPending}
        onConfirm={() => {
          if (deleteTarget) deleteMutation.mutate(deleteTarget.id)
        }}
      />
    </>
  )
}
