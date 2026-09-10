import * as React from "react"
import { Link, useNavigate } from "react-router-dom"
import { useTranslation } from "react-i18next"
import {
  AlertTriangle,
  ArrowRight,
  ArrowUpRight,
  CalendarClock,
  CheckCircle2,
  ChevronLeft,
  ChevronRight,
  CircleDollarSign,
  Clock3,
  Coins,
  HandCoins,
  KeyRound,
  MailWarning,
  Plus,
  RefreshCw,
  Snowflake,
  Target,
  TicketCheck,
  Users,
  WalletCards,
} from "lucide-react"
import {
  Area,
  AreaChart,
  CartesianGrid,
  ResponsiveContainer,
  Tooltip as RechartsTooltip,
  XAxis,
  YAxis,
} from "recharts"

import { useAccounts, useOperationsOverview } from "@/api/queries"
import type { AccountView, OperationTask, OperationsOverview } from "@/api/types"
import { AmountPrivacyToggle } from "@/components/amount-privacy-toggle"
import { PageHeader } from "@/components/page-header"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { Skeleton } from "@/components/ui/skeleton"
import {
  StatDetailDialog,
  type StatDetailState,
} from "@/components/stat-detail-dialog"
import { DuePaidDialog, type DuePaidTarget } from "@/features/calendar/DuePaidDialog"
import { PlusRentalDialog } from "@/features/plus-rentals/PlusRentalDialog"
import { SubscriptionDialog } from "@/features/subscriptions/SubscriptionDialog"
import { useAmountPrivacy } from "@/hooks/use-amount-privacy"
import { maskAmount } from "@/lib/amount-privacy"
import { cn } from "@/lib/utils"

function formatCents(cents: number) {
  return `¥${(cents / 100).toFixed(2)}`
}

function formatAxisCents(cents: number) {
  const value = cents / 100
  if (Math.abs(value) >= 10_000) return `¥${(value / 10_000).toFixed(1)}万`
  return `¥${Math.round(value)}`
}

function DashboardKpi({
  icon: Icon,
  label,
  value,
  hint,
  tone = "brand",
  onClick,
}: {
  icon: React.ComponentType<{ className?: string }>
  label: string
  value: React.ReactNode
  hint: string
  tone?: "brand" | "success" | "warning" | "default"
  onClick: () => void
}) {
  const toneClass = {
    brand: "bg-brand/10 text-brand",
    success: "bg-success/10 text-success",
    warning: "bg-warning/15 text-warning-foreground dark:text-warning",
    default: "bg-chart-2/10 text-chart-2",
  }[tone]

  return (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        "dashboard-stat group relative min-h-[124px] min-w-0 overflow-hidden rounded-xl border p-4 text-left outline-none transition-[border-color,background-color,box-shadow] hover:border-brand/25 hover:shadow-lift focus-visible:ring-2 focus-visible:ring-brand/45",
        `dashboard-stat--${tone}`,
      )}
    >
      <span aria-hidden="true" className="dashboard-stat-orbit" />
      <span className="relative flex items-start justify-between gap-3">
        <span className={cn("grid size-9 shrink-0 place-items-center rounded-lg", toneClass)}>
          <Icon className="size-[18px]" />
        </span>
        <ArrowRight className="size-4 text-muted-foreground/50 transition-transform group-hover:translate-x-0.5 group-hover:text-foreground" />
      </span>
      <span className="relative mt-3 block text-xs font-medium text-muted-foreground">{label}</span>
      <span className="display-numeral relative mt-1.5 block truncate text-[23px] font-semibold leading-none">
        {value}
      </span>
      <span className="relative mt-2 block truncate text-[11px] text-muted-foreground">{hint}</span>
    </button>
  )
}

interface TrendPoint {
  label: string
  netCents: number
  grossCents: number
  refundCents: number
}

function TrendTooltip({
  active,
  payload,
  amountsHidden,
}: {
  active?: boolean
  payload?: Array<{ payload: TrendPoint }>
  amountsHidden: boolean
}) {
  const point = payload?.[0]?.payload
  if (!active || !point) return null
  return (
    <div className="min-w-40 rounded-md border bg-popover p-3 text-xs shadow-lg">
      <p className="mb-2 font-semibold">{point.label}</p>
      <div className="grid gap-1.5 text-muted-foreground">
        <span className="flex items-center justify-between gap-4">
          <span>净收入</span>
          <strong className="text-foreground">
            {maskAmount(amountsHidden, formatCents(point.netCents))}
          </strong>
        </span>
        <span className="flex items-center justify-between gap-4">
          <span>原实收</span>
          <strong className="text-foreground">
            {maskAmount(amountsHidden, formatCents(point.grossCents))}
          </strong>
        </span>
        {point.refundCents > 0 ? (
          <span className="flex items-center justify-between gap-4">
            <span>退款</span>
            <strong className="text-destructive">
              {maskAmount(amountsHidden, formatCents(point.refundCents))}
            </strong>
          </span>
        ) : null}
      </div>
    </div>
  )
}

