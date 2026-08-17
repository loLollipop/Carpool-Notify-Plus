import * as React from "react"
import { useTranslation } from "react-i18next"
import {
  Bar,
  BarChart,
  CartesianGrid,
  Cell,
  ComposedChart,
  Line,
  ReferenceLine,
  ResponsiveContainer,
  Tooltip as ChartTooltip,
  XAxis,
  YAxis,
} from "recharts"
import {
  ArrowRight,
  CalendarClock,
  CheckCircle2,
  CircleGauge,
  ExternalLink,
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
  refreshGoalMarket,
  scheduleGoalBulkNextPrice,
  updateBusinessGoal,
} from "@/api/endpoints"
import { useAppMutation } from "@/api/mutations"
import { useGoals } from "@/api/queries"
import type {
  BusinessGoal,
  BusinessGoalInput,
  ForecastScenario,
  GoalCenter,
  PricingCandidate,
} from "@/api/types"
import { AmountPrivacyToggle } from "@/components/amount-privacy-toggle"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { PageHeader } from "@/components/page-header"
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

function inputYuan(cents: number) {
  return (cents / 100).toFixed(2)
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
}: {
  label: string
  value: React.ReactNode
  detail: string
  icon: React.ComponentType<{ className?: string }>
}) {
  return (
    <div className="min-w-0 border-l border-border/70 px-3 py-3.5 first:border-l-0 sm:px-4">
      <div className="flex items-center gap-2 text-xs font-medium text-muted-foreground">
        <Icon className="size-3.5 text-brand" />
        <span className="truncate">{label}</span>
      </div>
      <div className="display-numeral mt-2 truncate text-lg leading-none sm:text-xl">{value}</div>
      <p className="mt-0.5 hidden truncate text-[11px] text-muted-foreground sm:block">{detail}</p>
    </div>
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
          />
          <Metric
            label={t("goals.futureMonthly")}
            value={visibleYuan(forecast?.baseline.monthly_profit_cents ?? 0, amountsHidden)}
            detail={t("goals.activeRecurring", { count: forecast?.active_recurring_count ?? 0 })}
            icon={TrendingUp}
          />
          <Metric
            label={t("goals.estimatedDate")}
            value={forecast?.baseline.projected_date || "-"}
            detail={t("goals.autoCalculated")}
            icon={CalendarClock}
          />
        </div>
      </div>
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
}: {
  icon: React.ReactNode
  label: string
  value: React.ReactNode
  hint: string
}) {
  return (
    <div className="rounded-lg border bg-card p-4 shadow-card">
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
    </div>
  )
}

