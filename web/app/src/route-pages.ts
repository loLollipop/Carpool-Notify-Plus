import { lazy } from "react"

// Keep route imports in one manifest so App only describes navigation structure.
// Calling a loader before React renders the page warms the same browser module cache.
const pageLoaders = {
  login: () =>
    import("@/features/auth/LoginPage").then(({ LoginPage }) => ({ default: LoginPage })),
  redeem: () =>
    import("@/features/redemptions/RedeemPage").then(({ RedeemPage }) => ({
      default: RedeemPage,
    })),
  dashboard: () =>
    import("@/features/dashboard/DashboardPage").then(({ DashboardPage }) => ({
      default: DashboardPage,
    })),
  calendar: () =>
    import("@/features/calendar/CalendarPage").then(({ CalendarPage }) => ({
      default: CalendarPage,
    })),
  goals: () =>
    import("@/features/goals/GoalsPage").then(({ GoalsPage }) => ({ default: GoalsPage })),
  users: () =>
    import("@/features/subscriptions/CardsPage").then(({ CardsPage }) => ({
      default: CardsPage,
    })),
  plusRentals: () =>
    import("@/features/plus-rentals/PlusRentalsPage").then(({ PlusRentalsPage }) => ({
      default: PlusRentalsPage,
    })),
  redemptions: () =>
    import("@/features/redemptions/RedemptionsPage").then(({ RedemptionsPage }) => ({
      default: RedemptionsPage,
    })),
  accounts: () =>
    import("@/features/accounts/AccountsPage").then(({ AccountsPage }) => ({
      default: AccountsPage,
    })),
  afterSales: () =>
    import("@/features/after-sales/AfterSalesPage").then(({ AfterSalesPage }) => ({
      default: AfterSalesPage,
    })),
  bills: () =>
    import("@/features/bills/BillsPage").then(({ BillsPage }) => ({ default: BillsPage })),
  settings: () =>
    import("@/features/settings/SettingsPage").then(({ SettingsPage }) => ({
      default: SettingsPage,
    })),
  notFound: () =>
    import("@/features/misc/NotFoundPage").then(({ NotFoundPage }) => ({
      default: NotFoundPage,
    })),
}

export const LoginPage = lazy(pageLoaders.login)
export const RedeemPage = lazy(pageLoaders.redeem)
export const DashboardPage = lazy(pageLoaders.dashboard)
export const CalendarPage = lazy(pageLoaders.calendar)
export const GoalsPage = lazy(pageLoaders.goals)
export const CardsPage = lazy(pageLoaders.users)
export const PlusRentalsPage = lazy(pageLoaders.plusRentals)
export const RedemptionsPage = lazy(pageLoaders.redemptions)
export const AccountsPage = lazy(pageLoaders.accounts)
export const AfterSalesPage = lazy(pageLoaders.afterSales)
export const BillsPage = lazy(pageLoaders.bills)
export const SettingsPage = lazy(pageLoaders.settings)
export const NotFoundPage = lazy(pageLoaders.notFound)

const routeLoaders: Record<string, () => Promise<unknown>> = {
  "/": pageLoaders.dashboard,
  "/login": pageLoaders.login,
  "/redeem": pageLoaders.redeem,
  "/calendar": pageLoaders.calendar,
  "/goals": pageLoaders.goals,
  "/users": pageLoaders.users,
  "/cards": pageLoaders.users,
  "/plus-rentals": pageLoaders.plusRentals,
  "/redemptions": pageLoaders.redemptions,
  "/accounts": pageLoaders.accounts,
  "/after-sales": pageLoaders.afterSales,
  "/bills": pageLoaders.bills,
  "/settings": pageLoaders.settings,
}

export function preloadRoute(pathname: string) {
  const normalizedPath = pathname.replace(/\/+$/, "") || "/"
  const loader = routeLoaders[normalizedPath]
  if (loader) {
    void loader().catch(() => undefined)
  }
}
