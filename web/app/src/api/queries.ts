import { useQuery, type QueryClient } from "@tanstack/react-query"

import {
  fetchAdminProfile,
  fetchAccountOptions,
  fetchAccounts,
  fetchAfterSales,
  fetchBills,
  fetchCalendar,
  fetchDashboard,
  fetchGoals,
  fetchOperationsOverview,
  fetchRedemptionCodes,
  fetchRedemptions,
  fetchSettings,
  fetchSubscriptions,
} from "./endpoints"

// All data queries live under the "data" root key so any mutation can
// invalidate the whole tree in one call (single-user app, cheap refetch).
export const queryKeys = {
  data: ["data"] as const,
  adminProfile: ["admin-profile"] as const,
  calendar: (month?: string) => ["data", "calendar", month ?? "current"] as const,
  dashboard: ["data", "dashboard"] as const,
  operationsOverview: ["data", "operations-overview"] as const,
  goals: ["data", "goals"] as const,
  redemptions: (status?: "pending" | "invited" | "rejected" | "all") =>
    ["data", "redemptions", status ?? "all"] as const,
  redemptionCodes: ["data", "redemption-codes"] as const,
  subscriptions: ["data", "subscriptions"] as const,
  accounts: ["data", "accounts"] as const,
  afterSales: ["data", "after-sales"] as const,
  accountOptions: (includeSeatId: number) => ["data", "account-options", includeSeatId] as const,
  bills: ["data", "bills"] as const,
  settings: ["data", "settings"] as const,
}

export function useAdminProfile() {
  return useQuery({
    queryKey: queryKeys.adminProfile,
    queryFn: fetchAdminProfile,
    staleTime: Number.POSITIVE_INFINITY,
  })
}

export function invalidateAppData(queryClient: QueryClient) {
  return queryClient.invalidateQueries({ queryKey: queryKeys.data })
}

export function useCalendar(month?: string) {
  return useQuery({
    queryKey: queryKeys.calendar(month),
    queryFn: () => fetchCalendar(month),
    placeholderData: (previous) => previous,
  })
}

export function useDashboard() {
  return useQuery({ queryKey: queryKeys.dashboard, queryFn: fetchDashboard })
}

export function useOperationsOverview() {
  return useQuery({
    queryKey: queryKeys.operationsOverview,
    queryFn: fetchOperationsOverview,
    refetchInterval: 30_000,
    refetchIntervalInBackground: false,
    refetchOnWindowFocus: "always",
    refetchOnReconnect: "always",
  })
}

export function useGoals() {
  return useQuery({ queryKey: queryKeys.goals, queryFn: fetchGoals })
}

export function useSubscriptions() {
  return useQuery({ queryKey: queryKeys.subscriptions, queryFn: fetchSubscriptions })
}

export function useRedemptions(status?: "pending" | "invited" | "rejected" | "all") {
  return useQuery({
    queryKey: queryKeys.redemptions(status),
    queryFn: () => fetchRedemptions(status),
    // Customers already poll their pending status every five seconds. Keep the
    // administrator queue on the same cadence so newly submitted applications
    // appear without a manual page refresh. React Query pauses this interval
    // while the tab is in the background to avoid unnecessary server traffic.
    refetchInterval: 5_000,
    refetchIntervalInBackground: false,
    refetchOnWindowFocus: "always",
    refetchOnReconnect: "always",
  })
}

export function useRedemptionCodes() {
  return useQuery({ queryKey: queryKeys.redemptionCodes, queryFn: fetchRedemptionCodes })
}

export function useAccounts() {
  return useQuery({ queryKey: queryKeys.accounts, queryFn: fetchAccounts })
}

export function useAfterSales() {
  return useQuery({
    queryKey: queryKeys.afterSales,
    queryFn: fetchAfterSales,
    refetchInterval: 60_000,
  })
}

export function useAccountOptions(includeSeatId = 0, enabled = true) {
  return useQuery({
    queryKey: queryKeys.accountOptions(includeSeatId),
    queryFn: () => fetchAccountOptions(includeSeatId),
    enabled,
  })
}

export function useBills() {
  return useQuery({ queryKey: queryKeys.bills, queryFn: fetchBills })
}

export function useSettings() {
  return useQuery({ queryKey: queryKeys.settings, queryFn: fetchSettings })
}
