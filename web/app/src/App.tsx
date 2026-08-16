import { lazy, Suspense } from "react"
import { Navigate, Route, Routes } from "react-router-dom"

import { AppShell } from "@/components/app-shell"
import { Skeleton } from "@/components/ui/skeleton"
import { LoginPage } from "@/features/auth/LoginPage"
import { RequireAuth } from "@/features/auth/RequireAuth"
import { AccountsPage } from "@/features/accounts/AccountsPage"
import { CalendarPage } from "@/features/calendar/CalendarPage"
import { CardsPage } from "@/features/subscriptions/CardsPage"
import { NotFoundPage } from "@/features/misc/NotFoundPage"
import { RedeemPage } from "@/features/redemptions/RedeemPage"
import { RedemptionsPage } from "@/features/redemptions/RedemptionsPage"
import { SettingsPage } from "@/features/settings/SettingsPage"

// recharts is heavy; keep chart pages in their own chunks.
const DashboardPage = lazy(() =>
  import("@/features/dashboard/DashboardPage").then((module) => ({
    default: module.DashboardPage,
  })),
)
const BillsPage = lazy(() =>
  import("@/features/bills/BillsPage").then((module) => ({ default: module.BillsPage })),
)
const AfterSalesPage = lazy(() =>
  import("@/features/after-sales/AfterSalesPage").then((module) => ({
    default: module.AfterSalesPage,
  })),
)
const PlusRentalsPage = lazy(() =>
  import("@/features/plus-rentals/PlusRentalsPage").then((module) => ({
    default: module.PlusRentalsPage,
  })),
)
const GoalsPage = lazy(() =>
  import("@/features/goals/GoalsPage").then((module) => ({ default: module.GoalsPage })),
)

export default function App() {
  return (
    <Routes>
      <Route path="/login" element={<LoginPage />} />
      <Route path="/redeem" element={<RedeemPage />} />
      <Route
        element={
          <RequireAuth>
            <AppShell />
          </RequireAuth>
        }
      >
        <Route
          path="/"
          element={
            <Suspense fallback={<Skeleton className="h-96 rounded-xl" />}>
              <DashboardPage />
            </Suspense>
          }
        />
        <Route path="/calendar" element={<CalendarPage />} />
        <Route
          path="/goals"
          element={
            <Suspense fallback={<Skeleton className="h-96 rounded-xl" />}>
              <GoalsPage />
            </Suspense>
          }
        />
        <Route path="/users" element={<CardsPage />} />
        <Route
          path="/plus-rentals"
          element={
            <Suspense fallback={<Skeleton className="h-96 rounded-xl" />}>
              <PlusRentalsPage />
            </Suspense>
          }
        />
        <Route path="/cards" element={<Navigate to="/users" replace />} />
        <Route path="/redemptions" element={<RedemptionsPage />} />
        <Route path="/accounts" element={<AccountsPage />} />
        <Route
          path="/after-sales"
          element={
            <Suspense fallback={<Skeleton className="h-96 rounded-xl" />}>
              <AfterSalesPage />
            </Suspense>
          }
        />
        <Route
          path="/bills"
          element={
            <Suspense fallback={<Skeleton className="h-96 rounded-xl" />}>
              <BillsPage />
            </Suspense>
          }
        />
        <Route path="/settings" element={<SettingsPage />} />
        <Route path="*" element={<NotFoundPage />} />
      </Route>
    </Routes>
  )
}
