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

export default function App() {
  return (
    <Routes>
      <Route path="/login" element={<LoginPage />} />
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
        <Route path="/users" element={<CardsPage />} />
        <Route path="/cards" element={<Navigate to="/users" replace />} />
        <Route path="/accounts" element={<AccountsPage />} />
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
