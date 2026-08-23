import * as React from "react"
import { Link, useNavigate } from "react-router-dom"
import { useTranslation } from "react-i18next"
import {
  AlertTriangle,
  ArrowRight,
  CalendarClock,
  CheckCircle2,
  CircleDollarSign,
  Clock3,
  Coins,
  Gauge,
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
  Bar,
  CartesianGrid,
  ComposedChart,
  Line,
  ResponsiveContainer,
  Tooltip as RechartsTooltip,
  XAxis,
  YAxis,
} from "recharts"

import { useOperationsOverview } from "@/api/queries"
import type { OperationTask, OperationsOverview } from "@/api/types"
import { AmountPrivacyToggle } from "@/components/amount-privacy-toggle"
import { PageHeader } from "@/components/page-header"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { Skeleton } from "@/components/ui/skeleton"
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
    default: "bg-muted text-muted-foreground",
  }[tone]

  return (
    <button
      type="button"
      onClick={onClick}
      className="group min-h-[112px] min-w-0 rounded-lg border bg-card p-4 text-left outline-none transition-[border-color,background-color,box-shadow] hover:border-input hover:bg-accent/20 hover:shadow-lift focus-visible:ring-2 focus-visible:ring-brand/45"
    >
      <span className="flex items-start justify-between gap-3">
        <span className={cn("grid size-9 shrink-0 place-items-center rounded-md", toneClass)}>
          <Icon className="size-[18px]" />
        </span>
        <ArrowRight className="size-4 text-muted-foreground/50 transition-transform group-hover:translate-x-0.5 group-hover:text-foreground" />
      </span>
      <span className="mt-3 block text-xs font-medium text-muted-foreground">{label}</span>
      <span className="display-numeral mt-1 block truncate text-[22px] font-semibold leading-none">
        {value}
      </span>
      <span className="mt-2 block truncate text-[11px] text-muted-foreground">{hint}</span>
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
  const current = data.at(-1)?.netCents ?? 0
  const previous = data.at(-2)?.netCents ?? 0
  const change = previous === 0 ? null : Math.round(((current - previous) / Math.abs(previous)) * 100)

  return (
    <Card className="min-w-0 gap-4 p-4">
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
      {data.length === 0 ? (
        <div className="grid h-[260px] place-items-center rounded-lg border border-dashed text-sm text-muted-foreground">
          {t("dash.trendEmpty")}
        </div>
      ) : (
        <div className="h-[260px] min-w-0">
          <ResponsiveContainer width="100%" height="100%">
            <ComposedChart data={data} margin={{ top: 12, right: 10, bottom: 0, left: 0 }}>
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
                cursor={{ fill: "var(--muted)", opacity: 0.35 }}
                content={<TrendTooltip amountsHidden={amountsHidden} />}
              />
              <Bar
                dataKey="netCents"
                name="净收入"
                fill="var(--chart-1)"
                fillOpacity={0.82}
                radius={[4, 4, 0, 0]}
                maxBarSize={32}
              />
              <Line
                type="monotone"
                dataKey="grossCents"
                name="原实收"
                stroke="var(--chart-5)"
                strokeWidth={2}
                dot={{ r: 3, fill: "var(--card)", strokeWidth: 2 }}
                activeDot={{ r: 5 }}
              />
            </ComposedChart>
          </ResponsiveContainer>
        </div>
      )}
      <div className="grid grid-cols-3 divide-x rounded-lg border bg-muted/15 py-2.5 text-center">
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
  const tasks = (overview.tasks ?? []).slice(0, 8)

  const kindLabel = (kind: OperationTask["kind"]) => t(`dash.workbench.taskKinds.${kind}`)
  const timingLabel = (task: OperationTask) => {
    if (task.due_at_label) return task.due_at_label
    if (!task.due_date) return ""
    if (task.days_remaining < 0) return t("dueStatus.overdueDays", { count: Math.abs(task.days_remaining) })
    if (task.days_remaining === 0) return t("dueStatus.today")
    return t("dueStatus.days", { count: task.days_remaining })
  }

  return (
    <Card className="min-h-0 gap-3 p-4">
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
        <div className="grid min-h-0 gap-1.5 overflow-hidden">
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
                  "flex min-w-0 items-center gap-2.5 rounded-md border border-l-2 bg-muted/10 px-2.5 py-2",
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

      <Button asChild variant="ghost" size="sm" className="self-end">
        <Link to="/calendar?view=tasks">
          {t("dash.workbench.allTasks")}
          <ArrowRight />
        </Link>
      </Button>
    </Card>
  )
}

function CapacityCard({ overview }: { overview: OperationsOverview }) {
  const { t } = useTranslation()
  const { capacity } = overview
  const occupied = Math.max(0, capacity.seat_used - capacity.seat_frozen)
  const total = Math.max(1, capacity.seat_total)
  const occupiedPercent = (occupied / total) * 100
  const frozenPercent = (capacity.seat_frozen / total) * 100

  return (
    <Card className="gap-4 p-4">
      <div className="flex items-start justify-between gap-3">
        <div>
          <h2 className="panel-heading text-sm font-semibold">{t("dash.workbench.capacityTitle")}</h2>
          <p className="mt-1 text-xs text-muted-foreground">
            {t("dash.workbench.capacityHint", { count: capacity.account_count })}
          </p>
        </div>
        <div className="display-numeral text-2xl font-semibold text-brand">
          {capacity.utilization_percent}%
        </div>
      </div>
      <div className="flex h-2.5 overflow-hidden rounded-full bg-muted" aria-label={`${capacity.utilization_percent}%`}>
        <span className="bg-brand" style={{ width: `${occupiedPercent}%` }} />
        <span className="bg-gold" style={{ width: `${frozenPercent}%` }} />
      </div>
      <div className="grid grid-cols-3 gap-2">
        {[
          { label: t("dash.workbench.occupied"), value: occupied, tone: "text-brand" },
          { label: t("dash.workbench.frozen"), value: capacity.seat_frozen, tone: "text-gold" },
          { label: t("dash.workbench.free"), value: capacity.seat_free, tone: "text-success" },
        ].map((item) => (
          <div key={item.label} className="rounded-md border bg-muted/10 px-3 py-2.5">
            <p className="text-[10px] text-muted-foreground">{item.label}</p>
            <p className={cn("display-numeral mt-1 text-lg font-semibold", item.tone)}>{item.value}</p>
          </div>
        ))}
      </div>
      <Button asChild variant="outline" size="sm" className="w-full">
        <Link to="/accounts">
          <KeyRound />
          {t("dash.workbench.manageCapacity")}
        </Link>
      </Button>
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
  const actions = [
    {
      icon: TicketCheck,
      label: t("dash.workbench.pendingRedemptions"),
      value: overview.work.pending_redemption_count,
      to: "/redemptions",
    },
    {
      icon: HandCoins,
      label: t("dash.workbench.pendingAfterSales"),
      value: overview.work.pending_after_sales_count,
      to: "/after-sales",
    },
    {
      icon: Snowflake,
      label: t("dash.workbench.releasingSeats"),
      value: overview.capacity.seat_releasing_7d,
      to: "/accounts",
    },
  ]

  return (
    <Card className="gap-4 p-4">
      <div className="flex items-start justify-between gap-3">
        <div>
          <h2 className="panel-heading text-sm font-semibold">{t("dash.workbench.decisionTitle")}</h2>
          <p className="mt-1 text-xs text-muted-foreground">{t("dash.workbench.decisionHint")}</p>
        </div>
        <Target className="size-5 text-brand" />
      </div>
      {goal ? (
        <Link to="/goals" className="group block rounded-lg border bg-muted/10 p-3 hover:bg-muted/25">
          <div className="flex items-center justify-between gap-3">
            <span className="truncate text-xs font-semibold">{goal.name}</span>
            <span className="display-numeral text-sm font-semibold text-brand">
              {Math.round(goal.progress_percent)}%
            </span>
          </div>
          <div className="mt-2 h-1.5 overflow-hidden rounded-full bg-muted">
            <div className="h-full rounded-full bg-brand" style={{ width: `${Math.min(100, goal.progress_percent)}%` }} />
          </div>
          <div className="mt-2 flex items-center justify-between gap-3 text-[10px] text-muted-foreground">
            <span>{maskAmount(amountsHidden, formatCents(goal.current_profit_cents))}</span>
            <span>
              {goal.projected_date
                ? t("dash.workbench.projectedDate", { date: goal.projected_date })
                : t("dash.workbench.forecastCollecting")}
            </span>
          </div>
        </Link>
      ) : (
        <Button asChild variant="outline" className="w-full">
          <Link to="/goals">{t("dash.workbench.createGoal")}</Link>
        </Button>
      )}
      <div className="grid gap-1.5">
        {actions.map((item) => {
          const Icon = item.icon
          return (
            <Link
              key={item.label}
              to={item.to}
              className="flex items-center gap-2.5 rounded-md border bg-muted/10 px-3 py-2 transition-colors hover:bg-muted/30"
            >
              <Icon className="size-4 text-muted-foreground" />
              <span className="min-w-0 flex-1 truncate text-xs font-medium">{item.label}</span>
              <span className={cn("display-numeral text-sm font-semibold", item.value > 0 ? "text-destructive" : "text-success")}>
                {item.value}
              </span>
              <ArrowRight className="size-3.5 text-muted-foreground" />
            </Link>
          )
        })}
      </div>
      <Button asChild variant="ghost" size="sm" className="w-full">
        <Link to="/goals">
          <Gauge />
          {t("dash.workbench.viewAnalysis")}
        </Link>
      </Button>
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

  return (
    <div className="flex flex-col gap-4">
      <PageHeader
        title={t("dash.workbench.title")}
        titleAccessory={<AmountPrivacyToggle amountsHidden={amountsHidden} onToggle={toggleAmounts} />}
        actions={
          <div className="flex flex-wrap items-center gap-2">
            <Button variant="outline" size="icon" aria-label={t("common.refresh")} onClick={() => void overviewQuery.refetch()}>
              <RefreshCw className={cn(overviewQuery.isFetching && "animate-spin")} />
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

          <section className="grid min-h-[430px] gap-4 xl:grid-cols-[minmax(0,1.25fr)_minmax(360px,0.75fr)]">
            <CashflowCard overview={overview} amountsHidden={amountsHidden} />
            <OperationsQueue
              overview={overview}
              amountsHidden={amountsHidden}
              onCollect={openCollect}
            />
          </section>

          <section className="grid gap-4 lg:grid-cols-2">
            <CapacityCard overview={overview} />
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
    </div>
  )
}
