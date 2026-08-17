import * as React from "react"
import { useTranslation } from "react-i18next"
import { Link } from "react-router-dom"
import {
  Area,
  AreaChart,
  CartesianGrid,
  ReferenceLine,
  ResponsiveContainer,
  Tooltip as RechartsTooltip,
  XAxis,
  YAxis,
} from "recharts"
import {
  AlertTriangle,
  ArrowDownRight,
  ArrowUpRight,
  CalendarClock,
  CheckCircle2,
  CreditCard,
  HandCoins,
  Minus,
  Plus,
  Wallet,
} from "lucide-react"

import { useAccounts, useBills, useCalendar, useDashboard, useSubscriptions } from "@/api/queries"
import type {
  AccountView,
  BillsSummary,
  CalendarMonth,
  Dashboard,
  SubscriptionView,
} from "@/api/types"
import { AmountPrivacyToggle } from "@/components/amount-privacy-toggle"
import { NumberTicker } from "@/components/number-ticker"
import { PageHeader } from "@/components/page-header"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { Skeleton } from "@/components/ui/skeleton"
import { cn } from "@/lib/utils"
import { AMOUNT_MASK, VALUE_MASK, maskAmount, maskValue } from "@/lib/amount-privacy"
import { useAmountPrivacy } from "@/hooks/use-amount-privacy"
import { PlusRentalDialog } from "@/features/plus-rentals/PlusRentalDialog"
import { isOneMonthRentalCron } from "@/features/plus-rentals/rental-mode"
import { SubscriptionDialog } from "@/features/subscriptions/SubscriptionDialog"

const CHART_ANIMATION = { animationDuration: 900, animationEasing: "ease-out" } as const

function formatCents(cents: number) {
  return `¥${(cents / 100).toLocaleString("zh-CN", {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  })}`
}

function formatAxisCents(cents: number) {
  const yuan = cents / 100
  const absoluteYuan = Math.abs(yuan)
  if (absoluteYuan >= 10_000) {
    const compact = yuan / 10_000
    return `¥${compact.toLocaleString("zh-CN", {
      minimumFractionDigits: Number.isInteger(compact) ? 0 : 1,
      maximumFractionDigits: 1,
    })}万`
  }
  return `¥${yuan.toLocaleString("zh-CN", { maximumFractionDigits: 0 })}`
}

// ---- Shared bits ---------------------------------------------------------------

function ChartCard({
  title,
  desc,
  action,
  children,
  className,
  delay = 0,
}: {
  title: string
  desc: string
  action?: React.ReactNode
  children: React.ReactNode
  className?: string
  delay?: number
}) {
  return (
    <Card
      className={cn("gap-4 p-5 animate-fade-up", className)}
      style={{ animationDelay: `${delay}ms` }}
    >
      <div className="flex items-start justify-between gap-2">
        <div>
          <h2 className="panel-heading text-sm font-semibold">{title}</h2>
          <p className="mt-0.5 text-xs text-muted-foreground">{desc}</p>
        </div>
        {action}
      </div>
      {children}
    </Card>
  )
}

function ChartEmpty({ text }: { text: string }) {
  return (
    <div className="flex h-40 items-center justify-center text-sm text-muted-foreground">
      {text}
    </div>
  )
}

function ChartTooltip({
  active,
  payload,
  amountsHidden = false,
}: {
  active?: boolean
  amountsHidden?: boolean
  payload?: { payload: {
    name?: string
    label?: string
    cents?: number
    grossCents?: number
    refundCents?: number
    value?: number
    count?: number
  } }[]
}) {
  const { t } = useTranslation()
  if (!active || !payload || payload.length === 0) return null
  const item = payload[0].payload
  return (
    <div className="rounded-lg border bg-popover px-3 py-2 text-xs text-popover-foreground animate-fade-in">
      <div className="font-medium">{item.name ?? item.label}</div>
      {amountsHidden ? (
        <div className="mt-1 text-muted-foreground">{t("privacy.amountHidden")}</div>
      ) : item.grossCents !== undefined ? (
        <div className="mt-1.5 grid gap-1 tabular-nums text-muted-foreground">
          <div>{t("bills.colGross")} <span className="float-right ml-4 text-foreground">{formatCents(item.grossCents)}</span></div>
          <div>{t("bills.colRefund")} <span className="float-right ml-4 text-destructive">-{formatCents(item.refundCents ?? 0)}</span></div>
          <div>{t("bills.colNet")} <span className="float-right ml-4 font-medium text-success">{formatCents(item.cents ?? 0)}</span></div>
        </div>
      ) : (
        <div className="mt-0.5 tabular-nums text-muted-foreground">
          {item.cents !== undefined ? formatCents(item.cents) : item.value}
          {item.count !== undefined ? ` · ${t("bills.countSuffix", { count: item.count })}` : ""}
        </div>
      )}
    </div>
  )
}

