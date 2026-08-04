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
  BellRing,
  CheckCircle2,
  Layers,
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
          <h2 className="text-sm font-semibold">{title}</h2>
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
  payload?: { payload: { name?: string; label?: string; cents?: number; value?: number; count?: number } }[]
}) {
  const { t } = useTranslation()
  if (!active || !payload || payload.length === 0) return null
  const item = payload[0].payload
  return (
    <div className="rounded-lg border bg-popover px-3 py-2 text-xs text-popover-foreground animate-fade-in">
      <div className="font-medium">{item.name ?? item.label}</div>
      <div className="mt-0.5 tabular-nums text-muted-foreground">
        {item.cents !== undefined ? formatCents(item.cents) : item.value}
        {item.count !== undefined ? ` · ${t("bills.countSuffix", { count: item.count })}` : ""}
      </div>
    </div>
  )
}

// ---- KPI ------------------------------------------------------------------------

type KpiTone = "brand" | "success" | "violet"

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
    violet: "bg-violet/10 text-violet",
  }
  return (
    <Card
      className="group gap-0 p-5 transition-colors duration-300 animate-fade-up hover:border-foreground/15"
      style={{ animationDelay: `${delay}ms` }}
    >
      <div className="flex items-center justify-between text-xs font-medium text-muted-foreground">
        <span>{label}</span>
        <span
          className={cn(
            "grid size-8 place-items-center rounded-lg transition-transform duration-300 group-hover:scale-110",
            toneClass[tone],
          )}
        >
          {icon}
        </span>
      </div>
      <div className="display-numeral mt-4 text-[28px] leading-none">
        <NumberTicker value={value} />
      </div>
      <div className="mt-2.5 truncate text-xs text-muted-foreground">{hint}</div>
    </Card>
  )
}

