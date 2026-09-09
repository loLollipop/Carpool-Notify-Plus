import * as React from "react"
import { NavLink, Outlet, useLocation, useNavigate } from "react-router-dom"
import { useTranslation } from "react-i18next"
import {
  Bell,
  CalendarDays,
  CircleDollarSign,
  Clock3,
  ChevronRight,
  CreditCard,
  ExternalLink,
  FlaskConical,
  Languages,
  LayoutDashboard,
  HandCoins,
  KeyRound,
  Menu,
  Moon,
  PanelLeftClose,
  PanelLeftOpen,
  ReceiptText,
  Settings,
  Sun,
  TicketCheck,
  Target,
  Users,
} from "lucide-react"
import { useTheme } from "next-themes"

import { supportedLanguages } from "@/lib/i18n"
import { exitSandboxMode } from "@/lib/sandbox-mode"
import { cn } from "@/lib/utils"
import { APP_NAME, BrandIcon } from "@/components/brand"
import { AdminAccountMenu } from "@/components/admin-account-menu"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog"
import { Skeleton } from "@/components/ui/skeleton"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { useAuth } from "@/features/auth/auth-state"
import { useRedemptions, useRenewalApplications } from "@/api/queries"
import { useSandboxMode } from "@/hooks/use-sandbox-mode"
import { preloadRoute } from "@/route-pages"

const SIDEBAR_STORAGE_KEY = "carpool-notify:sidebar-collapsed"

function BrandMark({
  compact = false,
  hideLabelOnNarrow = false,
}: {
  compact?: boolean
  hideLabelOnNarrow?: boolean
}) {
  return (
    <NavLink
      to="/"
      aria-label={APP_NAME}
      className={cn(
        "group flex min-w-0 items-center gap-3 select-none",
        compact && "w-full justify-center",
      )}
    >
      <BrandIcon />
      <span
        className={cn(
          "min-w-0",
          compact && "hidden",
          hideLabelOnNarrow && "max-[440px]:hidden",
        )}
      >
        <span className="block truncate text-[15px] font-semibold text-[var(--sidebar-foreground)]">
          {APP_NAME}
        </span>
      </span>
    </NavLink>
  )
}

function ThemeToggle() {
  const { resolvedTheme, setTheme } = useTheme()
  const { t } = useTranslation()

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          variant="ghost"
          size="icon-sm"
          className="border border-border bg-card text-muted-foreground hover:bg-accent hover:text-foreground"
          aria-label={t("nav.theme")}
          onClick={() => setTheme(resolvedTheme === "dark" ? "light" : "dark")}
        >
          <Sun className="size-4 scale-100 rotate-0 transition-all duration-300 dark:scale-0 dark:-rotate-90" />
          <Moon className="absolute size-4 scale-0 rotate-90 transition-all duration-300 dark:scale-100 dark:rotate-0" />
        </Button>
      </TooltipTrigger>
      <TooltipContent>{t("nav.theme")}</TooltipContent>
    </Tooltip>
  )
}