// ---- KPI ------------------------------------------------------------------------

type KpiTone = "brand" | "success" | "gold" | "violet"

function KpiCard({
  label,
  value,
  hint,
  icon,
  tone,
  delay,
  amountsHidden,
}: {
  label: string
  value: string | number
  hint: string
  icon: React.ReactNode
  tone: KpiTone
  delay: number
  amountsHidden: boolean
}) {
  const toneClass: Record<KpiTone, string> = {
    brand: "bg-brand/10 text-brand",
    success: "bg-success/10 text-success",
    gold: "bg-gold/12 text-gold",
    violet: "bg-violet/10 text-violet",
  }
  return (
    <Card
      className="group relative min-h-[112px] gap-0 overflow-hidden p-4 transition-[border-color,box-shadow] duration-200 animate-fade-up hover:border-input hover:shadow-lift"
      style={{ animationDelay: `${delay}ms` }}
    >
      <div className="flex min-w-0 items-start gap-3">
        <span
          className={cn(
            "grid size-10 shrink-0 place-items-center rounded-md transition-colors duration-200",
            toneClass[tone],
          )}
        >
          {icon}
        </span>
        <div className="min-w-0 flex-1">
          <div className="truncate text-xs font-medium text-muted-foreground">{label}</div>
          <div className="display-numeral mt-2 text-[27px] leading-none">
            <NumberTicker value={maskAmount(amountsHidden, value)} />
          </div>
          <div className="mt-2 line-clamp-2 text-[11px] leading-4 text-muted-foreground">{hint}</div>
        </div>
      </div>
    </Card>
  )
}

function KpiRow({
  dashboard,
  summary,
  accountCount,
  calendar,
  amountsHidden,
}: {
  dashboard: Dashboard
  summary: BillsSummary | undefined
  accountCount: number
  calendar: CalendarMonth | undefined
  amountsHidden: boolean
}) {
  const { t } = useTranslation()

  return (
    <section aria-label={t("dash.title")} className="mb-4 grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-5">
      <KpiCard
        label={t("dash.kpiRevenue")}
        value={`¥${summary?.total_amount_yuan ?? "0.00"}`}
        hint={t("dash.kpiRevenueHint", {
          count: maskValue(amountsHidden, summary?.bill_count ?? 0),
          refund: amountsHidden ? VALUE_MASK : summary?.total_refund_yuan ?? "0.00",
        })}
        icon={<Wallet className="size-4" />}
        tone="brand"
        delay={0}
        amountsHidden={amountsHidden}
      />
      <KpiCard
        label={t("dash.kpiProfit")}
        value={`¥${dashboard.total_profit_yuan}`}
        hint={t("dash.kpiProfitHint", {
          margin: amountsHidden ? VALUE_MASK : dashboard.profit_margin_percent,
          net: amountsHidden ? VALUE_MASK : dashboard.net_revenue_yuan,
          refund: amountsHidden ? VALUE_MASK : dashboard.total_refund_yuan,
        })}
        icon={<ArrowUpRight className="size-4" />}
        tone="success"
        delay={70}
        amountsHidden={amountsHidden}
      />
      <KpiCard
        label={t("dash.kpiRefund")}
        value={`¥${dashboard.total_refund_yuan}`}
        hint={t("dash.kpiRefundHint", {
          gross: amountsHidden ? VALUE_MASK : dashboard.total_amount_yuan,
        })}
        icon={<HandCoins className="size-4" />}
        tone="violet"
        delay={140}
        amountsHidden={amountsHidden}
      />
      <KpiCard
        label={t("dash.kpiAccountCost")}
        value={`¥${dashboard.total_cost_yuan}`}
        hint={t("dash.kpiAccountCostHint", {
          accounts: maskValue(amountsHidden, accountCount),
          active: maskValue(amountsHidden, dashboard.active_count),
        })}
        icon={<CreditCard className="size-4" />}
        tone="gold"
        delay={210}
        amountsHidden={amountsHidden}
      />
      <KpiCard
        label={t("dash.kpiPendingMonth")}
        value={`¥${calendar?.pending_month_amount_yuan ?? "0.00"}`}
        hint={t("dash.kpiPendingMonthHint", {
          count: maskValue(amountsHidden, calendar?.pending_month_count ?? 0),
        })}
        icon={<CalendarClock className="size-4" />}
        tone="violet"
        delay={280}
        amountsHidden={amountsHidden}
      />
    </section>
  )
}

