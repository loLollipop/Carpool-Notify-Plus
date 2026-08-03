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
}: {
  label: string
  value: string | number
  hint: React.ReactNode
  icon: React.ReactNode
  onClick?: () => void
  delay?: number
  hintTone?: "success"
}) {
  const Component = onClick ? "button" : "article"
  return (
    <Card
      className={cn(
        "group relative gap-0 overflow-hidden p-0 transition-colors duration-300 animate-fade-up",
        onClick && "cursor-pointer hover:border-foreground/15",
      )}
      style={{ animationDelay: `${delay}ms` }}
    >
      <Component
        type={onClick ? "button" : undefined}
        onClick={onClick}
        className="flex w-full flex-col items-start gap-0 p-5 text-left outline-none"
      >
        <div className="flex w-full items-center justify-between text-xs font-medium text-muted-foreground">
          <span>{label}</span>
          <span className="grid size-8 place-items-center rounded-lg bg-brand/10 text-brand transition-transform duration-300 group-hover:scale-110">
            {icon}
          </span>
        </div>
        <div className="display-numeral mt-4 text-[30px] leading-none">
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
        onClick={onFilterAll}
        delay={0}
      />
      <KpiCard
        label={t("dashboard.pending")}
        value={pendingCount}
        hint={t("dashboard.pendingHint", { active: dashboard.active_count })}
        icon={<CircleDot className="size-4" />}
        onClick={onFilterPending}
        delay={60}
      />
      <KpiCard
        label={t("dashboard.archived")}
        value={dashboard.archived_count}
        hint={t("dashboard.archivedHint")}
        icon={<LogOut className="size-4" />}
        delay={120}
      />
      <KpiCard
        label={t("dashboard.notifySuccess")}
        value={dashboard.notify_success_30d}
        hint={t("dashboard.notifyHint", { failed: dashboard.notify_failed_30d })}
        hintTone={dashboard.notify_failed_30d === 0 ? "success" : undefined}
        icon={<BellRing className="size-4" />}
        delay={180}
      />
    </section>
  )
}