function LanguageToggle() {
  const { i18n, t } = useTranslation()

  return (
    <DropdownMenu>
      <Tooltip>
        <TooltipTrigger asChild>
          <DropdownMenuTrigger asChild>
            <Button
              variant="ghost"
              size="icon-sm"
              className="border border-border bg-card text-muted-foreground hover:bg-accent hover:text-foreground"
              aria-label={t("nav.language")}
            >
              <Languages className="size-4" />
            </Button>
          </DropdownMenuTrigger>
        </TooltipTrigger>
        <TooltipContent>{t("nav.language")}</TooltipContent>
      </Tooltip>
      <DropdownMenuContent align="end">
        <DropdownMenuRadioGroup
          value={i18n.language}
          onValueChange={(language) => void i18n.changeLanguage(language)}
        >
          {supportedLanguages.map((language) => (
            <DropdownMenuRadioItem key={language.code} value={language.code}>
              {language.label}
            </DropdownMenuRadioItem>
          ))}
        </DropdownMenuRadioGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

function ApplicationNotifications() {
  const navigate = useNavigate()
  const [open, setOpen] = React.useState(false)
  const redemptionsQuery = useRedemptions("pending")
  const renewalsQuery = useRenewalApplications("pending")
  const redemptionCount = redemptionsQuery.data?.pending_count ?? 0
  const renewalCount = renewalsQuery.data?.pending_count ?? 0
  const pendingCount = redemptionCount + renewalCount
  const redemptions = redemptionsQuery.data?.redemptions ?? []
  const renewals = renewalsQuery.data?.renewals ?? []
  const loading = redemptionsQuery.isPending || renewalsQuery.isPending

  const goTo = (path: string) => {
    setOpen(false)
    navigate(path)
  }

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <Tooltip>
        <TooltipTrigger asChild>
          <DialogTrigger asChild>
            <Button
              variant="ghost"
              size="icon-sm"
              className="relative border border-border bg-card text-muted-foreground hover:bg-accent hover:text-foreground"
              aria-label={pendingCount > 0 ? `待处理申请 ${pendingCount} 条` : "暂无待处理申请"}
            >
              <Bell className="size-4" />
              {pendingCount > 0 ? (
                <span className="absolute -right-2 -top-2 grid min-w-5 place-items-center rounded-full border-2 border-card bg-destructive px-1 text-[10px] font-bold leading-4 text-destructive-foreground tabular-nums">
                  {pendingCount > 99 ? "99+" : pendingCount}
                </span>
              ) : null}
            </Button>
          </DialogTrigger>
        </TooltipTrigger>
        <TooltipContent>
          {pendingCount > 0
            ? `兑换 ${redemptionCount} · 续费 ${renewalCount}`
            : "暂无待处理申请"}
        </TooltipContent>
      </Tooltip>

      <DialogContent className="grid max-h-[82dvh] grid-rows-[auto_minmax(0,1fr)] gap-0 overflow-hidden p-0 sm:max-w-xl">
        <DialogHeader className="border-b px-5 py-5 pr-12 sm:px-6">
          <DialogTitle className="flex items-center gap-2">
            <span className="grid size-8 place-items-center rounded-lg bg-brand/10 text-brand">
              <Bell className="size-4" />
            </span>
            待处理通知
          </DialogTitle>
          <DialogDescription>
            {pendingCount > 0 ? `共 ${pendingCount} 条申请等待处理` : "当前没有需要处理的申请"}
          </DialogDescription>
        </DialogHeader>

        <div className="min-h-0 overflow-y-auto p-4 sm:p-5">
          {loading ? (
            <div className="grid gap-3" aria-label="正在加载待处理申请">
              <Skeleton className="h-24 rounded-xl" />
              <Skeleton className="h-24 rounded-xl" />
            </div>
          ) : pendingCount === 0 ? (
            <div className="grid min-h-56 place-items-center text-center">
              <div>
                <span className="mx-auto grid size-12 place-items-center rounded-full border bg-muted/40 text-muted-foreground">
                  <Bell className="size-5" />
                </span>
                <p className="mt-4 text-sm font-semibold">无待处理申请</p>
                <p className="mt-1 text-xs text-muted-foreground">新的兑换或续费申请会在这里显示</p>
              </div>
            </div>
          ) : (
            <div className="grid gap-5">
              {redemptions.length > 0 ? (
                <section aria-labelledby="pending-redemptions-title">
                  <div className="mb-2.5 flex items-center justify-between gap-3">
                    <h3 id="pending-redemptions-title" className="flex items-center gap-2 text-sm font-semibold">
                      <TicketCheck className="size-4 text-brand" />
                      兑换申请
                    </h3>
                    <span className="rounded-full bg-brand/10 px-2 py-0.5 text-xs font-semibold text-brand tabular-nums">
                      {redemptionCount}
                    </span>
                  </div>
                  <div className="grid gap-2">
                    {redemptions.map((view) => (
                      <div key={view.application.id} className="flex items-center gap-3 rounded-xl border bg-muted/20 p-3">
                        <span className="grid size-9 shrink-0 place-items-center rounded-lg bg-gold/10 text-gold">
                          <Clock3 className="size-4" />
                        </span>
                        <div className="min-w-0 flex-1">
                          <p className="truncate text-sm font-semibold">{view.application.customer_email}</p>
                          <p className="mt-0.5 truncate text-xs text-muted-foreground">
                            {view.application.customer_contact || view.application.redeem_code} · {view.created_at_label}
                          </p>
                        </div>
                        <Button size="sm" onClick={() => goTo(`/redemptions?application=${view.application.id}`)}>
                          前往处理
                        </Button>
                      </div>
                    ))}
                  </div>
                </section>
              ) : null}

              {renewals.length > 0 ? (
                <section aria-labelledby="pending-renewals-title">
                  <div className="mb-2.5 flex items-center justify-between gap-3">
                    <h3 id="pending-renewals-title" className="flex items-center gap-2 text-sm font-semibold">
                      <CreditCard className="size-4 text-success" />
                      续费申请
                    </h3>
                    <span className="rounded-full bg-success/10 px-2 py-0.5 text-xs font-semibold text-success tabular-nums">
                      {renewalCount}
                    </span>
                  </div>
                  <div className="grid gap-2">
                    {renewals.map((view) => (
                      <div key={view.application.id} className="flex items-center gap-3 rounded-xl border bg-muted/20 p-3">
                        <span className="grid size-9 shrink-0 place-items-center rounded-lg bg-success/10 text-success">
                          <CreditCard className="size-4" />
                        </span>
                        <div className="min-w-0 flex-1">
                          <p className="truncate text-sm font-semibold">{view.application.customer_email}</p>
                          <p className="mt-0.5 truncate text-xs text-muted-foreground">
                            ¥{view.amount_yuan} · {view.application.due_date} · {view.created_at_label}
                          </p>
                        </div>
                        <Button size="sm" onClick={() => goTo(`/redemptions?section=renewals&renewal=${view.application.id}`)}>
                          前往处理
                        </Button>
                      </div>
                    ))}
                  </div>
                </section>
              ) : null}
            </div>
          )}
        </div>
      </DialogContent>
    </Dialog>
  )
}

export function AppShell() {
  const { t } = useTranslation()
  const { logout } = useAuth()
  const sandbox = useSandboxMode()
  const navigate = useNavigate()
  const location = useLocation()
  const [openNavTooltip, setOpenNavTooltip] = React.useState<{
    pathname: string
    to: string
  } | null>(null)
  const [sidebarCollapsed, setSidebarCollapsed] = React.useState(() => {
    try {
      return window.localStorage.getItem(SIDEBAR_STORAGE_KEY) === "true"
    } catch {
      return false
    }
  })

  const workbenchNav = [
    { to: "/", label: t("nav.dashboard"), icon: LayoutDashboard, end: true },
    { to: "/calendar", label: t("nav.calendar"), icon: CalendarDays, end: true },
    { to: "/goals", label: t("nav.goals"), icon: Target, end: true },
  ]
  const customerNav = [
    { to: "/users", label: t("nav.users"), icon: Users, end: true },
    { to: "/plus-rentals", label: t("nav.plusRentals"), icon: CircleDollarSign, end: true },
    { to: "/redemptions", label: t("nav.redemptions"), icon: TicketCheck, end: true },
    { to: "/after-sales", label: t("nav.afterSales"), icon: HandCoins, end: true },
  ]
  const assetNav = [
    { to: "/accounts", label: t("nav.accounts"), icon: KeyRound, end: true },
    { to: "/bills", label: t("nav.bills"), icon: ReceiptText, end: true },
  ]
  const systemNav = [
    { to: "/settings", label: t("nav.settings"), icon: Settings, end: true },
  ]
  const navItems = [...workbenchNav, ...customerNav, ...assetNav, ...systemNav]
  const currentItem =
    navItems.find((item) =>
      item.to === "/" ? location.pathname === "/" : location.pathname.startsWith(item.to),
    ) ?? navItems[0]

  React.useEffect(() => {
    try {
      window.localStorage.setItem(SIDEBAR_STORAGE_KEY, String(sidebarCollapsed))
    } catch {
      // Keep the UI usable when browser storage is unavailable.
    }
  }, [sidebarCollapsed])

  const handleLogout = () => {
    void logout().then(() => navigate("/login", { replace: true }))
  }

  const sandboxRedeemPath = sandbox.enabled
    ? `/redeem?sandbox=${encodeURIComponent(sandbox.accessToken)}${
        sandbox.redemptionCodes[0]
          ? `&code=${encodeURIComponent(sandbox.redemptionCodes[0])}`
          : ""
      }`
    : ""

  const renderNavItems = (items: typeof navItems) =>
    items.map((item) => {
      const Icon = item.icon
      return (
        <Tooltip
          key={item.to}
          open={
            sidebarCollapsed &&
            openNavTooltip?.pathname === location.pathname &&
            openNavTooltip.to === item.to
          }
          onOpenChange={(open) =>
            setOpenNavTooltip((current) =>
              open
                ? { pathname: location.pathname, to: item.to }
                : current?.pathname === location.pathname && current.to === item.to
                  ? null
                  : current,
            )
          }
          delayDuration={300}
          disableHoverableContent
        >
          <TooltipTrigger asChild>
            <NavLink
              to={item.to}
              end={item.end}
              aria-label={item.label}
              onFocus={() => preloadRoute(item.to)}
              onPointerEnter={() => preloadRoute(item.to)}
              onClick={() => setOpenNavTooltip(null)}
              className={({ isActive }) =>
                cn(
                  "group relative flex h-10 items-center gap-3 rounded-lg px-2.5 text-sm font-medium transition-[background-color,color,box-shadow]",
                  sidebarCollapsed && "justify-center px-0",
                  isActive
                    ? "bg-brand/10 text-brand ring-1 ring-inset ring-brand/15 dark:bg-brand/15 dark:ring-brand/25"
                    : "text-[var(--sidebar-muted)] hover:bg-accent hover:text-[var(--sidebar-foreground)]",
                )
              }
            >
              {({ isActive }) => (
                <>
                  <span
                    className={cn(
                      "grid size-7 shrink-0 place-items-center rounded-md transition-colors",
                      isActive && "text-brand",
                    )}
                  >
                    <Icon className="size-[17px]" />
                  </span>
                  <span className={cn("truncate", sidebarCollapsed && "hidden")}>{item.label}</span>
                  {isActive && !sidebarCollapsed ? (
                    <span
                      className="absolute left-0 top-1/2 h-5 w-[3px] -translate-y-1/2 rounded-r-full bg-brand"
                      aria-hidden="true"
                    />
                  ) : null}
                </>
              )}
            </NavLink>
          </TooltipTrigger>
          {sidebarCollapsed ? <TooltipContent side="right">{item.label}</TooltipContent> : null}
        </Tooltip>
      )
    })

  return (
    <div
      className={cn(
        "admin-shell min-h-dvh bg-background lg:grid lg:transition-[grid-template-columns] lg:duration-300",
        sidebarCollapsed
          ? "lg:grid-cols-[76px_minmax(0,1fr)]"
          : "lg:grid-cols-[248px_minmax(0,1fr)]",
      )}
    >
      <aside
        onPointerLeave={() => setOpenNavTooltip(null)}
        className={cn(
          "fixed inset-y-0 left-0 z-40 hidden flex-col border-r border-[var(--sidebar-border)] bg-[linear-gradient(180deg,var(--sidebar)_0%,color-mix(in_oklab,var(--sidebar)_94%,var(--brand))_100%)] text-[var(--sidebar-foreground)] transition-[width] duration-300 lg:flex",
          sidebarCollapsed ? "w-[76px]" : "w-[248px]",
        )}
      >
        <div className={cn("flex h-16 items-center border-b border-[var(--sidebar-border)]", sidebarCollapsed ? "px-3" : "px-5")}>
          <BrandMark compact={sidebarCollapsed} />
        </div>

        <nav className="flex-1 overflow-y-auto px-3 py-4" aria-label="Primary navigation">
          {[
            { label: t("nav.workbench"), items: workbenchNav },
            { label: t("nav.customers"), items: customerNav },
            { label: t("nav.assets"), items: assetNav },
          ].map((group, index) => (
            <div
              key={group.label}
              className={cn(
                "space-y-1",
                index > 0 && "mt-3 border-t border-[var(--sidebar-border)]/80 pt-3",
              )}
            >
              <p className={cn("mb-1.5 px-2.5 text-[10px] font-semibold uppercase tracking-[0.12em] text-[var(--sidebar-muted)]", sidebarCollapsed && "sr-only")}>
                {group.label}
              </p>
              {renderNavItems(group.items)}
            </div>
          ))}
          <div className="mt-3 space-y-1 border-t border-[var(--sidebar-border)] pt-3">
            <p className={cn("mb-1.5 px-2.5 text-[10px] font-semibold uppercase tracking-[0.12em] text-[var(--sidebar-muted)]", sidebarCollapsed && "sr-only")}>
              {t("nav.system")}
            </p>
            {renderNavItems(systemNav)}
          </div>
        </nav>

        <div className="space-y-1 border-t border-[var(--sidebar-border)] p-3">
          <Button
            variant="ghost"
            className={cn(
              "w-full justify-start text-[var(--sidebar-muted)] hover:bg-accent hover:text-[var(--sidebar-foreground)]",
              sidebarCollapsed && "justify-center px-0",
            )}
            onClick={() => {
              setOpenNavTooltip(null)
              setSidebarCollapsed((value) => !value)
            }}
            aria-label={sidebarCollapsed ? t("nav.expand") : t("nav.collapse")}
          >
            {sidebarCollapsed ? <PanelLeftOpen className="size-4" /> : <PanelLeftClose className="size-4" />}
            <span className={cn(sidebarCollapsed && "hidden")}>{t("nav.collapse")}</span>
          </Button>
        </div>
      </aside>

      <div className="min-w-0 lg:col-start-2">
        <header className="sticky top-0 z-40 hidden h-16 items-center border-b border-border/70 bg-card/88 px-8 backdrop-blur-xl lg:flex">
          <div className="flex min-w-0 items-center gap-2 text-sm">
            <span className="font-medium text-muted-foreground">{APP_NAME}</span>
            <ChevronRight className="size-3.5 text-muted-foreground/70" />
            <span className="truncate font-semibold text-foreground">{currentItem.label}</span>
          </div>
          <div className="ml-auto flex items-center gap-2">
            <ApplicationNotifications />
            <LanguageToggle />
            <ThemeToggle />
            <AdminAccountMenu placement="header" onLogout={handleLogout} />
          </div>
        </header>

        <header className="sticky top-0 z-40 border-b border-border/70 bg-card/92 backdrop-blur-xl lg:hidden">
          <div className="flex h-16 items-center gap-2 px-4">
            <BrandMark hideLabelOnNarrow />
            <div className="ml-auto flex items-center gap-1">
              <ApplicationNotifications />
              <LanguageToggle />
              <ThemeToggle />
              <AdminAccountMenu placement="header" onLogout={handleLogout} />
              <DropdownMenu>
                <Tooltip>
                  <TooltipTrigger asChild>
                    <DropdownMenuTrigger asChild>
                      <Button variant="outline" size="icon-sm" aria-label="Open navigation">
                        <Menu className="size-4" />
                      </Button>
                    </DropdownMenuTrigger>
                  </TooltipTrigger>
                  <TooltipContent>Menu</TooltipContent>
                </Tooltip>
                <DropdownMenuContent align="end" className="w-64 p-2">
                  {navItems.map((item) => {
                    const Icon = item.icon
                    return (
                      <DropdownMenuItem key={item.to} asChild className="mb-0.5 p-0 last:mb-0">
                        <NavLink
                          to={item.to}
                          end={item.end}
                          onFocus={() => preloadRoute(item.to)}
                          onPointerEnter={() => preloadRoute(item.to)}
                          className={({ isActive }) =>
                            cn(
                              "flex w-full items-center gap-3 rounded-sm px-3 py-2.5",
                              isActive && "bg-brand/10 text-brand",
                            )
                          }
                        >
                          <Icon className="size-4" />
                          {item.label}
                        </NavLink>
                      </DropdownMenuItem>
                    )
                  })}
                </DropdownMenuContent>
              </DropdownMenu>
            </div>
          </div>
        </header>

        {sandbox.enabled ? (
          <div className="border-b border-amber-300/70 bg-amber-50 text-amber-950 dark:border-amber-700/60 dark:bg-amber-950/45 dark:text-amber-100">
            <div className="mx-auto flex w-full max-w-[1600px] flex-col gap-3 px-4 py-3 sm:flex-row sm:items-center sm:px-6 lg:px-8">
              <div className="flex min-w-0 items-start gap-3 sm:items-center">
                <span className="grid size-8 shrink-0 place-items-center rounded-md bg-amber-500/15 text-amber-700 dark:text-amber-300">
                  <FlaskConical className="size-4" />
                </span>
                <div className="min-w-0">
                  <p className="text-sm font-semibold">业务演练模式</p>
                  <p className="text-xs leading-5 text-amber-800/80 dark:text-amber-200/75">
                    当前账号、账单、兑换和售后操作仅写入独立沙盒，不会进入正式统计，也不会发送真实通知。
                  </p>
                </div>
              </div>
              <div className="flex shrink-0 gap-2 sm:ml-auto">
                <Button variant="outline" size="sm" className="border-amber-300 bg-white/60 dark:border-amber-700 dark:bg-amber-950/40" asChild>
                  <a href={sandboxRedeemPath} target="_blank" rel="noreferrer">
                    <ExternalLink data-slot="icon" />
                    测试兑换页
                  </a>
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  className="border-amber-300 bg-white/60 dark:border-amber-700 dark:bg-amber-950/40"
                  onClick={() => {
                    exitSandboxMode()
                    window.location.assign("/")
                  }}
                >
                  退出演练
                </Button>
              </div>
            </div>
          </div>
        ) : null}

        <main className="mx-auto w-full max-w-[1600px] px-4 py-5 sm:px-6 sm:py-6 lg:px-8 lg:py-6">
          <React.Suspense fallback={<Skeleton className="h-96 rounded-xl" />}>
            <Outlet />
          </React.Suspense>
        </main>
      </div>
    </div>
  )
}
