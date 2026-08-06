import { NavLink, Outlet, useNavigate } from "react-router-dom"
import { useTranslation } from "react-i18next"
import {
  CalendarDays,
  Car,
  Languages,
  LayoutDashboard,
  LogOut,
  Menu,
  Moon,
  ReceiptText,
  Settings,
  Sun,
  TicketCheck,
  Users,
} from "lucide-react"
import { useTheme } from "next-themes"

import { supportedLanguages } from "@/lib/i18n"
import { cn } from "@/lib/utils"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { useAuth } from "@/features/auth/auth-context"

function BrandMark({ inverted = false }: { inverted?: boolean }) {
  return (
    <NavLink to="/" className="group flex min-w-0 items-center gap-3 select-none">
      <img
        src="/logo.png"
        alt=""
        aria-hidden="true"
        width={34}
        height={34}
        draggable={false}
        className="size-8 shrink-0 transition-transform duration-300 group-hover:-rotate-6 lg:size-[34px]"
      />
      <span className="min-w-0 leading-none">
        <span
          className={cn(
            "block truncate text-[15px] font-semibold",
            inverted && "text-[var(--sidebar-foreground)]",
          )}
        >
          Carpool Notify
        </span>
        <span
          className={cn(
            "mt-1 hidden text-[11px] text-muted-foreground lg:block",
            inverted && "text-[var(--sidebar-muted)]",
          )}
        >
          Workspace Console
        </span>
      </span>
    </NavLink>
  )
}

function ThemeToggle({ inverted = false }: { inverted?: boolean }) {
  const { resolvedTheme, setTheme } = useTheme()
  const { t } = useTranslation()

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          variant="ghost"
          size="icon-sm"
          className={cn(
            inverted &&
              "text-[var(--sidebar-muted)] hover:bg-white/10 hover:text-[var(--sidebar-foreground)]",
          )}
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

function LanguageToggle({ inverted = false }: { inverted?: boolean }) {
  const { i18n, t } = useTranslation()

  return (
    <DropdownMenu>
      <Tooltip>
        <TooltipTrigger asChild>
          <DropdownMenuTrigger asChild>
            <Button
              variant="ghost"
              size="icon-sm"
              className={cn(
                inverted &&
                  "text-[var(--sidebar-muted)] hover:bg-white/10 hover:text-[var(--sidebar-foreground)]",
              )}
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

export function AppShell() {
  const { t } = useTranslation()
  const { logout } = useAuth()
  const navigate = useNavigate()

  const navItems = [
    { to: "/", label: t("nav.dashboard"), icon: LayoutDashboard, end: true },
    { to: "/calendar", label: t("nav.calendar"), icon: CalendarDays, end: true },
    { to: "/users", label: t("nav.users"), icon: Users, end: true },
    { to: "/redemptions", label: t("nav.redemptions"), icon: TicketCheck, end: true },
    { to: "/accounts", label: t("nav.accounts"), icon: Car, end: true },
    { to: "/bills", label: t("nav.bills"), icon: ReceiptText, end: true },
    { to: "/settings", label: t("nav.settings"), icon: Settings, end: true },
  ]

  const handleLogout = () => {
    void logout().then(() => navigate("/login", { replace: true }))
  }

  return (
    <div className="min-h-dvh bg-background lg:grid lg:grid-cols-[236px_minmax(0,1fr)]">
      <aside className="fixed inset-y-0 left-0 z-40 hidden w-[236px] flex-col border-r border-[var(--sidebar-border)] bg-[var(--sidebar)] text-[var(--sidebar-foreground)] shadow-[8px_0_32px_color-mix(in_oklab,var(--foreground)_7%,transparent)] lg:flex">
        <div className="flex h-[76px] items-center border-b border-[var(--sidebar-border)] px-5">
          <BrandMark inverted />
        </div>

        <nav className="flex-1 space-y-1 overflow-y-auto px-3 py-5" aria-label="Primary navigation">
          {navItems.map((item) => {
            const Icon = item.icon
            return (
              <NavLink
                key={item.to}
                to={item.to}
                end={item.end}
                className={({ isActive }) =>
                  cn(
                    "group relative flex h-11 items-center gap-3 rounded-md px-2.5 text-sm font-medium text-[var(--sidebar-muted)] transition-colors",
                    "hover:bg-white/[0.06] hover:text-[var(--sidebar-foreground)]",
                    isActive &&
                      "bg-white/[0.09] text-[var(--sidebar-foreground)] shadow-[inset_3px_0_0_var(--brand)]",
                  )
                }
              >
                <span
                  className={cn(
                    "grid size-8 shrink-0 place-items-center rounded-md transition-colors group-hover:bg-white/[0.06]",
                  )}
                >
                  <Icon className="size-[17px]" />
                </span>
                <span>{item.label}</span>
              </NavLink>
            )
          })}
        </nav>

        <div className="border-t border-[var(--sidebar-border)] p-3">
          <div className="flex items-center justify-between rounded-md border border-white/[0.06] bg-white/[0.04] p-1.5">
            <div className="flex items-center">
              <LanguageToggle inverted />
              <ThemeToggle inverted />
            </div>
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  variant="ghost"
                  size="icon-sm"
                  className="text-[var(--sidebar-muted)] hover:bg-white/10 hover:text-[var(--sidebar-foreground)]"
                  aria-label={t("nav.logout")}
                  onClick={handleLogout}
                >
                  <LogOut className="size-4" />
                </Button>
              </TooltipTrigger>
              <TooltipContent>{t("nav.logout")}</TooltipContent>
            </Tooltip>
          </div>
        </div>
      </aside>

      <div className="min-w-0 lg:col-start-2">
        <header className="sticky top-0 z-40 border-b bg-card/95 shadow-sm backdrop-blur lg:hidden">
          <div className="flex h-16 items-center gap-2 px-4">
            <BrandMark />
            <div className="ml-auto flex items-center gap-1">
              <LanguageToggle />
              <ThemeToggle />
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
                  <DropdownMenuSeparator />
                  <DropdownMenuItem variant="destructive" onSelect={handleLogout}>
                    <LogOut className="size-4" />
                    {t("nav.logout")}
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            </div>
          </div>
        </header>

        <main className="mx-auto w-full max-w-[1480px] px-4 py-6 sm:px-6 sm:py-8 lg:px-8 lg:py-9">
          <Outlet />
        </main>
      </div>
    </div>
  )
}