// ---- Revenue trend --------------------------------------------------------------

function RevenueTrendCard({
  summary,
  amountsHidden,
  className,
  delay,
}: {
  summary: BillsSummary
  amountsHidden: boolean
  className?: string
  delay?: number
}) {
  const { t } = useTranslation()
  const data = (summary.monthly_trend ?? []).map((item) => ({
    label: item.label,
    cents: item.amount_cents,
    grossCents: item.gross_amount_cents,
    refundCents: item.refund_cents,
    count: item.count,
  }))
  const latestCents = data.at(-1)?.cents ?? 0
  const previousCents = data.at(-2)?.cents
  const changePercent = previousCents
    ? ((latestCents - previousCents) / Math.abs(previousCents)) * 100
    : null
  const periodCents = data.reduce((total, item) => total + item.cents, 0)
  const changeTone = changePercent === null || changePercent === 0
    ? "text-muted-foreground"
    : changePercent > 0
      ? "text-success"
      : "text-destructive"
  const changeText = amountsHidden
    ? VALUE_MASK
    : changePercent === null
      ? "—"
      : `${changePercent > 0 ? "+" : ""}${changePercent.toFixed(1)}%`

  return (
    <ChartCard
      title={t("dash.trendTitle")}
      desc={t("dash.trendDesc")}
      className={className}
      delay={delay}
    >
      {summary.bill_count === 0 ? (
        <ChartEmpty text={t("dash.trendEmpty")} />
      ) : (
        <div data-testid="revenue-trend-card" className="flex min-h-[260px] flex-1 flex-col gap-4">
          <div className="grid grid-cols-2 gap-px overflow-hidden rounded-lg border bg-border sm:grid-cols-3">
            <div className="min-w-0 bg-card px-3 py-3">
              <p className="truncate text-[11px] font-medium text-muted-foreground">
                {t("dash.trendCurrent")}
              </p>
              <p className="display-numeral mt-1 truncate text-lg leading-none">
                {amountsHidden ? AMOUNT_MASK : formatCents(latestCents)}
              </p>
            </div>
            <div className="min-w-0 bg-card px-3 py-3">
              <p className="truncate text-[11px] font-medium text-muted-foreground">
                {t("dash.trendChange")}
              </p>
              <div className={cn("mt-1 flex items-center gap-1", changeTone)}>
                {changePercent === null || changePercent === 0 ? (
                  <Minus className="size-3.5 shrink-0" />
                ) : changePercent > 0 ? (
                  <ArrowUpRight className="size-3.5 shrink-0" />
                ) : (
                  <ArrowDownRight className="size-3.5 shrink-0" />
                )}
                <span className="display-numeral truncate text-lg leading-none">{changeText}</span>
              </div>
            </div>
            <div className="col-span-2 min-w-0 bg-card px-3 py-3 sm:col-span-1">
              <p className="truncate text-[11px] font-medium text-muted-foreground">
                {t("dash.trendPeriodTotal")}
              </p>
              <p className="display-numeral mt-1 truncate text-lg leading-none">
                {amountsHidden ? AMOUNT_MASK : formatCents(periodCents)}
              </p>
            </div>
          </div>

          <div className="min-h-[180px] flex-1">
            <ResponsiveContainer width="100%" height="100%">
              <AreaChart data={data} margin={{ top: 12, right: 12, bottom: 0, left: 0 }}>
                <defs>
                  <linearGradient id="dashboard-revenue-fill" x1="0" y1="0" x2="0" y2="1">
                    <stop offset="0%" stopColor="var(--chart-1)" stopOpacity={0.28} />
                    <stop offset="72%" stopColor="var(--chart-1)" stopOpacity={0.07} />
                    <stop offset="100%" stopColor="var(--chart-1)" stopOpacity={0.01} />
                  </linearGradient>
                </defs>
                <CartesianGrid
                  vertical={false}
                  stroke="var(--border)"
                  strokeDasharray="2 7"
                  strokeOpacity={0.82}
                />
                <XAxis
                  dataKey="label"
                  axisLine={false}
                  tickLine={false}
                  tick={{ fontSize: 11, fill: "var(--muted-foreground)" }}
                  dy={6}
                  padding={{ left: 8, right: 8 }}
                />
                <YAxis
                  width={64}
                  domain={["auto", "auto"]}
                  axisLine={false}
                  tickLine={false}
                  tick={{ fontSize: 10, fill: "var(--muted-foreground)" }}
                  tickFormatter={(value) => (amountsHidden ? AMOUNT_MASK : formatAxisCents(value))}
                />
                <RechartsTooltip
                  cursor={{
                    stroke: "var(--chart-1)",
                    strokeDasharray: "3 5",
                    strokeOpacity: 0.28,
                    strokeWidth: 1,
                  }}
                  content={<ChartTooltip amountsHidden={amountsHidden} />}
                />
                <ReferenceLine
                  y={0}
                  stroke="var(--muted-foreground)"
                  strokeOpacity={0.24}
                  strokeWidth={1}
                />
                <Area
                  type="monotone"
                  dataKey="cents"
                  baseValue={0}
                  stroke="var(--chart-1)"
                  strokeWidth={2.5}
                  fill="url(#dashboard-revenue-fill)"
                  dot={{
                    r: 3,
                    fill: "var(--card)",
                    stroke: "var(--chart-1)",
                    strokeWidth: 2,
                  }}
                  activeDot={{
                    r: 5,
                    fill: "var(--chart-1)",
                    stroke: "var(--card)",
                    strokeWidth: 3,
                  }}
                  {...CHART_ANIMATION}
                />
              </AreaChart>
            </ResponsiveContainer>
          </div>
        </div>
      )}
    </ChartCard>
  )
}

