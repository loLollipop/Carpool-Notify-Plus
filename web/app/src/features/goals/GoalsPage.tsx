import * as React from "react"
import { useTranslation } from "react-i18next"
import {
  Area,
  AreaChart,
  CartesianGrid,
  ResponsiveContainer,
  Tooltip as ChartTooltip,
  XAxis,
  YAxis,
} from "recharts"
import {
  ArrowUpRight,
  CalendarClock,
  CheckCircle2,
  CircleGauge,
  ExternalLink,
  Pencil,
  Plus,
  RefreshCw,
  Search,
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
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              {t("common.cancel")}
            </Button>
            <Button type="submit" disabled={mutation.isPending}>
              {t("common.save")}
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
    <div className="min-w-0 border-t border-border/70 px-4 py-4 first:border-t-0 sm:border-l sm:border-t-0 sm:first:border-l-0 sm:px-5">
      <div className="flex items-center gap-2 text-xs font-medium text-muted-foreground">
        <Icon className="size-3.5 text-brand" />
        {label}
      </div>
      <div className="mt-2 truncate text-xl font-semibold tabular-nums">{value}</div>
      <p className="mt-1 truncate text-[11px] text-muted-foreground">{detail}</p>
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
      <div className="grid lg:grid-cols-[minmax(0,1.4fr)_minmax(320px,0.6fr)]">
        <div className="p-5 sm:p-6">
          <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
            <div className="min-w-0">
              <div className="flex flex-wrap items-center gap-2">
                <Badge variant={data.reached ? "success" : "brand"}>
                  {data.reached ? <CheckCircle2 /> : <Target />}
                  {data.reached ? t("goals.reached") : t("goals.inProgress")}
                </Badge>
              </div>
              <h2 className="mt-3 truncate text-xl font-semibold sm:text-2xl">{data.goal.name}</h2>
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

          <div className="mt-8 flex items-end justify-between gap-4">
            <div>
              <p className="text-xs font-medium text-muted-foreground">{t("goals.earned")}</p>
              <p className="mt-1 text-3xl font-semibold tabular-nums sm:text-4xl">
                {visibleYuan(data.earned_profit_cents, amountsHidden)}
              </p>
            </div>
            <p className="shrink-0 text-sm font-semibold text-brand tabular-nums">{progress}%</p>
          </div>
          <div className="mt-4 h-2 overflow-hidden rounded-full bg-muted">
            <div
              className="h-full rounded-full bg-brand transition-[width] duration-700"
              style={{ width: `${Math.max(progress, progress > 0 ? 2 : 0)}%` }}
            />
          </div>
          <div className="mt-3 flex flex-col gap-1.5 text-xs text-muted-foreground sm:flex-row sm:items-center sm:justify-between sm:gap-3">
            <span>{t("goals.baselineLocked")}</span>
            <span className="tabular-nums sm:text-right">
              {t("goals.targetValue", {
                value: visibleYuan(data.goal.target_profit_cents, amountsHidden),
              })}
            </span>
          </div>
        </div>

        <div className="grid border-t border-border/70 sm:grid-cols-3 lg:grid-cols-1 lg:border-l lg:border-t-0">
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
        "grid grid-cols-[minmax(0,1fr)_auto] items-center gap-4 border-t px-4 py-3 first:border-t-0",
        emphasis && "bg-brand/[0.055]",
      )}
    >
      <div className="min-w-0">
        <div className="flex items-center gap-2">
          <span className="text-sm font-semibold">{label}</span>
        </div>
        <p className="mt-1 text-xs text-muted-foreground tabular-nums">
          {available
            ? t("goals.monthlyPace", {
                amount: visibleYuan(scenario.monthly_profit_cents, amountsHidden),
                months: scenario.months_needed,
              })
            : t("goals.forecastUnavailable")}
        </p>
      </div>
      <span className="text-sm font-semibold tabular-nums">
        {scenario.projected_date || "-"}
      </span>
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
    <section className="overflow-hidden rounded-lg border bg-card shadow-card">
      <div className="flex items-center justify-between gap-3 border-b px-5 py-4">
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
    <section className="min-h-[310px] rounded-lg border bg-card p-5 shadow-card">
      <div className="flex items-center justify-between gap-3">
        <div>
          <h2 className="text-sm font-semibold">{t("goals.trendTitle")}</h2>
          <p className="mt-1 text-xs text-muted-foreground">{t("goals.trendRange")}</p>
        </div>
        <TrendingUp className="size-5 text-success" />
      </div>
      <div className="mt-5 h-[225px] w-full">
        <ResponsiveContainer width="100%" height="100%">
          <AreaChart data={trend} margin={{ top: 8, right: 8, left: -16, bottom: 0 }}>
            <defs>
              <linearGradient id="goal-profit-fill" x1="0" y1="0" x2="0" y2="1">
                <stop offset="0%" stopColor="var(--success)" stopOpacity={0.28} />
                <stop offset="100%" stopColor="var(--success)" stopOpacity={0.02} />
              </linearGradient>
            </defs>
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
            <ChartTooltip
              cursor={{ stroke: "var(--border)" }}
              formatter={(value) => [
                visibleYuan(Number(value ?? 0), amountsHidden),
                t("goals.monthProfit"),
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
            <Area
              type="monotone"
              dataKey="profit_cents"
              stroke="var(--success)"
              strokeWidth={2}
              fill="url(#goal-profit-fill)"
            />
          </AreaChart>
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

  return (
    <section className="overflow-hidden rounded-lg border bg-card shadow-card">
      <div className="flex flex-col gap-3 border-b px-5 py-4 sm:flex-row sm:items-center sm:justify-between">
        <div className="flex items-center gap-3">
          <span className="grid size-9 place-items-center rounded-md bg-brand/10 text-brand">
            <ArrowUpRight className="size-4" />
          </span>
          <div>
            <div className="flex flex-wrap items-center gap-2">
              <h2 className="text-sm font-semibold">{t("goals.pricingTitle")}</h2>
              <Badge variant={recommendationBadge[pricing.action]}>
                {t(`goals.action.${pricing.action}`)}
              </Badge>
              {market.stale ? <Badge variant="warning">{t("goals.cached")}</Badge> : null}
            </div>
            <p className="mt-1 text-xs text-muted-foreground">
              {snapshot
                ? t("goals.marketUpdated", { count: snapshot.sample_count })
                : t("goals.marketUnavailable")}
            </p>
          </div>
        </div>
        <div className="flex items-center gap-2">
          <Button variant="outline" size="sm" asChild>
            <a href={market.source_url} target="_blank" rel="noreferrer">
              PriceAI
              <ExternalLink />
            </a>
          </Button>
          <Button variant="outline" size="sm" onClick={onRefresh} disabled={refreshing}>
            <RefreshCw className={cn(refreshing && "animate-spin")} />
            {t("goals.refreshMarket")}
          </Button>
        </div>
      </div>

      <div className="grid lg:grid-cols-[minmax(300px,0.75fr)_minmax(0,1.25fr)]">
        <div className="border-b p-5 lg:border-b-0 lg:border-r">
          <div className="flex items-end justify-between gap-4">
            <div>
              <p className="text-xs font-medium text-muted-foreground">{t("goals.seatUtilization")}</p>
              <p className="mt-1 text-3xl font-semibold tabular-nums">{pricing.utilization_percent}%</p>
            </div>
            <p className="text-xs text-muted-foreground tabular-nums">
              {t("goals.seatUsage", {
                used: pricing.seat_used,
                total: pricing.seat_total,
                available: pricing.seat_available,
              })}
            </p>
          </div>
          <div className="mt-4 grid grid-cols-10 gap-1.5" aria-hidden="true">
            {Array.from({ length: 10 }).map((_, index) => (
              <span
                key={index}
                className={cn(
                  "h-2 rounded-sm bg-muted",
                  index < Math.round(pricing.utilization_percent / 10) && "bg-brand",
                )}
              />
            ))}
          </div>
          <div className="mt-5 grid grid-cols-2 gap-4 border-t pt-4">
            <div>
              <p className="text-[11px] text-muted-foreground">{t("goals.internalMedian")}</p>
              <p className="mt-1 text-base font-semibold tabular-nums">
                {visibleYuan(pricing.internal_median_price_cents, amountsHidden)}
              </p>
            </div>
            <div>
              <p className="text-[11px] text-muted-foreground">{t("goals.seatCostFloor")}</p>
              <p className="mt-1 text-base font-semibold tabular-nums">
                {visibleYuan(pricing.seat_cost_floor_cents, amountsHidden)}
              </p>
            </div>
          </div>
        </div>

        <div className="p-5">
          {snapshot ? (
            <>
              <div className="grid grid-cols-3 gap-3">
                {[
                  [t("goals.marketLow"), snapshot.low_price_cents],
                  [t("goals.marketMedian"), snapshot.median_price_cents],
                  [t("goals.marketHigh"), snapshot.high_price_cents],
                ].map(([label, value], index) => (
                  <div key={String(label)} className="min-w-0 border-l pl-3 first:border-l-0 first:pl-0">
                    <p className="truncate text-[11px] text-muted-foreground">{label}</p>
                    <p className={cn("mt-1 truncate text-lg font-semibold tabular-nums", index === 1 && "text-brand")}>
                      {visibleYuan(Number(value), amountsHidden)}
                    </p>
                  </div>
                ))}
              </div>
              <div className="mt-5 rounded-md border border-brand/15 bg-brand/[0.045] p-4">
                <p className="text-sm font-semibold">{t(`goals.actionTitle.${pricing.action}`)}</p>
                <p className="mt-1.5 text-xs leading-6 text-muted-foreground">
                  {(pricing.reason_codes ?? []).map((reason) => t(`goals.reason.${reason}`)).join(" ")}
                </p>
                {pricing.suggested_low_price_cents > 0 ? (
                  <div className="mt-3 flex items-center justify-between gap-3 border-t border-brand/10 pt-3">
                    <span className="text-xs font-medium text-muted-foreground">{t("goals.suggestedRange")}</span>
                    <span className="text-sm font-semibold tabular-nums text-brand">
                      {visibleYuan(pricing.suggested_low_price_cents, amountsHidden)}
                      <span className="mx-1.5 text-muted-foreground">-</span>
                      {visibleYuan(pricing.suggested_high_price_cents, amountsHidden)}
                    </span>
                  </div>
                ) : null}
              </div>
            </>
          ) : (
            <div className="grid min-h-40 place-items-center text-center">
              <div>
                <RefreshCw className="mx-auto size-5 text-muted-foreground" />
                <p className="mt-3 text-sm font-medium">{t("goals.marketUnavailable")}</p>
                {market.warning ? (
                  <p className="mt-1 max-w-md text-xs text-muted-foreground">{market.warning}</p>
                ) : null}
              </div>
            </div>
          )}
        </div>
      </div>
    </section>
  )
}

type PricingFilter = "recommended" | "below_market" | "scheduled" | "all"

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

  const visibleEligibleIDs = filtered
    .filter((candidate) => candidate.eligible)
    .map((candidate) => candidate.subscription_id)
  const allVisibleSelected =
    visibleEligibleIDs.length > 0 && visibleEligibleIDs.every((id) => selected.has(id))
  const someVisibleSelected = visibleEligibleIDs.some((id) => selected.has(id))
  const selectedCandidates = candidates.filter((candidate) => selected.has(candidate.subscription_id))
  const parsedNextPriceCents = /^\d+(\.\d{1,2})?$/.test(nextPrice.trim())
    ? Math.round(Number(nextPrice) * 100)
    : 0
  const hasUnchangedPrice = selectedCandidates.some(
    (candidate) => candidate.current_price_cents === parsedNextPriceCents,
  )
  const canSubmit = selectedCandidates.length > 0 && parsedNextPriceCents > 0 && !hasUnchangedPrice
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
    setSelected(new Set(recommended.map((candidate) => candidate.subscription_id)))
    if (!nextPrice && data.market.snapshot?.median_price_cents) {
      setNextPrice(inputYuan(data.market.snapshot.median_price_cents))
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
            onChange={(event) => setSearch(event.target.value)}
            placeholder={t("goals.searchCustomer")}
            aria-label={t("goals.searchCustomer")}
          />
        </div>
        <Select value={filter} onValueChange={(value) => setFilter(value as PricingFilter)}>
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
            {filtered.map((candidate) => {
              const contact = candidate.customer_wechat || candidate.customer_email || "-"
              return (
                <TableRow
                  key={candidate.subscription_id}
                  data-state={selected.has(candidate.subscription_id) ? "selected" : undefined}
                  className={!candidate.eligible ? "opacity-60" : undefined}
                >
                  <TableCell>
                    <Checkbox
                      checked={selected.has(candidate.subscription_id)}
                      onCheckedChange={(checked) => toggleCandidate(candidate, checked === true)}
                      aria-label={t("goals.selectCustomer", { name: candidate.name })}
                      disabled={!candidate.eligible}
                    />
                  </TableCell>
                  <TableCell className="min-w-48 whitespace-normal">
                    <div className="font-medium">{candidate.name}</div>
                    <div className="mt-0.5 text-xs text-muted-foreground">{contact}</div>
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

      <div className="grid gap-4 border-t bg-muted/25 p-4 lg:grid-cols-[minmax(0,1fr)_minmax(260px,360px)_auto] lg:items-end">
        <div>
          <p className="text-sm font-semibold">
            {t("goals.selectedCount", { count: selectedCandidates.length })}
          </p>
          <p className="mt-1 text-xs leading-5 text-muted-foreground">
            {t("goals.currentPeriodProtected")}
          </p>
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
                placeholder={data.market.snapshot ? inputYuan(data.market.snapshot.median_price_cents) : "0.00"}
              />
            </div>
            <Button
              type="button"
              variant="outline"
              onClick={() => {
                if (data.market.snapshot) setNextPrice(inputYuan(data.market.snapshot.median_price_cents))
              }}
              disabled={!data.market.snapshot}
            >
              {t("goals.useMedian")}
            </Button>
          </div>
          {hasUnchangedPrice ? (
            <p className="text-xs text-destructive">{t("goals.samePriceWarning")}</p>
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
    <section className="overflow-hidden rounded-lg border bg-card shadow-card">
      <div className="border-b px-5 py-4">
        <h2 className="text-sm font-semibold">{t("goals.historyTitle")}</h2>
      </div>
      <div className="divide-y">
        {history.map((item) => (
          <div key={item.goal.id} className="grid gap-3 px-5 py-3 sm:grid-cols-[minmax(0,1fr)_auto_auto] sm:items-center">
            <div className="min-w-0">
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

  const refreshMutation = useAppMutation(refreshGoalMarket)
  const completeMutation = useAppMutation(
    (goalId: number) => completeBusinessGoal(goalId),
    { onSuccess: () => setCompleteOpen(false) },
  )

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
          <Skeleton className="h-[340px] rounded-lg" />
          <div className="grid gap-4 xl:grid-cols-2">
            <Skeleton className="h-[310px] rounded-lg" />
            <Skeleton className="h-[310px] rounded-lg" />
          </div>
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

          <div className="grid gap-4 xl:grid-cols-[minmax(360px,0.8fr)_minmax(0,1.2fr)]">
            {query.data.forecast ? (
              <ForecastPanel data={query.data.forecast} amountsHidden={amountsHidden} />
            ) : (
              <section className="grid min-h-[310px] place-items-center rounded-lg border bg-card p-8 text-center shadow-card">
                <div>
                  <CircleGauge className="mx-auto size-6 text-muted-foreground" />
                  <p className="mt-3 text-sm font-medium">{t("goals.noForecast")}</p>
                </div>
              </section>
            )}
            <ProfitTrend data={query.data} amountsHidden={amountsHidden} />
          </div>

          <MarketPanel
            data={query.data}
            amountsHidden={amountsHidden}
            refreshing={refreshMutation.isPending}
            onRefresh={() => refreshMutation.mutate()}
          />
          <BulkPricingPanel data={query.data} amountsHidden={amountsHidden} />
          <GoalHistory data={query.data} amountsHidden={amountsHidden} />
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