type SegmentChartDatum = {
  key: string
  label: string
  count: number
  color: string
}

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
  const queue = [...candidates]
    .sort((left, right) => {
      if (left.recommended !== right.recommended) return left.recommended ? -1 : 1
      if (left.readiness_score !== right.readiness_score) {
        return right.readiness_score - left.readiness_score
      }
      return (left.next_review_date || "9999-12-31").localeCompare(right.next_review_date || "9999-12-31")
    })
  const visibleQueue = queue.slice(0, 8)
  const protectedUsers =
    (analysis.risk_segments ?? []).find((segment) => segment.key === "high")?.count ?? 0

  return (
    <div className="space-y-4">
      <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
        <RepricingMetric
          icon={<Users className="size-4" />}
          label={t("goals.repricing.analyzedUsers")}
          value={analysis.total_count}
          hint={t("goals.repricing.analyzedUsersHint")}
        />
        <RepricingMetric
          icon={<CalendarClock className="size-4" />}
          label={t("goals.repricing.averageRelationship")}
          value={t("goals.repricing.daysValue", { count: analysis.average_relationship_days })}
          hint={t("goals.repricing.averageRelationshipHint", {
            periods: (analysis.average_paid_periods ?? 0).toFixed(1),
          })}
        />
        <RepricingMetric
          icon={<ShieldCheck className="size-4" />}
          label={t("goals.repricing.protectedUsers")}
          value={protectedUsers}
          hint={t("goals.repricing.protectedUsersHint")}
        />
        <RepricingMetric
          icon={<CheckCircle2 className="size-4" />}
          label={t("goals.repricing.nextBatch")}
          value={analysis.recommended_count}
          hint={t("goals.repricing.nextBatchMetricHint")}
        />
      </div>

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
            <h2 className="text-sm font-semibold">{t("goals.repricing.userAnalysisTitle")}</h2>
            <p className="mt-1 text-xs text-muted-foreground">{t("goals.repricing.userAnalysisHint")}</p>
          </div>
          <Button variant="outline" size="sm" onClick={onOpenBulkPricing}>
            {t("goals.repricing.manageBatch")}
            <ArrowRight />
          </Button>
        </div>
        {visibleQueue.length > 0 ? (
          <div className="divide-y">
            {visibleQueue.map((candidate) => {
              const targetPrice = candidate.next_price_cents ?? candidate.suggested_price_cents
              const decisionKey = candidate.recommended
                ? "adjust_now"
                : candidate.next_price_cents !== null
                  ? "scheduled"
                  : candidate.blocked_code === "eligible"
                    ? "hold"
                    : candidate.blocked_code
              const riskVariant =
                candidate.adjustment_risk === "low"
                  ? "success"
                  : candidate.adjustment_risk === "medium"
                    ? "warning"
                    : "destructive"
              const score = Math.max(0, Math.min(candidate.readiness_score, 100))
              return (
                <div
                  key={candidate.subscription_id}
                  className="grid gap-3 px-5 py-3 lg:grid-cols-[minmax(0,1.05fr)_minmax(150px,0.8fr)_minmax(160px,0.85fr)_minmax(210px,1.1fr)] lg:items-center"
                >
                  <div className="min-w-0">
                    <p className="truncate text-sm font-medium">{candidate.name}</p>
                    <p
                      className="mt-0.5 truncate text-[11px] text-muted-foreground"
                      title={candidate.customer_email || undefined}
                    >
                      {candidate.customer_email || "-"}
                    </p>
                  </div>
                  <div className="min-w-0">
                    <div className="flex flex-wrap items-center gap-1.5">
                      <Badge variant="outline">
                        {t(`goals.repricing.relationship.${candidate.relationship_stage}`)}
                      </Badge>
                      <Badge variant={riskVariant}>
                        {t(`goals.repricing.risk.${candidate.adjustment_risk}`)}
                      </Badge>
                    </div>
                    <p className="mt-1.5 text-[11px] tabular-nums text-muted-foreground">
                      {t("goals.relationshipStatus", {
                        days: candidate.relationship_days,
                        periods: candidate.paid_period_count,
                      })}
                    </p>
                  </div>
                  <div className="min-w-0">
                    <div className="flex items-center justify-between gap-2 text-xs">
                      <span className="font-medium tabular-nums">
                        {visibleYuan(candidate.current_price_cents, amountsHidden)}
                      </span>
                      <span className="text-muted-foreground">
                        {candidate.price_gap_percent > 0
                          ? t("goals.repricing.belowTypicalPercent", { value: candidate.price_gap_percent })
                          : candidate.price_gap_percent < 0
                            ? t("goals.repricing.aboveTypicalPercent", {
                                value: Math.abs(candidate.price_gap_percent),
                              })
                            : t("goals.repricing.nearTypical")}
                      </span>
                    </div>
                    <div
                      className="mt-2 h-1.5 overflow-hidden rounded-full bg-muted"
                      aria-label={t("goals.repricing.readinessValue", { value: score })}
                    >
                      <div className="h-full rounded-full bg-brand" style={{ width: `${score}%` }} />
                    </div>
                    <p className="mt-1 text-[10px] text-muted-foreground">
                      {t("goals.repricing.readinessScore", { value: score })}
                    </p>
                  </div>
                  <div className="min-w-0">
                    <div className="flex flex-wrap items-center gap-1.5">
                      <Badge
                        variant={
                          candidate.recommended
                            ? "success"
                            : candidate.next_price_cents !== null
                              ? "brand"
                              : "secondary"
                        }
                      >
                        {t(`goals.repricing.decision.${decisionKey}`)}
                      </Badge>
                      {candidate.recommended && targetPrice > candidate.current_price_cents ? (
                        <span className="text-[11px] font-medium tabular-nums text-brand">
                          {visibleYuan(candidate.current_price_cents, amountsHidden)} →{" "}
                          {visibleYuan(targetPrice, amountsHidden)}
                          {candidate.suggested_increase_percent > 0
                            ? ` (+${candidate.suggested_increase_percent}%)`
                            : ""}
                        </span>
                      ) : null}
                    </div>
                    <p className="mt-1.5 line-clamp-2 text-[11px] leading-4 text-muted-foreground">
                      {(candidate.analysis_codes ?? [])
                        .slice(0, 3)
                        .map((code) => t(`goals.repricing.signal.${code}`))
                        .join(" · ")}
                    </p>
                    {candidate.next_review_date ? (
                      <p className="mt-1 text-[10px] tabular-nums text-muted-foreground">
                        {t("goals.repricing.reviewOn", { date: candidate.next_review_date })}
                      </p>
                    ) : null}
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
      </Card>

      <Card className="gap-0 overflow-hidden p-0 shadow-card">
        <div className="border-b px-5 py-4">
          <h2 className="text-sm font-semibold">{t("goals.repricing.principlesTitle")}</h2>
          <p className="mt-1 text-xs text-muted-foreground">{t("goals.repricing.principlesHint")}</p>
        </div>
        <div className="grid divide-y md:grid-cols-3 md:divide-x md:divide-y-0">
          {(["fairness", "gradual", "stability"] as const).map((principle, index) => (
            <article key={principle} className="p-5">
              <p className="font-mono text-[10px] font-semibold text-brand">0{index + 1}</p>
              <h3 className="mt-2 text-sm font-semibold">
                {t(`goals.repricing.principle.${principle}.title`)}
              </h3>
              <p className="mt-2 text-xs leading-5 text-muted-foreground">
                {t(`goals.repricing.principle.${principle}.desc`)}
              </p>
            </article>
          ))}
        </div>
        <div className="border-t bg-muted/20 px-5 py-3 text-[11px] leading-5 text-muted-foreground">
          {t("goals.repricing.scoreMethod")}
        </div>
      </Card>
    </div>
  )
}

type PricingFilter = "recommended" | "below_market" | "scheduled" | "all"
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
  const [confirmOpen, setConfirmOpen] = React.useState(false)
  const [page, setPage] = React.useState(1)

  const filtered = React.useMemo(() => {
    const keyword = search.trim().toLocaleLowerCase()
    return candidates.filter((candidate) => {
      const matchesFilter =
        filter === "all" ||
        (filter === "recommended" && candidate.recommended) ||
        (filter === "scheduled" && candidate.next_price_cents !== null) ||
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
        setConfirmOpen(false)
        setSelected(new Set())
        setNextPrice("")
      },
    },
  )

  function toggleCandidate(candidate: PricingCandidate, checked: boolean) {
    if (!candidate.eligible) return
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
            <p className="mt-1 max-w-2xl text-xs leading-5 text-muted-foreground">
              {t("goals.bulkPricingDesc")}
            </p>
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
                      <div className="mt-1 text-[11px] text-destructive">{candidate.blocked_reason}</div>
                    ) : null}
                  </TableCell>
                  <TableCell className="min-w-40 whitespace-normal">
                    <div>{candidate.account_name || "-"}</div>
                    <div className="mt-0.5 text-xs text-muted-foreground">{candidate.seat_name || "-"}</div>
                  </TableCell>
                  <TableCell className="font-semibold tabular-nums">
                    {visibleYuan(candidate.current_price_cents, amountsHidden)}
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
                      <div className="mt-1 text-[11px] text-brand tabular-nums">
                        {t("goals.suggestedCustomerPrice", {
                          price: visibleYuan(candidate.suggested_price_cents, amountsHidden),
                          cap: visibleYuan(candidate.max_increase_price_cents, amountsHidden),
                        })}
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
              start: pageStart + 1,
              end: Math.min(pageStart + pricingPageSize, filtered.length),
              total: filtered.length,
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

      <div className="grid gap-4 border-t bg-muted/25 p-4 lg:grid-cols-[minmax(0,1fr)_minmax(260px,360px)_auto] lg:items-end">
        <div>
          <p className="text-sm font-semibold">
            {t("goals.selectedCount", { count: selectedCandidates.length })}
          </p>
          <p className="mt-1 text-xs leading-5 text-muted-foreground">
            {t("goals.currentPeriodProtected")}
          </p>
          {selectedCandidates.length > 0 ? (
            <p className="mt-1 text-xs tabular-nums text-muted-foreground">
              {t("goals.selectedSafeCeiling", {
                price: visibleYuan(selectedSafeCeiling, amountsHidden),
              })}
            </p>
          ) : null}
        </div>
        <div className="grid gap-2">
          <Label htmlFor="bulk-next-price">{t("goals.bulkNextPrice")}</Label>
          <div className="flex gap-2">
            <div className="relative min-w-0 flex-1">
              <span className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-sm text-muted-foreground">¥</span>
              <Input
                id="bulk-next-price"
                className="pl-7 tabular-nums"
                value={nextPrice}
                onChange={(event) => setNextPrice(event.target.value)}
                inputMode="decimal"
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
        </div>
        <Button disabled={!canSubmit || mutation.isPending} onClick={() => setConfirmOpen(true)}>
          <SlidersHorizontal />
          {t("goals.scheduleBulkPrice")}
        </Button>
      </div>

      <ConfirmDialog
        open={confirmOpen}
        onOpenChange={setConfirmOpen}
        title={t("goals.bulkConfirmTitle")}
        description={t("goals.bulkConfirmDesc", {
          count: selectedCandidates.length,
          price: `¥${nextPrice}`,
        })}
        actionLabel={t("goals.scheduleBulkPrice")}
        pending={mutation.isPending}
        onConfirm={() => mutation.mutate()}
      />
    </section>
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
              className="grid h-11 w-full grid-cols-3 bg-muted/70 p-1 sm:w-fit sm:min-w-[520px]"
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
