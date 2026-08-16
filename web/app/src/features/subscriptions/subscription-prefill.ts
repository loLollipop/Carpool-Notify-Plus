import type {
  CalendarOccurrence,
  SeatView,
  SubscriptionBusinessType,
  SubscriptionView,
} from "@/api/types"

/** Normalized prefill payload for the subscription dialog (edit mode). */
export interface SubscriptionPrefill {
  id: number
  businessType: SubscriptionBusinessType
  name: string
  priceYuan: string
  nextPriceYuan: string
  nextPriceEffectiveDueDate: string
  nextDueDate: string
  costYuan: string
  cronExpr: string
  offsets: number[]
  remark: string
  tradeUrl: string
  customerEmail: string
  customerWechat: string
  accountId: number
  seatId: number
  boardedAt: string
}

export function parseOffsetsText(offsetsText: string): number[] {
  return offsetsText
    .split(",")
    .map((part) => part.trim())
    .filter((part) => part !== "")
    .map((part) => Number(part))
    .filter((value) => Number.isInteger(value) && value >= 0)
}

export function prefillFromView(view: SubscriptionView): SubscriptionPrefill {
  return {
    id: view.subscription.id,
    businessType: view.subscription.business_type || "team",
    name: view.subscription.name,
    priceYuan: view.price_yuan,
    nextPriceYuan: view.next_price_yuan,
    nextPriceEffectiveDueDate: view.next_price_effective_due_date,
    nextDueDate: view.next_due_date,
    costYuan: view.cost_yuan,
    cronExpr: view.subscription.cron_expr,
    offsets: view.subscription.notify_offsets ?? [],
    remark: view.subscription.remark,
    tradeUrl: view.subscription.trade_url,
    customerEmail: view.subscription.customer_email,
    customerWechat: view.subscription.customer_wechat,
    accountId: view.account_id,
    seatId: view.seat_id,
    boardedAt: view.boarded_at,
  }
}

export function prefillFromOccurrence(occurrence: CalendarOccurrence): SubscriptionPrefill {
  return {
    id: occurrence.subscription_id,
    businessType: occurrence.business_type || "team",
    name: occurrence.name,
    priceYuan: occurrence.current_price_yuan,
    nextPriceYuan: occurrence.next_price_yuan,
    nextPriceEffectiveDueDate: occurrence.next_price_effective_due_date,
    nextDueDate: occurrence.due_date,
    costYuan: occurrence.cost_yuan,
    cronExpr: occurrence.cron_expr,
    offsets: parseOffsetsText(occurrence.offsets_text),
    remark: occurrence.remark,
    tradeUrl: occurrence.trade_url,
    customerEmail: occurrence.customer_email,
    customerWechat: occurrence.customer_wechat,
    accountId: occurrence.account_id,
    seatId: occurrence.seat_id,
    boardedAt: occurrence.boarded_at,
  }
}

export function prefillFromSeat(seat: SeatView): SubscriptionPrefill {
  return {
    id: seat.active_subscription_id,
    businessType: seat.active_business_type || "team",
    name: seat.active_subscription_name,
    priceYuan: seat.active_price_yuan,
    nextPriceYuan: seat.active_next_price_yuan,
    nextPriceEffectiveDueDate: seat.active_next_price_effective_due_date,
    nextDueDate: "",
    costYuan: seat.active_cost_yuan,
    cronExpr: seat.active_cron_expr,
    offsets: parseOffsetsText(seat.active_offsets_text),
    remark: seat.active_remark,
    tradeUrl: seat.active_trade_url,
    customerEmail: seat.active_customer_email,
    customerWechat: seat.active_customer_wechat,
    accountId: seat.active_account_id,
    seatId: seat.seat.id,
    boardedAt: seat.active_boarded_at,
  }
}

/** Today in Asia/Shanghai as YYYY-MM-DD. */
export function todayShanghai(): string {
  return new Date().toLocaleDateString("en-CA", { timeZone: "Asia/Shanghai" })
}