function CashflowCard({
  overview,
  amountsHidden,
}: {
  overview: OperationsOverview
  amountsHidden: boolean
}) {
  const { t } = useTranslation()
  const data: TrendPoint[] = overview.finance.monthly_trend.map((month) => ({
    label: month.label,
    netCents: month.amount_cents,
    grossCents: month.gross_amount_cents,
    refundCents: month.refund_cents,
  }))
  const firstActiveIndex = data.findIndex((point) => point.grossCents !== 0 || point.refundCents !== 0)
  const visibleData = firstActiveIndex <= 0
    ? data
    : data.slice(Math.max(0, Math.min(firstActiveIndex - 1, data.length - 3)))
  const hasVisibleRefunds = visibleData.some((point) => point.refundCents > 0)
  const hasGrossReference = visibleData.some((point) => point.grossCents !== point.netCents)
  const current = visibleData.at(-1)?.netCents ?? 0
  const previous = visibleData.at(-2)?.netCents ?? 0
  const change = previous === 0 ? null : Math.round(((current - previous) / Math.abs(previous)) * 100)

  return (
    <Card className="dashboard-panel min-w-0 gap-4 overflow-hidden p-5">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h2 className="panel-heading text-sm font-semibold">{t("dash.workbench.cashflowTitle")}</h2>
          <p className="mt-1 text-xs text-muted-foreground">{t("dash.workbench.cashflowHint")}</p>
        </div>
        <div className="text-right">
          <div className="display-numeral text-xl font-semibold">
            {maskAmount(amountsHidden, formatCents(current))}
          </div>
          <div className={cn("text-[11px]", change !== null && change < 0 ? "text-destructive" : "text-success")}>
            {change === null ? t("dash.workbench.noComparison") : `${change >= 0 ? "+" : ""}${change}% ${t("dash.workbench.vsPrevious")}`}
          </div>
        </div>
      </div>
      {visibleData.length === 0 ? (
        <div className="grid h-[260px] place-items-center rounded-lg border border-dashed text-sm text-muted-foreground">
          {t("dash.trendEmpty")}
        </div>
      ) : (
        <div className="h-[260px] min-w-0">
          <ResponsiveContainer width="100%" height="100%">
            <AreaChart data={visibleData} margin={{ top: 14, right: 10, bottom: 0, left: 0 }}>
              <defs>
                <linearGradient id="dashboardNetIncome" x1="0" y1="0" x2="0" y2="1">
                  <stop offset="0%" stopColor="var(--chart-1)" stopOpacity={0.34} />
                  <stop offset="72%" stopColor="var(--chart-1)" stopOpacity={0.08} />
                  <stop offset="100%" stopColor="var(--chart-1)" stopOpacity={0} />
                </linearGradient>
              </defs>
              <CartesianGrid vertical={false} stroke="var(--border)" strokeDasharray="3 7" />
              <XAxis
                dataKey="label"
                axisLine={false}
                tickLine={false}
                tick={{ fontSize: 11, fill: "var(--muted-foreground)" }}
                dy={7}
              />
              <YAxis
                width={62}
                axisLine={false}
                tickLine={false}
                tick={{ fontSize: 10, fill: "var(--muted-foreground)" }}
                tickFormatter={(value: number) => (amountsHidden ? "***" : formatAxisCents(value))}
              />
              <RechartsTooltip
                cursor={{ stroke: "var(--border)", strokeWidth: 1 }}
                content={<TrendTooltip amountsHidden={amountsHidden} />}
              />
              {hasGrossReference ? (
                <Area
                  type="monotone"
                  dataKey="grossCents"
                  name="原实收"
                  stroke="var(--chart-2)"
                  strokeOpacity={0.82}
                  strokeWidth={1.75}
                  strokeDasharray="6 5"
                  fill="transparent"
                  dot={false}
                  activeDot={false}
                />
              ) : null}
              {hasVisibleRefunds ? (
                <Area
                  type="monotone"
                  dataKey="refundCents"
                  name="退款"
                  stroke="var(--destructive)"
                  strokeOpacity={0.55}
                  strokeWidth={1.5}
                  fill="var(--destructive)"
                  fillOpacity={0.08}
                  dot={false}
                  activeDot={false}
                />
              ) : null}
              <Area
                type="monotone"
                dataKey="netCents"
                name="净收入"
                stroke="var(--chart-1)"
                strokeWidth={2.5}
                fill="url(#dashboardNetIncome)"
                dot={false}
                activeDot={{ r: 5, fill: "var(--card)", stroke: "var(--chart-1)", strokeWidth: 2.5 }}
              />
            </AreaChart>
          </ResponsiveContainer>
        </div>
      )}
      <div className="dashboard-total-strip grid grid-cols-3 divide-x rounded-lg border py-2.5 text-center">
        <div className="px-2">
          <p className="text-[10px] text-muted-foreground">{t("dash.kpiRevenue")}</p>
          <p className="mt-1 truncate text-xs font-semibold tabular-nums">
            {maskAmount(amountsHidden, `¥${overview.dashboard.total_amount_yuan}`)}
          </p>
        </div>
        <div className="px-2">
          <p className="text-[10px] text-muted-foreground">{t("dash.kpiRefund")}</p>
          <p className="mt-1 truncate text-xs font-semibold text-destructive tabular-nums">
            {maskAmount(amountsHidden, `¥${overview.dashboard.total_refund_yuan}`)}
          </p>
        </div>
        <div className="px-2">
          <p className="text-[10px] text-muted-foreground">{t("dash.kpiProfit")}</p>
          <p className="mt-1 truncate text-xs font-semibold text-success tabular-nums">
            {maskAmount(amountsHidden, `¥${overview.dashboard.total_profit_yuan}`)}
          </p>
        </div>
      </div>
    </Card>
  )
}

