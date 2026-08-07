import { useQuery, type QueryClient } from "@tanstack/react-query"

import {
  fetchAccountOptions,
  fetchAccounts,
  fetchAfterSales,
  fetchBills,
  fetchCalendar,
  fetchDashboard,
  fetchRedemptionCodes,
  fetchRedemptions,
  fetchSettings,
  fetchSubscriptions,
} from "./endpoints"

// All data queries live under the "data" root key so any mutation can
// invalidate the whole tree in one call (single-user app, cheap refetch).
export const queryKeys = {
  data: ["data"] as const,
  calendar: (month?: string) => ["data", "calendar", month ?? "current"] as const,
  dashboard: ["data", "dashboard"] as const,
  redemptions: (status?: "pending" | "invited" | "all") =>
    ["data", "redemptions", status ?? "all"] as const,
  redemptionCodes: ["data", "redemption-codes"] as const,
  subscriptions: ["data", "subscriptions"] as const,
  accounts: ["data", "accounts"] as const,
  afterSales: ["data", "after-sales"] as const,
  accountOptions: (includeSeatId: number) => ["data", "account-options", includeSeatId] as const,
  bills: ["data", "bills"] as const,
  settings: ["data", "settings"] as const,
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

export function useSubscriptions() {
  return useQuery({ queryKey: queryKeys.subscriptions, queryFn: fetchSubscriptions })
}

export function useRedemptions(status?: "pending" | "invited" | "all") {
  return useQuery({
    queryKey: queryKeys.redemptions(status),
    queryFn: () => fetchRedemptions(status),
  })
}

export function useRedemptionCodes() {
  return useQuery({ queryKey: queryKeys.redemptionCodes, queryFn: fetchRedemptionCodes })
}

export function useAccounts() {
  return useQuery({ queryKey: queryKeys.accounts, queryFn: fetchAccounts })
}

export function useAfterSales() {
  return useQuery({ queryKey: queryKeys.afterSales, queryFn: fetchAfterSales })
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
