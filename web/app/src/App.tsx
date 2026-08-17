import * as React from "react"
import { Navigate, Route, Routes, useLocation } from "react-router-dom"

import { Skeleton } from "@/components/ui/skeleton"
import { RequireAuth } from "@/features/auth/RequireAuth"
import {
  AccountsPage,
  AfterSalesPage,
  BillsPage,
  CalendarPage,
  CardsPage,
  DashboardPage,
  GoalsPage,
  LoginPage,
  NotFoundPage,
  PlusRentalsPage,
  RedeemPage,
  RedemptionsPage,
  SettingsPage,
  preloadRoute,
} from "@/route-pages"

const loadAppShell = () =>
  import("@/components/app-shell").then(({ AppShell }) => ({ default: AppShell }))
const AppShell = React.lazy(loadAppShell)

const publicRoutes = new Set(["/login", "/redeem"])

function PublicPage({ children }: { children: React.ReactNode }) {
  return (
    <React.Suspense
      fallback={
        <div className="grid min-h-dvh place-items-center p-6">
          <Skeleton className="h-72 w-full max-w-md rounded-2xl" />
        </div>
      }
    >
      {children}
    </React.Suspense>
  )
}

export default function App() {
  const location = useLocation()

  React.useEffect(() => {
    const pathname = location.pathname.replace(/\/+$/, "") || "/"
    preloadRoute(pathname)
    if (!publicRoutes.has(pathname)) {
      void loadAppShell().catch(() => undefined)
    }
  }, [location.pathname])

  return (
    <Routes>
      <Route
        path="/login"
        element={
          <PublicPage>
            <LoginPage />
          </PublicPage>
        }
      />
      <Route
        path="/redeem"
        element={
          <PublicPage>
            <RedeemPage />
          </PublicPage>
        }
      />
      <Route
        element={
          <RequireAuth>
            <React.Suspense
              fallback={
                <div className="grid min-h-dvh place-items-center p-6">
                  <Skeleton className="h-72 w-full max-w-6xl rounded-2xl" />
                </div>
              }
            >
              <AppShell />
            </React.Suspense>
          </RequireAuth>
        }
      >
        <Route path="/" element={<DashboardPage />} />
        <Route path="/calendar" element={<CalendarPage />} />
        <Route path="/goals" element={<GoalsPage />} />
        <Route path="/users" element={<CardsPage />} />
        <Route path="/plus-rentals" element={<PlusRentalsPage />} />
        <Route path="/cards" element={<Navigate to="/users" replace />} />
        <Route path="/redemptions" element={<RedemptionsPage />} />
        <Route path="/accounts" element={<AccountsPage />} />
        <Route path="/after-sales" element={<AfterSalesPage />} />
        <Route path="/bills" element={<BillsPage />} />
        <Route path="/settings" element={<SettingsPage />} />
        <Route path="*" element={<NotFoundPage />} />
      </Route>
    </Routes>
  )
}
