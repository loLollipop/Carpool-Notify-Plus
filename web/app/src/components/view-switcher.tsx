import { useTranslation } from "react-i18next"
import { Link, useLocation } from "react-router-dom"

import { cn } from "@/lib/utils"

/** Calendar / cards view toggle shown on both subscription pages. */
export function ViewSwitcher() {
  const { t } = useTranslation()
  const { pathname } = useLocation()

  const items = [
    { to: "/calendar", label: t("nav.calendarView"), active: pathname === "/calendar" },
    { to: "/cards", label: t("nav.cardsView"), active: pathname.startsWith("/cards") },
  ]

  return (
    <div
      role="group"
      aria-label={t("nav.subscriptions")}
      className="inline-flex items-center rounded-lg border bg-muted/50 p-0.5"
    >
      {items.map((item) => (
        <Link
          key={item.to}
          to={item.to}
          aria-current={item.active ? "page" : undefined}
          className={cn(
            "rounded-md px-3 py-1.5 text-[13px] font-medium transition-colors",
            item.active
              ? "border border-border bg-background text-foreground"
              : "border border-transparent text-muted-foreground hover:text-foreground",
          )}
        >
          {item.label}
        </Link>
      ))}
    </div>
  )
}