function taskIcon(task: OperationTask) {
  switch (task.kind) {
    case "redemption":
      return TicketCheck
    case "renewal_review":
      return CircleDollarSign
    case "after_sales":
      return HandCoins
    case "notification_failed":
      return MailWarning
    case "seat_release":
      return Snowflake
    case "account_renewal":
      return KeyRound
    case "plus_due":
    case "plus_overdue":
      return CircleDollarSign
    default:
      return CalendarClock
  }
}

function OperationsQueue({
  overview,
  amountsHidden,
  onCollect,
}: {
  overview: OperationsOverview
  amountsHidden: boolean
  onCollect: (task: OperationTask) => void
}) {
  const { t } = useTranslation()
  const allTasks = overview.tasks ?? []
  const pageSize = 5
  const pageCount = Math.max(1, Math.ceil(allTasks.length / pageSize))
  const [page, setPage] = React.useState(0)
  const currentPage = Math.min(page, pageCount - 1)
  const tasks = allTasks.slice(currentPage * pageSize, (currentPage + 1) * pageSize)

  const kindLabel = (kind: OperationTask["kind"]) => t(`dash.workbench.taskKinds.${kind}`)
  const timingLabel = (task: OperationTask) => {
    if (task.due_at_label) return task.due_at_label
    if (!task.due_date) return ""
    if (task.days_remaining < 0) return t("dueStatus.overdueDays", { count: Math.abs(task.days_remaining) })
    if (task.days_remaining === 0) return t("dueStatus.today")
    return t("dueStatus.days", { count: task.days_remaining })
  }

  return (
    <Card className="dashboard-panel h-[430px] min-h-0 gap-3 overflow-hidden p-5">
      <div className="flex items-start justify-between gap-3">
        <div>
          <h2 className="panel-heading text-sm font-semibold">{t("dash.workbench.queueTitle")}</h2>
          <p className="mt-1 text-xs text-muted-foreground">
            {overview.work.urgent_count > 0
              ? t("dash.workbench.queueHint", { count: overview.work.urgent_count })
              : t("dash.workbench.queueClear")}
          </p>
        </div>
        {overview.work.urgent_count > 0 ? (
          <Badge variant="destructive">{overview.work.urgent_count}</Badge>
        ) : (
          <Badge variant="success">{t("dash.workbench.healthy")}</Badge>
        )}
      </div>

      {tasks.length === 0 ? (
        <div className="grid min-h-56 flex-1 place-items-center rounded-lg border border-dashed text-center">
          <div>
            <CheckCircle2 className="mx-auto size-7 text-success" />
            <p className="mt-2 text-sm font-medium">{t("dash.workbench.queueEmpty")}</p>
          </div>
        </div>
      ) : (
        <div className="flex min-h-0 flex-1 flex-col gap-1.5 overflow-y-auto pr-1">
          {tasks.map((task) => {
            const Icon = taskIcon(task)
            const identifier = task.customer_email || task.customer_wechat || task.name || task.account_name
            const canCollect =
              (task.kind === "team_due" ||
                task.kind === "team_overdue" ||
                task.kind === "plus_due" ||
                task.kind === "plus_overdue") &&
              !task.one_month_rental
            return (
              <div
                key={task.id}
                className={cn(
                  "dashboard-queue-row flex min-w-0 items-center gap-2.5 rounded-lg border border-l-2 px-2.5 py-2",
                  task.tone === "critical"
                    ? "border-l-destructive"
                    : task.tone === "warning"
                      ? "border-l-gold"
                      : "border-l-brand",
                )}
              >
                <span
                  className={cn(
                    "grid size-8 shrink-0 place-items-center rounded-md",
                    task.tone === "critical"
                      ? "bg-destructive/10 text-destructive"
                      : task.tone === "warning"
                        ? "bg-warning/15 text-warning-foreground dark:text-warning"
                        : "bg-brand/10 text-brand",
                  )}
                >
                  <Icon className="size-4" />
                </span>
                <span className="min-w-0 flex-1">
                  <span className="flex min-w-0 items-center gap-2">
                    <span className="truncate text-xs font-semibold">{identifier}</span>
                    <span className="shrink-0 text-[10px] text-muted-foreground">
                      {kindLabel(task.kind)}
                    </span>
                  </span>
                  <span className="mt-0.5 flex min-w-0 items-center gap-2 text-[10px] text-muted-foreground">
                    <span className="truncate">{[task.account_name, task.seat_name].filter(Boolean).join(" · ")}</span>
                    {timingLabel(task) ? <span className="shrink-0">{timingLabel(task)}</span> : null}
                  </span>
                </span>
                {task.amount_yuan ? (
                  <span className="hidden shrink-0 text-xs font-semibold tabular-nums sm:block">
                    {maskAmount(amountsHidden, `¥${task.amount_yuan}`)}
                  </span>
                ) : null}
                {canCollect ? (
                  <Button
                    size="sm"
                    className="h-7 shrink-0 px-2.5 text-[11px]"
                    onClick={() => onCollect(task)}
                  >
                    {task.kind.startsWith("plus_")
                      ? t("dash.workbench.recordRenewal")
                      : t("dash.workbench.recordPaid")}
                  </Button>
                ) : (
                  <Button asChild variant="outline" size="sm" className="h-7 shrink-0 px-2.5 text-[11px]">
                    <Link to={task.route}>{t("dash.workbench.handle")}</Link>
                  </Button>
                )}
              </div>
            )
          })}
        </div>
      )}

      <div className="flex shrink-0 items-center justify-between gap-2 border-t border-border/75 pt-2.5">
        {pageCount > 1 ? (
          <div className="dashboard-pager" aria-label="待办分页">
            <Button
              variant="ghost"
              size="icon-sm"
              className="size-7"
              aria-label="上一页待办"
              disabled={currentPage === 0}
              onClick={() => setPage(Math.max(0, currentPage - 1))}
            >
              <ChevronLeft className="size-3.5" />
            </Button>
            <span className="tabular-nums">{currentPage + 1} / {pageCount}</span>
            <Button
              variant="ghost"
              size="icon-sm"
              className="size-7"
              aria-label="下一页待办"
              disabled={currentPage + 1 >= pageCount}
              onClick={() => setPage(Math.min(pageCount - 1, currentPage + 1))}
            >
              <ChevronRight className="size-3.5" />
            </Button>
          </div>
        ) : <span />}
        <Button asChild variant="ghost" size="sm" className="h-7 px-2 text-[11px]">
          <Link to="/calendar?view=tasks">
            {t("dash.workbench.allTasks")}
            <ArrowRight />
          </Link>
        </Button>
      </div>
    </Card>
  )
}

