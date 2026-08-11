// DTO types mirroring the Go JSON tags (internal/service + internal/model).

export type SubscriptionBusinessType = "team" | "plus"

export interface Subscription {
  id: number
  name: string
  business_type: SubscriptionBusinessType
  price_per_person_cents: number
  cost_cents: number
  is_resale: boolean
  agency_fee_cents: number
  cron_expr: string
  notify_offsets: number[] | null
  channels: string[] | null
  remark: string
  trade_url: string
  customer_email: string
  customer_wechat: string
  seat_id: number
  account_id: number
  account_name: string
  seat_name: string
  subscription_type: string
  boarded_at: string
  archived_at: string | null
  cancellation_requested_at: string | null
  cancellation_expires_at: string | null
  cancellation_case_id: number
  deleted_at: string | null
  created_at: string
  updated_at: string
}

export interface SubscriptionView {
  subscription: Subscription
  price_yuan: string
  cost_yuan: string
  allocated_cost_yuan?: string
  agency_fee_yuan: string
  profit_yuan: string
  allocated_profit_yuan?: string
  cycle_desc: string
  next_due_date: string
  days_remaining: number
  cycle_days: number
  channel_labels: string[] | null
  offsets_text: string
  last_error: string
  account_id: number
  account_name: string
  seat_id: number
  seat_name: string
  boarded_at: string
  archived_at_label: string
  cancellation_pending: boolean
  cancellation_case_id: number
  cancellation_expires_at_label: string
  bill_count: number
  can_soft_delete: boolean
}

export type RedemptionStatusValue = "pending" | "invited" | "rejected"
export type RedemptionCodeStatusValue = "unused" | "used" | "disabled"

export interface RedemptionApplication {
  id: number
  tracking_token: string
  customer_email: string
  customer_contact: string
  redeem_code: string
  request_note: string
  status: RedemptionStatusValue
  assigned_account_id: number
  assigned_seat_id: number
  assigned_subscription_id: number
  operator_note: string
  invited_at: string | null
  created_at: string
  updated_at: string
}

export interface RedemptionCode {
  id: number
  code: string
  status: RedemptionCodeStatusValue
  note: string
  used_by_application_id: number
  used_at: string | null
  created_at: string
  updated_at: string
}

export interface RedemptionCodeView {
  code: RedemptionCode
  status_label: string
  created_at_label: string
  used_at_label: string
  application_email: string
}

export interface RedemptionApplicationView {
  application: RedemptionApplication
  created_at_label: string
  invited_at_label: string
  account_name: string
  account_email: string
  account_space_name: string
  seat_name: string
  subscription_name: string
}

export interface RedemptionStatus {
  status: RedemptionStatusValue
  customer_email: string
  created_at_label: string
  invited_at_label: string
  rejection_reason: string
}

export interface CalendarOccurrence {
  subscription_id: number
  name: string
  business_type: SubscriptionBusinessType
  due_date: string
  day_number: number
  weekday_label: string
  price_yuan: string
  cost_yuan: string
  agency_fee_yuan: string
  is_resale: boolean
  profit_yuan: string
  cycle_desc: string
  reminder_label: string
  channel_labels: string
  paid: boolean
  account_name: string
  seat_name: string
  account_id: number
  seat_id: number
  days_remaining: number
  trade_url: string
  cron_expr: string
  offsets_text: string
  customer_email: string
  customer_wechat: string
  channels: string[] | null
  remark: string
  boarded_at: string
}

export interface CalendarDay {
  date: string
  date_label: string
  day_number: number
  in_month: boolean
  is_weekend: boolean
  is_today: boolean
  occurrences: CalendarOccurrence[] | null
}

export interface CalendarMonth {
  month_value: string
  month_label: string
  previous_month: string
  next_month: string
  current_month: string
  occurrences: CalendarOccurrence[] | null
  paid_in_month_occurrences: CalendarOccurrence[] | null
  archived_subscriptions: SubscriptionView[] | null
  days: CalendarDay[]
  total_count: number
  paid_count: number
  pending_count: number
  pending_month_count: number
  pending_month_amount_yuan: string
  archived_count: number
}

export interface AmountBar {
  name: string
  account_name: string
  amount_yuan: string
  amount_cents: number
}

