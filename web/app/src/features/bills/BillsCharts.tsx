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
import { ChevronLeft, ChevronRight } from "lucide-react"

import type { BillsSummary } from "@/api/types"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { AMOUNT_MASK, VALUE_MASK, maskAmount, maskValue } from "@/lib/amount-privacy"
import { cn } from "@/lib/utils"

const CHART_COLORS = [
  "var(--chart-1)",
  "var(--chart-2)",
  "var(--chart-3)",
  "var(--chart-4)",
  "var(--chart-5)",
]

const AMOUNT_ITEMS_PER_PAGE = 4
const DONUT_DETAILS_PER_PAGE = 5

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
    <div
      className={cn(
        "pointer-events-none w-max min-w-36 max-w-44 sm:max-w-72",
        "overflow-hidden rounded-lg border bg-popover px-3 py-2 text-xs text-popover-foreground",
        "shadow-lg animate-fade-in",
      )}
    >
      <div className="break-all font-medium leading-4 [overflow-wrap:anywhere]">{item.name}</div>
      {amountsHidden ? (
        <div className="mt-1 text-muted-foreground">{t("privacy.amountHidden")}</div>
      ) : item.grossCents !== undefined ? (
        <div className="mt-1.5 grid gap-1 tabular-nums text-muted-foreground">
          <div className="grid grid-cols-[minmax(0,1fr)_auto] gap-x-4">
            <span>{t("bills.colGross")}</span>
            <span className="text-foreground">{formatCents(item.grossCents)}</span>
          </div>
          <div className="grid grid-cols-[minmax(0,1fr)_auto] gap-x-4">
            <span>{t("bills.colRefund")}</span>
            <span className="text-destructive">
              {(item.refundCents ?? 0) > 0 ? "-" : ""}{formatCents(item.refundCents ?? 0)}
            </span>
          </div>
          <div className="grid grid-cols-[minmax(0,1fr)_auto] gap-x-4">
            <span>{t("bills.colNet")}</span>
            <span className="font-medium text-success">{formatCents(item.cents)}</span>
          </div>
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

function getPositiveAccounts(summary: BillsSummary) {
  return (summary.accounts ?? [])
    .filter((item) => item.amount_cents > 0)
    .map((item) => ({
      key: item.key,
      name: item.account_name,
      cents: item.amount_cents,
      count: item.count,
    }))
    .sort((accountA, accountB) => accountB.cents - accountA.cents)
}

function subscribeToReducedMotion(onChange: () => void) {
  if (typeof window === "undefined" || typeof window.matchMedia !== "function") {
    return () => undefined
  }

  const media = window.matchMedia("(prefers-reduced-motion: reduce)")
  media.addEventListener("change", onChange)
  return () => media.removeEventListener("change", onChange)
}

function getReducedMotionSnapshot() {
  return typeof window !== "undefined"
    && typeof window.matchMedia === "function"
    && window.matchMedia("(prefers-reduced-motion: reduce)").matches
}

export function AmountDistributionCard({
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
        key: `subscription:${bar.subscription_id}`,
        name: bar.customer_email || bar.name,
        cents: bar.amount_cents,
      }))
    }
    return getPositiveAccounts(summary)
  }, [summary, mode])

  const pageCount = Math.max(1, Math.ceil(data.length / AMOUNT_ITEMS_PER_PAGE))
  const safePage = Math.min(page, pageCount)
  const pageStartIndex = (safePage - 1) * AMOUNT_ITEMS_PER_PAGE
  const pagedData = data.slice(pageStartIndex, pageStartIndex + AMOUNT_ITEMS_PER_PAGE)
  const maxCents = Math.max(1, ...data.map((item) => item.cents))

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
          <div className="grid min-h-[164px] content-start gap-3 py-1">
            {pagedData.map((item, index) => {
              const width = Math.max(5, (item.cents / maxCents) * 100)
              return (
                <div key={item.key} className="grid grid-cols-[24px_minmax(0,1fr)] items-center gap-3">
                  <span className="font-mono text-[10px] font-semibold text-muted-foreground/70">
                    {String(pageStartIndex + index + 1).padStart(2, "0")}
                  </span>
                  <div className="min-w-0">
                    <div className="mb-1.5 flex items-center justify-between gap-3 text-xs">
                      <span className="truncate font-medium" title={item.name}>{item.name}</span>
                      <span className="shrink-0 font-semibold tabular-nums">
                        {amountsHidden ? AMOUNT_MASK : formatCents(item.cents)}
                      </span>
                    </div>
                    <div className="h-2 overflow-hidden rounded-full bg-muted">
                      <div
                        className="h-full rounded-full bg-gradient-to-r from-brand/70 to-brand transition-[width] duration-500"
                        style={{ width: `${width}%`, opacity: Math.max(0.55, 1 - index * 0.15) }}
                      />
                    </div>
                  </div>
                </div>
              )
            })}
          </div>

          {pageCount > 1 ? (
            <div className="flex items-center justify-end border-t pt-3 text-xs text-muted-foreground">
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
                  {maskValue(amountsHidden, safePage)} / {maskValue(amountsHidden, pageCount)}
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

function getAccountDonutColor(index: number) {
  const baseColor = CHART_COLORS[index % CHART_COLORS.length]
  const tone = Math.floor(index / CHART_COLORS.length)

  if (tone === 0) return baseColor

  const blendColor = tone % 2 === 0 ? "var(--background)" : "var(--foreground)"
  const baseWeight = Math.max(58, 82 - Math.floor((tone - 1) / 2) * 12)
  return `color-mix(in oklch, ${baseColor} ${baseWeight}%, ${blendColor})`
}

export function AccountDonutCard({
  summary,
  amountsHidden,
  onSelectAccount,
}: {
  summary: BillsSummary
  amountsHidden: boolean
  onSelectAccount: (accountName: string) => void
}) {
  const { t } = useTranslation()
  const [detailsPage, setDetailsPage] = React.useState(1)
  const reduceMotion = React.useSyncExternalStore(
    subscribeToReducedMotion,
    getReducedMotionSnapshot,
    () => false,
  )
  const rankedAccounts = React.useMemo(() => getPositiveAccounts(summary), [summary])

  const totalCents = rankedAccounts.reduce((sum, item) => sum + item.cents, 0)
  const donutRows = rankedAccounts.map((item, index) => ({
    ...item,
    fill: getAccountDonutColor(index),
  }))
  const detailsPageCount = Math.max(1, Math.ceil(donutRows.length / DONUT_DETAILS_PER_PAGE))
  const safeDetailsPage = Math.min(detailsPage, detailsPageCount)
  const visibleAccounts = donutRows.slice(
    (safeDetailsPage - 1) * DONUT_DETAILS_PER_PAGE,
    safeDetailsPage * DONUT_DETAILS_PER_PAGE,
  )
  const donutPaddingAngle = donutRows.length > 16 ? 0.45 : donutRows.length > 8 ? 1 : 2.2

  return (
    <Card
      className={cn(
        "relative gap-4 p-5 animate-fade-up",
        detailsPageCount > 1 && "pb-14",
      )}
      style={{ animationDelay: "180ms" }}
    >
      <div>
        <h2 className="panel-heading text-sm font-semibold">{t("bills.chartAccountTitle")}</h2>
        <p className="mt-1 text-xs text-muted-foreground">{t("bills.chartAccountDesc")}</p>
      </div>

      {rankedAccounts.length === 0 ? (
        <p className="py-10 text-center text-sm text-muted-foreground">
          {t("bills.chartEmptyAccounts")}
        </p>
      ) : (
        <div className="flex flex-col items-stretch gap-4 sm:flex-row sm:items-center sm:gap-5">
          <div className="relative isolate mx-auto h-[170px] w-[170px] shrink-0 sm:mx-0 [&_.recharts-pie-sector]:cursor-pointer">
            <div className="pointer-events-none absolute inset-0 z-0 grid place-items-center">
              <div className="grid gap-0.5 text-center">
                <span className="text-[10px] text-muted-foreground">
                  {t("bills.chartAccountTotal")}
                </span>
                <span className="display-numeral text-sm font-semibold tabular-nums">
                  {maskAmount(amountsHidden, formatCents(totalCents))}
                </span>
              </div>
            </div>
            <PieChart className="relative z-10" width={170} height={170}>
              <RechartsTooltip
                content={<ChartTooltip amountsHidden={amountsHidden} />}
                wrapperStyle={{ zIndex: 30, pointerEvents: "none" }}
              />
              <Pie
                data={donutRows}
                dataKey="cents"
                nameKey="name"
                innerRadius={52}
                outerRadius={80}
                startAngle={90}
                endAngle={-270}
                paddingAngle={donutPaddingAngle}
                strokeWidth={0}
                isAnimationActive={!reduceMotion}
                animationBegin={120}
                animationDuration={1000}
                animationEasing="ease-out"
                onClick={(entry: { name?: string }) => {
                  if (entry?.name) onSelectAccount(entry.name)
                }}
              >
                {donutRows.map((item) => (
                  <Cell key={item.key} fill={item.fill} />
                ))}
              </Pie>
            </PieChart>
          </div>
          <div className="min-h-[170px] min-w-0 flex-1 sm:pt-3">
            <ul className="grid min-h-[136px] content-start gap-2">
              {visibleAccounts.map((item) => {
                const sharePercent = totalCents > 0 ? (item.cents / totalCents) * 100 : 0
                return (
                  <li key={item.key}>
                    <button
                      type="button"
                      className={cn(
                        "-mx-1.5 grid w-[calc(100%+0.75rem)] min-w-0",
                        "grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-2 rounded-md",
                        "px-1.5 py-1 text-left text-xs transition-colors hover:bg-accent",
                      )}
                      onClick={() => onSelectAccount(item.name)}
                    >
                      <i
                        className="size-2.5 shrink-0 rounded-[3px]"
                        style={{ background: item.fill }}
                      />
                      <span className="truncate" title={item.name}>{item.name}</span>
                      <span className="shrink-0 tabular-nums text-muted-foreground">
                        {maskAmount(amountsHidden, formatCents(item.cents))} ·{" "}
                        {t("bills.countSuffix", { count: maskValue(amountsHidden, item.count) })} ·{" "}
                        {amountsHidden ? VALUE_MASK : `${sharePercent.toFixed(1)}%`}
                      </span>
                    </button>
                  </li>
                )
              })}
            </ul>
          </div>
        </div>
      )}
      {detailsPageCount > 1 ? (
        <div className="absolute right-5 bottom-4 flex items-center gap-1.5 text-xs text-muted-foreground">
          <Button
            type="button"
            variant="ghost"
            size="icon-sm"
            aria-label={t("cards.prevPage")}
            disabled={safeDetailsPage <= 1}
            onClick={() => setDetailsPage(safeDetailsPage - 1)}
          >
            <ChevronLeft />
          </Button>
          <span className="min-w-12 text-center tabular-nums" aria-live="polite">
            {safeDetailsPage} / {detailsPageCount}
          </span>
          <Button
            type="button"
            variant="ghost"
            size="icon-sm"
            aria-label={t("cards.nextPage")}
            disabled={safeDetailsPage >= detailsPageCount}
            onClick={() => setDetailsPage(safeDetailsPage + 1)}
          >
            <ChevronRight />
          </Button>
        </div>
      ) : null}
    </Card>
  )
}

export function MonthlyTrendCard({
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
        <p className="mt-1 text-xs text-muted-foreground">{t("bills.chartTrendDesc")}</p>
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
                tickFormatter={(value) => (amountsHidden ? AMOUNT_MASK : formatAxisCents(value))}
                tickMargin={8}
                width={58}
              />
              <RechartsTooltip
                cursor={{ fill: "var(--accent)" }}
                content={<ChartTooltip amountsHidden={amountsHidden} />}
                wrapperStyle={{ zIndex: 30, pointerEvents: "none" }}
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
                    Number(value) === 0
                      ? ""
                      : amountsHidden
                        ? AMOUNT_MASK
                        : formatCents(Number(value)),
                }}
              />
            </BarChart>
          </ResponsiveContainer>
        </div>
      )}
    </Card>
  )
}
