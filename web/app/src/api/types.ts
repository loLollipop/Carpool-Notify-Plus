// DTO types mirroring the Go JSON tags (internal/service + internal/model).

export type SubscriptionBusinessType = "team" | "plus"

export interface Subscription {
  id: number
  name: string
  business_type: SubscriptionBusinessType
  price_per_person_cents: number
  next_price_cents: number | null
  next_price_effective_due_date: string
  cost_cents: number
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
  next_price_yuan: string
  next_price_effective_due_date: string
  cost_yuan: string
  allocated_cost_yuan?: string
  profit_yuan: string
  allocated_profit_yuan?: string
  cycle_desc: string
  next_due_date: string
  days_remaining: number
  cycle_days: number
  current_period_start_date: string
  current_period_end_date: string
  current_period_paid: boolean
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
  current_price_yuan: string
  next_price_yuan: string
  next_price_effective_due_date: string
  cost_yuan: string
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
  subscription_id: number
  name: string
  customer_email: string
  account_name: string
  amount_yuan: string
  amount_cents: number
}

export interface AccountBreakdown {
  key: string
  account_id: number
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
  total_profit_cents: number
  profit_margin_percent: string
  notify_success_30d: number
  notify_failed_30d: number
  notification_activity_30d: NotificationActivity[] | null
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
  active_next_price_yuan: string
  active_next_price_effective_due_date: string
  active_cost_yuan: string
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
  business_type: SubscriptionBusinessType
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
  summary_cases: AfterSalesCaseView[]
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
  cost_cents: number
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
  total_amount_yuan: string
  total_refund_yuan: string
  net_amount_yuan: string
  total_cost_cents: number
  total_cost_yuan: string
  total_profit_cents: number
  total_profit_yuan: string
  this_month_count: number
  this_month_amount_yuan: string
  this_month_refund_yuan: string
  this_month_net_amount_yuan: string
  average_amount_yuan: string
  amount_by_subscription: AmountBar[] | null
  accounts: AccountBreakdown[] | null
  refund_details: RefundDetail[] | null
  monthly_trend: MonthAmountBar[]
  max_month_cents: number
}

export interface RefundDetail {
  id: number
  bill_id: number
  subscription_id: number
  business_type: SubscriptionBusinessType
  customer_email: string
  customer_wechat: string
  account_name: string
  period_end: string
  processed_month: string
  processed_at_label: string
  amount_cents: number
  amount_yuan: string
  note: string
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
  price_yuan: string
  price_change_applies: boolean
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
  current_price_yuan?: string
  next_price_yuan?: string
  next_price_effective_due_date?: string
}

export interface NotificationActivity {
  id: number
  subscription_id: number
  subscription_name: string
  customer_email: string
  customer_wechat: string
  due_date: string
  channel: string
  status: "success" | "failed"
  updated_at_label: string
  last_error: string
}

export interface SubscriptionInput {
  name: string
  business_type: SubscriptionBusinessType
  price_yuan: string
  next_price_yuan: string
  cost_yuan: string
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
  cron_expr: string
  notify_offsets: number[]
  boarded_at: string
  remark: string
  trade_url: string
  operator_note: string
}

export interface BusinessGoal {
  id: number
  name: string
  target_profit_cents: number
  baseline_profit_cents: number
  result_profit_cents: number
  status: "active" | "completed"
  completed_at: string | null
  created_at: string
  updated_at: string
}

export interface BusinessGoalProgress {
  goal: BusinessGoal
  current_profit_cents: number
  earned_profit_cents: number
  remaining_profit_cents: number
  progress_percent: number
  reached: boolean
}

export interface CompletedGoalView {
  goal: BusinessGoal
  progress_percent: number
  reached: boolean
}

export interface ProfitMonth {
  month: string
  revenue_cents: number
  cost_cents: number
  refund_cents: number
  profit_cents: number
}

export interface ForecastScenario {
  monthly_profit_cents: number
  months_needed: number
  projected_date: string
}

export interface ProfitForecast {
  source: "run_rate" | "unavailable"
  active_recurring_count: number
  run_rate_monthly_profit_cents: number
  conservative: ForecastScenario
  baseline: ForecastScenario
  optimistic: ForecastScenario
}

export interface MarketPriceSnapshot {
  id: number
  provider: string
  product: string
  low_price_cents: number
  median_price_cents: number
  high_price_cents: number
  sample_count: number
  source_updated_at: string
  created_at: string
}