function KpiRow({
  dashboard,
  summary,
  pendingCount,
}: {
  dashboard: Dashboard
  summary: BillsSummary | undefined
  pendingCount: number
}) {
  const { t } = useTranslation()

  const notifyTotal = dashboard.notify_success_30d + dashboard.notify_failed_30d
  const notifyRate =
    notifyTotal === 0
      ? "—"
      : `${((dashboard.notify_success_30d / notifyTotal) * 100).toFixed(1).replace(/\.0$/, "")}%`

  return (
    <section aria-label={t("dash.title")} className="mb-4 grid grid-cols-2 gap-3 xl:grid-cols-4">
      <KpiCard
        label={t("dash.kpiMonth")}
        value={`¥${summary?.this_month_amount_yuan ?? "0.00"}`}
        hint={t("dash.kpiMonthHint", { count: summary?.this_month_count ?? 0 })}
        icon={<Wallet className="size-4" />}
        tone="brand"
        delay={0}
      />
      <KpiCard
        label={t("dash.kpiProfit")}
        value={`¥${dashboard.total_profit_yuan}`}
        hint={t("dash.kpiProfitHint", {
          margin: dashboard.profit_margin_percent,
          total: dashboard.total_amount_yuan,
          agencyFee: dashboard.total_agency_fee_yuan,
        })}
        icon={<ArrowUpRight className="size-4" />}
        tone="success"
        delay={70}
      />
      <KpiCard
        label={t("dash.kpiActive")}
        value={dashboard.active_count}
        hint={t("dash.kpiActiveHint", {
          accounts: dashboard.accounts?.length ?? 0,
          pending: pendingCount,
        })}
        icon={<Layers className="size-4" />}
        tone="violet"
        delay={140}
      />
      <KpiCard
        label={t("dash.kpiNotify")}
        value={notifyRate}
        hint={t("dash.kpiNotifyHint", {
          success: dashboard.notify_success_30d,
          failed: dashboard.notify_failed_30d,
        })}
        icon={<BellRing className="size-4" />}
        tone="brand"
        delay={210}
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
    count: item.count,
  }))

  return (
    <ChartCard
      title={t("dash.trendTitle")}
      desc={t("dash.trendDesc")}
      className={cn("h-full", className)}
      delay={delay}
    >
      {summary.bill_count === 0 ? (
        <ChartEmpty text={t("dash.trendEmpty")} />
      ) : (
        <div className="h-[260px]">
          <ResponsiveContainer width="100%" height="100%">
            <AreaChart data={data} margin={{ top: 8, right: 8, bottom: 0, left: 8 }}>
              <defs>
                <linearGradient id="trendFill" x1="0" y1="0" x2="0" y2="1">
                  <stop offset="0%" stopColor="var(--brand)" stopOpacity={0.2} />
                  <stop offset="100%" stopColor="var(--brand)" stopOpacity={0.02} />
                </linearGradient>
              </defs>
              <CartesianGrid vertical={false} stroke="var(--border)" strokeDasharray="3 4" />
              <XAxis
                dataKey="label"
                axisLine={false}
                tickLine={false}
                tick={{ fontSize: 11, fill: "var(--muted-foreground)" }}
                dy={6}
              />
              <YAxis hide domain={[0, "dataMax"]} />
              <RechartsTooltip
                cursor={{ stroke: "var(--border)", strokeWidth: 1 }}
                content={<ChartTooltip />}
              />
              <Area
                type="monotone"
                dataKey="cents"
                stroke="var(--brand)"
                strokeWidth={2}
                fill="url(#trendFill)"
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
  actionLabel: string
  to: string
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

  if (overdue.length > 0) {
    items.push({
      key: "overdue",
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
      key: "dueSoon",
      tone: "warning",
      title: t("dash.health.dueSoonTitle", { count: dueSoon.length }),
      detail: t("dash.health.dueSoonDetail", {
        names: summarizeNames(dueSoon.map((item) => item.name)),
      }),
      actionLabel: t("dash.health.goCalendar"),
      to: "/calendar",
      count: dueSoon.length,
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
      to: "/cards",
      count: missingCustomerEmail.length,
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
      to: "/cards",
      count: missingReminderOffsets.length,
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
  }

  if (subscriptions.length === 0) {
    items.push({
      key: "empty",
      tone: "warning",
      title: t("dash.health.emptyTitle"),
      detail: t("dash.health.emptyDetail"),
      actionLabel: t("dash.health.goAccounts"),
      to: "/accounts",
    })
  }

  if (items.length === 0) {
    items.push({
      key: "healthy",
      tone: "success",
      title: t("dash.health.healthyTitle"),
      detail: t("dash.health.healthyDetail"),
      actionLabel: t("dash.health.goCalendar"),
      to: "/calendar",
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
      }).slice(0, 5),
    [accounts, calendar, dashboard, subscriptions, t],
  )

  return (
    <Card className="h-full gap-4 p-5 animate-fade-up" style={{ animationDelay: `${delay}ms` }}>
      <div className="flex items-start justify-between gap-2">
        <div>
          <h2 className="text-sm font-semibold">{t("dash.health.title")}</h2>
          <p className="mt-0.5 text-xs text-muted-foreground">{t("dash.health.desc")}</p>
        </div>
        <span className="grid size-8 shrink-0 place-items-center rounded-lg bg-brand/10 text-brand">
          <CheckCircle2 className="size-4" />
        </span>
      </div>

      {pending ? (
        <div className="grid gap-2.5">
          {Array.from({ length: 4 }).map((_, index) => (
            <Skeleton key={index} className="h-[58px] rounded-lg" />
          ))}
        </div>
      ) : error ? (
        <div className="rounded-lg border border-dashed px-3 py-8 text-center text-sm text-muted-foreground">
          {t("common.loadFailed")}
        </div>
      ) : (
        <div className="grid gap-2.5">
          {items.map((item) => (
            <div
              key={item.key}
              className="flex items-start gap-3 rounded-lg border bg-muted/25 px-3 py-2.5"
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
              <Button variant="ghost" size="sm" asChild className="shrink-0">
                <Link to={item.to}>{item.actionLabel}</Link>
              </Button>
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

  const isPending = dashboardQuery.isPending || billsQuery.isPending
  const isError = dashboardQuery.isError || billsQuery.isError

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
    <>
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
        <div className="grid gap-4">
          <div className="grid grid-cols-2 gap-3 xl:grid-cols-4">
            {Array.from({ length: 4 }).map((_, index) => (
              <Skeleton key={index} className="h-[118px] rounded-xl" />
            ))}
          </div>
          <Skeleton className="h-[320px] rounded-xl" />
        </div>
      ) : isError ? (
        <Card className="items-center gap-3 py-16 text-center animate-fade-up">
          <p className="text-sm text-muted-foreground">{t("common.loadFailed")}</p>
          <Button variant="outline" onClick={refetchAll}>
            {t("common.retry")}
          </Button>
        </Card>
      ) : dashboard ? (
        <>
          <KpiRow
            dashboard={dashboard}
            summary={summary}
            pendingCount={calendarQuery.data?.pending_count ?? 0}
          />

          <div className="grid items-stretch gap-4 xl:grid-cols-[minmax(0,1.35fr)_minmax(360px,0.65fr)]">
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
        </>
      ) : null}

      <SubscriptionDialog open={dialogOpen} onOpenChange={setDialogOpen} prefill={null} />
    </>
  )
}
