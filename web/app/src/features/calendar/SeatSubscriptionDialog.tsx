import * as React from "react"
import { useTranslation } from "react-i18next"
import { Link } from "react-router-dom"
import { ExternalLink, UsersRound } from "lucide-react"

import type { CalendarOccurrence, SubscriptionView } from "@/api/types"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"

export interface SeatSubscriptionInfo {
  subscriptionId: number
  name: string
  accountName: string
  seatName: string
  statusLabel: string
  statusTone: "success" | "warning" | "secondary"
  isResale: boolean
  priceYuan: string
  costYuan: string
  agencyFeeYuan: string
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
    name: occurrence.name,
    accountName: occurrence.account_name,
    seatName: occurrence.seat_name,
    statusLabel: occurrence.paid ? t("dueStatus.paid") : t("calendar.legendPending"),
    statusTone: occurrence.paid ? "success" : "warning",
    isResale: occurrence.is_resale,
    priceYuan: occurrence.price_yuan,
    costYuan: occurrence.cost_yuan,
    agencyFeeYuan: occurrence.agency_fee_yuan,
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
    name: view.subscription.name,
    accountName: view.account_name,
    seatName: view.seat_name,
    statusLabel: t("calendar.filterArchived"),
    statusTone: "secondary",
    isResale: view.subscription.is_resale,
    priceYuan: view.price_yuan,
    costYuan: view.cost_yuan,
    agencyFeeYuan: view.agency_fee_yuan,
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

function ViewItem({
  label,
  mono,
  children,
}: {
  label: string
  mono?: boolean
  children: React.ReactNode
}) {
  return (
    <div className="min-w-0">
      <div className="text-[11px] font-medium tracking-wide text-muted-foreground">{label}</div>
      <div className={mono ? "truncate font-mono text-[13px]" : "truncate text-[13px]"}>
        {children}
      </div>
    </div>
  )
}

/** Read-only seat subscription details, opened from agenda account badges. */
export function SeatSubscriptionDialog({
  open,
  onOpenChange,
  info,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  info: SeatSubscriptionInfo | null
}) {
  const { t } = useTranslation()
  const manageHref = React.useMemo(() => {
    if (!info) return "/users"
    const params = new URLSearchParams()
    params.set("subscription", String(info.subscriptionId))
    if (info.archived) params.set("filter", "archived")
    const query = info.customerEmail || info.customerWechat || info.accountName || info.name
    if (query) params.set("q", query)
    return `/users?${params.toString()}`
  }, [info])

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{t("calendar.viewSeatTitle")}</DialogTitle>
          <DialogDescription>{t("calendar.viewSeatDesc")}</DialogDescription>
        </DialogHeader>

        {info ? (
          <div className="grid gap-4">
            <div>
              <div className="text-base font-semibold">{info.accountName || info.name}</div>
              <div className="mt-1.5 flex flex-wrap items-center gap-1.5">
                <Badge variant="secondary" className="font-normal">
                  {info.customerEmail || info.name}
                </Badge>
                {info.isResale ? (
                  <Badge variant="outline" className="font-normal">
                    {t("cards.resale")}
                  </Badge>
                ) : null}
                <Badge variant={info.statusTone} className="font-normal">
                  {info.statusLabel}
                </Badge>
              </div>
            </div>

            <div className="grid grid-cols-2 gap-x-4 gap-y-2.5 sm:grid-cols-3">
              <ViewItem label={t("bills.viewPrice")}>
                <span className="tabular-nums">¥{info.priceYuan}</span>
              </ViewItem>
              <ViewItem label={t("bills.viewCost")}>
                <span className="tabular-nums">¥{info.costYuan}</span>
              </ViewItem>
              {info.isResale ? (
                <ViewItem label={t("cards.agencyFee")}>
                  <span className="tabular-nums">¥{info.agencyFeeYuan}</span>
                </ViewItem>
              ) : (
                <ViewItem label={t("bills.viewProfit")}>
                  <span className="tabular-nums">¥{info.profitYuan}</span>
                </ViewItem>
              )}
              <ViewItem label={t("bills.viewCycle")}>{info.cycleDesc}</ViewItem>
              <ViewItem label={t("bills.viewCron")} mono>
                {info.cronExpr}
              </ViewItem>
              <ViewItem label={t("bills.viewOffsets")}>{info.offsetsText || "—"}</ViewItem>
              <ViewItem label={t("bills.viewBoarded")}>
                <span className="tabular-nums">{info.boardedAt || "—"}</span>
              </ViewItem>
              <ViewItem label={info.extraDateLabel}>
                <span className="tabular-nums">{info.extraDate || "—"}</span>
              </ViewItem>
              <ViewItem label={t("bills.viewChannels")}>
                {info.channelLabels}
              </ViewItem>
              <ViewItem label={t("subscriptionDialog.customerEmail")}>
                {info.customerEmail || "—"}
              </ViewItem>
              <ViewItem label={t("subscriptionDialog.customerWechat")}>
                {info.customerWechat || "—"}
              </ViewItem>
            </div>

            <div>
              <div className="text-[11px] font-medium tracking-wide text-muted-foreground">
                {t("bills.viewRemark")}
              </div>
              <p className="mt-0.5 text-[13px] leading-relaxed whitespace-pre-wrap">
                {info.remark || "—"}
              </p>
            </div>

            <div>
              <div className="text-[11px] font-medium tracking-wide text-muted-foreground">
                {t("bills.viewTradeUrl")}
              </div>
              {info.tradeUrl ? (
                <a
                  href={info.tradeUrl}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="mt-0.5 block truncate text-[13px] text-brand underline-offset-2 hover:underline"
                >
                  {info.tradeUrl}
                </a>
              ) : (
                <p className="mt-0.5 text-[13px] text-muted-foreground">
                  {t("bills.viewTradeEmpty")}
                </p>
              )}
            </div>
          </div>
        ) : null}

        <DialogFooter>
          {info ? (
            <Button variant="outline" asChild>
              <Link to={manageHref} onClick={() => onOpenChange(false)}>
                <UsersRound data-slot="icon" />
                {t("calendar.manageUser")}
              </Link>
            </Button>
          ) : null}
          {info?.tradeUrl ? (
            <Button variant="ghost" asChild>
              <a href={info.tradeUrl} target="_blank" rel="noopener noreferrer">
                <ExternalLink data-slot="icon" />
                {t("common.openLink")}
              </a>
            </Button>
          ) : null}
          <Button type="button" onClick={() => onOpenChange(false)}>
            {t("common.close")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