type CapacitySegment = "occupied" | "frozen" | "free"

function CapacityCard({
  overview,
  onOpenSegment,
}: {
  overview: OperationsOverview
  onOpenSegment: (segment: CapacitySegment) => void
}) {
  const { t } = useTranslation()
  const { capacity } = overview
  const occupied = Math.max(0, capacity.seat_used - capacity.seat_frozen)
  const total = Math.max(1, capacity.seat_total)
  const occupiedPercent = (occupied / total) * 100
  const frozenPercent = (capacity.seat_frozen / total) * 100
  const occupiedAngle = Math.min(360, Math.max(0, occupiedPercent * 3.6))
  const frozenAngle = Math.min(360, occupiedAngle + Math.max(0, frozenPercent * 3.6))
  const segments = [
    {
      key: "occupied" as const,
      label: t("dash.workbench.occupied"),
      value: occupied,
      percent: occupiedPercent,
      dotClass: "bg-brand",
      barClass: "bg-brand",
      valueClass: "text-brand",
    },
    {
      key: "frozen" as const,
      label: t("dash.workbench.frozen"),
      value: capacity.seat_frozen,
      percent: frozenPercent,
      dotClass: "bg-gold",
      barClass: "bg-gold",
      valueClass: "text-gold",
    },
    {
      key: "free" as const,
      label: t("dash.workbench.free"),
      value: capacity.seat_free,
      percent: (capacity.seat_free / total) * 100,
      dotClass: "bg-success",
      barClass: "bg-success",
      valueClass: "text-success",
    },
  ]

  return (
    <Card className="dashboard-panel relative min-h-[320px] gap-4 overflow-hidden p-5">
      <div aria-hidden="true" className="pointer-events-none absolute -bottom-16 -left-16 size-48 rounded-full bg-brand/[0.055] blur-3xl" />
      <div className="relative flex items-start justify-between gap-3">
        <div>
          <h2 className="panel-heading text-sm font-semibold">{t("dash.workbench.capacityTitle")}</h2>
          <p className="mt-1 text-xs text-muted-foreground">
            {t("dash.workbench.capacityHint", { count: capacity.account_count })}
          </p>
        </div>
        <Button asChild variant="ghost" size="sm" className="-mr-2 h-8 shrink-0 px-2 text-xs">
          <Link to="/accounts">
            {t("dash.workbench.manageCapacity")}
            <ArrowRight />
          </Link>
        </Button>
      </div>

      <div className="relative grid flex-1 items-center gap-5 sm:grid-cols-[minmax(164px,0.75fr)_minmax(0,1.25fr)]">
        <div className="flex items-center justify-center">
          <div
            role="img"
            aria-label={t("dash.workbench.capacityAria", {
              percent: capacity.utilization_percent,
              used: capacity.seat_used,
              total: capacity.seat_total,
            })}
            className="relative grid size-40 shrink-0 place-items-center rounded-full p-[13px] shadow-[0_18px_42px_-26px_color-mix(in_oklab,var(--brand)_70%,transparent)]"
            style={{
              background: `conic-gradient(from -90deg, var(--brand) 0deg ${occupiedAngle}deg, var(--gold) ${occupiedAngle}deg ${frozenAngle}deg, color-mix(in oklab, var(--muted) 78%, var(--card)) ${frozenAngle}deg 360deg)`,
            }}
          >
            <div className="grid size-full place-items-center rounded-full border bg-card shadow-[inset_0_1px_0_color-mix(in_oklab,var(--foreground)_6%,transparent)]">
              <div className="text-center">
                <p className="display-numeral text-[30px] font-semibold text-brand">
                  {capacity.utilization_percent}%
                </p>
                <p className="mt-2 text-[10px] font-medium text-muted-foreground">
                  {t("dash.workbench.capacityUsed", {
                    used: capacity.seat_used,
                    total: capacity.seat_total,
                  })}
                </p>
              </div>
            </div>
          </div>
        </div>

        <div className="grid gap-2.5">
          {segments.map((item) => (
            <button
              key={item.key}
              type="button"
              onClick={() => onOpenSegment(item.key)}
              className="group rounded-lg border bg-card/75 px-3 py-2.5 text-left shadow-[0_8px_24px_-22px_color-mix(in_oklab,var(--foreground)_45%,transparent)] transition-[border-color,background-color,box-shadow] hover:border-brand/30 hover:bg-accent/35 hover:shadow-lift focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand/45"
              aria-label={`查看${item.label}席位`}
            >
              <div className="flex items-center gap-2.5">
                <span className={cn("size-2 shrink-0 rounded-full", item.dotClass)} />
                <span className="min-w-0 flex-1 text-xs font-medium text-muted-foreground">{item.label}</span>
                <span className={cn("display-numeral text-lg font-semibold", item.valueClass)}>{item.value}</span>
                <span className="w-8 text-right text-[10px] tabular-nums text-muted-foreground">
                  {Math.round(item.percent)}%
                </span>
                <ArrowRight className="size-3.5 shrink-0 text-muted-foreground/45 transition-transform group-hover:translate-x-0.5 group-hover:text-brand" />
              </div>
              <div className="ml-[18px] mt-2 h-1 overflow-hidden rounded-full bg-muted">
                <div
                  className={cn("h-full rounded-full", item.barClass)}
                  style={{ width: `${Math.min(100, Math.max(0, item.percent))}%` }}
                />
              </div>
            </button>
          ))}
        </div>
      </div>
    </Card>
  )
}