// ---- Operations health -----------------------------------------------------------

type HealthTone = "critical" | "warning" | "success"

interface HealthItem {
  key: string
  tone: HealthTone
  title: string
  detail: string
  actionLabel?: string
  to?: string
  count?: number
}

function summarizeNames(names: string[]) {
  return names.slice(0, 3).join(" / ")
}

function buildHealthItems({
  dashboard,
  subscriptions,
  accounts,
  t,
}: {
  dashboard: Dashboard
  subscriptions: SubscriptionView[]
  accounts: AccountView[]
  t: ReturnType<typeof useTranslation>["t"]
}) {
  const items: HealthItem[] = []
  const actionableTeam = subscriptions.filter(
    (view) =>
      view.subscription.business_type !== "plus" && !view.cancellation_pending,
  )
  const overdue = actionableTeam.filter((view) => view.days_remaining < 0)
  const dueSoon = actionableTeam.filter(
    (view) => view.days_remaining >= 0 && view.days_remaining <= 7,
  )
  const actionablePlus = subscriptions.filter(
    (view) =>
      view.subscription.business_type === "plus" &&
      !view.cancellation_pending,
  )
  const plusDue = actionablePlus.filter(
    (view) => !isOneMonthRentalCron(view.subscription.cron_expr) && view.days_remaining <= 7,
  )
  const plusOverdue = plusDue.filter((view) => view.days_remaining < 0)
  const plusOneMonthDue = actionablePlus.filter(
    (view) => isOneMonthRentalCron(view.subscription.cron_expr) && view.days_remaining <= 7,
  )
  const plusOneMonthOverdue = plusOneMonthDue.filter((view) => view.days_remaining < 0)
  const missingCustomerEmail = subscriptions.filter(
    (view) =>
      view.subscription.business_type !== "plus" &&
      !view.cancellation_pending &&
      view.subscription.customer_email.trim() === "",
  )
  const missingReminderOffsets = subscriptions.filter(
    (view) =>
      view.subscription.business_type !== "plus" &&
      !view.cancellation_pending &&
      (view.subscription.notify_offsets?.length ?? 0) === 0,
  )
  const activeTeamAccounts = accounts.filter((view) =>
    (view.seats ?? []).some((seat) => seat.occupied),
  )
  const missingAccountCost = activeTeamAccounts.filter((view) => view.account.cost_cents <= 0)
  const missingOpenedAt = accounts.filter(
    (view) => view.seat_used > 0 && view.account.opened_at.trim() === "",
  )

  if (subscriptions.length === 0) {
    items.push({
      key: "due",
      tone: "warning",
      title: t("dash.health.emptyTitle"),
      detail: t("dash.health.emptyDetail"),
      actionLabel: t("dash.health.goAccounts"),
      to: "/accounts",
    })
  } else if (overdue.length > 0) {
    items.push({
      key: "due",
      tone: "critical",
      title: t("dash.health.overdueTitle", { count: overdue.length }),
      detail: t("dash.health.overdueDetail", {
        names: summarizeNames(overdue.map((view) => view.subscription.name)),
      }),
      actionLabel: t("dash.health.goCards"),
      to: "/users?filter=pending",
      count: overdue.length,
    })
  } else if (dueSoon.length > 0) {
    items.push({
      key: "due",
      tone: "warning",
      title: t("dash.health.dueSoonTitle", { count: dueSoon.length }),
      detail: t("dash.health.dueSoonDetail", {
        names: summarizeNames(dueSoon.map((view) => view.subscription.name)),
      }),
      actionLabel: t("dash.health.goCards"),
      to: "/users?filter=due",
      count: dueSoon.length,
    })
  } else {
    items.push({
      key: "due",
      tone: "success",
      title: t("dash.health.dueHealthyTitle"),
      detail: t("dash.health.dueHealthyDetail"),
    })
  }

  if (plusDue.length > 0) {
    items.push({
      key: "plusDue",
      tone: plusOverdue.length > 0 ? "critical" : "warning",
      title: t(
        plusOverdue.length > 0
          ? "dash.health.plusOverdueTitle"
          : "dash.health.plusDueSoonTitle",
        { count: plusDue.length },
      ),
      detail: t("dash.health.plusDueDetail", {
        names: summarizeNames(plusDue.map((view) => view.subscription.name)),
      }),
      actionLabel: t("dash.health.goPlusRentals"),
      to: "/plus-rentals?filter=due",
      count: plusDue.length,
    })
  } else {
    items.push({
      key: "plusDue",
      tone: "success",
      title: t("dash.health.plusDueHealthyTitle"),
      detail: t("dash.health.plusDueHealthyDetail"),
    })
  }

  if (plusOneMonthDue.length > 0) {
    items.push({
      key: "plusOneMonthDue",
      tone: plusOneMonthOverdue.length > 0 ? "critical" : "warning",
      title: t(
        plusOneMonthOverdue.length > 0
          ? "dash.health.plusOneMonthOverdueTitle"
          : "dash.health.plusOneMonthDueSoonTitle",
        { count: plusOneMonthDue.length },
      ),
      detail: t("dash.health.plusOneMonthDetail", {
        names: summarizeNames(plusOneMonthDue.map((view) => view.subscription.name)),
      }),
      actionLabel: t("dash.health.goPlusRentals"),
      to: "/plus-rentals?filter=due",
      count: plusOneMonthDue.length,
    })
  }

  if (missingCustomerEmail.length > 0) {
    items.push({
      key: "customerEmail",
      tone: "warning",
      title: t("dash.health.customerEmailTitle", { count: missingCustomerEmail.length }),
      detail: t("dash.health.customerEmailDetail", {
        names: summarizeNames(missingCustomerEmail.map((view) => view.subscription.name)),
      }),
      actionLabel: t("dash.health.goCards"),
      to: "/users",
      count: missingCustomerEmail.length,
    })
  } else {
    items.push({
      key: "customerEmail",
      tone: "success",
      title: t("dash.health.customerEmailHealthyTitle"),
      detail: t("dash.health.customerEmailHealthyDetail"),
    })
  }

  if (missingReminderOffsets.length > 0) {
    items.push({
      key: "reminders",
      tone: "warning",
      title: t("dash.health.remindersTitle", { count: missingReminderOffsets.length }),
      detail: t("dash.health.remindersDetail", {
        names: summarizeNames(missingReminderOffsets.map((view) => view.subscription.name)),
      }),
      actionLabel: t("dash.health.goCards"),
      to: "/users",
      count: missingReminderOffsets.length,
    })
  } else {
    items.push({
      key: "reminders",
      tone: "success",
      title: t("dash.health.remindersHealthyTitle"),
      detail: t("dash.health.remindersHealthyDetail"),
    })
  }

  if (missingAccountCost.length > 0) {
    items.push({
      key: "accountCost",
      tone: "warning",
      title: t("dash.health.accountCostTitle", { count: missingAccountCost.length }),
      detail: t("dash.health.accountCostDetail", {
        names: summarizeNames(missingAccountCost.map((view) => view.account.name)),
      }),
      actionLabel: t("dash.health.goAccounts"),
      to: "/accounts",
      count: missingAccountCost.length,
    })
  } else {
    items.push({
      key: "accountCost",
      tone: "success",
      title: t("dash.health.accountCostHealthyTitle"),
      detail: t("dash.health.accountCostHealthyDetail"),
    })
  }

  if (missingOpenedAt.length > 0) {
    items.push({
      key: "openedAt",
      tone: "warning",
      title: t("dash.health.openedAtTitle", { count: missingOpenedAt.length }),
      detail: t("dash.health.openedAtDetail", {
        names: summarizeNames(missingOpenedAt.map((view) => view.account.name)),
      }),
      actionLabel: t("dash.health.goAccounts"),
      to: "/accounts",
      count: missingOpenedAt.length,
    })
  } else {
    items.push({
      key: "openedAt",
      tone: "success",
      title: t("dash.health.openedAtHealthyTitle"),
      detail: t("dash.health.openedAtHealthyDetail"),
    })
  }

  if (dashboard.notify_failed_30d > 0) {
    items.push({
      key: "notify",
      tone: "critical",
      title: t("dash.health.notifyTitle", { count: dashboard.notify_failed_30d }),
      detail: t("dash.health.notifyDetail"),
      actionLabel: t("dash.health.goSettings"),
      to: "/settings",
      count: dashboard.notify_failed_30d,
    })
  } else {
    items.push({
      key: "notify",
      tone: "success",
      title: t("dash.health.notifyHealthyTitle"),
      detail: t("dash.health.notifyHealthyDetail"),
    })
  }

  return items
}