export interface MarketPriceView {
  available: boolean
  stale: boolean
  warning: string
  source_url: string
  snapshot: MarketPriceSnapshot | null
  history: MarketPriceSnapshot[] | null
}

export interface PricingRecommendation {
  action: "raise" | "hold" | "fill" | "lower_test" | "insufficient"
  reason_codes: string[] | null
  internal_median_price_cents: number
  suggested_low_price_cents: number
  suggested_high_price_cents: number
  seat_cost_floor_cents: number
  seat_total: number
  seat_used: number
  seat_available: number
  utilization_percent: number
  new_sale_discount_percent: number
}

export interface PricingCandidate {
  subscription_id: number
  name: string
  customer_email: string
  customer_wechat: string
  account_name: string
  seat_name: string
  current_price_cents: number
  market_monthly_price_cents: number
  next_price_cents: number | null
  next_price_effective_date: string
  next_due_date: string
  market_position: "below_low" | "below_median" | "market_range" | "above_high" | "unavailable"
  gap_to_market_median_cents: number
  suggested_price_cents: number
  suggested_monthly_price_cents: number
  max_increase_price_cents: number
  paid_period_count: number
  last_paid_date: string
  relationship_days: number
  last_price_increase_date: string
  blocked_code:
    | "eligible"
    | "account_banned"
    | "after_sales"
    | "after_sales_recovery"
    | "invalid_schedule"
    | "scheduled"
    | "protection"
    | "cooldown"
    | "exempted"
  next_review_date: string
  suggested_monthly_uplift_cents: number
  scheduled_monthly_uplift_cents: number
  monthly_revenue_cents: number
  customer_group_size: number
  customer_group_current_price_cents: number
  customer_group_monthly_revenue_cents: number
  customer_group_id: number
  customer_tier: "core" | "mainstay" | "optimize"
  relationship_stage: "new" | "developing" | "established" | "loyal"
  customer_quality_score: number
  relationship_score: number
  loyalty_score: number
  contact_strength_score: number
  relationship_health_score: number
  relationship_level: "fragile" | "developing" | "stable" | "trusted"
  relationship_profile_confidence: "low" | "medium" | "high"
  primary_relationship_task:
    | "repair_trust"
    | "complete_contact"
    | "observe_first_renewal"
    | "protect_key_account"
    | "maintain_low_frequency"
    | "strengthen_habit"
  needs_contact_followup: boolean
  relationship_signal_codes: string[] | null
  adjustment_risk: "low" | "medium" | "high"
  readiness_score: number
  price_gap_percent: number
  suggested_increase_percent: number
  analysis_codes: string[] | null
  expedited_review: boolean
  exemption_count: number
  last_exempted_at: string
  exemption_review_date: string
  exemption_reason_code: string
  renewal_count: number
  renewal_evidence: "unpaid" | "initial" | "renewed" | "increase_accepted"
  verified_price_cents: number
  verified_monthly_price_cents: number
  verified_price_index: number | null
  price_pressure_score: number
  price_stable_days: number
  paid_periods_after_increase: number
  after_sales_case_count: number
  recommended: boolean
  eligible: boolean
  blocked_reason: string
}

export interface RepricingWindow {
  key: "ready" | "next_30" | "next_60" | "later" | "on_hold"
  count: number
  monthly_uplift_cents: number
}

export interface RepricingSegment {
  key: string
  count: number
}

export interface CustomerTierSummary {
  key: "core" | "mainstay" | "optimize"
  count: number
  customer_count: number
  monthly_revenue_cents: number
  revenue_share_percent: number
  average_price_cents: number
  lowest_price_cents: number
  highest_price_cents: number
  recommended_count: number
  scheduled_count: number
}

export interface RepricingAnalysis {
  total_count: number
  customer_count: number
  eligible_count: number
  recommended_count: number
  scheduled_count: number
  protected_count: number
  below_market_count: number
  estimated_monthly_uplift_cents: number
  pipeline_monthly_uplift_cents: number
  scheduled_monthly_uplift_cents: number
  windows: RepricingWindow[] | null
  average_relationship_days: number
  average_paid_periods: number
  average_price_pressure_score: number
  first_cycle_subscription_count: number
  repeat_subscription_count: number
  repeat_customer_count: number
  increased_price_accepted_count: number
  active_exemption_count: number
  relationship_segments: RepricingSegment[] | null
  risk_segments: RepricingSegment[] | null
  price_segments: RepricingSegment[] | null
  customer_tiers: CustomerTierSummary[] | null
}

