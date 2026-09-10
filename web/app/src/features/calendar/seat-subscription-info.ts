import type { CalendarOccurrence } from "@/api/types"
import { formatAccountLabel } from "@/lib/account-display"

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
    accountName: formatAccountLabel(occurrence.account_serial, occurrence.account_name),
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
