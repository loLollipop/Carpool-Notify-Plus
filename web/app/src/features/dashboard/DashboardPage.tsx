import * as React from "react"
import { useTranslation } from "react-i18next"
import { Link } from "react-router-dom"
import {
  Area,
  AreaChart,
  CartesianGrid,
  ResponsiveContainer,
  Tooltip as RechartsTooltip,
  XAxis,
  YAxis,
} from "recharts"
import {
  AlertTriangle,
  ArrowUpRight,
  CalendarClock,
  CheckCircle2,
  CreditCard,
  HandCoins,
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
import { NumberTicker } from "@/components/number-ticker"
import { PageHeader } from "@/components/page-header"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { Skeleton } from "@/components/ui/skeleton"
import { cn } from "@/lib/utils"
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
}: {
  active?: boolean
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
      {item.grossCents !== undefined ? (
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
}: {
  label: string
  value: string | number
  hint: string
  icon: React.ReactNode
  tone: KpiTone
  delay: number
}) {
  const toneClass: Record<KpiTone, string> = {
    brand: "bg-brand/10 text-brand",
    success: "bg-success/10 text-success",
    gold: "bg-gold/12 text-gold",
    violet: "bg-violet/10 text-violet",
  }
  return (
    <Card
      className="group relative min-h-[112px] gap-0 overflow-hidden p-4 transition-[border-color,box-shadow] duration-200 animate-fade-up hover:border-input hover:shadow-[0_8px_24px_color-mix(in_oklab,var(--foreground)_6%,transparent)]"
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
            <NumberTicker value={value} />
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
}: {
  dashboard: Dashboard
  summary: BillsSummary | undefined
  accountCount: number
  calendar: CalendarMonth | undefined
}) {
  const { t } = useTranslation()

  return (
    <section aria-label={t("dash.title")} className="mb-4 grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-5">
      <KpiCard
        label={t("dash.kpiRevenue")}
        value={`¥${summary?.total_amount_yuan ?? "0.00"}`}
        hint={t("dash.kpiRevenueHint", {
          count: summary?.bill_count ?? 0,
          refund: summary?.total_refund_yuan ?? "0.00",
        })}
        icon={<Wallet className="size-4" />}
        tone="brand"
        delay={0}
      />
      <KpiCard
        label={t("dash.kpiProfit")}
        value={`¥${dashboard.total_profit_yuan}`}
        hint={t("dash.kpiProfitHint", {
          margin: dashboard.profit_margin_percent,
          net: dashboard.net_revenue_yuan,
          refund: dashboard.total_refund_yuan,
        })}
        icon={<ArrowUpRight className="size-4" />}
        tone="success"
        delay={70}
      />
      <KpiCard
        label={t("dash.kpiRefund")}
        value={`¥${dashboard.total_refund_yuan}`}
        hint={t("dash.kpiRefundHint", { gross: dashboard.total_amount_yuan })}
        icon={<HandCoins className="size-4" />}
        tone="violet"
        delay={140}
      />
      <KpiCard
        label={t("dash.kpiAccountCost")}
        value={`¥${dashboard.total_cost_yuan}`}
        hint={t("dash.kpiAccountCostHint", {
          accounts: accountCount,
          active: dashboard.active_count,
        })}
        icon={<CreditCard className="size-4" />}
        tone="gold"
        delay={210}
      />
      <KpiCard
        label={t("dash.kpiPendingMonth")}
        value={`¥${calendar?.pending_month_amount_yuan ?? "0.00"}`}
        hint={t("dash.kpiPendingMonthHint", {
          count: calendar?.pending_month_count ?? 0,
        })}
        icon={<CalendarClock className="size-4" />}
        tone="violet"
        delay={280}
      />
    </section>
  )
}

// ---- Revenue trend (area) -------------------------------------------------------

function RevenueTrendCard({ summary, className, delay }: { summary: BillsSummary; className?: string; delay?: number }) {
  const { t } = useTranslation()
  const data = (summary.monthly_trend ?? []).map((item) => ({
    label: item.label,
    cents: item.amount_cents,
    grossCents: item.gross_amount_cents,
    refundCents: item.refund_cents,
    count: item.count,
  }))

  return (
    <ChartCard
      title={t("dash.trendTitle")}
      desc={t("dash.trendDesc")}
      className={cn("h-full min-h-0", className)}
      delay={delay}
    >
      {summary.bill_count === 0 ? (
        <ChartEmpty text={t("dash.trendEmpty")} />
      ) : (
        <div className="min-h-[260px] flex-1">
          <ResponsiveContainer width="100%" height="100%">
            <AreaChart data={data} margin={{ top: 8, right: 8, bottom: 0, left: 0 }}>
              <CartesianGrid vertical={false} stroke="var(--border)" strokeDasharray="2 6" />
              <XAxis
                dataKey="label"
                axisLine={{ stroke: "var(--border)" }}
                tickLine={{ stroke: "var(--border)" }}
                tick={{ fontSize: 11, fill: "var(--muted-foreground)" }}
                dy={6}
              />
              <YAxis
                width={64}
                domain={["auto", "auto"]}
                axisLine={{ stroke: "var(--border)" }}
                tickLine={{ stroke: "var(--border)" }}
                tick={{ fontSize: 10, fill: "var(--muted-foreground)" }}
                tickFormatter={formatAxisCents}
              />
              <RechartsTooltip
                cursor={{ stroke: "var(--border)", strokeWidth: 1 }}
                content={<ChartTooltip />}
              />
              <Area
                type="monotone"
                dataKey="cents"
                stroke="var(--brand)"
                strokeWidth={2}
                fill="var(--brand)"
                fillOpacity={0.08}
                dot={false}
                activeDot={{ r: 4, strokeWidth: 0 }}
                {...CHART_ANIMATION}
              />
            </AreaChart>
          </ResponsiveContainer>
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
  calendar,
  subscriptions,
  accounts,
  t,
}: {
  dashboard: Dashboard
  calendar: CalendarMonth | undefined
  subscriptions: SubscriptionView[]
  accounts: AccountView[]
  t: ReturnType<typeof useTranslation>["t"]
}) {
  const items: HealthItem[] = []
  const currentMonth = calendar?.month_value ?? ""
  const pendingOccurrences = (calendar?.occurrences ?? []).filter(
    (occurrence) =>
      !occurrence.paid && (currentMonth === "" || occurrence.due_date.slice(0, 7) === currentMonth),
  )
  const overdue = pendingOccurrences.filter((occurrence) => occurrence.days_remaining < 0)
  const dueSoon = pendingOccurrences.filter(
    (occurrence) => occurrence.days_remaining >= 0 && occurrence.days_remaining <= 7,
  )
  const missingCustomerEmail = subscriptions.filter(
    (view) => view.subscription.customer_email.trim() === "",
  )
  const missingReminderOffsets = subscriptions.filter(
    (view) => (view.subscription.notify_offsets?.length ?? 0) === 0,
  )
  const activeSaleAccounts = accounts.filter((view) =>
    (view.seats ?? []).some((seat) => seat.occupied && !seat.active_is_resale),
  )
  const missingAccountCost = activeSaleAccounts.filter((view) => view.account.cost_cents <= 0)
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
        names: summarizeNames(overdue.map((item) => item.name)),
      }),
      actionLabel: t("dash.health.goCalendar"),
      to: "/calendar",
      count: overdue.length,
    })
  } else if (dueSoon.length > 0) {
    items.push({
      key: "due",
      tone: "warning",
      title: t("dash.health.dueSoonTitle", { count: dueSoon.length }),
      detail: t("dash.health.dueSoonDetail", {
        names: summarizeNames(dueSoon.map((item) => item.name)),
      }),
      actionLabel: t("dash.health.goCalendar"),
      to: "/calendar",
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
  calendar,
  subscriptions,
  accounts,
  pending,
  error,
  delay = 0,
}: {
  dashboard: Dashboard
  calendar: CalendarMonth | undefined
  subscriptions: SubscriptionView[]
  accounts: AccountView[]
  pending: boolean
  error: boolean
  delay?: number
}) {
  const { t } = useTranslation()
  const toneClass: Record<HealthTone, string> = {
    critical: "destructive",
    warning: "warning",
    success: "success",
  }
  const iconClass: Record<HealthTone, string> = {
    critical: "bg-destructive/10 text-destructive",
    warning: "bg-warning/15 text-warning-foreground dark:text-warning",
    success: "bg-success/12 text-success",
  }

  const items = React.useMemo(
    () =>
      buildHealthItems({
        dashboard,
        calendar,
        subscriptions,
        accounts,
        t,
      }),
    [accounts, calendar, dashboard, subscriptions, t],
  )
  const overallTone: HealthTone = items.some((item) => item.tone === "critical")
    ? "critical"
    : items.some((item) => item.tone === "warning")
      ? "warning"
      : "success"

  return (
    <Card
      className="h-fit min-h-0 self-start gap-2 overflow-hidden p-4 animate-fade-up xl:sticky xl:top-0"
      style={{ animationDelay: `${delay}ms` }}
    >
      <div className="flex items-start justify-between gap-2">
        <div>
          <h2 className="panel-heading text-sm font-semibold">{t("dash.health.title")}</h2>
          <p className="mt-0.5 text-xs text-muted-foreground">{t("dash.health.desc")}</p>
        </div>
        <span
          className={cn(
            "grid size-8 shrink-0 place-items-center rounded-lg",
            iconClass[overallTone],
          )}
        >
          {overallTone === "success" ? (
            <CheckCircle2 className="size-4" />
          ) : (
            <AlertTriangle className="size-4" />
          )}
        </span>
      </div>

      {pending ? (
        <div className="grid gap-1.5">
          {Array.from({ length: 4 }).map((_, index) => (
            <Skeleton key={index} className="h-[58px] rounded-lg" />
          ))}
        </div>
      ) : error ? (
        <div className="rounded-lg border border-dashed px-3 py-8 text-center text-sm text-muted-foreground">
          {t("common.loadFailed")}
        </div>
      ) : (
        <div className="grid max-h-[calc(100dvh-18rem)] gap-1.5 overflow-y-auto pr-1">
          {items.map((item) => (
            <div
              key={item.key}
              className={cn(
                "flex items-start gap-3 rounded-md border border-l-2 bg-muted/25 px-3 py-1",
                item.tone === "critical" && "border-l-destructive",
                item.tone === "warning" && "border-l-gold",
                item.tone === "success" && "border-l-success",
              )}
            >
              <span
                className={cn(
                  "mt-0.5 grid size-7 shrink-0 place-items-center rounded-md",
                  iconClass[item.tone],
                )}
              >
                {item.tone === "success" ? (
                  <CheckCircle2 className="size-3.5" />
                ) : (
                  <AlertTriangle className="size-3.5" />
                )}
              </span>
              <div className="min-w-0 flex-1">
                <div className="flex min-w-0 flex-wrap items-center gap-1.5">
                  <span className="truncate text-sm font-medium">{item.title}</span>
                  {item.count !== undefined ? (
                    <Badge
                      variant={toneClass[item.tone] as "destructive" | "warning" | "success"}
                      className="font-normal"
                    >
                      {item.count}
                    </Badge>
                  ) : null}
                </div>
                <p className="mt-0.5 line-clamp-2 text-xs text-muted-foreground">{item.detail}</p>
              </div>
              {item.to && item.actionLabel ? (
                <Button variant="ghost" size="sm" asChild className="shrink-0">
                  <Link to={item.to}>{item.actionLabel}</Link>
                </Button>
              ) : null}
            </div>
          ))}
        </div>
      )}
    </Card>
  )
}

// ---- Page -------------------------------------------------------------------------

export function DashboardPage() {
  const { t } = useTranslation()
  const dashboardQuery = useDashboard()
  const billsQuery = useBills()
  const calendarQuery = useCalendar()
  const subscriptionsQuery = useSubscriptions()
  const accountsQuery = useAccounts()

  const [dialogOpen, setDialogOpen] = React.useState(false)

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
    <div className="flex flex-col xl:h-[calc(100dvh-7rem)] xl:min-h-0 xl:overflow-hidden">
      <PageHeader
        title={t("dash.title")}
        description={t("dash.desc")}
        actions={
          <Button onClick={() => setDialogOpen(true)}>
            <Plus data-slot="icon" />
            {t("nav.newSubscription")}
          </Button>
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
          />

          <div className="grid min-h-0 flex-1 items-start gap-4 xl:grid-cols-[minmax(0,1.35fr)_minmax(360px,0.65fr)]">
            {summary ? <RevenueTrendCard summary={summary} delay={100} /> : null}
            <OperationsHealthCard
              dashboard={dashboard}
              calendar={calendarQuery.data}
              subscriptions={subscriptionsQuery.data?.subscriptions ?? []}
              accounts={accountsQuery.data ?? []}
              pending={healthPending}
              error={healthError}
              delay={160}
            />
          </div>
        </div>
      ) : null}

      <SubscriptionDialog open={dialogOpen} onOpenChange={setDialogOpen} prefill={null} />
    </div>
  )
}