export type CustomerBenefitType =
  | "renewal_milestone"
  | "loyalty_care"
  | "price_increase_thanks"
  | "service_recovery"
  | "manual"

export interface CustomerBenefitCandidate {
  subscription_id: number
  customer_email: string
  customer_wechat: string
  display_name: string
  customer_tier: "core" | "mainstay" | "optimize"
  seat_count: number
  current_cycle_value_cents: number
  renewal_count: number
  relationship_days: number
  next_due_date: string
  last_paid_date: string
  last_benefit_date: string
  next_eligible_date: string
  recommended_date: string
  reason_code:
    | "manual_review"
    | "service_in_progress"
    | "service_recovery"
    | "first_cycle_observe"
    | "benefit_cooldown"
    | "increase_accepted"
    | "optimize_no_subsidy"
    | "first_renewal"
    | "core_retention"
    | "repeat_retention"
  suggested_benefit_type: CustomerBenefitType
  status: "recommended" | "upcoming" | "observe" | "cooldown" | "hold" | "blocked"
  recommended: boolean
  selectable: boolean
}

export interface CustomerBenefitView {
  id: number
  batch_id: string
  subscription_id: number
  benefit_type: CustomerBenefitType
  benefit_name: string
  actual_cost_cents: number
  perceived_value_cents: number
  benefit_date: string
  next_due_date_snapshot: string
  customer_email_snapshot: string
  customer_wechat_snapshot: string
  customer_tier_snapshot: string
  customer_group_size_snapshot: number
  current_price_cents_snapshot: number
  renewal_count_snapshot: number
  recommendation_code: string
  note: string
  created_at: string
  outcome: "pending" | "renewed" | "not_renewed"
}

export interface CustomerCareSummary {
  customer_count: number
  recommended_count: number
  upcoming_count: number
  benefit_count: number
  total_actual_cost_cents: number
  total_perceived_value_cents: number
  evaluated_benefit_count: number
  renewed_after_benefit_count: number
}

export interface ForecastModelReadiness {
  key: "beta_binomial" | "discrete_survival" | "bg_nbd" | "uplift"
  status: "collecting" | "ready" | "needs_control"
  current_samples: number
  required_samples: number
  detail_code: string
}

export interface PredictionReadiness {
  active_model: "evidence_only" | "beta_binomial"
  renewal_outcome_count: number
  renewal_success_count: number
  churn_outcome_count: number
  first_cycle_subscription_count: number
  repeat_subscription_count: number
  estimated_renewal_percent: number | null
  estimate_low_percent: number | null
  estimate_high_percent: number | null
  models: ForecastModelReadiness[] | null
}

export interface CustomerCareCenter {
  summary: CustomerCareSummary
  candidates: CustomerBenefitCandidate[] | null
  history: CustomerBenefitView[] | null
  prediction: PredictionReadiness
}

export interface GoalCenter {
  active_goal: BusinessGoalProgress | null
  history: CompletedGoalView[] | null
  trend: ProfitMonth[] | null
  forecast: ProfitForecast | null
  market: MarketPriceView
  pricing: PricingRecommendation
  pricing_candidates: PricingCandidate[] | null
  repricing_analysis: RepricingAnalysis
  customer_care: CustomerCareCenter
}

export interface BusinessGoalInput {
  name: string
  target_profit_yuan: string
}

export interface BulkNextPriceInput {
  subscription_ids: number[]
  next_price_yuan: string
}

export interface ManualNextPricesInput {
  items: Array<{
    subscription_id: number
    next_price_yuan: string
  }>
}

export interface BulkPricingExemptionInput {
  subscription_ids: number[]
  review_cycles: number
  reason_code:
    | "loyalty_reward"
    | "multi_seat_retention"
    | "price_observation"
    | "relationship_investment"
    | "manual"
  note: string
}

export interface RecordCustomerBenefitsInput {
  subscription_ids: number[]
  benefit_type: CustomerBenefitType
  benefit_name: string
  actual_cost_yuan: string
  perceived_value_yuan: string
  benefit_date: string
  note: string
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