function DecisionCard({
  overview,
  amountsHidden,
}: {
  overview: OperationsOverview
  amountsHidden: boolean
}) {
  const { t } = useTranslation()
  const goal = overview.goal
  const goalProgress = Math.min(100, Math.max(0, goal?.progress_percent ?? 0))
  const nextSeatReleaseTask = (overview.notifications ?? []).find(
    (task) => task.kind === "seat_release",
  )
  const actions = [
    {
      icon: TicketCheck,
      label: t("dash.workbench.pendingReviews"),
      value: (overview.work.pending_redemption_count ?? 0) + (overview.work.pending_renewal_count ?? 0),
      to: overview.work.pending_redemption_count > 0 ? "/redemptions" : "/redemptions?section=renewals",
      activeClass: "border-destructive/25 bg-destructive/[0.045]",
      iconClass: "bg-destructive/10 text-destructive",
    },
    {
      icon: HandCoins,
      label: t("dash.workbench.pendingAfterSales"),
      value: overview.work.pending_after_sales_count,
      to: "/after-sales",
      activeClass: "border-gold/30 bg-gold/[0.055]",
      iconClass: "bg-gold/10 text-gold",
    },
    {
      icon: Snowflake,
      label: t("dash.workbench.releasingSeats"),
      value: overview.capacity.seat_releasing_7d,
      to: nextSeatReleaseTask?.route || "/accounts",
      activeClass: "border-brand/25 bg-brand/[0.045]",
      iconClass: "bg-brand/10 text-brand",
    },
  ]

  return (
    <Card className="dashboard-panel relative min-h-[320px] gap-3 overflow-hidden p-5">
      <div aria-hidden="true" className="pointer-events-none absolute -right-16 -top-20 size-52 rounded-full bg-brand/[0.06] blur-3xl" />
      <div className="relative flex items-start justify-between gap-3">
        <div>
          <h2 className="panel-heading text-sm font-semibold">{t("dash.workbench.decisionTitle")}</h2>
          <p className="mt-1 text-xs text-muted-foreground">{t("dash.workbench.decisionHint")}</p>
        </div>
        <Button asChild variant="ghost" size="sm" className="-mr-2 h-8 shrink-0 px-2 text-xs">
          <Link to="/goals">
            {t("dash.workbench.viewAnalysis")}
            <ArrowRight />
          </Link>
        </Button>
      </div>
      {goal ? (
        <Link
          to="/goals"
          className="group relative block overflow-hidden rounded-xl border border-brand/20 bg-brand/[0.045] p-3.5 transition-[border-color,background-color,box-shadow] hover:border-brand/35 hover:bg-brand/[0.07] hover:shadow-[0_16px_34px_-28px_color-mix(in_oklab,var(--brand)_70%,transparent)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand/45"
        >
          <div aria-hidden="true" className="absolute -right-7 -top-9 size-28 rounded-full border border-brand/10" />
          <div aria-hidden="true" className="absolute -right-3 -top-5 size-16 rounded-full border border-brand/15" />
          <div className="relative flex items-start justify-between gap-4">
            <span className="min-w-0">
              <span className="flex items-center gap-2 text-[10px] font-medium uppercase tracking-[0.14em] text-brand">
                <Target className="size-3.5" />
                {t("dash.workbench.goalProgress")}
              </span>
              <span className="mt-1.5 block truncate text-sm font-semibold">{goal.name}</span>
            </span>
            <span className="display-numeral shrink-0 text-[28px] font-semibold text-brand">
              {Math.round(goal.progress_percent)}%
            </span>
          </div>
          <div className="relative mt-3 h-2 overflow-hidden rounded-full border border-brand/10 bg-card/80">
            <div className="h-full rounded-full bg-brand transition-[width]" style={{ width: `${goalProgress}%` }} />
          </div>
          <div className="relative mt-3 grid grid-cols-2 gap-2 sm:grid-cols-[0.85fr_0.85fr_1.3fr]">
            <span className="min-w-0 rounded-md border border-brand/10 bg-card/65 px-2.5 py-2">
              <span className="block text-[9px] text-muted-foreground">{t("dash.workbench.goalCurrent")}</span>
              <strong className="display-numeral mt-1 block truncate text-xs">
                {maskAmount(amountsHidden, formatCents(goal.current_profit_cents))}
              </strong>
            </span>
            <span className="min-w-0 rounded-md border border-brand/10 bg-card/65 px-2.5 py-2">
              <span className="block text-[9px] text-muted-foreground">{t("dash.workbench.goalTarget")}</span>
              <strong className="display-numeral mt-1 block truncate text-xs">
                {maskAmount(amountsHidden, formatCents(goal.target_profit_cents))}
              </strong>
            </span>
            <span className="col-span-2 flex min-w-0 items-center gap-2 rounded-md border border-brand/10 bg-card/65 px-2.5 py-2 text-[10px] text-muted-foreground sm:col-span-1">
              <CalendarClock className="size-3.5 shrink-0 text-brand" />
              <span className="truncate">
              {goal.projected_date
                ? t("dash.workbench.projectedDate", { date: goal.projected_date })
                : t("dash.workbench.forecastCollecting")}
              </span>
            </span>
          </div>
        </Link>
      ) : (
        <Button asChild variant="outline" className="w-full">
          <Link to="/goals">{t("dash.workbench.createGoal")}</Link>
        </Button>
      )}
      <div className="relative grid flex-1 gap-2 sm:grid-cols-3">
        {actions.map((item) => {
          const Icon = item.icon
          return (
            <Link
              key={item.label}
              to={item.to}
              className={cn(
                "group flex min-h-[86px] min-w-0 flex-col rounded-lg border bg-card/70 p-3 transition-[border-color,background-color,box-shadow] hover:border-input hover:bg-muted/20 hover:shadow-[0_12px_28px_-25px_color-mix(in_oklab,var(--foreground)_55%,transparent)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand/45",
                item.value > 0 && item.activeClass,
              )}
            >
              <span className="flex items-start justify-between gap-2">
                <span className={cn("grid size-7 place-items-center rounded-md", item.value > 0 ? item.iconClass : "bg-muted text-muted-foreground")}>
                  <Icon className="size-3.5" />
                </span>
                <ArrowRight className="size-3.5 text-muted-foreground/55 transition-transform group-hover:translate-x-0.5 group-hover:text-foreground" />
              </span>
              <span className="mt-auto flex min-w-0 items-end justify-between gap-2 pt-3">
                <span className="min-w-0 text-[11px] font-medium leading-tight text-muted-foreground">{item.label}</span>
                <span className={cn("display-numeral shrink-0 text-xl font-semibold", item.value > 0 ? "text-foreground" : "text-success")}>
                  {item.value}
                </span>
              </span>
            </Link>
          )
        })}
      </div>
    </Card>
  )
}