function OperationsHealthCard({
  dashboard,
  subscriptions,
  accounts,
  pending,
  error,
  delay = 0,
}: {
  dashboard: Dashboard
  subscriptions: SubscriptionView[]
  accounts: AccountView[]
  pending: boolean
  error: boolean
  delay?: number
}) {
  const { t } = useTranslation()
  const iconClass: Record<HealthTone, string> = {
    critical: "bg-destructive/10 text-destructive",
    warning: "bg-warning/15 text-warning-foreground dark:text-warning",
    success: "bg-success/12 text-success",
  }
  const toneTextClass: Record<HealthTone, string> = {
    critical: "text-destructive",
    warning: "text-warning-foreground dark:text-warning",
    success: "text-success",
  }

  const items = React.useMemo(
    () =>
      buildHealthItems({
        dashboard,
        subscriptions,
        accounts,
        t,
      }),
    [accounts, dashboard, subscriptions, t],
  )
  const overallTone: HealthTone = items.some((item) => item.tone === "critical")
    ? "critical"
    : items.some((item) => item.tone === "warning")
      ? "warning"
      : "success"
  const attentionItems = items.filter((item) => item.tone !== "success")
  const healthyItems = items.filter((item) => item.tone === "success")
  const healthPercent = Math.round((healthyItems.length / Math.max(items.length, 1)) * 100)

  return (
    <Card
      data-testid="operations-health-card"
      className="h-full min-w-0 self-stretch gap-3 overflow-hidden p-4 animate-fade-up"
      style={{ animationDelay: `${delay}ms` }}
    >
      <div className="flex items-center justify-between gap-3 rounded-lg border bg-muted/20 px-3 py-2.5">
        <div className="min-w-0">
          <h2 className="panel-heading text-sm font-semibold">{t("dash.health.title")}</h2>
          <p className="mt-0.5 truncate text-[11px] text-muted-foreground">
            {attentionItems.length > 0
              ? t("dash.health.attentionSummary", { count: attentionItems.length })
              : t("dash.health.allHealthy")}
          </p>
        </div>
        <div className="flex shrink-0 items-center gap-2.5">
          <div className="text-right">
            <div className={cn("display-numeral text-xl leading-none", toneTextClass[overallTone])}>
              {healthPercent}%
            </div>
            <div className="mt-1 text-[10px] text-muted-foreground">
              {t("dash.health.passedCount", { passed: healthyItems.length, total: items.length })}
            </div>
          </div>
          <span
            aria-hidden="true"
            className="relative grid size-10 place-items-center rounded-full"
            style={{
              background: `conic-gradient(var(--${overallTone === "critical" ? "destructive" : overallTone === "warning" ? "warning" : "success"}) ${healthPercent}%, var(--muted) 0)`,
            }}
          >
            <span className="grid size-7 place-items-center rounded-full bg-card">
              {overallTone === "success" ? (
                <CheckCircle2 className="size-3.5 text-success" />
              ) : (
                <AlertTriangle className={cn("size-3.5", overallTone === "critical" ? "text-destructive" : "text-warning")} />
              )}
            </span>
          </span>
        </div>
      </div>

      {pending ? (
        <div className="grid flex-1 grid-rows-4 gap-2">
          {Array.from({ length: 4 }).map((_, index) => (
            <Skeleton key={index} className="h-[58px] rounded-lg" />
          ))}
        </div>
      ) : error ? (
        <div className="rounded-lg border border-dashed px-3 py-8 text-center text-sm text-muted-foreground">
          {t("common.loadFailed")}
        </div>
      ) : (
        <div className="flex flex-1 flex-col gap-3">
          {attentionItems.length > 0 ? (
            <section aria-labelledby="health-attention-title">
              <div className="mb-1.5 flex items-center justify-between gap-2">
                <h3 id="health-attention-title" className="text-xs font-semibold">
                  {t("dash.health.attentionTitle")}
                </h3>
                <Badge variant={overallTone === "critical" ? "destructive" : "warning"} className="font-normal">
                  {attentionItems.length}
                </Badge>
              </div>
              <div className="grid gap-1.5">
                {attentionItems.map((item) => {
                  const content = (
                    <>
                      <span className={cn("grid size-7 shrink-0 place-items-center rounded-md", iconClass[item.tone])}>
                        <AlertTriangle className="size-3.5" />
                      </span>
                      <span className="min-w-0 flex-1 truncate text-xs font-semibold">
                        {item.title}
                      </span>
                      {item.to ? <ArrowUpRight className="size-3.5 shrink-0 text-muted-foreground" /> : null}
                    </>
                  )
                  const itemClassName = cn(
                    "flex min-h-11 w-full min-w-0 max-w-full items-center gap-2.5 overflow-hidden rounded-md border border-l-2 bg-muted/20 px-2.5 py-2 transition-colors",
                    item.tone === "critical" ? "border-l-destructive" : "border-l-gold",
                    item.to && "hover:border-input hover:bg-muted/40",
                  )

                  return item.to ? (
                    <Link
                      key={item.key}
                      to={item.to}
                      aria-label={`${item.title}，${item.actionLabel ?? ""}`}
                      className={itemClassName}
                    >
                      {content}
                    </Link>
                  ) : (
                    <div key={item.key} className={itemClassName}>{content}</div>
                  )
                })}
              </div>
            </section>
          ) : null}

          {healthyItems.length > 0 ? (
            <section
              aria-labelledby="health-passed-title"
              className="flex min-h-0 flex-1 flex-col rounded-lg border bg-muted/10 p-2.5"
            >
              <div className="mb-2 flex items-center justify-between gap-2">
                <h3 id="health-passed-title" className="text-xs font-semibold">
                  {t("dash.health.passedTitle")}
                </h3>
                <span className="text-[10px] text-muted-foreground">
                  {t("dash.health.passedCount", { passed: healthyItems.length, total: items.length })}
                </span>
              </div>
              <div className="grid min-h-0 flex-1 auto-rows-fr grid-cols-2 gap-1.5">
                {healthyItems.map((item, index) => (
                  <div
                    key={item.key}
                    title={item.detail}
                    className="flex min-h-11 min-w-0 flex-col justify-between gap-3 rounded-md border border-success/20 bg-card/70 p-3"
                  >
                    <span className="flex w-full items-start justify-between gap-2">
                      <span className="grid size-7 shrink-0 place-items-center rounded-md bg-success/10 text-success">
                        <CheckCircle2 className="size-3.5" />
                      </span>
                      <span className="font-mono text-[9px] font-semibold tracking-[0.08em] text-success/65">
                        PASS {String(index + 1).padStart(2, "0")}
                      </span>
                    </span>
                    <span className="truncate text-xs font-semibold">{item.title}</span>
                  </div>
                ))}
              </div>
            </section>
          ) : null}
        </div>
      )}
    </Card>
  )
}

