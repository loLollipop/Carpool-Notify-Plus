import { NavLink, Outlet, useLocation, useNavigate } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { Languages, LogOut, Moon, Sun } from "lucide-react"
import { useTheme } from "next-themes"

import { supportedLanguages } from "@/lib/i18n"
import { cn } from "@/lib/utils"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { useAuth } from "@/features/auth/auth-context"

function BrandMark() {
  return (
    <NavLink to="/" className="group flex items-center gap-2.5 select-none">
      <img
        src="/logo.png"
        alt=""
        aria-hidden="true"
        width={28}
        height={28}
        draggable={false}
        className="size-7 transition-transform duration-300 group-hover:-rotate-6"
      />
      <span className="hidden sm:flex flex-col leading-none">
        <span className="text-[15px] font-semibold tracking-tight">Carpool Notify</span>
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
            <Button variant="ghost" size="icon-sm" aria-label={t("nav.language")}>
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
  const location = useLocation()

  const navItems = [
    { to: "/", label: t("nav.dashboard"), end: true },
    { to: "/calendar", label: t("nav.subscriptions"), end: true },
    { to: "/accounts", label: t("nav.accounts"), end: true },
    { to: "/bills", label: t("nav.bills"), end: true },
    { to: "/settings", label: t("nav.settings"), end: true },
  ]

  // “订阅” covers both /calendar and /cards.
  const isSubscriptionsActive =
    location.pathname === "/calendar" || location.pathname.startsWith("/cards")

  return (
    <div className="min-h-dvh flex flex-col">
      <header className="sticky top-0 z-40 border-b bg-background/85 backdrop-blur-md">
        <div className="mx-auto flex h-14 w-full max-w-[1400px] items-center gap-6 px-4 sm:px-6">
          <BrandMark />
          <nav className="flex items-center gap-1 text-sm" aria-label="主导航">
            {navItems.map((item) => (
              <NavLink
                key={item.to}
                to={item.to}
                end={item.end}
                className={({ isActive }) => {
                  const active = item.to === "/calendar" ? isSubscriptionsActive : isActive
                  return cn(
                    "relative rounded-md px-3 py-1.5 font-medium text-muted-foreground transition-colors hover:text-foreground",
                    "after:absolute after:inset-x-3 after:-bottom-[13px] after:h-[2px] after:rounded-full after:bg-brand after:origin-left after:scale-x-0 after:transition-transform after:duration-300",
                    active && "text-foreground after:scale-x-100",
                  )
                }}
              >
                {item.label}
              </NavLink>
            ))}
          </nav>
          <div className="ml-auto flex items-center gap-1">
            <LanguageToggle />
            <ThemeToggle />
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  variant="ghost"
                  size="icon-sm"
                  aria-label={t("nav.logout")}
                  onClick={() => {
                    void logout().then(() => navigate("/login", { replace: true }))
                  }}
                >
                  <LogOut className="size-4" />
                </Button>
              </TooltipTrigger>
              <TooltipContent>{t("nav.logout")}</TooltipContent>
            </Tooltip>
          </div>
        </div>
      </header>
      <main className="mx-auto w-full max-w-[1400px] flex-1 px-4 py-6 sm:px-6 sm:py-8">
        <Outlet />
      </main>
    </div>
  )
}
