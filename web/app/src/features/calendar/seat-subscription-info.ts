import type { CalendarOccurrence, SubscriptionView } from "@/api/types"

export interface SeatSubscriptionInfo {
  subscriptionId: number
  businessType: "team" | "plus"
  name: string
  accountName: string
  seatName: string
  statusLabel: string
  statusTone: "success" | "warning" | "secondary"
  priceYuan: string
  costYuan: string
  profitYuan: string
  cycleDesc: string
  cronExpr: string
  offsetsText: string
  channelLabels: string
  boardedAt: string
  extraDateLabel: string
  extraDate: string
  customerEmail: string
  customerWechat: string
  remark: string
  tradeUrl: string
  archived: boolean
}

type Translate = (key: string) => string

export function seatInfoFromOccurrence(
  occurrence: CalendarOccurrence,
  t: Translate,
): SeatSubscriptionInfo {
  return {
    subscriptionId: occurrence.subscription_id,
    businessType: occurrence.business_type || "team",
    name: occurrence.name,
    accountName: occurrence.account_name,
    seatName: occurrence.seat_name,
    statusLabel: occurrence.paid ? t("dueStatus.paid") : t("calendar.legendPending"),
    statusTone: occurrence.paid ? "success" : "warning",
    priceYuan: occurrence.price_yuan,
    costYuan: occurrence.cost_yuan,
    profitYuan: occurrence.profit_yuan,
    cycleDesc: occurrence.cycle_desc,
    cronExpr: occurrence.cron_expr,
    offsetsText: occurrence.offsets_text,
    channelLabels: occurrence.channel_labels,
    boardedAt: occurrence.boarded_at,
    extraDateLabel: t("calendar.dueDate"),
    extraDate: occurrence.due_date,
    customerEmail: occurrence.customer_email,
    customerWechat: occurrence.customer_wechat,
    remark: occurrence.remark,
    tradeUrl: occurrence.trade_url,
    archived: false,
  }
}

export function seatInfoFromArchived(view: SubscriptionView, t: Translate): SeatSubscriptionInfo {
  return {
    subscriptionId: view.subscription.id,
    businessType: view.subscription.business_type || "team",
    name: view.subscription.name,
    accountName: view.account_name,
    seatName: view.seat_name,
    statusLabel: t("calendar.filterArchived"),
    statusTone: "secondary",
    priceYuan: view.price_yuan,
    costYuan: view.cost_yuan,
    profitYuan: view.profit_yuan,
    cycleDesc: view.cycle_desc,
    cronExpr: view.subscription.cron_expr,
    offsetsText: view.offsets_text,
    channelLabels: (view.channel_labels ?? []).join(" · "),
    boardedAt: view.boarded_at,
    extraDateLabel: t("calendar.archivedAt"),
    extraDate: view.archived_at_label,
    customerEmail: view.subscription.customer_email,
    customerWechat: view.subscription.customer_wechat,
    remark: view.subscription.remark,
    tradeUrl: view.subscription.trade_url,
    archived: true,
  }
}
