import * as React from "react"
import { useTranslation } from "react-i18next"
import {
  Area,
  AreaChart,
  CartesianGrid,
  ResponsiveContainer,
  Tooltip as RechartsTooltip,
  XAxis,
  YAxis,
} from "recharts"
import { ArrowUpRight, BellRing, Layers, Plus, Wallet } from "lucide-react"

import { useBills, useCalendar, useDashboard } from "@/api/queries"
import type { BillsSummary, Dashboard } from "@/api/types"
import { NumberTicker } from "@/components/number-ticker"
import { PageHeader } from "@/components/page-header"
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
      className={className}
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

// ---- Page -------------------------------------------------------------------------

export function DashboardPage() {
  const { t } = useTranslation()
  const dashboardQuery = useDashboard()
  const billsQuery = useBills()
  const calendarQuery = useCalendar()

  const [dialogOpen, setDialogOpen] = React.useState(false)

  const isPending = dashboardQuery.isPending || billsQuery.isPending
  const isError = dashboardQuery.isError || billsQuery.isError

  const dashboard = dashboardQuery.data
  const summary = billsQuery.data?.summary

  const refetchAll = () => {
    void dashboardQuery.refetch()
    void billsQuery.refetch()
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

          {summary ? <RevenueTrendCard summary={summary} delay={100} /> : null}
        </>
      ) : null}

      <SubscriptionDialog open={dialogOpen} onOpenChange={setDialogOpen} prefill={null} />
    </>
  )
}