export function DashboardPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { amountsHidden, toggleAmounts } = useAmountPrivacy()
  const overviewQuery = useOperationsOverview()
  const [plusDialogOpen, setPlusDialogOpen] = React.useState(false)
  const [teamDialogOpen, setTeamDialogOpen] = React.useState(false)
  const [duePaidTarget, setDuePaidTarget] = React.useState<DuePaidTarget | null>(null)
  const [capacitySegment, setCapacitySegment] = React.useState<CapacitySegment | null>(null)
  const accountsQuery = useAccounts(capacitySegment !== null)

  const overview = overviewQuery.data
  const openCollect = (task: OperationTask) => {
    setDuePaidTarget({
      subscriptionId: task.subscription_id,
      name: task.customer_email || task.customer_wechat || task.name,
      priceYuan: task.amount_yuan,
      cycleDesc: task.cycle_desc,
      dueDate: task.due_date,
      kind: task.kind.startsWith("plus_") ? "plus" : "team",
    })
  }

  const capacityDetail = React.useMemo<StatDetailState | null>(() => {
    if (!capacitySegment) return null
    const labels: Record<CapacitySegment, string> = {
      occupied: t("dash.workbench.occupied"),
      frozen: t("dash.workbench.frozen"),
      free: t("dash.workbench.free"),
    }
    const matches = (accountsQuery.data ?? []).flatMap((account: AccountView) =>
      (account.seats ?? [])
        .filter((seat) => {
          if (capacitySegment === "frozen") return seat.frozen
          if (capacitySegment === "occupied") return seat.occupied && !seat.frozen
          return !seat.occupied && !seat.frozen
        })
        .map((seat) => ({ account, seat })),
    )
    return {
      title: `${labels[capacitySegment]}席位`,
      description: "按母号序号列出当前席位状态，无需离开仪表盘。",
      emptyText: accountsQuery.isPending
        ? "正在加载席位明细…"
        : accountsQuery.isError
          ? "席位明细加载失败，请刷新后重试。"
          : `暂无${labels[capacitySegment]}席位`,
      items: matches.map(({ account, seat }) => ({
        id: seat.seat.id,
        title: `${account.account.id} · ${account.account.email || account.account.name}`,
        subtitle: account.account.space_name || account.account.name,
        meta: [
          seat.seat.name,
          capacitySegment === "frozen"
            ? seat.frozen_customer_email || seat.frozen_subscription_name
            : seat.active_customer_email || seat.active_subscription_name,
          capacitySegment === "frozen" && seat.frozen_until_label
            ? `冷却至 ${seat.frozen_until_label}`
            : null,
        ],
        value: labels[capacitySegment],
        valueTone: capacitySegment === "free"
          ? "success" as const
          : capacitySegment === "frozen"
            ? "warning" as const
            : "default" as const,
      })),
    }
  }, [accountsQuery.data, accountsQuery.isError, accountsQuery.isPending, capacitySegment, t])

  return (
    <div className="dashboard-stage flex min-w-0 flex-col gap-4 overflow-x-clip">
      <PageHeader
        title={t("dash.workbench.title")}
        titleAccessory={<AmountPrivacyToggle amountsHidden={amountsHidden} onToggle={toggleAmounts} />}
        actions={
          <div className="flex flex-wrap items-center gap-2">
            <Button variant="outline" size="icon" aria-label={t("common.refresh")} onClick={() => void overviewQuery.refetch()}>
              <RefreshCw className={cn(overviewQuery.isFetching && "animate-spin")} />
            </Button>
            <Button
              asChild
              variant="outline"
              className="group bg-white text-slate-800 shadow-sm hover:bg-slate-50 hover:text-slate-950 dark:bg-white dark:text-slate-900 dark:hover:bg-slate-100 dark:hover:text-slate-950"
            >
              <a href="/redeem" target="_blank" rel="noreferrer">
                <ArrowUpRight className="size-4 stroke-[2.2] transition-transform group-hover:translate-x-0.5 group-hover:-translate-y-0.5" />
                前往兑换页
              </a>
            </Button>
            <Button onClick={() => setPlusDialogOpen(true)}>
              <Plus />
              {t("dash.newPlusRental")}
            </Button>
            <Button onClick={() => setTeamDialogOpen(true)}>
              <Plus />
              {t("dash.newTeamUser")}
            </Button>
          </div>
        }
      />

      {overviewQuery.isPending ? (
        <div className="grid gap-4">
          <div className="grid grid-cols-2 gap-3 xl:grid-cols-4">
            {Array.from({ length: 4 }).map((_, index) => (
              <Skeleton key={index} className="h-[112px] rounded-lg" />
            ))}
          </div>
          <div className="grid gap-4 xl:grid-cols-[minmax(0,1.25fr)_minmax(360px,0.75fr)]">
            <Skeleton className="h-[430px] rounded-lg" />
            <Skeleton className="h-[430px] rounded-lg" />
          </div>
        </div>
      ) : overviewQuery.isError ? (
        <Card className="items-center gap-3 py-16 text-center">
          <AlertTriangle className="size-6 text-destructive" />
          <p className="text-sm text-muted-foreground">{t("common.loadFailed")}</p>
          <Button variant="outline" onClick={() => void overviewQuery.refetch()}>{t("common.retry")}</Button>
        </Card>
      ) : overview ? (
        <>
          <section className="grid grid-cols-2 gap-3 xl:grid-cols-4" aria-label={t("dash.workbench.monthSummary")}>
            <DashboardKpi
              icon={WalletCards}
              label={t("dash.workbench.collectedMonth")}
              value={maskAmount(amountsHidden, `¥${overview.finance.this_month_net_amount_yuan}`)}
              hint={t("dash.workbench.collectedHint", { count: overview.finance.this_month_count })}
              tone="success"
              onClick={() => navigate("/bills")}
            />
            <DashboardKpi
              icon={Clock3}
              label={t("dash.workbench.pendingMonth")}
              value={maskAmount(amountsHidden, `¥${overview.this_month_pending_amount_yuan}`)}
              hint={t("dash.workbench.pendingHint", { count: overview.this_month_pending_count })}
              tone="warning"
              onClick={() => navigate("/calendar?view=tasks")}
            />
            <DashboardKpi
              icon={Coins}
              label={t("dash.workbench.projectedProfit")}
              value={maskAmount(amountsHidden, formatCents(overview.projected_monthly_profit_cents))}
              hint={t("dash.workbench.projectedProfitHint", { count: overview.active_recurring_count })}
              onClick={() => navigate("/goals")}
            />
            <DashboardKpi
              icon={Users}
              label={t("dash.workbench.utilization")}
              value={`${overview.capacity.utilization_percent}%`}
              hint={t("dash.workbench.utilizationHint", {
                used: overview.capacity.seat_used,
                total: overview.capacity.seat_total,
                free: overview.capacity.seat_free,
              })}
              tone="default"
              onClick={() => navigate("/accounts")}
            />
          </section>

          <section className="grid min-h-[430px] gap-4 xl:h-[430px] xl:min-h-0 xl:grid-cols-[minmax(0,1.25fr)_minmax(360px,0.75fr)]">
            <CashflowCard overview={overview} amountsHidden={amountsHidden} />
            <OperationsQueue
              overview={overview}
              amountsHidden={amountsHidden}
              onCollect={openCollect}
            />
          </section>

          <section className="grid items-stretch gap-4 xl:grid-cols-2">
            <CapacityCard overview={overview} onOpenSegment={setCapacitySegment} />
            <DecisionCard overview={overview} amountsHidden={amountsHidden} />
          </section>
        </>
      ) : null}

      <PlusRentalDialog open={plusDialogOpen} onOpenChange={setPlusDialogOpen} prefill={null} />
      <SubscriptionDialog open={teamDialogOpen} onOpenChange={setTeamDialogOpen} prefill={null} />
      <DuePaidDialog
        open={duePaidTarget !== null}
        onOpenChange={(open) => {
          if (!open) setDuePaidTarget(null)
        }}
        target={duePaidTarget}
      />
      <StatDetailDialog
        open={capacitySegment !== null}
        onOpenChange={(open) => {
          if (!open) setCapacitySegment(null)
        }}
        detail={capacityDetail}
      />
    </div>
  )
}
