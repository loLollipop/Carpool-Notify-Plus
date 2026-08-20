import * as React from "react"
import { useTranslation } from "react-i18next"
import {
  Bar,
  BarChart,
  CartesianGrid,
  Cell,
  ComposedChart,
  Line,
  Pie,
  PieChart,
  ReferenceLine,
  ResponsiveContainer,
  Tooltip as ChartTooltip,
  XAxis,
  YAxis,
} from "recharts"
import {
  ArrowRight,
  BrainCircuit,
  CalendarClock,
  CheckCircle2,
  CircleGauge,
  Clock3,
  ExternalLink,
  Gift,
  HeartHandshake,
  History,
  MessageCircle,
  Pencil,
  Plus,
  RefreshCw,
  Search,
  ShieldCheck,
  SlidersHorizontal,
  Target,
  TrendingUp,
  Users,
  WalletCards,
} from "lucide-react"

import {
  completeBusinessGoal,
  createBusinessGoal,
  exemptGoalBulkPricing,
  recordGoalCustomerBenefits,
  refreshGoalMarket,
  scheduleGoalBulkNextPrice,
  updateBusinessGoal,
} from "@/api/endpoints"
import { useAppMutation } from "@/api/mutations"
import { useGoals } from "@/api/queries"
import type {
  BusinessGoal,
  BusinessGoalInput,
  BulkPricingExemptionInput,
  CustomerBenefitView,
  CustomerTierSummary,
  CustomerBenefitCandidate,
  CustomerBenefitType,
  ForecastScenario,
  GoalCenter,
  PricingCandidate,
  RecordCustomerBenefitsInput,
} from "@/api/types"
import { AmountPrivacyToggle } from "@/components/amount-privacy-toggle"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { PageHeader } from "@/components/page-header"
import {
  StatDetailDialog,
  type StatDetailState,
} from "@/components/stat-detail-dialog"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { Checkbox } from "@/components/ui/checkbox"
import {
  Dialog,
  DialogContent,
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
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Textarea } from "@/components/ui/textarea"
import { useAmountPrivacy } from "@/hooks/use-amount-privacy"
import { maskAmount } from "@/lib/amount-privacy"
import { cn } from "@/lib/utils"

function yuan(cents: number) {
  return `\u00a5${(cents / 100).toLocaleString("zh-CN", {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  })}`
}

function visibleYuan(cents: number, hidden: boolean) {
  return maskAmount(hidden, yuan(cents))
}

function compactYuan(cents: number) {
  return `\u00a5${(cents / 100).toLocaleString("zh-CN", {
    minimumFractionDigits: 0,
    maximumFractionDigits: 2,
  })}`
}

function visibleCompactYuan(cents: number, hidden: boolean) {
  return maskAmount(hidden, compactYuan(cents))
}

type RepricingDetailKey = "users" | "relationship" | "protected" | "batch"

function repricingDetailAmountCents(
  key: RepricingDetailKey,
  candidate: PricingCandidate,
) {
  if (key !== "users") return candidate.monthly_revenue_cents
  return candidate.customer_group_monthly_revenue_cents > 0
    ? candidate.customer_group_monthly_revenue_cents
    : candidate.monthly_revenue_cents
}

function inputYuan(cents: number) {
  return (cents / 100).toFixed(2)
}

function shanghaiToday() {
  const parts = new Intl.DateTimeFormat("en-US", {
    timeZone: "Asia/Shanghai",
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
  }).formatToParts(new Date())
  const value = (type: "year" | "month" | "day") =>
    parts.find((part) => part.type === type)?.value ?? ""
  return `${value("year")}-${value("month")}-${value("day")}`
}

