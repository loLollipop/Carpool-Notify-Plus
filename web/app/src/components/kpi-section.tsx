import { useTranslation } from "react-i18next"
import { BellRing, CircleDot, Layers, LogOut } from "lucide-react"

import type { Dashboard } from "@/api/types"
import { cn } from "@/lib/utils"
import { NumberTicker } from "@/components/number-ticker"
import { Card } from "@/components/ui/card"
import { Skeleton } from "@/components/ui/skeleton"

function KpiCard({
  label,
  value,
  hint,
  icon,
  onClick,
  delay = 0,
  hintTone,
  tone,
}: {
  label: string
  value: string | number
  hint: React.ReactNode
  icon: React.ReactNode
  onClick?: () => void
  delay?: number
  hintTone?: "success"
  tone: "brand" | "gold" | "coral" | "success"
}) {
  const Component = onClick ? "button" : "article"
  const toneClass = {
    brand: "bg-brand/10 text-brand",
    gold: "bg-gold/12 text-gold",
    coral: "bg-coral/10 text-coral",
    success: "bg-success/10 text-success",
  }[tone]
  const toneColor = {
    brand: "var(--brand)",
    gold: "var(--gold)",
    coral: "var(--coral)",
    success: "var(--success)",
  }[tone]
  return (
    <Card
      className={cn(
        "group relative gap-0 overflow-hidden p-0 transition-[border-color,box-shadow] duration-300 animate-fade-up",
        onClick &&
          "cursor-pointer hover:border-foreground/20 hover:shadow-[0_14px_36px_color-mix(in_oklab,var(--foreground)_7%,transparent)]",
      )}
      style={
        {
          animationDelay: `${delay}ms`,
          "--metric-color": toneColor,
        } as React.CSSProperties
      }
    >
      <span className="metric-rule" />
      <Component
        type={onClick ? "button" : undefined}
        onClick={onClick}
        className="flex w-full flex-col items-start gap-0 p-5 text-left outline-none"
      >
        <div className="flex w-full items-center justify-between text-xs font-medium text-muted-foreground">
          <span>{label}</span>
          <span className={cn("grid size-9 place-items-center rounded-md border border-current/10", toneClass)}>
            {icon}
          </span>
        </div>
        <div className="display-numeral mt-5 text-[30px] leading-none">
          <NumberTicker value={value} />
        </div>
        <div
          className={cn(
            "mt-2.5 text-xs text-muted-foreground",
            hintTone === "success" && "text-success",
          )}
        >
          {hint}
        </div>
        {onClick ? (
          <span className="absolute inset-x-0 bottom-0 h-[2px] origin-left scale-x-0 bg-brand transition-transform duration-300 group-hover:scale-x-100" />
        ) : null}
      </Component>
    </Card>
  )
}

export function KpiSectionSkeleton() {
  return (
    <div className="mb-6 grid grid-cols-2 gap-3 lg:grid-cols-4">
      {Array.from({ length: 4 }).map((_, index) => (
        <Skeleton key={index} className="h-[118px] rounded-xl" />
      ))}
    </div>
  )
}

export function KpiSection({
  dashboard,
  pendingCount,
  onFilterAll,
  onFilterPending,
}: {
  dashboard: Dashboard
  pendingCount: number
  onFilterAll?: () => void
  onFilterPending?: () => void
}) {
  const { t } = useTranslation()

  return (
    <section aria-label="数据统计" className="mb-6 grid grid-cols-2 gap-3 lg:grid-cols-4">
      <KpiCard
        label={t("dashboard.subscriptions")}
        value={dashboard.subscription_count}
        hint={t("dashboard.subscriptionsHint", { accounts: dashboard.accounts?.length ?? 0 })}
        icon={<Layers className="size-4" />}
        tone="brand"
        onClick={onFilterAll}
        delay={0}
      />
      <KpiCard
        label={t("dashboard.pending")}
        value={pendingCount}
        hint={t("dashboard.pendingHint", { active: dashboard.active_count })}
        icon={<CircleDot className="size-4" />}
        tone="gold"
        onClick={onFilterPending}
        delay={60}
      />
      <KpiCard
        label={t("dashboard.archived")}
        value={dashboard.archived_count}
        hint={t("dashboard.archivedHint")}
        icon={<LogOut className="size-4" />}
        tone="coral"
        delay={120}
      />
      <KpiCard
        label={t("dashboard.notifySuccess")}
        value={dashboard.notify_success_30d}
        hint={t("dashboard.notifyHint", { failed: dashboard.notify_failed_30d })}
        hintTone={dashboard.notify_failed_30d === 0 ? "success" : undefined}
        icon={<BellRing className="size-4" />}
        tone="success"
        delay={180}
      />
    </section>
  )
}
