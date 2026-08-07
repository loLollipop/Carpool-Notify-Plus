import { api } from "./client"
import type {
  AccountInput,
  AccountOption,
  AccountView,
  AfterSalesCaseInput,
  AfterSalesCaseView,
  AfterSalesSummary,
  BillInput,
  BillView,
  BillsSummary,
  CalendarMonth,
  CronPreview,
  Dashboard,
  DuePeriodOption,
  ReminderPreview,
  RedemptionApplicationView,
  RedemptionCodeGenerateInput,
  RedemptionCodeView,
  RedeemPageSettings,
  RedemptionInviteInput,
  RedemptionStatus,
  RedemptionSubmitInput,
  Settings,
  SettingsInput,
  SubscriptionInput,
  SubscriptionView,
} from "./types"

interface MessageResult {
  ok: boolean
  message?: string
}

// ---- Session ----

export function getSession() {
  return api<{ authenticated: boolean }>("/api/session", { silent401: true })
}

export function login(password: string) {
  return api<MessageResult>("/api/login", { method: "POST", body: { password }, silent401: true })
}

export function logout() {
  return api<MessageResult>("/api/logout", { method: "POST" })
}

// ---- Queries ----

export function fetchCalendar(month?: string) {
  const query = month ? `?month=${encodeURIComponent(month)}` : ""
  return api<{ calendar: CalendarMonth }>(`/api/calendar${query}`).then((r) => r.calendar)
}

export function fetchDashboard() {
  return api<{ dashboard: Dashboard }>("/api/dashboard").then((r) => r.dashboard)
}

export function fetchSubscriptions() {
  return api<{ subscriptions: SubscriptionView[] | null; archived: SubscriptionView[] | null }>(
    "/api/subscriptions",
  )
}

export function fetchRedemptions(status?: "pending" | "invited" | "all") {
  const query = status && status !== "all" ? `?status=${encodeURIComponent(status)}` : ""
  return api<{ redemptions: RedemptionApplicationView[] | null; pending_count: number }>(
    `/api/redemptions${query}`,
  ).then((r) => ({
    redemptions: r.redemptions ?? [],
    pending_count: r.pending_count,
  }))
}

export function fetchRedemptionCodes() {
  return api<{
    codes: RedemptionCodeView[] | null
    available_count: number
    used_count: number
    disabled_count: number
  }>("/api/redemption-codes").then((r) => ({
    codes: r.codes ?? [],
    available_count: r.available_count,
    used_count: r.used_count,
    disabled_count: r.disabled_count,
  }))
}

export function fetchAccounts() {
  return api<{ accounts: AccountView[] | null }>("/api/accounts").then((r) => r.accounts ?? [])
}

export function fetchAfterSales() {
  return api<{
    cases: AfterSalesCaseView[] | null
    summary: AfterSalesSummary
  }>("/api/after-sales").then((r) => ({ cases: r.cases ?? [], summary: r.summary }))
}

export function fetchAccountOptions(includeSeatId = 0) {
  const query = includeSeatId > 0 ? `?include_seat_id=${includeSeatId}` : ""
  return api<{ accounts: AccountOption[] | null }>(`/api/account-options${query}`).then(
    (r) => r.accounts ?? [],
  )
}

export function fetchBills() {
  return api<{ bills: BillView[] | null; summary: BillsSummary }>("/api/bills")
}

export function fetchSettings() {
  return api<Settings & { ok: boolean }>("/api/settings")
}

export function fetchCronPreview(cronExpr: string, boardedAt: string, count = 5) {
  const query = `?cron_expr=${encodeURIComponent(cronExpr)}&boarded_at=${encodeURIComponent(
    boardedAt,
  )}&count=${count}`
  return api<CronPreview & { ok: boolean }>(`/api/cron/preview${query}`)
}

export function fetchReminderPreview(subscriptionId: number) {
  return api<ReminderPreview & { ok: boolean }>(
    `/api/subscriptions/${subscriptionId}/reminder-preview`,
  )
}

export function fetchDuePeriods(subscriptionId: number, preferred?: string) {
  const query = preferred ? `?preferred=${encodeURIComponent(preferred)}` : ""
  return api<{ periods: DuePeriodOption[] | null }>(
    `/api/subscriptions/${subscriptionId}/due-periods${query}`,
  ).then((r) => r.periods ?? [])
}

export function fetchRedemptionStatus(token: string) {
  return api<{ redemption: RedemptionStatus }>(`/api/redeem/${encodeURIComponent(token)}`).then(
    (r) => r.redemption,
  )
}