export interface AccountBreakdown {
  account_name: string
  type: string
  count: number
  amount_yuan: string
  amount_cents: number
}

export interface Dashboard {
  subscription_count: number
  active_count: number
  archived_count: number
  total_amount_yuan: string
  total_refund_cents: number
  total_refund_yuan: string
  net_revenue_cents: number
  net_revenue_yuan: string
  total_cost_yuan: string
  total_profit_yuan: string
  total_agency_fee_yuan: string
  profit_margin_percent: string
  notify_success_30d: number
  notify_failed_30d: number
  amount_by_subscription: AmountBar[] | null
  accounts: AccountBreakdown[] | null
}

export interface Account {
  id: number
  name: string
  remark: string
  payment_method: string
  email: string
  space_name: string
  opened_at: string
  cost_cents: number
  total_cost_cents: number
  zero_renewal_next_month: boolean
  banned_at: string
  ban_note: string
  created_at: string
  updated_at: string
}

export interface Seat {
  id: number
  account_id: number
  name: string
  created_at: string
  updated_at: string
}

export interface SeatView {
  seat: Seat
  occupied: boolean
  active_subscription_id: number
  active_subscription_name: string
  active_business_type: SubscriptionBusinessType
  active_price_yuan: string
  active_cost_yuan: string
  active_agency_fee_yuan: string
  active_is_resale: boolean
  active_cron_expr: string
  active_offsets_text: string
  active_remark: string
  active_trade_url: string
  active_customer_email: string
  active_customer_wechat: string
  active_account_id: number
  active_boarded_at: string
  linked_subscription_count: number
  can_delete: boolean
}

export interface AccountView {
  account: Account
  seats: SeatView[] | null
  seat_total: number
  seat_used: number
  is_full: boolean
  can_delete: boolean
}

export interface SandboxAccount {
  id: number
  name: string
  purpose: string
}

export interface SandboxStatus {
  ready: boolean
  access_token: string
  seeded_at: string
  redemption_codes: string[] | null
  accounts: SandboxAccount[] | null
  subscription_count: number
}

export type AfterSalesStatus = "pending" | "review" | "refunded" | "reassigned"
export type AfterSalesSource = "account_ban" | "customer_cancellation"

export interface AfterSalesCase {
  id: number
  account_id: number
  subscription_id: number
  bill_id: number
  account_name: string
  account_email: string
  account_space_name: string
  customer_email: string
  customer_wechat: string
  period_start: string
  period_end: string
  banned_date: string
  warranty_days: number
  used_days: number
  remaining_days: number
  paid_amount_cents: number
  refund_amount_cents: number
  replacement_account_id: number
  replacement_seat_id: number
  replacement_account_name: string
  replacement_account_email: string
  replacement_space_name: string
  replacement_seat_name: string
  source: AfterSalesSource
  expires_at: string | null
  status: AfterSalesStatus
  note: string
  processed_at: string | null
  created_at: string
  updated_at: string
}

export interface AfterSalesCaseView {
  case: AfterSalesCase
  paid_amount_yuan: string
  refund_amount_yuan: string
  status_label: string
  processed_at_label: string
  expires_at_label: string
}

export interface AfterSalesSummary {
  total_count: number
  pending_count: number
  review_count: number
  refunded_count: number
  reassigned_count: number
  pending_refund_cents: number
  pending_refund_yuan: string
  refunded_amount_cents: number
  refunded_amount_yuan: string
}

export interface AfterSalesPage {
  cases: AfterSalesCaseView[]
  summary: AfterSalesSummary
}

export interface SeatOption {
  id: number
  name: string
  account_id: number
  free: boolean
}

export interface AccountOption {
  id: number
  name: string
  remark: string
  payment_method: string
  email: string
  space_name: string
  opened_at: string
  cost_yuan: string
  total_cost_yuan: string
  zero_renewal_next_month: boolean
  seat_total: number
  seat_used: number
  is_full: boolean
  seats: SeatOption[] | null
}

