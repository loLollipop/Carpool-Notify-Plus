import { useTranslation } from "react-i18next"
import { BellRing, CheckCircle2, CircleDot, Layers, LogOut } from "lucide-react"

import type { Dashboard } from "@/api/types"
import { cn } from "@/lib/utils"
import { NumberTicker } from "@/components/number-ticker"
import { Card } from "@/components/ui/card"
import { Skeleton } from "@/components/ui/skeleton"

export type KpiDetailKey = "subscriptions" | "pending" | "renewed" | "archived" | "notifications"

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
  tone: "brand" | "cyan" | "gold" | "neutral" | "success"
}) {
  const Component = onClick ? "button" : "article"
  const toneClass = {
    brand: "bg-brand/10 text-brand",
    cyan: "bg-cyan/10 text-cyan",
    gold: "bg-gold/12 text-gold",
    neutral: "bg-muted text-muted-foreground",
    success: "bg-success/10 text-success",
  }[tone]
  return (
    <Card
      className={cn(
        "group relative gap-0 overflow-hidden p-0 transition-[border-color,background-color] duration-200 animate-fade-up",
        onClick && "cursor-pointer hover:border-input hover:bg-accent/30",
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
          <span className={cn("grid size-9 place-items-center rounded-md", toneClass)}>
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

export function KpiSectionSkeleton({ count = 4 }: { count?: 4 | 5 }) {
  return (
    <div
      className={cn(
        "mb-6 grid grid-cols-2 gap-3",
        count === 5 ? "lg:grid-cols-3 xl:grid-cols-5" : "lg:grid-cols-4",
      )}
    >
      {Array.from({ length: count }).map((_, index) => (
        <Skeleton key={index} className="h-[118px] rounded-xl" />
      ))}
    </div>
  )
}

export function KpiSection({
  dashboard,
  pendingCount,
  pendingMode = "unpaid",
  renewedCount,
  onFilterAll,
  onFilterPending,
  onFilterRenewed,
  onOpenDetail,
}: {
  dashboard: Dashboard
  pendingCount: number
  pendingMode?: "unpaid" | "monthDue"
  renewedCount?: number
  onFilterAll?: () => void
  onFilterPending?: () => void
  onFilterRenewed?: () => void
  onOpenDetail: (key: KpiDetailKey) => void
}) {
  const { t } = useTranslation()
  const showsRenewed = renewedCount !== undefined

  return (
    <section
      aria-label="数据统计"
      className={cn(
        "mb-6 grid grid-cols-2 gap-3",
        showsRenewed ? "lg:grid-cols-3 xl:grid-cols-5" : "lg:grid-cols-4",
      )}
    >
      <KpiCard
        label={t("dashboard.subscriptions")}
        value={dashboard.subscription_count}
        hint={t("dashboard.subscriptionsHint", { accounts: dashboard.accounts?.length ?? 0 })}
        icon={<Layers className="size-4" />}
        tone="brand"
        onClick={() => {
          onFilterAll?.()
          onOpenDetail("subscriptions")
        }}
        delay={0}
      />
      <KpiCard
        label={t(pendingMode === "monthDue" ? "dashboard.monthDue" : "dashboard.pending")}
        value={pendingCount}
        hint={t(
          pendingMode === "monthDue" ? "dashboard.monthDueHint" : "dashboard.pendingHint",
          { active: dashboard.active_count },
        )}
        icon={<CircleDot className="size-4" />}
        tone="gold"
        onClick={() => {
          onFilterPending?.()
          onOpenDetail("pending")
        }}
        delay={60}
      />
      {showsRenewed ? (
        <KpiCard
          label={t("dashboard.monthRenewed")}
          value={renewedCount}
          hint={t("dashboard.monthRenewedHint")}
          icon={<CheckCircle2 className="size-4" />}
          tone="cyan"
          onClick={() => {
            onFilterRenewed?.()
            onOpenDetail("renewed")
          }}
          delay={120}
        />
      ) : null}
      <KpiCard
        label={t("dashboard.archived")}
        value={dashboard.archived_count}
        hint={t("dashboard.archivedHint")}
        icon={<LogOut className="size-4" />}
        tone="neutral"
        onClick={() => onOpenDetail("archived")}
        delay={showsRenewed ? 180 : 120}
      />
      <KpiCard
        label={t("dashboard.notifySuccess")}
        value={dashboard.notify_success_30d}
        hint={t("dashboard.notifyHint", { failed: dashboard.notify_failed_30d })}
        hintTone={dashboard.notify_failed_30d === 0 ? "success" : undefined}
        icon={<BellRing className="size-4" />}
        tone="success"
        onClick={() => onOpenDetail("notifications")}
        delay={showsRenewed ? 240 : 180}
      />
    </section>
  )
}