export function fetchRedeemPageSettings() {
  return api<{ redeem_page: RedeemPageSettings }>("/api/redeem-settings").then(
    (r) => r.redeem_page,
  )
}

export function submitRedemptionApplication(input: RedemptionSubmitInput) {
  return api<MessageResult & { tracking_token: string; status: "pending" | "invited" }>(
    "/api/redeem",
    { method: "POST", body: input },
  )
}

// ---- Subscription mutations ----

export function createSubscription(input: SubscriptionInput) {
  return api<MessageResult>("/api/subscriptions", { method: "POST", body: input })
}

export function updateSubscription(id: number, input: SubscriptionInput) {
  return api<MessageResult>(`/api/subscriptions/${id}`, { method: "PUT", body: input })
}

export function archiveSubscription(id: number) {
  return api<MessageResult>(`/api/subscriptions/${id}/archive`, { method: "POST" })
}

export function softDeleteSubscription(id: number) {
  return api<MessageResult>(`/api/subscriptions/${id}`, { method: "DELETE" })
}

export function copySubscription(id: number, seatId = 0) {
  return api<MessageResult>(`/api/subscriptions/${id}/copy`, {
    method: "POST",
    body: { seat_id: seatId },
  })
}

export function testNotifySubscription(id: number) {
  return api<MessageResult>(`/api/subscriptions/${id}/test-notify`, { method: "POST" })
}

export function sendCustomerEmail(id: number) {
  return api<MessageResult>(`/api/subscriptions/${id}/send-customer-email`, { method: "POST" })
}

export function setDuePaid(id: number, dueDate: string, paid: boolean) {
  return api<{ ok: boolean; paid: boolean; due_date: string; next_due_date?: string }>(
    `/api/subscriptions/${id}/due/${dueDate}/paid`,
    { method: "POST", body: { paid } },
  )
}

export function inviteRedemption(id: number, input: RedemptionInviteInput) {
  return api<MessageResult & { subscription_id: number }>(`/api/redemptions/${id}/invite`, {
    method: "POST",
    body: input,
  })
}

export function generateRedemptionCodes(input: RedemptionCodeGenerateInput) {
  return api<MessageResult & { codes: RedemptionCodeView[] | null }>("/api/redemption-codes", {
    method: "POST",
    body: input,
  }).then((r) => ({
    ...r,
    codes: r.codes ?? [],
  }))
}

export function disableRedemptionCode(id: number) {
  return api<MessageResult>(`/api/redemption-codes/${id}/disable`, { method: "POST" })
}

export function enableRedemptionCode(id: number) {
  return api<MessageResult>(`/api/redemption-codes/${id}/enable`, { method: "POST" })
}

export function deleteRedemptionCode(id: number) {
  return api<MessageResult>(`/api/redemption-codes/${id}`, { method: "DELETE" })
}

// ---- Account mutations ----

export function createAccount(input: AccountInput) {
  return api<MessageResult>("/api/accounts", { method: "POST", body: input })
}

export function updateAccount(id: number, input: AccountInput) {
  return api<MessageResult>(`/api/accounts/${id}`, { method: "PUT", body: input })
}

export function deleteAccount(id: number) {
  return api<MessageResult>(`/api/accounts/${id}`, { method: "DELETE" })
}

export function banAccount(id: number, input: { banned_date: string; note: string }) {
  return api<MessageResult & { created_count: number }>(`/api/accounts/${id}/ban`, {
    method: "POST",
    body: input,
  })
}

export function updateAfterSalesCase(id: number, input: AfterSalesCaseInput) {
  return api<MessageResult>(`/api/after-sales/${id}`, { method: "PUT", body: input })
}

export function setAfterSalesRefunded(id: number, refunded: boolean) {
  return api<MessageResult>(`/api/after-sales/${id}/refunded`, {
    method: "POST",
    body: { refunded },
  })
}

// ---- Bill mutations ----

export function updateBill(id: number, input: BillInput) {
  return api<MessageResult>(`/api/bills/${id}`, { method: "PUT", body: input })
}

export function deleteBill(id: number) {
  return api<MessageResult>(`/api/bills/${id}`, { method: "DELETE" })
}

// ---- Settings mutations ----

export function saveSettings(input: SettingsInput) {
  return api<MessageResult>("/api/settings", { method: "PUT", body: input })
}

export function testSettingsNotify() {
  return api<MessageResult>("/api/settings/test-notify", { method: "POST" })
}

export function previewSettingsTemplate(kind: "notify" | "customer", template: string) {
  return api<{ rendered: string; sample_name: string; subject: string }>(
    "/api/settings/template-preview",
    { method: "POST", body: { kind, template } },
  )
}