export interface BillView {
  id: number
  subscription_id: number
  subscription_name: string
  business_type: SubscriptionBusinessType
  account_name: string
  account_email: string
  account_space_name: string
  account_opened_at: string
  seat_name: string
  customer_email: string
  customer_wechat: string
  due_date: string
  amount_yuan: string
  amount_cents: number
  refund_yuan: string
  refund_cents: number
  net_amount_yuan: string
  net_amount_cents: number
  note: string
  paid_at_label: string
  paid_at: string
  archived: boolean
  status_label: string
  trade_url: string
  price_yuan: string
  cost_yuan: string
  agency_fee_yuan: string
  is_resale: boolean
  profit_yuan: string
  cycle_desc: string
  cron_expr: string
  offsets_text: string
  remark: string
  boarded_at: string
  archived_at_label: string
  channel_labels: string
  account_id: number
  seat_id: number
}

export interface MonthAmountBar {
  month: string
  label: string
  count: number
  amount_yuan: string
  amount_cents: number
  gross_amount_yuan: string
  gross_amount_cents: number
  refund_yuan: string
  refund_cents: number
  width_percent: number
}

export interface BillsSummary {
  bill_count: number
  active_count: number
  archived_count: number
  resale_bill_count: number
  total_amount_yuan: string
  total_refund_yuan: string
  net_amount_yuan: string
  total_agency_fee_yuan: string
  this_month_count: number
  this_month_amount_yuan: string
  this_month_refund_yuan: string
  this_month_net_amount_yuan: string
  this_month_agency_fee_yuan: string
  average_amount_yuan: string
  amount_by_subscription: AmountBar[] | null
  accounts: AccountBreakdown[] | null
  monthly_trend: MonthAmountBar[]
  max_month_cents: number
}

export interface ChannelSetting {
  key: string
  label: string
  enabled: boolean
  configured: boolean
  operator_configured: boolean
}

export interface NotificationConfig {
  smtp: {
    host: string
    port: number
    username: string
    from: string
    to: string
    password_set: boolean
  }
  iyuu: {
    token_set: boolean
  }
  gotify: {
    url: string
    token_set: boolean
  }
}

export interface Settings {
  notify_template: string
  customer_email_template: string
  enabled_channels: string[] | null
  channels: ChannelSetting[]
  notification_config: NotificationConfig
  redeem_page: RedeemPageSettings
}

export interface RedeemPageSettings {
  announcement_title: string
  announcement_intro: string
  announcement_items: string[]
  support_title: string
  support_description: string
  support_contact_label: string
  support_wechat_id: string
  support_qr_data_url: string
}

export interface DuePeriodOption {
  start_date: string
  end_date: string
  label: string
  paid: boolean
  preferred: boolean
}

export interface CronPreview {
  description: string
  timezone: string
  times: string[] | null
}

export interface ReminderPreview {
  to: string
  subject: string
  body: string
}

export interface SubscriptionInput {
  name: string
  business_type: SubscriptionBusinessType
  price_yuan: string
  cost_yuan: string
  is_resale: boolean
  agency_fee_yuan: string
  cron_expr: string
  notify_offsets: number[]
  remark: string
  trade_url: string
  customer_email: string
  customer_wechat: string
  account_id: number
  seat_id: number
  boarded_at: string
}

export interface RedemptionSubmitInput {
  customer_email: string
  customer_contact: string
  redeem_code: string
  request_note: string
}

export interface RedemptionCodeGenerateInput {
  count: number
  note: string
}

export interface RedemptionInviteInput {
  seat_id: number
  price_yuan: string
  is_resale: boolean
  agency_fee_yuan: string
  cron_expr: string
  notify_offsets: number[]
  boarded_at: string
  remark: string
  trade_url: string
  operator_note: string
}

export interface RedemptionRejectInput {
  reason: string
}

export interface AccountInput {
  name: string
  remark: string
  payment_method: string
  email: string
  space_name: string
  opened_at: string
  cost_yuan: string
  total_cost_yuan: string
  zero_renewal_next_month: boolean
  seat_count: number
}

export interface BillInput {
  amount_yuan: string
  note: string
}

export interface AfterSalesCaseInput {
  refund_amount_yuan: string
  note: string
}

export interface SettingsInput {
  notify_template: string
  customer_email_template: string
  channels: string[]
  redeem_page: RedeemPageSettings
  notification_config: {
    smtp: {
      host: string
      port: number
      username: string
      password: string
      from: string
      to: string
    }
    iyuu: {
      token: string
    }
    gotify: {
      url: string
      token: string
    }
  }
}