// ---- Page -------------------------------------------------------------------------

export function DashboardPage() {
  const { t } = useTranslation()
  const { amountsHidden, toggleAmounts } = useAmountPrivacy()
  const dashboardQuery = useDashboard()
  const billsQuery = useBills()
  const calendarQuery = useCalendar()
  const subscriptionsQuery = useSubscriptions()
  const accountsQuery = useAccounts()

  const [plusDialogOpen, setPlusDialogOpen] = React.useState(false)
  const [teamDialogOpen, setTeamDialogOpen] = React.useState(false)

  const isPending = dashboardQuery.isPending || billsQuery.isPending || calendarQuery.isPending
  const isError = dashboardQuery.isError || billsQuery.isError || calendarQuery.isError

  const dashboard = dashboardQuery.data
  const summary = billsQuery.data?.summary
  const healthPending =
    calendarQuery.isPending || subscriptionsQuery.isPending || accountsQuery.isPending
  const healthError = calendarQuery.isError || subscriptionsQuery.isError || accountsQuery.isError

  const refetchAll = () => {
    void dashboardQuery.refetch()
    void billsQuery.refetch()
    void calendarQuery.refetch()
    void subscriptionsQuery.refetch()
    void accountsQuery.refetch()
  }

  return (
    <div className="flex flex-col lg:min-h-[calc(100dvh-7rem)]">
      <PageHeader
        title={t("dash.title")}
        titleAccessory={
          <AmountPrivacyToggle amountsHidden={amountsHidden} onToggle={toggleAmounts} />
        }
        actions={
          <div className="flex flex-wrap items-center gap-2">
            <Button onClick={() => setPlusDialogOpen(true)}>
              <Plus data-slot="icon" />
              {t("dash.newPlusRental")}
            </Button>
            <Button onClick={() => setTeamDialogOpen(true)}>
              <Plus data-slot="icon" />
              {t("dash.newTeamUser")}
            </Button>
          </div>
        }
      />

      {isPending ? (
        <div className="flex flex-1 flex-col gap-4">
          <div className="grid grid-cols-2 gap-3 xl:grid-cols-4">
            {Array.from({ length: 4 }).map((_, index) => (
              <Skeleton key={index} className="h-[118px] rounded-xl" />
            ))}
          </div>
          <Skeleton className="min-h-[320px] flex-1 rounded-xl" />
        </div>
      ) : isError ? (
        <Card className="flex-1 items-center justify-center gap-3 py-16 text-center animate-fade-up">
          <p className="text-sm text-muted-foreground">{t("common.loadFailed")}</p>
          <Button variant="outline" onClick={refetchAll}>
            {t("common.retry")}
          </Button>
        </Card>
      ) : dashboard ? (
        <div className="flex min-h-0 flex-1 flex-col">
          <KpiRow
            dashboard={dashboard}
            summary={summary}
            accountCount={accountsQuery.data?.length ?? dashboard.accounts?.length ?? 0}
            calendar={calendarQuery.data}
            amountsHidden={amountsHidden}
          />

          <div className="grid min-h-[420px] flex-1 items-stretch gap-4 xl:grid-cols-[minmax(0,1.35fr)_minmax(360px,0.65fr)]">
            {summary ? (
              <RevenueTrendCard
                summary={summary}
                amountsHidden={amountsHidden}
                className="h-full min-w-0 overflow-hidden"
                delay={100}
              />
            ) : null}
            <OperationsHealthCard
              dashboard={dashboard}
              subscriptions={subscriptionsQuery.data?.subscriptions ?? []}
              accounts={accountsQuery.data ?? []}
              pending={healthPending}
              error={healthError}
              delay={160}
            />
          </div>
        </div>
      ) : null}

      <PlusRentalDialog open={plusDialogOpen} onOpenChange={setPlusDialogOpen} prefill={null} />
      <SubscriptionDialog open={teamDialogOpen} onOpenChange={setTeamDialogOpen} prefill={null} />
    </div>
  )
}