function GoalDialog({
  open,
  onOpenChange,
  goal,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  goal: BusinessGoal | null
}) {
  const { t } = useTranslation()
  const [name, setName] = React.useState(() => goal?.name ?? t("goals.defaultName"))
  const [target, setTarget] = React.useState(() =>
    goal ? inputYuan(goal.target_profit_cents) : "",
  )

  const mutation = useAppMutation(
    (input: BusinessGoalInput) =>
      goal ? updateBusinessGoal(goal.id, input) : createBusinessGoal(input),
    { onSuccess: () => onOpenChange(false) },
  )

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent aria-describedby={undefined} className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{goal ? t("goals.editTitle") : t("goals.createTitle")}</DialogTitle>
        </DialogHeader>
        <form
          aria-busy={mutation.isPending}
          className="grid gap-4"
          onSubmit={(event) => {
            event.preventDefault()
            mutation.mutate({
              name: name.trim(),
              target_profit_yuan: target.trim(),
            })
          }}
        >
          <div className="grid gap-2">
            <Label htmlFor="goal-name">{t("goals.name")}</Label>
            <Input
              id="goal-name"
              value={name}
              onChange={(event) => setName(event.target.value)}
              maxLength={80}
              required
              autoFocus
            />
          </div>
          <div className="grid gap-2">
            <Label htmlFor="goal-target">{t("goals.targetProfit")}</Label>
            <div className="relative">
              <span className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-sm text-muted-foreground">
                ¥
              </span>
              <Input
                id="goal-target"
                className="pl-7 tabular-nums"
                value={target}
                onChange={(event) => setTarget(event.target.value)}
                inputMode="decimal"
                pattern="\d+(\.\d{1,2})?"
                placeholder="10000.00"
                required
              />
            </div>
            <p className="text-xs leading-5 text-muted-foreground">{t("goals.forecastHint")}</p>
          </div>
          <DialogFooter className="mt-2">
            <Button
              type="button"
              variant="outline"
              disabled={mutation.isPending}
              onClick={() => onOpenChange(false)}
            >
              {t("common.cancel")}
            </Button>
            <Button type="submit" disabled={mutation.isPending}>
              {mutation.isPending ? t("common.saving") : t("common.save")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function Metric({
  label,
  value,
  detail,
  icon: Icon,
  onClick,
}: {
  label: string
  value: React.ReactNode
  detail: string
  icon: React.ComponentType<{ className?: string }>
  onClick?: () => void
}) {
  const Component = onClick ? "button" : "div"
  return (
    <Component
      type={onClick ? "button" : undefined}
      onClick={onClick}
      className={cn(
        "min-w-0 border-l border-border/70 px-3 py-3.5 text-left first:border-l-0 sm:px-4",
        onClick && "outline-none transition-colors hover:bg-accent/30 focus-visible:ring-2 focus-visible:ring-brand/45 focus-visible:ring-inset",
      )}
    >
      <div className="flex items-center gap-2 text-xs font-medium text-muted-foreground">
        <Icon className="size-3.5 text-brand" />
        <span className="truncate">{label}</span>
      </div>
      <div className="display-numeral mt-2 truncate text-lg leading-none sm:text-xl">{value}</div>
      <p className="mt-0.5 hidden truncate text-[11px] text-muted-foreground sm:block">{detail}</p>
    </Component>
  )
}

function ActiveGoalPanel({
  data,
  forecast,
  amountsHidden,
  onEdit,
  onComplete,
}: {
  data: NonNullable<GoalCenter["active_goal"]>
  forecast: GoalCenter["forecast"]
  amountsHidden: boolean
  onEdit: () => void
  onComplete: () => void
}) {
  const { t } = useTranslation()
  const progress = Math.max(0, Math.min(100, data.progress_percent))
  const [statDetail, setStatDetail] = React.useState<StatDetailState | null>(null)

  const openGoalDetail = (key: "remaining" | "monthly" | "date") => {
    if (key === "remaining") {
      setStatDetail({
        title: t("goals.remaining"),
        items: [
          { id: "earned", title: t("goals.earned"), value: visibleYuan(data.earned_profit_cents, amountsHidden), valueTone: "success" },
          { id: "target", title: t("goals.targetProfit"), value: visibleYuan(data.goal.target_profit_cents, amountsHidden) },
          { id: "remaining", title: t("goals.remaining"), value: visibleYuan(data.remaining_profit_cents, amountsHidden), valueTone: "warning" },
        ],
      })
      return
    }
    const scenarios = forecast
      ? [
          ["conservative", forecast.conservative],
          ["baseline", forecast.baseline],
          ["optimistic", forecast.optimistic],
        ] as const
      : []
    setStatDetail({
      title: key === "monthly" ? t("goals.futureMonthly") : t("goals.estimatedDate"),
      items: scenarios.map(([scenarioKey, scenario]) => ({
        id: scenarioKey,
        title: t(`goals.${scenarioKey}`),
        subtitle: scenario.projected_date || t("goals.forecastUnavailable"),
        value: key === "monthly"
          ? visibleYuan(scenario.monthly_profit_cents, amountsHidden)
          : scenario.projected_date || "-",
        valueTone: scenarioKey === "baseline" ? "success" : "default",
      })),
    })
  }

  return (
    <section className="overflow-hidden rounded-lg border bg-card shadow-card animate-fade-up">
      <div className="flex flex-col gap-3 border-b px-4 py-3 sm:flex-row sm:items-center sm:justify-between sm:px-5">
        <div className="flex min-w-0 items-center gap-3">
          <Badge className="shrink-0" variant={data.reached ? "success" : "brand"}>
            {data.reached ? <CheckCircle2 /> : <Target />}
            {data.reached ? t("goals.reached") : t("goals.inProgress")}
          </Badge>
          <h2 className="truncate text-base font-semibold sm:text-lg">{data.goal.name}</h2>
        </div>
        <div className="flex shrink-0 items-center gap-2">
          <Button variant="outline" size="sm" onClick={onEdit}>
            <Pencil />
            {t("common.edit")}
          </Button>
          <Button variant={data.reached ? "default" : "outline"} size="sm" onClick={onComplete}>
            <CheckCircle2 />
            {t("goals.complete")}
          </Button>
        </div>
      </div>

      <div className="grid xl:grid-cols-[minmax(0,1.1fr)_minmax(520px,0.9fr)]">
        <div className="p-4 sm:p-5">
          <div className="flex items-end justify-between gap-4">
            <div className="min-w-0">
              <p className="text-xs font-medium text-muted-foreground">{t("goals.earned")}</p>
              <p className="display-numeral mt-2 truncate text-[30px] leading-none sm:text-[34px]">
                {visibleYuan(data.earned_profit_cents, amountsHidden)}
              </p>
            </div>
            <p className="shrink-0 text-sm font-semibold text-brand tabular-nums">{progress}%</p>
          </div>
          <div className="mt-3 h-2 overflow-hidden rounded-full bg-muted">
            <div
              className="h-full rounded-full bg-brand transition-[width] duration-700"
              style={{ width: `${Math.max(progress, progress > 0 ? 2 : 0)}%` }}
            />
          </div>
          <div className="mt-2.5 flex flex-col gap-1 text-xs text-muted-foreground sm:flex-row sm:items-center sm:justify-between sm:gap-3">
            <span>{t("goals.currentProfitIncluded")}</span>
            <span className="tabular-nums sm:text-right">
              {t("goals.targetValue", {
                value: visibleYuan(data.goal.target_profit_cents, amountsHidden),
              })}
            </span>
          </div>
        </div>

        <div className="grid grid-cols-3 border-t border-border/70 xl:border-l xl:border-t-0">
          <Metric
            label={t("goals.remaining")}
            value={visibleYuan(data.remaining_profit_cents, amountsHidden)}
            detail={t("goals.forecastBasisShort")}
            icon={WalletCards}
            onClick={() => openGoalDetail("remaining")}
          />
          <Metric
            label={t("goals.futureMonthly")}
            value={visibleYuan(forecast?.baseline.monthly_profit_cents ?? 0, amountsHidden)}
            detail={t("goals.activeRecurring", { count: forecast?.active_recurring_count ?? 0 })}
            icon={TrendingUp}
            onClick={() => openGoalDetail("monthly")}
          />
          <Metric
            label={t("goals.estimatedDate")}
            value={forecast?.baseline.projected_date || "-"}
            detail={t("goals.autoCalculated")}
            icon={CalendarClock}
            onClick={() => openGoalDetail("date")}
          />
        </div>
      </div>
      <StatDetailDialog
        open={statDetail !== null}
        onOpenChange={(open) => {
          if (!open) setStatDetail(null)
        }}
        detail={statDetail}
      />
    </section>
  )
}

function ForecastRow({
  label,
  scenario,
  amountsHidden,
  emphasis = false,
}: {
  label: string
  scenario: ForecastScenario
  amountsHidden: boolean
  emphasis?: boolean
}) {
  const { t } = useTranslation()
  const available = Boolean(scenario.projected_date)
  return (
    <div
      className={cn(
        "grid flex-1 grid-cols-[minmax(0,1fr)_auto] content-center items-center gap-x-3 gap-y-1 border-t px-4 py-3 first:border-t-0",
        emphasis && "bg-brand/[0.055]",
      )}
    >
      <span className="min-w-0 truncate text-sm font-semibold">{label}</span>
      <span className="text-sm font-semibold tabular-nums">
        {scenario.projected_date || "-"}
      </span>
      <p className="col-span-2 text-[11px] text-muted-foreground tabular-nums">
        {available
          ? t("goals.monthlyPace", {
              amount: visibleYuan(scenario.monthly_profit_cents, amountsHidden),
              months: scenario.months_needed,
            })
          : t("goals.forecastUnavailable")}
      </p>
    </div>
  )
}

function ForecastPanel({
  data,
  amountsHidden,
}: {
  data: NonNullable<GoalCenter["forecast"]>
  amountsHidden: boolean
}) {
  const { t } = useTranslation()
  return (
    <section className="flex h-full min-h-[260px] flex-col overflow-hidden rounded-lg border bg-card shadow-card">
      <div className="flex items-center justify-between gap-3 border-b px-4 py-3.5">
        <div>
          <h2 className="text-sm font-semibold">{t("goals.forecastTitle")}</h2>
          <p className="mt-1 text-xs text-muted-foreground">
            {t(`goals.forecastSource.${data.source}`, { count: data.active_recurring_count })}
          </p>
        </div>
        <CircleGauge className="size-5 text-brand" />
      </div>
      <ForecastRow
        label={t("goals.conservative")}
        scenario={data.conservative}
        amountsHidden={amountsHidden}
      />
      <ForecastRow
        label={t("goals.baseline")}
        scenario={data.baseline}
        amountsHidden={amountsHidden}
        emphasis
      />
      <ForecastRow
        label={t("goals.optimistic")}
        scenario={data.optimistic}
        amountsHidden={amountsHidden}
      />
    </section>
  )
}

function ProfitTrend({ data, amountsHidden }: { data: GoalCenter; amountsHidden: boolean }) {
  const { t } = useTranslation()
  const trend = data.trend ?? []
  return (
    <section className="flex h-full min-h-[260px] flex-col rounded-lg border bg-card p-4 shadow-card">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h2 className="text-sm font-semibold">{t("goals.trendTitle")}</h2>
          <p className="mt-1 text-xs text-muted-foreground">{t("goals.trendRange")}</p>
        </div>
        <div className="flex items-center gap-3 text-[10px] font-medium text-muted-foreground">
          <span className="flex items-center gap-1.5">
            <span className="size-2.5 rounded-sm bg-success" />
            {t("goals.monthProfit")}
          </span>
          <span className="flex items-center gap-1.5">
            <span className="h-0.5 w-4 rounded-full bg-brand" />
            {t("goals.monthRevenue")}
          </span>
        </div>
      </div>
      <div className="mt-3 min-h-[182px] w-full flex-1">
        <ResponsiveContainer width="100%" height="100%">
          <ComposedChart data={trend} margin={{ top: 8, right: 8, left: -16, bottom: 0 }}>
            <CartesianGrid stroke="var(--border)" strokeDasharray="3 5" vertical={false} />
            <XAxis
              dataKey="month"
              axisLine={false}
              tickLine={false}
              tick={{ fill: "var(--muted-foreground)", fontSize: 11 }}
              tickFormatter={(value: string) => value.slice(5)}
            />
            <YAxis
              axisLine={false}
              tickLine={false}
              tick={{ fill: "var(--muted-foreground)", fontSize: 10 }}
              tickFormatter={(value: number) => (amountsHidden ? "*" : `${Math.round(value / 100)}`)}
            />
            <ReferenceLine y={0} stroke="var(--muted-foreground)" strokeOpacity={0.45} />
            <ChartTooltip
              cursor={{ fill: "color-mix(in oklab, var(--brand) 7%, transparent)" }}
              formatter={(value, name) => [
                visibleYuan(Number(value ?? 0), amountsHidden),
                name === "revenue_cents" ? t("goals.monthRevenue") : t("goals.monthProfit"),
              ]}
              labelFormatter={(label) => String(label)}
              contentStyle={{
                border: "1px solid var(--border)",
                borderRadius: 6,
                background: "var(--popover)",
                color: "var(--popover-foreground)",
                fontSize: 12,
              }}
            />
            <Bar dataKey="profit_cents" maxBarSize={24} radius={[4, 4, 4, 4]}>
              {trend.map((month) => (
                <Cell
                  key={month.month}
                  fill={month.profit_cents < 0 ? "var(--destructive)" : "var(--success)"}
                />
              ))}
            </Bar>
            <Line
              type="linear"
              dataKey="revenue_cents"
              stroke="var(--brand)"
              strokeWidth={2.25}
              dot={{ r: 2.5, fill: "var(--card)", strokeWidth: 2 }}
              activeDot={{ r: 4, strokeWidth: 2, fill: "var(--card)" }}
            />
          </ComposedChart>
        </ResponsiveContainer>
      </div>
    </section>
  )
}

const recommendationBadge = {
  raise: "success",
  hold: "brand",
  fill: "warning",
  lower_test: "warning",
  insufficient: "secondary",
} as const

function MarketPanel({
  data,
  amountsHidden,
  refreshing,
  onRefresh,
}: {
  data: GoalCenter
  amountsHidden: boolean
  refreshing: boolean
  onRefresh: () => void
}) {
  const { t } = useTranslation()
  const pricing = data.pricing
  const market = data.market
  const snapshot = market.snapshot
  const utilization = Math.min(100, Math.max(0, pricing.utilization_percent || 0))
  const newSaleDiscount = Math.max(0, pricing.new_sale_discount_percent || 0)
  const benchmarkData = snapshot
    ? [
        {
          label: t("goals.seatCostFloorShort"),
          shortLabel: t("goals.seatCostFloorAxis"),
          value: pricing.seat_cost_floor_cents,
          color: "var(--muted-foreground)",
        },
        {
          label: t("goals.internalMedianShort"),
          shortLabel: t("goals.internalMedianAxis"),
          value: pricing.internal_median_price_cents,
          color: "var(--brand)",
        },
        {
          label: t("goals.marketLow"),
          shortLabel: t("goals.marketLowShort"),
          value: snapshot.low_price_cents,
          color: "color-mix(in oklab, var(--success) 48%, var(--muted))",
        },
        {
          label: t("goals.marketMedianShort"),
          shortLabel: t("goals.marketMedianAxis"),
          value: snapshot.median_price_cents,
          color: "var(--success)",
        },
        {
          label: t("goals.marketHigh"),
          shortLabel: t("goals.marketHighShort"),
          value: snapshot.high_price_cents,
          color: "color-mix(in oklab, var(--success) 68%, var(--muted))",
        },
      ]
    : []

  return (
    <section className="h-full min-h-[260px] overflow-hidden rounded-lg border bg-card shadow-card">
      <div className="flex items-center justify-between gap-2 border-b px-4 py-3">
        <div className="flex min-w-0 items-center gap-2">
          <h2 className="truncate text-sm font-semibold">{t("goals.pricingTitle")}</h2>
          <Badge className="shrink-0" variant={recommendationBadge[pricing.action]}>
            {t(`goals.action.${pricing.action}`)}
          </Badge>
          {market.stale ? <Badge variant="warning">{t("goals.cached")}</Badge> : null}
        </div>
        <div className="flex shrink-0 items-center gap-1.5">
          <Button variant="ghost" size="icon-sm" asChild>
            <a href={market.source_url} target="_blank" rel="noreferrer" title="PriceAI">
              <ExternalLink />
              <span className="sr-only">PriceAI</span>
            </a>
          </Button>
          <Button
            variant="ghost"
            size="icon-sm"
            onClick={onRefresh}
            disabled={refreshing}
            title={t("goals.refreshMarket")}
          >
            <RefreshCw className={cn(refreshing && "animate-spin")} />
            <span className="sr-only">{t("goals.refreshMarket")}</span>
          </Button>
        </div>
      </div>

      <div className="p-4">
        <div className="grid grid-cols-[92px_minmax(0,1fr)] items-center gap-3">
          <div
            className="relative grid size-[88px] place-items-center rounded-full"
            style={{
              background: `conic-gradient(var(--brand) ${utilization * 3.6}deg, var(--muted) 0deg)`,
            }}
            role="img"
            aria-label={t("goals.seatUtilizationValue", { value: utilization })}
          >
            <span className="absolute inset-[9px] rounded-full bg-card" />
            <div className="relative text-center">
              <p className="text-lg font-semibold tabular-nums">{utilization}%</p>
              <p className="text-[9px] text-muted-foreground">{t("goals.utilizationShort")}</p>
            </div>
          </div>
          <div className="min-w-0">
            <p className="truncate text-xs font-semibold">{t(`goals.actionTitle.${pricing.action}`)}</p>
            <p className="mt-1 text-[10px] text-muted-foreground tabular-nums">
              {t("goals.seatUsage", {
                used: pricing.seat_used,
                total: pricing.seat_total,
                available: pricing.seat_available,
              })}
            </p>
            {pricing.suggested_low_price_cents > 0 ? (
              <div className="mt-2 rounded-md bg-brand/[0.06] px-2.5 py-2">
                <p className="text-[9px] font-medium text-muted-foreground">
                  {t(
                    pricing.suggested_low_price_cents === pricing.suggested_high_price_cents
                      ? "goals.suggestedPrice"
                      : "goals.suggestedRange",
                  )}
                </p>
                <p className="display-numeral mt-1 truncate text-base leading-none text-brand">
                  {visibleYuan(pricing.suggested_low_price_cents, amountsHidden)}
                  {pricing.suggested_low_price_cents !== pricing.suggested_high_price_cents ? (
                    <>
                      <span className="mx-1 text-muted-foreground">–</span>
                      {visibleYuan(pricing.suggested_high_price_cents, amountsHidden)}
                    </>
                  ) : null}
                </p>
                <p className="mt-1.5 line-clamp-2 text-[9px] leading-3.5 text-muted-foreground">
                  {newSaleDiscount > 0
                    ? t("goals.newSaleAdvantage", { value: newSaleDiscount })
                    : t("goals.newSaleMarginFloor")}
                </p>
              </div>
            ) : null}
          </div>
        </div>

        {snapshot ? (
          <div className="mt-3 border-t pt-2.5">
            <div className="flex items-center justify-between gap-3">
              <p className="text-[10px] font-medium text-muted-foreground">{t("goals.priceBenchmark")}</p>
              <p className="text-[9px] text-muted-foreground">
                {t("goals.marketSamples", { count: snapshot.sample_count })}
              </p>
            </div>
            <div className="mt-1 h-[118px] w-full">
              <ResponsiveContainer width="100%" height="100%">
                <BarChart
                  data={benchmarkData}
                  layout="vertical"
                  margin={{ top: 0, right: 4, left: 0, bottom: 0 }}
                >
                  <XAxis type="number" hide domain={[0, "dataMax"]} />
                  <YAxis
                    type="category"
                    dataKey="shortLabel"
                    axisLine={false}
                    tickLine={false}
                    width={48}
                    tick={{ fill: "var(--muted-foreground)", fontSize: 9 }}
                  />
                  <ChartTooltip
                    cursor={{ fill: "color-mix(in oklab, var(--brand) 6%, transparent)" }}
                    formatter={(value) => [visibleYuan(Number(value ?? 0), amountsHidden), t("goals.price")]}
                    labelFormatter={(label) =>
                      benchmarkData.find((item) => item.shortLabel === label)?.label ?? String(label)
                    }
                    contentStyle={{
                      border: "1px solid var(--border)",
                      borderRadius: 6,
                      background: "var(--popover)",
                      color: "var(--popover-foreground)",
                      fontSize: 11,
                    }}
                  />
                  <Bar dataKey="value" maxBarSize={11} radius={[0, 4, 4, 0]}>
                    {benchmarkData.map((item) => (
                      <Cell key={item.shortLabel} fill={item.color} />
                    ))}
                  </Bar>
                </BarChart>
              </ResponsiveContainer>
            </div>
          </div>
        ) : (
          <div className="mt-3 grid min-h-[120px] place-items-center border-t pt-3 text-center">
            <div>
              <RefreshCw className="mx-auto size-5 text-muted-foreground" />
              <p className="mt-2 text-xs font-medium">{t("goals.marketUnavailable")}</p>
            </div>
          </div>
        )}
      </div>
    </section>
  )
}

function RepricingMetric({
  icon,
  label,
  value,
  hint,
  onClick,
}: {
  icon: React.ReactNode
  label: string
  value: React.ReactNode
  hint: string
  onClick: () => void
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      aria-label={label}
      className="group relative rounded-lg border bg-card p-4 text-left shadow-card outline-none transition-[border-color,background-color,box-shadow] hover:border-input hover:bg-accent/25 hover:shadow-lift focus-visible:ring-2 focus-visible:ring-brand/45"
    >
      <div className="flex items-center justify-between gap-3">
        <p className="text-xs font-medium text-muted-foreground">{label}</p>
        <span className="grid size-8 shrink-0 place-items-center rounded-md bg-brand/10 text-brand">
          {icon}
        </span>
      </div>
      <p className="display-numeral mt-4 text-[28px] leading-none">{value}</p>
      <p className="mt-1 min-h-8 text-[11px] leading-4 text-muted-foreground">
        {hint}
      </p>
      <span className="absolute inset-x-0 bottom-0 h-0.5 origin-left scale-x-0 bg-brand transition-transform duration-300 group-hover:scale-x-100 group-focus-visible:scale-x-100" />
    </button>
  )
}

type SegmentChartDatum = {
  key: string
  label: string
  count: number
  color: string
}

type CustomerTierKey = PricingCandidate["customer_tier"]

const customerTierOrder: CustomerTierKey[] = ["core", "mainstay", "optimize"]

const customerTierVisuals = {
  core: {
    color: "var(--chart-1)",
    badgeClass: "border-transparent bg-chart-1/12 text-chart-1",
    labelClass: "text-chart-1",
    rankClass: "border-chart-1/25 bg-chart-1/10 text-chart-1",
  },
  mainstay: {
    color: "var(--chart-2)",
    badgeClass: "border-transparent bg-chart-2/12 text-chart-2",
    labelClass: "text-chart-2",
    rankClass: "border-chart-2/25 bg-chart-2/10 text-chart-2",
  },
  optimize: {
    color: "var(--warning)",
    badgeClass: "border-transparent bg-warning/15 text-warning-foreground dark:text-warning",
    labelClass: "text-warning-foreground dark:text-warning",
    rankClass: "border-warning/30 bg-warning/10 text-warning-foreground dark:text-warning",
  },
} as const

function segmentTotal(data: SegmentChartDatum[]) {
  return data.reduce((total, item) => total + item.count, 0)
}

function segmentPercent(count: number, total: number) {
  return total > 0 ? Math.round((count / total) * 100) : 0
}

function segmentAriaLabel(data: SegmentChartDatum[], total: number) {
  return data
    .map((item) => `${item.label} ${item.count}, ${segmentPercent(item.count, total)}%`)
    .join("; ")
}

function SegmentCardHeader({ title, hint }: { title: string; hint: string }) {
  return (
    <div className="border-b px-4 py-3">
      <h2 className="text-sm font-semibold">{title}</h2>
      <p className="mt-1 text-[11px] leading-4 text-muted-foreground">{hint}</p>
    </div>
  )
}

function CustomerTierPanel({
  tiers,
  amountsHidden,
  activeTier,
  onTierChange,
}: {
  tiers: CustomerTierSummary[]
  amountsHidden: boolean
  activeTier: CustomerTierKey | "all"
  onTierChange: (tier: CustomerTierKey | "all") => void
}) {
  const { t } = useTranslation()
  const tierMap = new Map(tiers.map((tier) => [tier.key, tier]))
  const orderedTiers = customerTierOrder.map((key) => {
    const tier = tierMap.get(key)
    return tier
      ? { ...tier, customer_count: tier.customer_count ?? tier.count }
      : {
        key,
        count: 0,
        customer_count: 0,
        monthly_revenue_cents: 0,
        revenue_share_percent: 0,
        average_price_cents: 0,
        lowest_price_cents: 0,
        highest_price_cents: 0,
        recommended_count: 0,
        scheduled_count: 0,
      }
  })
  const totalSeats = orderedTiers.reduce((sum, tier) => sum + tier.count, 0)
  const totalCustomers = orderedTiers.reduce((sum, tier) => sum + tier.customer_count, 0)
  const chartData = orderedTiers.map((tier) => ({
    ...tier,
    label: t(`goals.repricing.customerTier.${tier.key}.label`),
    color: customerTierVisuals[tier.key].color,
  }))
  const chartAriaLabel = chartData
    .map(
      (tier) =>
        `${tier.label}: ${tier.revenue_share_percent}%, ${t("goals.repricing.customerTier.customerSeatSummary", { customers: tier.customer_count, seats: tier.count })}`,
    )
    .join("; ")

  return (
    <Card className="gap-0 overflow-hidden p-0 shadow-card">
      <div className="flex flex-col gap-3 border-b px-5 py-4 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h2 className="text-sm font-semibold">{t("goals.repricing.customerTier.title")}</h2>
          <p className="mt-1 max-w-3xl text-[11px] leading-4 text-muted-foreground">
            {t("goals.repricing.customerTier.hint")}
          </p>
        </div>
        {activeTier !== "all" ? (
          <Button variant="ghost" size="sm" onClick={() => onTierChange("all")}>
            {t("goals.repricing.customerTier.showAll")}
          </Button>
        ) : null}
      </div>

      <div className="grid xl:grid-cols-[minmax(0,0.95fr)_minmax(520px,1.05fr)]">
        <div className="min-w-0 border-b p-4 xl:border-b-0 xl:border-r">
          <div>
            <h3 className="text-xs font-semibold">
              {t("goals.repricing.customerTier.chartTitle")}
            </h3>
            <p className="mt-1 text-[10px] leading-4 text-muted-foreground">
              {t("goals.repricing.customerTier.chartHint")}
            </p>
          </div>
          {totalSeats > 0 ? (
            <div className="mt-3 grid min-h-[286px] items-center gap-2 sm:grid-cols-[238px_minmax(0,1fr)]">
              <div
                className="relative h-[238px] min-w-0"
                role="img"
                aria-label={chartAriaLabel}
              >
                <ResponsiveContainer width="100%" height="100%">
                  <PieChart>
                    <Pie
                      data={chartData}
                      dataKey="monthly_revenue_cents"
                      nameKey="label"
                      cx="50%"
                      cy="50%"
                      startAngle={90}
                      endAngle={-270}
                      innerRadius={70}
                      outerRadius={100}
                      paddingAngle={3}
                      cornerRadius={6}
                      stroke="var(--card)"
                      strokeWidth={3}
                    >
                      {chartData.map((tier) => (
                        <Cell
                          key={tier.key}
                          fill={tier.color}
                          fillOpacity={activeTier === "all" || activeTier === tier.key ? 0.95 : 0.18}
                        />
                      ))}
                    </Pie>
                    <ChartTooltip
                      formatter={(value) => [
                        visibleYuan(Number(value ?? 0), amountsHidden),
                        t("goals.repricing.customerTier.monthlyRevenue"),
                      ]}
                      contentStyle={{
                        border: "1px solid var(--border)",
                        borderRadius: 8,
                        background: "var(--popover)",
                        color: "var(--popover-foreground)",
                        fontSize: 11,
                        boxShadow: "var(--shadow-card)",
                      }}
                    />
                  </PieChart>
                </ResponsiveContainer>
                <div className="pointer-events-none absolute inset-0 grid place-content-center text-center">
                  <span className="text-[10px] font-medium text-muted-foreground">
                    {t("goals.repricing.customerTier.totalCustomers")}
                  </span>
                  <span className="display-numeral mt-1 text-[30px] leading-none">{totalCustomers}</span>
                  <span className="mt-1 text-[9px] text-muted-foreground">
                    {t("goals.repricing.customerTier.totalSeats", { count: totalSeats })}
                  </span>
                </div>
              </div>

              <div className="grid gap-2">
                {chartData.map((tier) => {
                  const visual = customerTierVisuals[tier.key]
                  const isActive = activeTier === tier.key
                  return (
                    <button
                      key={tier.key}
                      type="button"
                      className={cn(
                        "rounded-lg border bg-card/70 p-2.5 text-left outline-none transition-[border-color,background-color,box-shadow] hover:border-brand/35 focus-visible:ring-2 focus-visible:ring-brand",
                        isActive && "border-brand/45 bg-brand/[0.045] shadow-card",
                      )}
                      aria-pressed={isActive}
                      onClick={() => onTierChange(isActive ? "all" : tier.key)}
                    >
                      <div className="flex items-center justify-between gap-3">
                        <span className="flex min-w-0 items-center gap-2">
                          <span className={cn("font-mono text-[9px] font-bold", visual.labelClass)}>
                            {t(`goals.repricing.customerTier.${tier.key}.grade`)}
                          </span>
                          <span className="truncate text-[11px] font-semibold">{tier.label}</span>
                        </span>
                        <span className="text-sm font-semibold tabular-nums">
                          {tier.revenue_share_percent}%
                        </span>
                      </div>
                      <div className="mt-2 h-1.5 overflow-hidden rounded-full bg-muted">
                        <span
                          className="block h-full rounded-full transition-[width,opacity] duration-500"
                          style={{
                            width: `${tier.revenue_share_percent}%`,
                            backgroundColor: visual.color,
                            opacity: activeTier === "all" || isActive ? 1 : 0.3,
                          }}
                        />
                      </div>
                      <div className="mt-2 flex items-center justify-between gap-2 text-[9px] text-muted-foreground">
                        <span>
                          {t("goals.repricing.customerTier.customerSeatSummary", {
                            customers: tier.customer_count,
                            seats: tier.count,
                          })}
                        </span>
                        <span className="tabular-nums">
                          {t("goals.repricing.customerTier.averagePrice")}{" "}
                          {tier.count > 0
                            ? visibleYuan(tier.average_price_cents, amountsHidden)
                            : "-"}
                        </span>
                      </div>
                    </button>
                  )
                })}
              </div>
            </div>
          ) : (
            <div className="grid h-[286px] place-items-center text-center">
              <p className="text-xs text-muted-foreground">
                {t("goals.repricing.customerTier.empty")}
              </p>
            </div>
          )}
        </div>

        <div className="grid auto-rows-fr gap-2 bg-muted/15 p-3">
          {orderedTiers.map((tier) => {
            const visual = customerTierVisuals[tier.key]
            const isActive = activeTier === tier.key
            const priceRange =
              tier.count > 0
                ? tier.lowest_price_cents === tier.highest_price_cents
                  ? String(visibleYuan(tier.lowest_price_cents, amountsHidden))
                  : `${visibleYuan(tier.lowest_price_cents, amountsHidden)} – ${visibleYuan(tier.highest_price_cents, amountsHidden)}`
                : "-"
            return (
              <button
                key={tier.key}
                type="button"
                className={cn(
                  "min-w-0 rounded-lg border bg-card p-3 text-left outline-none transition-[border-color,background-color,box-shadow] hover:border-brand/35 hover:bg-card focus-visible:ring-2 focus-visible:ring-brand",
                  isActive && "border-brand/45 bg-brand/[0.045] shadow-card",
                )}
                aria-pressed={isActive}
                onClick={() => onTierChange(isActive ? "all" : tier.key)}
              >
                <div className="flex min-w-0 items-start gap-3">
                  <span className={cn("grid size-10 shrink-0 place-items-center rounded-lg border font-mono text-xs font-bold", visual.rankClass)}>
                    {t(`goals.repricing.customerTier.${tier.key}.grade`)}
                  </span>
                  <div className="grid min-w-0 flex-1 gap-3 sm:grid-cols-[minmax(0,1fr)_220px]">
                    <div className="min-w-0">
                      <Badge variant="outline" className={visual.badgeClass}>
                        {t(`goals.repricing.customerTier.${tier.key}.label`)}
                      </Badge>
                      <p className="mt-2 text-[10px] leading-4 text-muted-foreground">
                        {t(`goals.repricing.customerTier.${tier.key}.strategy`)}
                      </p>
                      <div className="mt-2 flex min-h-5 flex-wrap gap-1.5">
                        {tier.recommended_count > 0 ? (
                          <span className="rounded-full bg-muted px-2 py-0.5 text-[9px] font-medium text-muted-foreground">
                            {t("goals.repricing.customerTier.recommended", {
                              count: tier.recommended_count,
                            })}
                          </span>
                        ) : null}
                        {tier.scheduled_count > 0 ? (
                          <span className="rounded-full bg-brand/10 px-2 py-0.5 text-[9px] font-medium text-brand">
                            {t("goals.repricing.customerTier.scheduled", {
                              count: tier.scheduled_count,
                            })}
                          </span>
                        ) : null}
                      </div>
                    </div>
                    <div className="grid grid-cols-3 gap-2 self-center rounded-md bg-muted/30 p-2.5">
                      <div>
                        <p className="text-[8px] text-muted-foreground">
                          {t("goals.repricing.customerTier.customerSeatCount")}
                        </p>
                        <p className="display-numeral mt-1 text-lg leading-none">
                          {tier.customer_count} / {tier.count}
                        </p>
                      </div>
                      <div>
                        <p className="text-[8px] text-muted-foreground">
                          {t("goals.repricing.customerTier.revenueShare")}
                        </p>
                        <p className="mt-1 text-sm font-semibold tabular-nums">
                          {tier.revenue_share_percent}%
                        </p>
                      </div>
                      <div className="min-w-0">
                        <p className="text-[8px] text-muted-foreground">
                          {t("goals.repricing.customerTier.averagePrice")}
                        </p>
                        <p className="mt-1 truncate text-[11px] font-semibold tabular-nums" title={priceRange}>
                          {tier.count > 0
                            ? visibleYuan(tier.average_price_cents, amountsHidden)
                            : "-"}
                        </p>
                      </div>
                    </div>
                  </div>
                </div>
              </button>
            )
          })}
        </div>
      </div>
      <div className="border-t bg-muted/20 px-5 py-2.5 text-[10px] leading-4 text-muted-foreground">
        {t("goals.repricing.customerTier.footnote")}
      </div>
    </Card>
  )
}

function RelationshipJourneyCard({
  title,
  hint,
  data,
}: {
  title: string
  hint: string
  data: SegmentChartDatum[]
}) {
  const { t } = useTranslation()
  const total = segmentTotal(data)
  return (
    <Card className="gap-0 overflow-hidden p-0 shadow-card">
      <SegmentCardHeader title={title} hint={hint} />
      <div
        className="relative grid min-h-[172px] grid-cols-4 gap-1 px-3 py-5"
        role="img"
        aria-label={segmentAriaLabel(data, total)}
      >
        <div
          className="pointer-events-none absolute left-[12.5%] right-[12.5%] top-[37px] h-px bg-gradient-to-r from-warning via-brand to-success opacity-45"
          aria-hidden="true"
        />
        {data.map((item, index) => {
          const percent = segmentPercent(item.count, total)
          return (
            <div key={item.key} className="relative flex min-w-0 flex-col items-center text-center">
              <span
                className="relative z-10 grid size-8 place-items-center rounded-full border bg-card text-[10px] font-semibold"
                style={{
                  borderColor: item.color,
                  color: item.color,
                  boxShadow:
                    item.count > 0
                      ? `0 0 0 4px color-mix(in oklab, ${item.color} 11%, transparent)`
                      : undefined,
                }}
                aria-hidden="true"
              >
                0{index + 1}
              </span>
              <p className="display-numeral mt-4 text-[24px] leading-none">{item.count}</p>
              <p className="mt-2 truncate text-[11px] font-medium">{item.label}</p>
              <p className="mt-1 text-[9px] text-muted-foreground">
                {t("goals.repricing.shareValue", { value: percent })}
              </p>
            </div>
          )
        })}
      </div>
    </Card>
  )
}

function PriceSpectrumCard({
  title,
  hint,
  data,
}: {
  title: string
  hint: string
  data: SegmentChartDatum[]
}) {
  const { t } = useTranslation()
  const total = segmentTotal(data)
  return (
    <Card className="gap-0 overflow-hidden p-0 shadow-card">
      <SegmentCardHeader title={title} hint={hint} />
      <div className="min-h-[172px] px-4 py-4">
        <div
          className="flex h-4 overflow-hidden rounded-full bg-muted ring-1 ring-border/70 ring-inset"
          role="img"
          aria-label={segmentAriaLabel(data, total)}
        >
          {data.map((item) =>
            item.count > 0 ? (
              <span
                key={item.key}
                className="h-full min-w-1 transition-[width] duration-500"
                style={{
                  width: `${(item.count / Math.max(total, 1)) * 100}%`,
                  background: item.color,
                }}
                title={`${item.label}: ${item.count}`}
              />
            ) : null,
          )}
        </div>
        <div className="mt-1.5 flex justify-between text-[9px] text-muted-foreground">
          <span>{t("goals.repricing.priceSpectrumLow")}</span>
          <span>{t("goals.repricing.priceSpectrumHigh")}</span>
        </div>
        <div className="mt-4 grid grid-cols-2 gap-x-4 gap-y-2">
          {data.map((item) => (
            <div key={item.key} className="flex min-w-0 items-center gap-2">
              <span className="size-2 shrink-0 rounded-full" style={{ background: item.color }} />
              <span className="min-w-0 flex-1 truncate text-[10px] text-muted-foreground">
                {item.label}
              </span>
              <span className="text-[11px] font-semibold tabular-nums">
                {item.count}
                <span className="ml-1 text-[9px] font-normal text-muted-foreground">
                  {segmentPercent(item.count, total)}%
                </span>
              </span>
            </div>
          ))}
        </div>
      </div>
    </Card>
  )
}

function riskConicGradient(data: SegmentChartDatum[], total: number) {
  if (total <= 0) return "var(--muted)"
  let cursor = 0
  const stops = data
    .filter((item) => item.count > 0)
    .map((item) => {
      const start = cursor
      cursor += (item.count / total) * 360
      return `${item.color} ${start}deg ${cursor}deg`
    })
  return `conic-gradient(from -90deg, ${stops.join(", ")})`
}

function RiskOrbitCard({
  title,
  hint,
  data,
}: {
  title: string
  hint: string
  data: SegmentChartDatum[]
}) {
  const { t } = useTranslation()
  const total = segmentTotal(data)
  return (
    <Card className="gap-0 overflow-hidden p-0 shadow-card">
      <SegmentCardHeader title={title} hint={hint} />
      <div className="grid min-h-[172px] grid-cols-[124px_minmax(0,1fr)] items-center gap-3 px-4 py-3">
        <div
          className="relative grid size-[112px] place-items-center rounded-full"
          style={{ background: riskConicGradient(data, total) }}
          role="img"
          aria-label={segmentAriaLabel(data, total)}
        >
          <span className="absolute inset-[12px] rounded-full border bg-card shadow-inner" />
          <div className="relative text-center">
            <p className="display-numeral text-[26px] leading-none">{total}</p>
            <p className="mt-1 text-[9px] text-muted-foreground">
              {t("goals.repricing.riskTotal")}
            </p>
          </div>
        </div>
        <div className="space-y-2.5">
          {data.map((item) => (
            <div key={item.key}>
              <div className="flex items-center justify-between gap-2 text-[10px]">
                <span className="flex min-w-0 items-center gap-2 text-muted-foreground">
                  <span className="size-2 shrink-0 rounded-full" style={{ background: item.color }} />
                  <span className="truncate">{item.label}</span>
                </span>
                <span className="font-semibold tabular-nums">
                  {item.count} · {segmentPercent(item.count, total)}%
                </span>
              </div>
              <div className="mt-1 h-1 overflow-hidden rounded-full bg-muted">
                <div
                  className="h-full rounded-full transition-[width] duration-500"
                  style={{
                    width: `${segmentPercent(item.count, total)}%`,
                    background: item.color,
                  }}
                />
              </div>
            </div>
          ))}
        </div>
      </div>
    </Card>
  )
}

const relationshipMetricColors = {
  customerQuality: "var(--brand)",
  relationship: "var(--success)",
  loyalty: "var(--warning)",
  contact: "var(--chart-2)",
} as const

function relationshipRadarPoints(values: number[], scale = 1) {
  const center = 56
  const radius = 40 * scale
  return values
    .map((value, index) => {
      const angle = -Math.PI / 2 + index * (Math.PI / 2)
      const distance = radius * Math.max(0, Math.min(value, 100)) / 100
      return `${center + Math.cos(angle) * distance},${center + Math.sin(angle) * distance}`
    })
    .join(" ")
}

function RelationshipRadar({ candidate }: { candidate: PricingCandidate }) {
  const { t } = useTranslation()
  const metrics = [
    {
      key: "customerQuality",
      value: candidate.customer_quality_score,
      color: relationshipMetricColors.customerQuality,
    },
    {
      key: "relationship",
      value: candidate.relationship_score,
      color: relationshipMetricColors.relationship,
    },
    {
      key: "loyalty",
      value: candidate.loyalty_score,
      color: relationshipMetricColors.loyalty,
    },
    {
      key: "contact",
      value: candidate.contact_strength_score,
      color: relationshipMetricColors.contact,
    },
  ] as const
  const values = metrics.map((metric) => metric.value)

  return (
    <div className="flex items-center gap-4">
      <svg
        viewBox="0 0 112 112"
        className="size-24 shrink-0 overflow-visible"
        role="img"
        aria-label={t("goals.repricing.profileScoreAria", {
          quality: candidate.customer_quality_score,
          relationship: candidate.relationship_score,
          loyalty: candidate.loyalty_score,
          contact: candidate.contact_strength_score,
        })}
      >
        {[0.25, 0.5, 0.75, 1].map((scale) => (
          <polygon
            key={scale}
            points={relationshipRadarPoints([100, 100, 100, 100], scale)}
            fill="none"
            stroke="currentColor"
            strokeWidth={scale === 1 ? 1 : 0.7}
            className="text-border"
          />
        ))}
        <line x1="56" y1="16" x2="56" y2="96" className="stroke-border" strokeWidth="0.7" />
        <line x1="16" y1="56" x2="96" y2="56" className="stroke-border" strokeWidth="0.7" />
        <polygon
          points={relationshipRadarPoints(values)}
          fill="color-mix(in oklab, var(--brand) 22%, transparent)"
          stroke="var(--brand)"
          strokeWidth="2"
          strokeLinejoin="round"
        />
        {values.map((value, index) => {
          const angle = -Math.PI / 2 + index * (Math.PI / 2)
          const distance = 40 * Math.max(0, Math.min(value, 100)) / 100
          return (
            <circle
              key={metrics[index].key}
              cx={56 + Math.cos(angle) * distance}
              cy={56 + Math.sin(angle) * distance}
              r="2.75"
              fill={metrics[index].color}
              stroke="var(--card)"
              strokeWidth="1.5"
            />
          )
        })}
      </svg>

      <div className="grid min-w-0 flex-1 grid-cols-2 gap-x-4 gap-y-2">
        {metrics.map((metric) => (
          <div key={metric.key} className="min-w-0">
            <div className="flex items-center justify-between gap-2 text-[10px] text-muted-foreground">
              <span className="flex min-w-0 items-center gap-1.5">
                <span className="size-1.5 shrink-0 rounded-full" style={{ background: metric.color }} />
                <span className="truncate">{t(`goals.repricing.score.${metric.key}`)}</span>
              </span>
              <span className="font-semibold tabular-nums text-foreground">{metric.value}</span>
            </div>
            <div className="mt-1 h-1 overflow-hidden rounded-full bg-muted">
              <div
                className="h-full rounded-full"
                style={{ width: `${metric.value}%`, background: metric.color }}
              />
            </div>
          </div>
        ))}
      </div>
    </div>
  )
}

function RepricingAnalysisPanel({
  data,
  amountsHidden,
  onOpenBulkPricing,
}: {
  data: GoalCenter
  amountsHidden: boolean
  onOpenBulkPricing: () => void
}) {
  const { t } = useTranslation()
  const analysis = data.repricing_analysis
  const candidates = React.useMemo(() => data.pricing_candidates ?? [], [data.pricing_candidates])
  const [activeTier, setActiveTier] = React.useState<CustomerTierKey | "all">("all")
  const [profilePage, setProfilePage] = React.useState(1)
  const [statDetail, setStatDetail] = React.useState<StatDetailState | null>(null)
  const profilePageSize = 6
  const queue = React.useMemo(() => {
    const grouped = new Map<number, PricingCandidate>()
    for (const candidate of candidates) {
      if (activeTier !== "all" && candidate.customer_tier !== activeTier) continue
      const groupID = candidate.customer_group_id || candidate.subscription_id
      const current = grouped.get(groupID)
      if (!current) {
        grouped.set(groupID, { ...candidate })
        continue
      }
      if (!current.customer_email && candidate.customer_email) {
        current.customer_email = candidate.customer_email
      }
      if (!current.customer_wechat && candidate.customer_wechat) {
        current.customer_wechat = candidate.customer_wechat
      }
    }
    return [...grouped.values()].sort((left, right) => {
      if (left.needs_contact_followup !== right.needs_contact_followup) {
        return left.needs_contact_followup ? -1 : 1
      }
      if (left.relationship_health_score !== right.relationship_health_score) {
        return left.relationship_health_score - right.relationship_health_score
      }
      return right.customer_quality_score - left.customer_quality_score
    })
  }, [activeTier, candidates])
  const profilePageCount = Math.max(1, Math.ceil(queue.length / profilePageSize))
  const currentProfilePage = Math.min(profilePage, profilePageCount)
  const profilePageStart = (currentProfilePage - 1) * profilePageSize
  const visibleQueue = queue.slice(profilePageStart, profilePageStart + profilePageSize)
  const handleTierChange = (tier: CustomerTierKey | "all") => {
    setActiveTier(tier)
    setProfilePage(1)
  }
  if (!analysis) {
    return (
      <Card className="items-center justify-center py-16 text-center">
        <p className="text-sm text-muted-foreground">{t("common.loadFailed")}</p>
      </Card>
    )
  }
  const relationshipData = (analysis.relationship_segments ?? []).map((segment, index) => ({
    ...segment,
    label: t(`goals.repricing.relationship.${segment.key}`),
    color:
      [
        "var(--warning)",
        "color-mix(in oklab, var(--warning) 55%, var(--brand))",
        "var(--brand)",
        "var(--success)",
      ][index] ?? "var(--brand)",
  }))
  const riskData = (analysis.risk_segments ?? []).map((segment, index) => ({
    ...segment,
    label: t(`goals.repricing.risk.${segment.key}`),
    color:
      ["var(--success)", "var(--warning)", "var(--destructive)"][index] ??
      "var(--muted-foreground)",
  }))
  const priceData = (analysis.price_segments ?? []).map((segment, index) => ({
    ...segment,
    label: t(`goals.repricing.priceBand.${segment.key}`),
    color:
      [
        "var(--destructive)",
        "var(--warning)",
        "var(--success)",
        "var(--brand)",
        "var(--muted-foreground)",
      ][index] ?? "var(--brand)",
  }))
  const protectedUsers =
    (analysis.risk_segments ?? []).find((segment) => segment.key === "high")?.count ?? 0

  const openAnalysisDetail = (key: RepricingDetailKey) => {
    let source = candidates
    if (key === "users") {
      const grouped = new Map<number, PricingCandidate>()
      for (const candidate of candidates) {
        const groupID = candidate.customer_group_id || candidate.subscription_id
        if (!grouped.has(groupID)) grouped.set(groupID, candidate)
      }
      source = [...grouped.values()]
    }
    if (key === "protected") source = source.filter((candidate) => candidate.adjustment_risk === "high")
    if (key === "batch") source = source.filter((candidate) => candidate.recommended)
    const title = key === "users"
      ? t("goals.repricing.analyzedUsers")
      : key === "relationship"
        ? t("goals.repricing.averageRelationship")
        : key === "protected"
          ? t("goals.repricing.protectedUsers")
          : t("goals.repricing.nextBatch")
    setStatDetail({
      title,
      items: source.map((candidate) => ({
        id: key === "users"
          ? candidate.customer_group_id || candidate.subscription_id
          : candidate.subscription_id,
        title: candidate.customer_email || candidate.customer_wechat || candidate.name,
        subtitle: candidate.customer_wechat || candidate.account_name,
        meta: [candidate.account_name, candidate.seat_name, candidate.next_due_date],
        value: key === "relationship"
          ? t("goals.repricing.daysValue", { count: candidate.relationship_days })
          : visibleYuan(repricingDetailAmountCents(key, candidate), amountsHidden),
        valueTone: key === "protected" ? "warning" : key === "batch" ? "success" : "default",
        searchText: `${candidate.relationship_level} ${candidate.customer_tier}`,
      })),
    })
  }

  return (
    <div className="space-y-4">
      <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
        <RepricingMetric
          icon={<Users className="size-4" />}
          label={t("goals.repricing.analyzedUsers")}
          value={analysis.customer_count ?? analysis.total_count}
          hint={t("goals.repricing.analyzedUsersHint", { seats: analysis.total_count })}
          onClick={() => openAnalysisDetail("users")}
        />
        <RepricingMetric
          icon={<CalendarClock className="size-4" />}
          label={t("goals.repricing.averageRelationship")}
          value={t("goals.repricing.daysValue", { count: analysis.average_relationship_days })}
          hint={t("goals.repricing.averageRelationshipHint", {
            periods: (analysis.average_paid_periods ?? 0).toFixed(1),
            repeats: analysis.repeat_subscription_count ?? 0,
          })}
          onClick={() => openAnalysisDetail("relationship")}
        />
        <RepricingMetric
          icon={<ShieldCheck className="size-4" />}
          label={t("goals.repricing.protectedUsers")}
          value={protectedUsers}
          hint={t("goals.repricing.protectedUsersHint", {
            exemptions: analysis.active_exemption_count ?? 0,
          })}
          onClick={() => openAnalysisDetail("protected")}
        />
        <RepricingMetric
          icon={<CheckCircle2 className="size-4" />}
          label={t("goals.repricing.nextBatch")}
          value={analysis.recommended_count}
          hint={t("goals.repricing.nextBatchMetricHint")}
          onClick={() => openAnalysisDetail("batch")}
        />
      </div>

      <CustomerTierPanel
        tiers={analysis.customer_tiers ?? []}
        amountsHidden={amountsHidden}
        activeTier={activeTier}
        onTierChange={handleTierChange}
      />
      <StatDetailDialog
        open={statDetail !== null}
        onOpenChange={(open) => {
          if (!open) setStatDetail(null)
        }}
        detail={statDetail}
      />

      <div className="grid items-stretch gap-4 lg:grid-cols-3">
        <RelationshipJourneyCard
          title={t("goals.repricing.relationshipTitle")}
          hint={t("goals.repricing.relationshipHint")}
          data={relationshipData}
        />
        <PriceSpectrumCard
          title={t("goals.repricing.pricePositionTitle")}
          hint={t("goals.repricing.pricePositionHint")}
          data={priceData}
        />
        <RiskOrbitCard
          title={t("goals.repricing.riskTitle")}
          hint={t("goals.repricing.riskHint")}
          data={riskData}
        />
      </div>

      <Card className="gap-0 overflow-hidden p-0 shadow-card">
        <div className="flex flex-col gap-3 border-b px-5 py-4 sm:flex-row sm:items-center sm:justify-between">
          <div>
            <div className="flex flex-wrap items-center gap-2">
              <h2 className="text-sm font-semibold">{t("goals.repricing.userAnalysisTitle")}</h2>
              {activeTier !== "all" ? (
                <Badge
                  variant="outline"
                  className={customerTierVisuals[activeTier].badgeClass}
                >
                  {t(`goals.repricing.customerTier.${activeTier}.label`)}
                </Badge>
              ) : null}
            </div>
          </div>
          <Button variant="outline" size="sm" onClick={onOpenBulkPricing}>
            {t("goals.repricing.manageBatch")}
            <ArrowRight />
          </Button>
        </div>
        {analysis.total_count > 0 ? (
          <div className="flex items-center gap-2 border-b bg-muted/25 px-5 py-2 text-[11px] text-muted-foreground">
            <BrainCircuit className="size-3.5 shrink-0 text-brand" />
            <span>{t("goals.repricing.profileNotice")}</span>
          </div>
        ) : null}
        {visibleQueue.length > 0 ? (
          <div className="divide-y">
            {visibleQueue.map((candidate) => {
              const customerLabel = candidate.customer_email || candidate.name
              const relationshipVariant =
                candidate.relationship_level === "trusted" || candidate.relationship_level === "stable"
                  ? "success"
                  : candidate.relationship_level === "developing"
                    ? "brand"
                    : "warning"
              const confidenceVariant =
                candidate.relationship_profile_confidence === "high"
                  ? "success"
                  : candidate.relationship_profile_confidence === "medium"
                    ? "secondary"
                    : "outline"
              return (
                <div
                  key={candidate.customer_group_id || candidate.subscription_id}
                  className="grid gap-5 px-5 py-4 xl:grid-cols-[minmax(210px,0.8fr)_minmax(350px,1.35fr)_minmax(250px,1fr)] xl:items-center"
                >
                  <div className="min-w-0">
                    <p className="truncate text-sm font-semibold" title={customerLabel}>
                      {customerLabel}
                    </p>
                    <p
                      className="mt-0.5 truncate text-[11px] text-muted-foreground"
                      title={candidate.name || undefined}
                    >
                      {candidate.name !== customerLabel ? candidate.name : candidate.account_name}
                    </p>
                    <div className="mt-2 flex flex-wrap items-center gap-1.5">
                      <Badge
                        variant="outline"
                        className={customerTierVisuals[candidate.customer_tier].badgeClass}
                      >
                        {t(`goals.repricing.customerTier.${candidate.customer_tier}.label`)}
                      </Badge>
                      {candidate.customer_group_size > 1 ? (
                        <Badge variant="brand">
                          {t("goals.repricing.multiSeatCustomer", {
                            count: candidate.customer_group_size,
                          })}
                        </Badge>
                      ) : null}
                    </div>
                    <div className="mt-2.5">
                      {candidate.needs_contact_followup ? (
                        <Badge variant="warning" className="font-normal">
                          <MessageCircle />
                          {t("goals.repricing.contactMissing")}
                        </Badge>
                      ) : (
                        <p className="flex items-center gap-1.5 truncate text-[11px] text-muted-foreground">
                          <MessageCircle className="size-3.5 shrink-0 text-success" />
                          <span className="truncate">{candidate.customer_wechat}</span>
                        </p>
                      )}
                    </div>
                    <div className="mt-3 inline-flex max-w-full items-center gap-2.5 rounded-md border border-brand/15 bg-brand/[0.055] px-2.5 py-2 shadow-[inset_0_1px_0_color-mix(in_oklab,var(--brand)_8%,transparent)]">
                      <span
                        aria-hidden="true"
                        className="grid size-7 shrink-0 place-items-center rounded-sm bg-brand/12 font-mono text-sm font-bold text-brand"
                      >
                        {candidate.customer_group_size > 1 ? "Σ" : "◆"}
                      </span>
                      <div className="min-w-0">
                        <p className="truncate font-mono text-sm font-semibold tabular-nums tracking-tight text-foreground">
                          {visibleCompactYuan(candidate.customer_group_monthly_revenue_cents, amountsHidden)}
                          <span className="ml-1 text-[10px] font-normal text-muted-foreground">
                            {t("goals.repricing.profilePriceSuffix")}
                          </span>
                        </p>
                        <p className="truncate text-[10px] text-muted-foreground">
                          {t(
                            candidate.customer_group_size > 1
                              ? "goals.repricing.profilePriceGroup"
                              : "goals.repricing.profilePriceSingle",
                            { count: candidate.customer_group_size },
                          )}
                        </p>
                      </div>
                    </div>
                  </div>

                  <RelationshipRadar candidate={candidate} />

                  <div className="min-w-0">
                    <div className="flex flex-wrap items-center gap-1.5">
                      <Badge variant={relationshipVariant}>
                        {t(`goals.repricing.relationshipLevel.${candidate.relationship_level}`)}
                      </Badge>
                      <Badge variant={confidenceVariant} className="font-normal">
                        {t(`goals.repricing.profileConfidence.${candidate.relationship_profile_confidence}`)}
                      </Badge>
                      <span className="ml-auto text-xs font-semibold tabular-nums text-brand">
                        {t("goals.repricing.profileOverall", {
                          value: candidate.relationship_health_score,
                        })}
                      </span>
                    </div>

                    <div className="mt-2.5 rounded-md border border-brand/10 bg-brand/[0.045] px-3 py-2.5">
                      <p className="text-[10px] font-medium text-muted-foreground">
                        {t("goals.repricing.primaryTaskTitle")}
                      </p>
                      <p className="mt-0.5 text-xs font-semibold text-foreground">
                        {t(`goals.repricing.primaryTask.${candidate.primary_relationship_task}`)}
                      </p>
                    </div>

                    <div className="mt-2 flex flex-wrap gap-1.5">
                      {(candidate.relationship_signal_codes ?? []).slice(0, 3).map((code) => (
                        <span
                          key={code}
                          className="rounded-sm bg-muted px-1.5 py-0.5 text-[10px] text-muted-foreground"
                        >
                          {t(`goals.repricing.relationshipSignal.${code}`)}
                        </span>
                      ))}
                    </div>
                  </div>
                </div>
              )
            })}
          </div>
        ) : (
          <div className="grid min-h-32 place-items-center p-6 text-center">
            <p className="text-sm text-muted-foreground">{t("goals.repricing.emptyQueue")}</p>
          </div>
        )}
        {queue.length > profilePageSize ? (
          <div className="flex flex-col gap-2 border-t bg-muted/10 px-5 py-3 sm:flex-row sm:items-center sm:justify-between">
            <p className="text-xs tabular-nums text-muted-foreground">
              {t("goals.repricing.profilePageStatus", {
                page: currentProfilePage,
                pageCount: profilePageCount,
              })}
            </p>
            <div className="flex items-center gap-2">
              <Button
                variant="outline"
                size="sm"
                className="min-h-10 flex-1 sm:min-h-8 sm:flex-none"
                disabled={currentProfilePage <= 1}
                onClick={() => setProfilePage(Math.max(1, currentProfilePage - 1))}
              >
                {t("goals.prevPage")}
              </Button>
              <Button
                variant="outline"
                size="sm"
                className="min-h-10 flex-1 sm:min-h-8 sm:flex-none"
                disabled={currentProfilePage >= profilePageCount}
                onClick={() => setProfilePage(Math.min(profilePageCount, currentProfilePage + 1))}
              >
                {t("goals.nextPage")}
              </Button>
            </div>
          </div>
        ) : null}
      </Card>
    </div>
  )
}

type PricingFilter = "recommended" | "below_market" | "scheduled" | "exempted" | "all"
type PricingExemptionReason = BulkPricingExemptionInput["reason_code"]
const pricingPageSize = 8

const marketPositionBadge = {
  below_low: "destructive",
  below_median: "warning",
  market_range: "success",
  above_high: "secondary",
  unavailable: "outline",
} as const

function candidateSearchText(candidate: PricingCandidate) {
  return [
    candidate.name,
    candidate.customer_email,
    candidate.customer_wechat,
    candidate.account_name,
    candidate.seat_name,
  ]
    .join(" ")
    .toLocaleLowerCase()
}

function BulkPricingPanel({
  data,
  amountsHidden,
}: {
  data: GoalCenter
  amountsHidden: boolean
}) {
  const { t } = useTranslation()
  const candidates = React.useMemo(() => data.pricing_candidates ?? [], [data.pricing_candidates])
  const [filter, setFilter] = React.useState<PricingFilter>("recommended")
  const [search, setSearch] = React.useState("")
  const [selected, setSelected] = React.useState<Set<number>>(() => new Set())
  const [nextPrice, setNextPrice] = React.useState("")
  const [pricingDialogOpen, setPricingDialogOpen] = React.useState(false)
  const [pricingConfirming, setPricingConfirming] = React.useState(false)
  const [exemptionOpen, setExemptionOpen] = React.useState(false)
  const [exemptionReason, setExemptionReason] =
    React.useState<PricingExemptionReason>("relationship_investment")
  const [exemptionCycles, setExemptionCycles] = React.useState("1")
  const [exemptionNote, setExemptionNote] = React.useState("")
  const [page, setPage] = React.useState(1)

  const filtered = React.useMemo(() => {
    const keyword = search.trim().toLocaleLowerCase()
    return candidates.filter((candidate) => {
      const matchesFilter =
        filter === "all" ||
        (filter === "recommended" && candidate.recommended) ||
        (filter === "scheduled" && candidate.next_price_cents !== null) ||
        (filter === "exempted" && candidate.blocked_code === "exempted") ||
        (filter === "below_market" &&
          (candidate.market_position === "below_low" || candidate.market_position === "below_median"))
      return matchesFilter && (!keyword || candidateSearchText(candidate).includes(keyword))
    })
  }, [candidates, filter, search])

  const pageCount = Math.max(1, Math.ceil(filtered.length / pricingPageSize))
  const currentPage = Math.min(page, pageCount)
  const pageStart = (currentPage - 1) * pricingPageSize
  const pageCandidates = filtered.slice(pageStart, pageStart + pricingPageSize)

  const visibleEligibleIDs = pageCandidates
    .filter((candidate) => candidate.eligible)
    .map((candidate) => candidate.subscription_id)
  const eligibleCandidateIDs = new Set(
    candidates
      .filter((candidate) => candidate.eligible)
      .map((candidate) => candidate.subscription_id),
  )
  const validSelected = new Set([...selected].filter((id) => eligibleCandidateIDs.has(id)))
  const allVisibleSelected =
    visibleEligibleIDs.length > 0 && visibleEligibleIDs.every((id) => validSelected.has(id))
  const someVisibleSelected = visibleEligibleIDs.some((id) => validSelected.has(id))
  const selectedCandidates = candidates.filter(
    (candidate) => candidate.eligible && selected.has(candidate.subscription_id),
  )
  const canExempt =
    selectedCandidates.length > 0 && selectedCandidates.every((candidate) => candidate.recommended)
  const parsedNextPriceCents = /^\d+(\.\d{1,2})?$/.test(nextPrice.trim())
    ? Math.round(Number(nextPrice) * 100)
    : 0
  const hasNonIncrease = selectedCandidates.some(
    (candidate) => candidate.current_price_cents >= parsedNextPriceCents,
  )
  const hasAboveSafeCap = selectedCandidates.some(
    (candidate) => parsedNextPriceCents > candidate.max_increase_price_cents,
  )
  const sharedSuggestedPrice = selectedCandidates.length
    ? Math.min(...selectedCandidates.map((candidate) => candidate.suggested_price_cents))
    : data.pricing.suggested_high_price_cents
  const highestSelectedCurrentPrice = selectedCandidates.length
    ? Math.max(...selectedCandidates.map((candidate) => candidate.current_price_cents))
    : 0
  const selectedSafeCeiling = selectedCandidates.length
    ? Math.min(...selectedCandidates.map((candidate) => candidate.max_increase_price_cents))
    : 0
  const canUseSharedSuggestion = sharedSuggestedPrice > highestSelectedCurrentPrice
  const canSubmit =
    selectedCandidates.length > 0 &&
    parsedNextPriceCents > 0 &&
    !hasNonIncrease &&
    !hasAboveSafeCap
  const recommendedCount = candidates.filter((candidate) => candidate.recommended).length

  const mutation = useAppMutation(
    () =>
      scheduleGoalBulkNextPrice({
        subscription_ids: selectedCandidates.map((candidate) => candidate.subscription_id),
        next_price_yuan: nextPrice.trim(),
      }),
    {
      onSuccess: () => {
        setPricingDialogOpen(false)
        setPricingConfirming(false)
        setSelected(new Set())
        setNextPrice("")
      },
    },
  )

  const exemptionMutation = useAppMutation(
    () =>
      exemptGoalBulkPricing({
        subscription_ids: selectedCandidates.map((candidate) => candidate.subscription_id),
        review_cycles: Number(exemptionCycles),
        reason_code: exemptionReason,
        note: exemptionNote.trim(),
      }),
    {
      onSuccess: () => {
        setExemptionOpen(false)
        setSelected(new Set())
        setExemptionNote("")
      },
    },
  )

  function toggleCandidate(candidate: PricingCandidate, checked: boolean) {
    if (!candidate.eligible) return
    if (checked && selected.size === 0) {
      if (nextPrice === "" && candidate.suggested_price_cents > candidate.current_price_cents) {
        setNextPrice(inputYuan(candidate.suggested_price_cents))
      }
      setPricingDialogOpen(true)
    }
    setSelected((current) => {
      const next = new Set(current)
      if (checked) next.add(candidate.subscription_id)
      else next.delete(candidate.subscription_id)
      return next
    })
  }

  function toggleVisible(checked: boolean) {
    if (checked && selected.size === 0 && visibleEligibleIDs.length > 0) {
      const visibleCandidates = pageCandidates.filter((candidate) => candidate.eligible)
      const visibleSuggestion = Math.min(
        ...visibleCandidates.map((candidate) => candidate.suggested_price_cents),
      )
      const visibleHighestCurrent = Math.max(
        ...visibleCandidates.map((candidate) => candidate.current_price_cents),
      )
      if (nextPrice === "" && visibleSuggestion > visibleHighestCurrent) {
        setNextPrice(inputYuan(visibleSuggestion))
      }
      setPricingDialogOpen(true)
    }
    setSelected((current) => {
      const next = new Set(current)
      for (const id of visibleEligibleIDs) {
        if (checked) next.add(id)
        else next.delete(id)
      }
      return next
    })
  }

  function selectRecommendations() {
    const recommended = candidates.filter((candidate) => candidate.recommended && candidate.eligible)
    setFilter("recommended")
    setPage(1)
    setSelected(new Set(recommended.map((candidate) => candidate.subscription_id)))
    setPricingDialogOpen(recommended.length > 0)
    const sharedSuggestion = recommended.length
      ? Math.min(...recommended.map((candidate) => candidate.suggested_price_cents))
      : 0
    const highestCurrent = recommended.length
      ? Math.max(...recommended.map((candidate) => candidate.current_price_cents))
      : 0
    if (!nextPrice && sharedSuggestion > highestCurrent) {
      setNextPrice(inputYuan(sharedSuggestion))
    }
  }

  function openPricingSettings() {
    if (nextPrice === "" && canUseSharedSuggestion && sharedSuggestedPrice > 0) {
      setNextPrice(inputYuan(sharedSuggestedPrice))
    }
    setPricingDialogOpen(true)
  }

  return (
    <section className="overflow-hidden rounded-lg border bg-card shadow-card">
      <div className="flex flex-col gap-3 border-b px-5 py-4 lg:flex-row lg:items-center lg:justify-between">
        <div className="flex items-start gap-3">
          <span className="grid size-9 shrink-0 place-items-center rounded-md bg-brand/10 text-brand">
            <SlidersHorizontal className="size-4" />
          </span>
          <div>
            <div className="flex flex-wrap items-center gap-2">
              <h2 className="text-sm font-semibold">{t("goals.bulkPricingTitle")}</h2>
              <Badge variant={recommendedCount > 0 ? "warning" : "secondary"}>
                {t("goals.recommendedCount", { count: recommendedCount })}
              </Badge>
            </div>
          </div>
        </div>
        <Button
          variant="outline"
          size="sm"
          onClick={selectRecommendations}
          disabled={recommendedCount === 0}
        >
          <Users />
          {t("goals.selectRecommended")}
        </Button>
      </div>

      <div className="grid gap-3 border-b bg-muted/20 p-4 lg:grid-cols-[minmax(220px,1fr)_190px_auto] lg:items-center">
        <div className="relative">
          <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            className="pl-9"
            value={search}
            onChange={(event) => {
              setSearch(event.target.value)
              setPage(1)
            }}
            placeholder={t("goals.searchCustomer")}
            aria-label={t("goals.searchCustomer")}
          />
        </div>
        <Select
          value={filter}
          onValueChange={(value) => {
            setFilter(value as PricingFilter)
            setPage(1)
          }}
        >
          <SelectTrigger aria-label={t("goals.priceFilter")}>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="recommended">{t("goals.filter.recommended")}</SelectItem>
            <SelectItem value="below_market">{t("goals.filter.below_market")}</SelectItem>
            <SelectItem value="scheduled">{t("goals.filter.scheduled")}</SelectItem>
            <SelectItem value="exempted">{t("goals.filter.exempted")}</SelectItem>
            <SelectItem value="all">{t("goals.filter.all")}</SelectItem>
          </SelectContent>
        </Select>
        <p className="text-xs tabular-nums text-muted-foreground lg:text-right">
          {t("goals.filterResult", { visible: filtered.length, total: candidates.length })}
        </p>
      </div>

      {filtered.length > 0 ? (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="w-10">
                <Checkbox
                  checked={allVisibleSelected ? true : someVisibleSelected ? "indeterminate" : false}
                  onCheckedChange={(checked) => toggleVisible(checked === true)}
                  aria-label={t("goals.selectVisible")}
                  disabled={visibleEligibleIDs.length === 0}
                />
              </TableHead>
              <TableHead>{t("goals.customer")}</TableHead>
              <TableHead>{t("goals.teamSeat")}</TableHead>
              <TableHead>{t("goals.currentPrice")}</TableHead>
              <TableHead>{t("goals.marketPositionLabel")}</TableHead>
              <TableHead>{t("goals.scheduledPrice")}</TableHead>
              <TableHead>{t("goals.nextRenewal")}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {pageCandidates.map((candidate) => {
              const contact = candidate.customer_wechat || candidate.customer_email || "-"
              return (
                <TableRow
                  key={candidate.subscription_id}
                  data-state={validSelected.has(candidate.subscription_id) ? "selected" : undefined}
                  className={!candidate.eligible ? "opacity-60" : undefined}
                >
                  <TableCell>
                    <Checkbox
                      checked={validSelected.has(candidate.subscription_id)}
                      onCheckedChange={(checked) => toggleCandidate(candidate, checked === true)}
                      aria-label={t("goals.selectCustomer", { name: candidate.name })}
                      disabled={!candidate.eligible}
                    />
                  </TableCell>
                  <TableCell className="min-w-48 whitespace-normal">
                    <div className="font-medium">{candidate.name}</div>
                    <div className="mt-0.5 text-xs text-muted-foreground">{contact}</div>
                    <div className="mt-1 text-[11px] text-muted-foreground tabular-nums">
                      {t("goals.relationshipStatus", {
                        days: candidate.relationship_days,
                        periods: candidate.paid_period_count,
                      })}
                    </div>
                    {!candidate.eligible ? (
                      <div
                        className={cn(
                          "mt-1 text-[11px]",
                          candidate.blocked_code === "exempted"
                            ? "text-success"
                            : "text-destructive",
                        )}
                      >
                        {candidate.blocked_reason}
                      </div>
                    ) : null}
                  </TableCell>
                  <TableCell className="min-w-40 whitespace-normal">
                    <div>{candidate.account_name || "-"}</div>
                    <div className="mt-0.5 text-xs text-muted-foreground">{candidate.seat_name || "-"}</div>
                  </TableCell>
                  <TableCell className="tabular-nums">
                    <div className="font-semibold">
                      {visibleYuan(candidate.current_price_cents, amountsHidden)}
                    </div>
                    {candidate.market_monthly_price_cents > 0 &&
                    candidate.market_monthly_price_cents !== candidate.current_price_cents ? (
                      <div className="mt-0.5 text-[11px] font-normal text-muted-foreground">
                        {t("goals.monthlyEquivalent", {
                          price: visibleYuan(candidate.market_monthly_price_cents, amountsHidden),
                        })}
                      </div>
                    ) : null}
                  </TableCell>
                  <TableCell>
                    <Badge variant={marketPositionBadge[candidate.market_position]}>
                      {t(`goals.marketPosition.${candidate.market_position}`)}
                    </Badge>
                    {candidate.gap_to_market_median_cents > 0 ? (
                      <div className="mt-1 text-[11px] text-muted-foreground tabular-nums">
                        {t("goals.belowMedianBy", {
                          amount: visibleYuan(candidate.gap_to_market_median_cents, amountsHidden),
                        })}
                      </div>
                    ) : null}
                    {candidate.suggested_price_cents > candidate.current_price_cents ? (
                      <div className="mt-1 text-[11px] tabular-nums">
                        <div className="text-brand">
                          {t("goals.suggestedCustomerPrice", {
                            price: visibleYuan(candidate.suggested_price_cents, amountsHidden),
                            cap: visibleYuan(candidate.max_increase_price_cents, amountsHidden),
                          })}
                        </div>
                        {candidate.suggested_monthly_price_cents > 0 &&
                        candidate.suggested_monthly_price_cents !== candidate.suggested_price_cents ? (
                          <div className="mt-0.5 text-muted-foreground">
                            {t("goals.monthlyEquivalent", {
                              price: visibleYuan(candidate.suggested_monthly_price_cents, amountsHidden),
                            })}
                          </div>
                        ) : null}
                      </div>
                    ) : null}
                  </TableCell>
                  <TableCell className="tabular-nums">
                    {candidate.next_price_cents !== null ? (
                      <>
                        <div className="font-medium text-brand">
                          {visibleYuan(candidate.next_price_cents, amountsHidden)}
                        </div>
                        <div className="mt-0.5 text-[11px] text-muted-foreground">
                          {candidate.next_price_effective_date}
                        </div>
                      </>
                    ) : (
                      <span className="text-muted-foreground">-</span>
                    )}
                  </TableCell>
                  <TableCell className="tabular-nums">{candidate.next_due_date || "-"}</TableCell>
                </TableRow>
              )
            })}
          </TableBody>
        </Table>
      ) : (
        <div className="grid min-h-36 place-items-center p-6 text-center">
          <div>
            <Search className="mx-auto size-5 text-muted-foreground" />
            <p className="mt-2 text-sm font-medium">{t("goals.noPricingCandidates")}</p>
          </div>
        </div>
      )}

      {filtered.length > pricingPageSize ? (
        <div className="flex flex-col gap-2 border-t bg-muted/10 px-4 py-2.5 sm:flex-row sm:items-center sm:justify-between">
          <p className="text-xs tabular-nums text-muted-foreground">
            {t("goals.pageStatus", {
              page: currentPage,
              pageCount,
            })}
          </p>
          <div className="flex items-center gap-2">
            <Button
              variant="outline"
              size="sm"
              className="min-h-10 flex-1 sm:min-h-8 sm:flex-none"
              disabled={currentPage <= 1}
              onClick={() => setPage(Math.max(1, currentPage - 1))}
            >
              {t("goals.prevPage")}
            </Button>
            <Button
              variant="outline"
              size="sm"
              className="min-h-10 flex-1 sm:min-h-8 sm:flex-none"
              disabled={currentPage >= pageCount}
              onClick={() => setPage(Math.min(pageCount, currentPage + 1))}
            >
              {t("goals.nextPage")}
            </Button>
          </div>
        </div>
      ) : null}

      <div className="flex flex-col gap-3 border-t bg-muted/20 px-4 py-3 sm:flex-row sm:items-center sm:justify-between">
        <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
          <p className="text-sm font-semibold">
            {t("goals.selectedCount", { count: selectedCandidates.length })}
          </p>
          {selectedCandidates.length > 0 ? (
            <p className="text-xs tabular-nums text-muted-foreground">
              {t("goals.selectedSafeCeiling", {
                price: visibleYuan(selectedSafeCeiling, amountsHidden),
              })}
            </p>
          ) : null}
        </div>
        <Button
          className="min-h-10 sm:min-h-9"
          disabled={selectedCandidates.length === 0}
          onClick={openPricingSettings}
        >
          <SlidersHorizontal />
          {t("goals.openBulkSettings")}
        </Button>
      </div>

      <Dialog
        open={pricingDialogOpen}
        onOpenChange={(open) => {
          setPricingDialogOpen(open)
          if (!open) setPricingConfirming(false)
        }}
      >
        <DialogContent aria-describedby={undefined} className="sm:max-w-xl">
          <DialogHeader>
            <DialogTitle>
              {pricingConfirming ? t("goals.bulkConfirmTitle") : t("goals.bulkDialogTitle")}
            </DialogTitle>
          </DialogHeader>

          {pricingConfirming ? (
            <div className="rounded-lg border bg-muted/25 p-4 text-sm leading-6">
              {t("goals.bulkConfirmDesc", {
                count: selectedCandidates.length,
                price: `¥${nextPrice}`,
              })}
            </div>
          ) : (
            <div className="grid gap-4">
              <div className="rounded-lg border bg-muted/20 p-3">
                <div className="flex items-center justify-between gap-3">
                  <p className="text-sm font-semibold">
                    {t("goals.selectedCount", { count: selectedCandidates.length })}
                  </p>
                  <span className="text-xs tabular-nums text-muted-foreground">
                    {t("goals.selectedSafeCeiling", {
                      price: visibleYuan(selectedSafeCeiling, amountsHidden),
                    })}
                  </span>
                </div>
                <div className="mt-2 flex flex-wrap gap-1.5">
                  {selectedCandidates.slice(0, 5).map((candidate) => (
                    <Badge key={candidate.subscription_id} variant="outline">
                      {candidate.name}
                    </Badge>
                  ))}
                  {selectedCandidates.length > 5 ? (
                    <Badge variant="secondary">
                      {t("goals.moreSelected", { count: selectedCandidates.length - 5 })}
                    </Badge>
                  ) : null}
                </div>
              </div>

              <div className="grid gap-2">
                <Label htmlFor="bulk-next-price">{t("goals.bulkNextPrice")}</Label>
                <div className="flex flex-col gap-2 sm:flex-row">
                  <div className="relative min-w-0 flex-1">
                    <span className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-sm text-muted-foreground">
                      ¥
                    </span>
                    <Input
                      id="bulk-next-price"
                      className="pl-7 tabular-nums"
                      value={nextPrice}
                      onChange={(event) => setNextPrice(event.target.value)}
                      inputMode="decimal"
                      autoComplete="off"
                      placeholder={sharedSuggestedPrice > 0 ? inputYuan(sharedSuggestedPrice) : "0.00"}
                    />
                  </div>
                  <Button
                    type="button"
                    variant="outline"
                    onClick={() => {
                      if (canUseSharedSuggestion) setNextPrice(inputYuan(sharedSuggestedPrice))
                    }}
                    disabled={!canUseSharedSuggestion}
                  >
                    {t("goals.useMedian")}
                  </Button>
                </div>
                {hasNonIncrease ? (
                  <p className="text-xs text-destructive">{t("goals.nonIncreaseWarning")}</p>
                ) : null}
                {hasAboveSafeCap ? (
                  <p className="text-xs text-destructive">{t("goals.aboveSafeCapWarning")}</p>
                ) : null}
                {selectedCandidates.length > 1 && !canUseSharedSuggestion ? (
                  <p className="text-xs text-warning">{t("goals.splitBatchWarning")}</p>
                ) : null}
                {selectedCandidates.length > 0 && !canExempt ? (
                  <p className="text-xs text-warning">{t("goals.exemptionSelectionHint")}</p>
                ) : null}
              </div>
            </div>
          )}

          {pricingConfirming ? (
            <DialogFooter>
              <Button
                variant="outline"
                onClick={() => setPricingConfirming(false)}
                disabled={mutation.isPending}
              >
                {t("goals.backToSettings")}
              </Button>
              <Button disabled={mutation.isPending} onClick={() => mutation.mutate()}>
                <SlidersHorizontal />
                {mutation.isPending ? t("common.saving") : t("goals.scheduleBulkPrice")}
              </Button>
            </DialogFooter>
          ) : (
            <DialogFooter className="sm:justify-between">
              <Button variant="ghost" onClick={() => setPricingDialogOpen(false)}>
                {t("goals.continueSelecting")}
              </Button>
              <div className="flex flex-col-reverse gap-2 sm:flex-row">
                <Button
                  variant="outline"
                  className="border-warning/45 bg-warning/10 text-warning-foreground hover:bg-warning/15 hover:text-warning-foreground dark:text-warning"
                  disabled={!canExempt || exemptionMutation.isPending}
                  onClick={() => {
                    setPricingDialogOpen(false)
                    setExemptionOpen(true)
                  }}
                >
                  <ShieldCheck />
                  {t("goals.exemptCurrentRound")}
                </Button>
                <Button
                  disabled={!canSubmit || mutation.isPending}
                  onClick={() => setPricingConfirming(true)}
                >
                  <SlidersHorizontal />
                  {t("goals.scheduleBulkPrice")}
                </Button>
              </div>
            </DialogFooter>
          )}
        </DialogContent>
      </Dialog>

      <Dialog open={exemptionOpen} onOpenChange={setExemptionOpen}>
        <DialogContent aria-describedby={undefined} className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>{t("goals.exemptionDialog.title")}</DialogTitle>
          </DialogHeader>
          <div className="rounded-md border bg-muted/30 px-3 py-2 text-xs leading-5 text-muted-foreground">
            {t("goals.exemptionDialog.summary", { count: selectedCandidates.length })}
          </div>
          <div className="grid gap-4">
            <div className="grid gap-2">
              <Label htmlFor="pricing-exemption-reason">
                {t("goals.exemptionDialog.reason")}
              </Label>
              <Select
                value={exemptionReason}
                onValueChange={(value) =>
                  setExemptionReason(value as PricingExemptionReason)
                }
              >
                <SelectTrigger id="pricing-exemption-reason">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {(
                    [
                      "relationship_investment",
                      "loyalty_reward",
                      "multi_seat_retention",
                      "price_observation",
                      "manual",
                    ] as PricingExemptionReason[]
                  ).map((reason) => (
                    <SelectItem key={reason} value={reason}>
                      {t(`goals.exemptionDialog.reasons.${reason}`)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="grid gap-2">
              <Label htmlFor="pricing-exemption-cycles">
                {t("goals.exemptionDialog.reviewCycle")}
              </Label>
              <Select value={exemptionCycles} onValueChange={setExemptionCycles}>
                <SelectTrigger id="pricing-exemption-cycles">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {[1, 2, 3].map((cycles) => (
                    <SelectItem key={cycles} value={String(cycles)}>
                      {t(`goals.exemptionDialog.cycles.${cycles}`)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="grid gap-2">
              <Label htmlFor="pricing-exemption-note">
                {t("goals.exemptionDialog.note")}
              </Label>
              <Textarea
                id="pricing-exemption-note"
                value={exemptionNote}
                onChange={(event) => setExemptionNote(event.target.value)}
                maxLength={500}
                placeholder={t("goals.exemptionDialog.notePlaceholder")}
              />
            </div>
          </div>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => setExemptionOpen(false)}
              disabled={exemptionMutation.isPending}
            >
              {t("common.cancel")}
            </Button>
            <Button
              onClick={() => exemptionMutation.mutate()}
              disabled={!canExempt || exemptionMutation.isPending}
            >
              {exemptionMutation.isPending
                ? t("common.saving")
                : t("goals.exemptionDialog.confirm")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </section>
  )
}

const customerCarePageSize = 8

function PredictionReadinessPanel({ data }: { data: GoalCenter }) {
  const { t } = useTranslation()
  const prediction = data.customer_care.prediction
  const models = prediction.models ?? []
  const hasEstimate = prediction.estimated_renewal_percent !== null
  const evidenceData = [
    {
      key: "first_cycle",
      label: t("goals.care.prediction.chart.firstCycle"),
      value: prediction.first_cycle_subscription_count,
      color: "var(--muted-foreground)",
    },
    {
      key: "repeat_seats",
      label: t("goals.care.prediction.chart.repeatSeats"),
      value: prediction.repeat_subscription_count,
      color: "var(--brand)",
    },
    {
      key: "renewals",
      label: t("goals.care.prediction.chart.renewals"),
      value: prediction.renewal_success_count,
      color: "var(--success)",
    },
    {
      key: "churns",
      label: t("goals.care.prediction.chart.churns"),
      value: prediction.churn_outcome_count,
      color: "var(--destructive)",
    },
  ]
  const seatEvidenceData = evidenceData.slice(0, 2)
  const outcomeEvidenceData = evidenceData.slice(2)
  const outcomeCount = outcomeEvidenceData.reduce((total, item) => total + item.value, 0)

  return (
    <section className="overflow-hidden rounded-lg border bg-card shadow-card">
      <div className="grid gap-5 p-5 xl:grid-cols-[minmax(380px,0.8fr)_minmax(0,1.2fr)] xl:items-center">
        <div className="min-w-0">
          <div className="flex items-center gap-2 text-brand">
            <BrainCircuit className="size-4" />
            <p className="text-xs font-semibold uppercase tracking-[0.16em]">
              {t("goals.care.prediction.eyebrow")}
            </p>
          </div>
          <div className="mt-2 flex flex-wrap items-center justify-between gap-2">
            <p className="text-sm font-semibold">{t("goals.care.prediction.chartTitle")}</p>
            {hasEstimate ? (
              <Badge
                variant="success"
                title={t("goals.care.prediction.range", {
                  low: prediction.estimate_low_percent,
                  high: prediction.estimate_high_percent,
                })}
              >
                {t("goals.care.prediction.estimate", {
                  value: prediction.estimated_renewal_percent,
                })}
              </Badge>
            ) : null}
          </div>
          <div
            className="mt-3 grid min-h-[190px] items-center gap-3 sm:grid-cols-[180px_minmax(0,1fr)]"
            role="img"
            aria-label={t("goals.care.prediction.chartAria", {
              first: prediction.first_cycle_subscription_count,
              repeat: prediction.repeat_subscription_count,
              renewals: prediction.renewal_success_count,
              churns: prediction.churn_outcome_count,
            })}
          >
            <div className="relative mx-auto h-[180px] w-[180px]">
              <ResponsiveContainer width="100%" height="100%">
                <PieChart>
                  <Pie
                    data={seatEvidenceData}
                    dataKey="value"
                    nameKey="label"
                    cx="50%"
                    cy="50%"
                    innerRadius={61}
                    outerRadius={79}
                    startAngle={90}
                    endAngle={-270}
                    paddingAngle={2}
                    cornerRadius={6}
                    stroke="transparent"
                  >
                    {seatEvidenceData.map((item) => (
                      <Cell key={item.key} fill={item.color} />
                    ))}
                  </Pie>
                  <Pie
                    data={outcomeEvidenceData}
                    dataKey="value"
                    nameKey="label"
                    cx="50%"
                    cy="50%"
                    innerRadius={38}
                    outerRadius={54}
                    startAngle={90}
                    endAngle={-270}
                    paddingAngle={3}
                    cornerRadius={5}
                    stroke="transparent"
                  >
                    {outcomeEvidenceData.map((item) => (
                      <Cell key={item.key} fill={item.color} />
                    ))}
                  </Pie>
                  <text
                    x="50%"
                    y="46%"
                    textAnchor="middle"
                    dominantBaseline="middle"
                    fill="var(--foreground)"
                    className="text-xl font-bold tabular-nums"
                  >
                    {outcomeCount}
                  </text>
                  <text
                    x="50%"
                    y="59%"
                    textAnchor="middle"
                    dominantBaseline="middle"
                    fill="var(--muted-foreground)"
                    className="text-[9px] font-medium"
                  >
                    {t("goals.care.prediction.chartOutcomeCenter")}
                  </text>
                  <ChartTooltip
                    formatter={(value, name) => [
                      t("goals.care.prediction.chartCount", { count: Number(value ?? 0) }),
                      String(name),
                    ]}
                    contentStyle={{
                      border: "1px solid var(--border)",
                      borderRadius: 8,
                      background: "var(--popover)",
                      color: "var(--popover-foreground)",
                      fontSize: 11,
                      boxShadow: "var(--shadow-card)",
                    }}
                  />
                </PieChart>
              </ResponsiveContainer>
              <div className="pointer-events-none absolute inset-1 rounded-full border border-foreground/[0.04]" />
            </div>

            <div className="grid gap-3">
              {[
                {
                  key: "seat",
                  title: t("goals.care.prediction.chartSeatRing"),
                  items: seatEvidenceData,
                },
                {
                  key: "outcome",
                  title: t("goals.care.prediction.chartOutcomeRing"),
                  items: outcomeEvidenceData,
                },
              ].map((group) => (
                <div key={group.key} className="rounded-md border border-border/60 bg-muted/20 px-3 py-2.5">
                  <p className="mb-2 text-[10px] font-semibold uppercase tracking-[0.12em] text-muted-foreground">
                    {group.title}
                  </p>
                  <div className="grid gap-1.5">
                    {group.items.map((item) => (
                      <div key={item.key} className="flex items-center gap-2 text-xs">
                        <span
                          className="size-2 shrink-0 rounded-full"
                          style={{
                            backgroundColor: item.color,
                            boxShadow: `0 0 0 3px color-mix(in oklab, ${item.color} 10%, transparent)`,
                          }}
                        />
                        <span className="min-w-0 flex-1 truncate text-muted-foreground">
                          {item.label}
                        </span>
                        <span className="font-semibold tabular-nums">{item.value}</span>
                      </div>
                    ))}
                  </div>
                </div>
              ))}
            </div>
          </div>
        </div>

        <div className="grid gap-2 sm:grid-cols-2">
          {models.map((model) => {
            const progress = Math.min(
              100,
              Math.round((model.current_samples / Math.max(model.required_samples, 1)) * 100),
            )
            return (
              <div key={model.key} className="rounded-md border bg-muted/15 p-3">
                <div className="flex items-start justify-between gap-3">
                  <div>
                    <p className="text-xs font-semibold">
                      {t(`goals.care.prediction.model.${model.key}`)}
                    </p>
                    <p className="mt-0.5 text-[11px] text-muted-foreground">
                      {t(`goals.care.prediction.detail.${model.detail_code}`)}
                    </p>
                  </div>
                  <Badge
                    variant={
                      model.status === "ready"
                        ? "success"
                        : model.status === "needs_control"
                          ? "warning"
                          : "secondary"
                    }
                  >
                    {t(`goals.care.prediction.status.${model.status}`)}
                  </Badge>
                </div>
                <div className="mt-3 h-1.5 overflow-hidden rounded-full bg-muted">
                  <div
                    className={cn(
                      "h-full rounded-full",
                      model.status === "ready" ? "bg-success" : "bg-brand",
                    )}
                    style={{ width: `${progress}%` }}
                  />
                </div>
                <p className="mt-1.5 text-right text-[10px] tabular-nums text-muted-foreground">
                  {model.current_samples} / {model.required_samples}
                </p>
              </div>
            )
          })}
        </div>
      </div>
    </section>
  )
}

type CustomerCareFilter =
  | "recommended"
  | "upcoming"
  | "observing"
  | "all"

const careStatusBadge = {
  recommended: "success",
  upcoming: "brand",
  observe: "secondary",
  cooldown: "warning",
  hold: "outline",
  blocked: "destructive",
} as const

function CustomerBenefitHistoryDialog({
  open,
  onOpenChange,
  history,
  amountsHidden,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  history: CustomerBenefitView[]
  amountsHidden: boolean
}) {
  const { t } = useTranslation()

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        aria-describedby={undefined}
        className="flex max-h-[85dvh] flex-col gap-0 overflow-hidden p-0 sm:max-w-3xl"
      >
        <DialogHeader className="border-b bg-muted/20 px-6 py-5">
          <div className="flex items-center gap-3 pr-8">
            <span className="grid size-10 shrink-0 place-items-center rounded-lg border border-brand/15 bg-brand/10 text-brand">
              <History className="size-4.5" />
            </span>
            <div className="min-w-0">
              <DialogTitle>{t("goals.care.historyTitle")}</DialogTitle>
              <p className="mt-1 text-xs text-muted-foreground">
                {t("goals.care.historyCount", { count: history.length })}
              </p>
            </div>
          </div>
        </DialogHeader>

        {history.length > 0 ? (
          <div className="min-h-0 flex-1 divide-y overflow-y-auto">
            {history.map((benefit) => {
              const contact =
                benefit.customer_email_snapshot || benefit.customer_wechat_snapshot || "-"
              return (
                <article
                  key={benefit.id}
                  className="grid gap-3 px-6 py-4 transition-colors hover:bg-muted/20 sm:grid-cols-[minmax(0,1.15fr)_minmax(0,1fr)_auto] sm:items-center"
                >
                  <div className="min-w-0">
                    <p className="truncate text-sm font-semibold" title={contact}>
                      {contact}
                    </p>
                    {benefit.customer_wechat_snapshot &&
                    benefit.customer_wechat_snapshot !== contact ? (
                      <p className="mt-1 truncate text-xs text-muted-foreground">
                        {benefit.customer_wechat_snapshot}
                      </p>
                    ) : null}
                  </div>

                  <div className="min-w-0">
                    <p className="truncate text-sm font-medium" title={benefit.benefit_name}>
                      {benefit.benefit_name}
                    </p>
                    <div className="mt-1.5 flex flex-wrap items-center gap-1.5">
                      <Badge variant="outline" className="font-normal">
                        {t(`goals.care.benefitType.${benefit.benefit_type}`)}
                      </Badge>
                      <span className="text-[11px] tabular-nums text-muted-foreground">
                        {benefit.benefit_date}
                      </span>
                    </div>
                  </div>

                  <div className="flex items-center justify-between gap-3 sm:flex-col sm:items-end">
                    <Badge
                      variant={
                        benefit.outcome === "renewed"
                          ? "success"
                          : benefit.outcome === "not_renewed"
                            ? "destructive"
                            : "secondary"
                      }
                    >
                      {t(`goals.care.outcome.${benefit.outcome}`)}
                    </Badge>
                    <div className="text-right text-[11px] leading-5 text-muted-foreground">
                      <p className="tabular-nums">
                        {t("goals.care.costValue", {
                          value: visibleYuan(benefit.actual_cost_cents, amountsHidden),
                        })}
                      </p>
                      <p className="tabular-nums">
                        {t("goals.care.perceivedValue", {
                          value: visibleYuan(benefit.perceived_value_cents, amountsHidden),
                        })}
                      </p>
                    </div>
                  </div>

                  {benefit.note ? (
                    <p className="rounded-md bg-muted/45 px-3 py-2 text-xs leading-5 whitespace-pre-wrap text-muted-foreground sm:col-span-3">
                      {benefit.note}
                    </p>
                  ) : null}
                </article>
              )
            })}
          </div>
        ) : (
          <div className="grid min-h-64 place-items-center p-8 text-center">
            <div>
              <span className="mx-auto grid size-11 place-items-center rounded-full bg-muted text-muted-foreground">
                <History className="size-5" />
              </span>
              <p className="mt-3 text-sm text-muted-foreground">
                {t("goals.care.historyEmpty")}
              </p>
            </div>
          </div>
        )}
      </DialogContent>
    </Dialog>
  )
}

function CustomerCarePanel({
  data,
  amountsHidden,
}: {
  data: GoalCenter
  amountsHidden: boolean
}) {
  const { t } = useTranslation()
  const care = data.customer_care
  const candidates = React.useMemo(() => care.candidates ?? [], [care.candidates])
  const history = care.history ?? []
  const [search, setSearch] = React.useState("")
  const [filter, setFilter] = React.useState<CustomerCareFilter>("all")
  const [page, setPage] = React.useState(1)
  const [selected, setSelected] = React.useState<Set<number>>(() => new Set())
  const [dialogOpen, setDialogOpen] = React.useState(false)
  const [historyOpen, setHistoryOpen] = React.useState(false)
  const [statDetail, setStatDetail] = React.useState<StatDetailState | null>(null)
  const [benefitType, setBenefitType] = React.useState<CustomerBenefitType>("loyalty_care")
  const [benefitName, setBenefitName] = React.useState("")
  const [actualCost, setActualCost] = React.useState("")
  const [perceivedValue, setPerceivedValue] = React.useState("")
  const [benefitDate, setBenefitDate] = React.useState(shanghaiToday)
  const [note, setNote] = React.useState("")

  const mutation = useAppMutation(
    (input: RecordCustomerBenefitsInput) => recordGoalCustomerBenefits(input),
    {
      onSuccess: () => {
        setDialogOpen(false)
        setSelected(new Set())
      },
    },
  )

  const validSelected = React.useMemo(() => {
    const selectableIDs = new Set(
      candidates.filter((candidate) => candidate.selectable).map((candidate) => candidate.subscription_id),
    )
    return new Set([...selected].filter((id) => selectableIDs.has(id)))
  }, [candidates, selected])
  const selectedCandidates = React.useMemo(
    () => candidates.filter((candidate) => validSelected.has(candidate.subscription_id)),
    [candidates, validSelected],
  )
  const filtered = React.useMemo(() => {
    const term = search.trim().toLowerCase()
    return candidates.filter((candidate) => {
      if (filter === "recommended" && !candidate.recommended) return false
      if (filter === "upcoming" && candidate.status !== "upcoming") return false
      if (
        filter === "observing" &&
        !["observe", "cooldown", "hold", "blocked"].includes(candidate.status)
      ) {
        return false
      }
      if (!term) return true
      return [candidate.display_name, candidate.customer_email, candidate.customer_wechat]
        .join(" ")
        .toLowerCase()
        .includes(term)
    })
  }, [candidates, filter, search])
  const pageCount = Math.max(1, Math.ceil(filtered.length / customerCarePageSize))
  const currentPage = Math.min(page, pageCount)
  const pageCandidates = filtered.slice(
    (currentPage - 1) * customerCarePageSize,
    currentPage * customerCarePageSize,
  )
  const visibleSelectableIDs = pageCandidates
    .filter((candidate) => candidate.selectable)
    .map((candidate) => candidate.subscription_id)
  const allVisibleSelected =
    visibleSelectableIDs.length > 0 &&
    visibleSelectableIDs.every((id) => validSelected.has(id))
  const someVisibleSelected = visibleSelectableIDs.some((id) => validSelected.has(id))

  function toggleCandidate(candidate: CustomerBenefitCandidate, checked: boolean) {
    setSelected((current) => {
      const next = new Set(current)
      if (checked) next.add(candidate.subscription_id)
      else next.delete(candidate.subscription_id)
      return next
    })
  }

  function toggleVisible(checked: boolean) {
    setSelected((current) => {
      const next = new Set(current)
      for (const id of visibleSelectableIDs) {
        if (checked) next.add(id)
        else next.delete(id)
      }
      return next
    })
  }

  function selectRecommended() {
    setSelected(
      new Set(
        candidates
          .filter((candidate) => candidate.recommended && candidate.selectable)
          .map((candidate) => candidate.subscription_id),
      ),
    )
    setFilter("recommended")
    setPage(1)
  }

  function openBenefitDialog() {
    const suggestedTypes = new Set(
      selectedCandidates.map((candidate) => candidate.suggested_benefit_type),
    )
    const suggestedType =
      suggestedTypes.size === 1 ? [...suggestedTypes][0] : "manual"
    setBenefitType(suggestedType)
    setBenefitName(t(`goals.care.defaultBenefitName.${suggestedType}`))
    setActualCost("")
    setPerceivedValue("")
    setBenefitDate(shanghaiToday())
    setNote("")
    setDialogOpen(true)
  }

  const summaryItems = [
    {
      key: "recommended",
      value: care.summary.recommended_count,
      icon: Gift,
    },
    {
      key: "upcoming",
      value: care.summary.upcoming_count,
      icon: Clock3,
    },
    {
      key: "cost",
      value: visibleYuan(care.summary.total_actual_cost_cents, amountsHidden),
      icon: WalletCards,
    },
    {
      key: "observed",
      value: `${care.summary.renewed_after_benefit_count}/${care.summary.evaluated_benefit_count}`,
      icon: HeartHandshake,
    },
  ] as const

  const openCareDetail = (key: typeof summaryItems[number]["key"]) => {
    if (key === "cost" || key === "observed") {
      const source = key === "cost"
        ? history.filter((benefit) => benefit.actual_cost_cents > 0)
        : history.filter((benefit) => benefit.outcome !== "pending")
      setStatDetail({
        title: t(`goals.care.metric.${key}`),
        items: source.map((benefit) => ({
          id: benefit.id,
          title: benefit.customer_email_snapshot || benefit.customer_wechat_snapshot || benefit.benefit_name,
          subtitle: benefit.customer_wechat_snapshot || benefit.benefit_name,
          meta: [benefit.benefit_date, t(`goals.care.benefitType.${benefit.benefit_type}`), benefit.note],
          value: key === "cost"
            ? visibleYuan(benefit.actual_cost_cents, amountsHidden)
            : t(`goals.care.outcome.${benefit.outcome}`),
          valueTone: key === "cost"
            ? "warning"
            : benefit.outcome === "renewed"
              ? "success"
              : "danger",
        })),
      })
      return
    }
    const source = key === "recommended"
      ? candidates.filter((candidate) => candidate.recommended)
      : candidates.filter((candidate) => candidate.status === "upcoming")
    setStatDetail({
      title: t(`goals.care.metric.${key}`),
      items: source.map((candidate) => ({
        id: candidate.subscription_id,
        title: candidate.customer_email || candidate.customer_wechat || candidate.display_name,
        subtitle: candidate.customer_wechat || candidate.display_name,
        meta: [candidate.recommended_date, candidate.next_due_date, t(`goals.care.reason.${candidate.reason_code}`)],
        value: visibleYuan(candidate.current_cycle_value_cents, amountsHidden),
        valueTone: key === "recommended" ? "success" : "warning",
      })),
    })
  }

  return (
    <div className="space-y-3">
      <PredictionReadinessPanel data={data} />

      <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
        {summaryItems.map((item) => (
          <Card key={item.key} className="group relative gap-0 overflow-hidden p-0 transition-[border-color,background-color,box-shadow] hover:border-input hover:bg-accent/25 hover:shadow-lift">
            <button
              type="button"
              onClick={() => openCareDetail(item.key)}
              aria-label={t(`goals.care.metric.${item.key}`)}
              className="w-full p-4 text-left outline-none focus-visible:ring-2 focus-visible:ring-brand/45 focus-visible:ring-inset"
            >
            <div className="flex items-center justify-between text-muted-foreground">
              <span className="text-xs font-medium">{t(`goals.care.metric.${item.key}`)}</span>
              <item.icon className="size-4" />
            </div>
            <p className="text-xl font-semibold tracking-tight tabular-nums">{item.value}</p>
            <p className="text-[11px] text-muted-foreground">
              {t(`goals.care.metricHint.${item.key}`)}
            </p>
            <span className="absolute inset-x-0 bottom-0 h-0.5 origin-left scale-x-0 bg-brand transition-transform duration-300 group-hover:scale-x-100 group-focus-within:scale-x-100" />
            </button>
          </Card>
        ))}
      </div>

      <section className="overflow-hidden rounded-lg border bg-card shadow-card">
          <div className="flex flex-col gap-3 border-b px-5 py-4 sm:flex-row sm:items-center sm:justify-between">
            <div className="flex items-start gap-3">
              <span className="grid size-9 shrink-0 place-items-center rounded-md bg-brand/10 text-brand">
                <HeartHandshake className="size-4" />
              </span>
              <div>
                <div className="flex flex-wrap items-center gap-2">
                  <h2 className="text-sm font-semibold">{t("goals.care.candidateTitle")}</h2>
                  <Badge variant="brand">
                    {t("goals.care.customerCount", { count: care.summary.customer_count })}
                  </Badge>
                </div>
                <p className="mt-0.5 text-xs text-muted-foreground">
                  {t("goals.care.candidateHint")}
                </p>
              </div>
            </div>
            <div className="flex flex-wrap items-center gap-2">
              <Button variant="outline" size="sm" onClick={() => setHistoryOpen(true)}>
                <History />
                {t("goals.care.historyTitle")}
                <span className="rounded-full bg-muted px-1.5 py-0.5 text-[10px] font-semibold tabular-nums text-muted-foreground">
                  {history.length}
                </span>
              </Button>
              <Button
                variant="outline"
                size="sm"
                onClick={selectRecommended}
                disabled={care.summary.recommended_count === 0}
              >
                <Gift />
                {t("goals.care.selectRecommended")}
              </Button>
            </div>
          </div>

          <div className="grid gap-3 border-b bg-muted/20 p-4 lg:grid-cols-[minmax(220px,1fr)_180px]">
            <div className="relative">
              <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                className="pl-9"
                value={search}
                onChange={(event) => {
                  setSearch(event.target.value)
                  setPage(1)
                }}
                placeholder={t("goals.care.search")}
              />
            </div>
            <Select
              value={filter}
              onValueChange={(value) => {
                setFilter(value as CustomerCareFilter)
                setPage(1)
              }}
            >
              <SelectTrigger aria-label={t("goals.care.filterLabel")}>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="recommended">{t("goals.care.filter.recommended")}</SelectItem>
                <SelectItem value="upcoming">{t("goals.care.filter.upcoming")}</SelectItem>
                <SelectItem value="observing">{t("goals.care.filter.observing")}</SelectItem>
                <SelectItem value="all">{t("goals.care.filter.all")}</SelectItem>
              </SelectContent>
            </Select>
          </div>

          {filtered.length > 0 ? (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className="w-10">
                    <Checkbox
                      checked={allVisibleSelected ? true : someVisibleSelected ? "indeterminate" : false}
                      onCheckedChange={(checked) => toggleVisible(checked === true)}
                      disabled={visibleSelectableIDs.length === 0}
                      aria-label={t("goals.selectVisible")}
                    />
                  </TableHead>
                  <TableHead>{t("goals.care.customer")}</TableHead>
                  <TableHead>{t("goals.care.value")}</TableHead>
                  <TableHead>{t("goals.care.evidence")}</TableHead>
                  <TableHead>{t("goals.care.timing")}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {pageCandidates.map((candidate) => (
                  <TableRow
                    key={candidate.subscription_id}
                    data-state={validSelected.has(candidate.subscription_id) ? "selected" : undefined}
                    className={!candidate.selectable ? "opacity-60" : undefined}
                  >
                    <TableCell>
                      <Checkbox
                        checked={validSelected.has(candidate.subscription_id)}
                        onCheckedChange={(checked) => toggleCandidate(candidate, checked === true)}
                        disabled={!candidate.selectable}
                        aria-label={t("goals.selectCustomer", { name: candidate.display_name })}
                      />
                    </TableCell>
                    <TableCell className="min-w-52 whitespace-normal">
                      <p className="font-medium">{candidate.display_name}</p>
                      {candidate.customer_wechat && candidate.customer_wechat !== candidate.display_name ? (
                        <p className="mt-0.5 text-xs text-muted-foreground">
                          {candidate.customer_wechat}
                        </p>
                      ) : null}
                      <div className="mt-1 flex flex-wrap gap-1">
                        <Badge variant="outline">
                          {t(`goals.repricing.customerTier.${candidate.customer_tier}.label`)}
                        </Badge>
                        {candidate.seat_count > 1 ? (
                          <Badge variant="brand">
                            {t("goals.care.seats", { count: candidate.seat_count })}
                          </Badge>
                        ) : null}
                      </div>
                    </TableCell>
                    <TableCell className="tabular-nums">
                      <p className="font-semibold">
                        {visibleYuan(candidate.current_cycle_value_cents, amountsHidden)}
                      </p>
                      <p className="mt-0.5 text-[11px] text-muted-foreground">
                        {t("goals.care.currentCycleValue")}
                      </p>
                    </TableCell>
                    <TableCell className="min-w-32 whitespace-normal">
                      <p className="text-xs">
                        {t("goals.care.renewalEvidence", { count: candidate.renewal_count })}
                      </p>
                      <p className="mt-1 text-[11px] text-muted-foreground">
                        {t("goals.care.relationshipDays", { count: candidate.relationship_days })}
                      </p>
                    </TableCell>
                    <TableCell className="min-w-48 whitespace-normal">
                      <div className="flex flex-wrap items-center gap-1.5">
                        <Badge variant={careStatusBadge[candidate.status]}>
                          {t(`goals.care.status.${candidate.status}`)}
                        </Badge>
                        {candidate.recommended_date ? (
                          <span className="text-xs tabular-nums text-muted-foreground">
                            {candidate.recommended_date}
                          </span>
                        ) : null}
                      </div>
                      <p className="mt-1.5 text-[11px] leading-4 text-muted-foreground">
                        {t(`goals.care.reason.${candidate.reason_code}`)}
                      </p>
                      {candidate.last_benefit_date ? (
                        <p className="mt-1 text-[10px] text-muted-foreground">
                          {t("goals.care.lastBenefit", { date: candidate.last_benefit_date })}
                        </p>
                      ) : null}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          ) : (
            <div className="grid min-h-40 place-items-center p-6 text-center">
              <div>
                <Gift className="mx-auto size-5 text-muted-foreground" />
                <p className="mt-2 text-sm font-medium">{t("goals.care.empty")}</p>
              </div>
            </div>
          )}

          {filtered.length > customerCarePageSize ? (
            <div className="flex items-center justify-between border-t bg-muted/10 px-4 py-2.5">
              <p className="text-xs tabular-nums text-muted-foreground">
                {t("goals.pageStatus", { page: currentPage, pageCount })}
              </p>
              <div className="flex gap-2">
                <Button
                  variant="outline"
                  size="sm"
                  disabled={currentPage <= 1}
                  onClick={() => setPage(Math.max(1, currentPage - 1))}
                >
                  {t("goals.prevPage")}
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  disabled={currentPage >= pageCount}
                  onClick={() => setPage(Math.min(pageCount, currentPage + 1))}
                >
                  {t("goals.nextPage")}
                </Button>
              </div>
            </div>
          ) : null}

          <div className="flex flex-col gap-3 border-t bg-muted/20 px-4 py-3 sm:flex-row sm:items-center sm:justify-between">
            <p className="text-sm font-semibold">
              {t("goals.care.selected", { count: selectedCandidates.length })}
            </p>
            <Button disabled={selectedCandidates.length === 0} onClick={openBenefitDialog}>
              <Gift />
              {t("goals.care.recordDelivered")}
            </Button>
          </div>
      </section>

      <CustomerBenefitHistoryDialog
        open={historyOpen}
        onOpenChange={setHistoryOpen}
        history={history}
        amountsHidden={amountsHidden}
      />

      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent aria-describedby={undefined} className="sm:max-w-xl">
          <DialogHeader>
            <DialogTitle>{t("goals.care.dialog.title")}</DialogTitle>
          </DialogHeader>
          <form
            className="grid gap-4"
            onSubmit={(event) => {
              event.preventDefault()
              mutation.mutate({
                subscription_ids: selectedCandidates.map((candidate) => candidate.subscription_id),
                benefit_type: benefitType,
                benefit_name: benefitName,
                actual_cost_yuan: actualCost,
                perceived_value_yuan: perceivedValue,
                benefit_date: benefitDate,
                note,
              })
            }}
          >
            <div className="rounded-md border bg-muted/25 p-3 text-xs leading-5 text-muted-foreground">
              {t("goals.care.dialog.summary", { count: selectedCandidates.length })}
            </div>
            <div className="grid gap-4 sm:grid-cols-2">
              <div className="grid gap-2">
                <Label htmlFor="benefit-type">{t("goals.care.dialog.type")}</Label>
                <Select
                  value={benefitType}
                  onValueChange={(value) => {
                    const nextType = value as CustomerBenefitType
                    setBenefitType(nextType)
                    setBenefitName(t(`goals.care.defaultBenefitName.${nextType}`))
                  }}
                >
                  <SelectTrigger id="benefit-type">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {(
                      [
                        "renewal_milestone",
                        "loyalty_care",
                        "price_increase_thanks",
                        "service_recovery",
                        "manual",
                      ] as CustomerBenefitType[]
                    ).map((type) => (
                      <SelectItem key={type} value={type}>
                        {t(`goals.care.benefitType.${type}`)}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div className="grid gap-2">
                <Label htmlFor="benefit-date">{t("goals.care.dialog.date")}</Label>
                <Input
                  id="benefit-date"
                  type="date"
                  max={shanghaiToday()}
                  value={benefitDate}
                  onChange={(event) => setBenefitDate(event.target.value)}
                  required
                />
              </div>
            </div>
            <div className="grid gap-2">
              <Label htmlFor="benefit-name">{t("goals.care.dialog.name")}</Label>
              <Input
                id="benefit-name"
                value={benefitName}
                onChange={(event) => setBenefitName(event.target.value)}
                placeholder={t("goals.care.dialog.namePlaceholder")}
                required
              />
            </div>
            <div className="grid gap-4 sm:grid-cols-2">
              <div className="grid gap-2">
                <Label htmlFor="benefit-cost">{t("goals.care.dialog.actualCost")}</Label>
                <div className="relative">
                  <span className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-sm text-muted-foreground">¥</span>
                  <Input
                    id="benefit-cost"
                    className="pl-7 tabular-nums"
                    inputMode="decimal"
                    value={actualCost}
                    onChange={(event) => setActualCost(event.target.value)}
                    placeholder="0.00"
                  />
                </div>
                <p className="text-[11px] text-muted-foreground">
                  {t("goals.care.dialog.actualCostHint")}
                </p>
              </div>
              <div className="grid gap-2">
                <Label htmlFor="benefit-value">{t("goals.care.dialog.perceivedValue")}</Label>
                <div className="relative">
                  <span className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-sm text-muted-foreground">¥</span>
                  <Input
                    id="benefit-value"
                    className="pl-7 tabular-nums"
                    inputMode="decimal"
                    value={perceivedValue}
                    onChange={(event) => setPerceivedValue(event.target.value)}
                    placeholder="0.00"
                  />
                </div>
                <p className="text-[11px] text-muted-foreground">
                  {t("goals.care.dialog.perceivedValueHint")}
                </p>
              </div>
            </div>
            <div className="grid gap-2">
              <Label htmlFor="benefit-note">{t("goals.care.dialog.note")}</Label>
              <Textarea
                id="benefit-note"
                value={note}
                onChange={(event) => setNote(event.target.value)}
                placeholder={t("goals.care.dialog.notePlaceholder")}
                rows={3}
              />
            </div>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setDialogOpen(false)}>
                {t("common.cancel")}
              </Button>
              <Button
                type="submit"
                disabled={mutation.isPending || selectedCandidates.length === 0 || !benefitName.trim()}
              >
                <Gift />
                {mutation.isPending ? t("common.saving") : t("goals.care.dialog.confirm")}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
      <StatDetailDialog
        open={statDetail !== null}
        onOpenChange={(open) => {
          if (!open) setStatDetail(null)
        }}
        detail={statDetail}
      />
    </div>
  )
}

function GoalHistory({ data, amountsHidden }: { data: GoalCenter; amountsHidden: boolean }) {
  const { t } = useTranslation()
  const history = data.history ?? []
  if (history.length === 0) return null
  return (
    <section className="h-full overflow-hidden rounded-lg border bg-card shadow-card">
      <div className="border-b px-4 py-3.5">
        <h2 className="text-sm font-semibold">{t("goals.historyTitle")}</h2>
      </div>
      <div className="max-h-[240px] divide-y overflow-y-auto">
        {history.map((item) => (
          <div key={item.goal.id} className="grid grid-cols-[minmax(0,1fr)_auto] items-center gap-x-3 gap-y-1 px-4 py-2.5">
            <div className="col-span-2 min-w-0">
              <p className="truncate text-sm font-medium">{item.goal.name}</p>
              <p className="mt-0.5 text-[11px] text-muted-foreground">
                {t("goals.completedAt", {
                  date: item.goal.completed_at?.slice(0, 10) || item.goal.updated_at.slice(0, 10),
                })}
              </p>
            </div>
            <span className="text-sm font-semibold tabular-nums">
              {visibleYuan(item.goal.result_profit_cents, amountsHidden)}
            </span>
            <Badge variant={item.reached ? "success" : "secondary"}>{item.progress_percent}%</Badge>
          </div>
        ))}
      </div>
    </section>
  )
}

function EmptyGoal({ onCreate }: { onCreate: () => void }) {
  const { t } = useTranslation()
  return (
    <Card className="min-h-[250px] items-center justify-center gap-0 p-8 text-center animate-fade-up">
      <span className="grid size-12 place-items-center rounded-lg bg-brand/10 text-brand">
        <Target className="size-6" />
      </span>
      <h2 className="mt-4 text-lg font-semibold">{t("goals.emptyTitle")}</h2>
      <p className="mt-1 max-w-md text-sm text-muted-foreground">{t("goals.emptyDesc")}</p>
      <Button className="mt-5" onClick={onCreate}>
        <Plus />
        {t("goals.create")}
      </Button>
    </Card>
  )
}

export function GoalsPage() {
  const { t } = useTranslation()
  const { amountsHidden, toggleAmounts } = useAmountPrivacy()
  const query = useGoals()
  const [dialogOpen, setDialogOpen] = React.useState(false)
  const [completeOpen, setCompleteOpen] = React.useState(false)
  const [workspaceTab, setWorkspaceTab] = React.useState("overview")

  const refreshMutation = useAppMutation(refreshGoalMarket)
  const completeMutation = useAppMutation(
    (goalId: number) => completeBusinessGoal(goalId),
    { onSuccess: () => setCompleteOpen(false) },
  )
  const recommendedPricingCount =
    query.data?.pricing_candidates?.filter((candidate) => candidate.recommended).length ?? 0
  const upcomingPricingCount =
    query.data?.repricing_analysis?.windows
      ?.filter((window) => window.key === "next_30" || window.key === "next_60")
      .reduce((total, window) => total + window.count, recommendedPricingCount) ??
    recommendedPricingCount
  const hasGoalHistory = (query.data?.history?.length ?? 0) > 0
  const recommendedCareCount = query.data?.customer_care?.summary.recommended_count ?? 0

  return (
    <div className="space-y-4 pb-4">
      <PageHeader
        title={t("goals.title")}
        titleAccessory={
          <AmountPrivacyToggle amountsHidden={amountsHidden} onToggle={toggleAmounts} />
        }
        actions={
          query.data?.active_goal ? null : (
            <Button onClick={() => setDialogOpen(true)}>
              <Plus />
              {t("goals.create")}
            </Button>
          )
        }
      />

      {query.isPending ? (
        <div className="grid gap-4">
          <Skeleton className="h-[220px] rounded-lg" />
          <Skeleton className="h-11 w-full rounded-lg sm:w-[520px]" />
          <Skeleton className="h-[280px] rounded-lg" />
        </div>
      ) : query.isError ? (
        <Card className="items-center justify-center gap-3 py-16 text-center">
          <p className="text-sm text-muted-foreground">{t("common.loadFailed")}</p>
          <Button variant="outline" onClick={() => void query.refetch()}>
            {t("common.retry")}
          </Button>
        </Card>
      ) : query.data ? (
        <>
          {query.data.active_goal ? (
            <ActiveGoalPanel
              data={query.data.active_goal}
              forecast={query.data.forecast}
              amountsHidden={amountsHidden}
              onEdit={() => setDialogOpen(true)}
              onComplete={() => setCompleteOpen(true)}
            />
          ) : (
            <EmptyGoal onCreate={() => setDialogOpen(true)} />
          )}

          <Tabs value={workspaceTab} onValueChange={setWorkspaceTab} className="gap-3">
            <TabsList
              className="grid h-11 w-full grid-cols-4 bg-muted/70 p-1 sm:w-fit sm:min-w-[700px]"
              aria-label={t("goals.workspaceNav")}
            >
              <TabsTrigger value="overview" className="h-9 px-3 text-xs sm:px-4 sm:text-sm">
                <CircleGauge />
                {t("goals.overviewTab")}
              </TabsTrigger>
              <TabsTrigger value="analysis" className="h-9 px-2 text-xs sm:px-4 sm:text-sm">
                <TrendingUp />
                {t("goals.repricingAnalysisTab")}
                {upcomingPricingCount > 0 ? (
                  <span className="rounded-full bg-brand/10 px-1.5 py-0.5 text-[10px] font-semibold leading-none text-brand">
                    {upcomingPricingCount}
                  </span>
                ) : null}
              </TabsTrigger>
              <TabsTrigger value="repricing" className="h-9 px-3 text-xs sm:px-4 sm:text-sm">
                <SlidersHorizontal />
                {t("goals.repricingTab")}
                {recommendedPricingCount > 0 ? (
                  <span className="rounded-full bg-warning/15 px-1.5 py-0.5 text-[10px] font-semibold leading-none text-warning-foreground">
                    {recommendedPricingCount}
                  </span>
                ) : null}
              </TabsTrigger>
              <TabsTrigger value="care" className="h-9 px-2 text-xs sm:px-4 sm:text-sm">
                <HeartHandshake />
                {t("goals.careTab")}
                {recommendedCareCount > 0 ? (
                  <span className="rounded-full bg-success/12 px-1.5 py-0.5 text-[10px] font-semibold leading-none text-success">
                    {recommendedCareCount}
                  </span>
                ) : null}
              </TabsTrigger>
            </TabsList>

            <TabsContent
              value="overview"
              forceMount
              className="mt-0 data-[state=inactive]:hidden animate-fade-in"
            >
              <div
                className={cn(
                  "grid items-stretch gap-4 lg:grid-cols-2 xl:grid-cols-[minmax(220px,0.72fr)_minmax(330px,1.15fr)_minmax(290px,0.94fr)]",
                  hasGoalHistory &&
                    "xl:grid-cols-[minmax(220px,0.72fr)_minmax(330px,1.15fr)_minmax(290px,0.94fr)_minmax(210px,0.65fr)]",
                )}
              >
                {query.data.forecast ? (
                  <ForecastPanel data={query.data.forecast} amountsHidden={amountsHidden} />
                ) : (
                  <section className="grid h-full min-h-[260px] place-items-center rounded-lg border bg-card p-6 text-center shadow-card">
                    <div>
                      <CircleGauge className="mx-auto size-6 text-muted-foreground" />
                      <p className="mt-3 text-sm font-medium">{t("goals.noForecast")}</p>
                    </div>
                  </section>
                )}
                <ProfitTrend data={query.data} amountsHidden={amountsHidden} />
                <div className="h-full lg:col-span-2 xl:col-span-1">
                  <MarketPanel
                    data={query.data}
                    amountsHidden={amountsHidden}
                    refreshing={refreshMutation.isPending}
                    onRefresh={() => refreshMutation.mutate()}
                  />
                </div>
                {hasGoalHistory ? (
                  <div className="h-full lg:col-span-2 xl:col-span-1">
                    <GoalHistory data={query.data} amountsHidden={amountsHidden} />
                  </div>
                ) : null}
              </div>
            </TabsContent>

            <TabsContent
              value="analysis"
              forceMount
              className="mt-0 data-[state=inactive]:hidden animate-fade-in"
            >
              <RepricingAnalysisPanel
                data={query.data}
                amountsHidden={amountsHidden}
                onOpenBulkPricing={() => setWorkspaceTab("repricing")}
              />
            </TabsContent>

            <TabsContent
              value="repricing"
              forceMount
              className="mt-0 data-[state=inactive]:hidden animate-fade-in"
            >
              <BulkPricingPanel data={query.data} amountsHidden={amountsHidden} />
            </TabsContent>

            <TabsContent
              value="care"
              forceMount
              className="mt-0 data-[state=inactive]:hidden animate-fade-in"
            >
              <CustomerCarePanel data={query.data} amountsHidden={amountsHidden} />
            </TabsContent>
          </Tabs>
        </>
      ) : null}

      {dialogOpen ? (
        <GoalDialog
          open={dialogOpen}
          onOpenChange={setDialogOpen}
          goal={query.data?.active_goal?.goal ?? null}
        />
      ) : null}
      <ConfirmDialog
        open={completeOpen}
        onOpenChange={setCompleteOpen}
        title={t("goals.completeTitle")}
        description={t("goals.completeDesc")}
        actionLabel={t("goals.complete")}
        pending={completeMutation.isPending}
        onConfirm={() => {
          const goalId = query.data?.active_goal?.goal.id
          if (goalId) completeMutation.mutate(goalId)
        }}
      />
    </div>
  )
}
