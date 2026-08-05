import { useTranslation } from "react-i18next"
import { ExternalLink } from "lucide-react"

import type { BillView } from "@/api/types"
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
import { getNextMonthlyRenewalDate } from "@/lib/account-renewal"

function ViewItem({ label, mono, children }: { label: string; mono?: boolean; children: React.ReactNode }) {
  return (
    <div className="min-w-0">
      <div className="text-[11px] font-medium tracking-wide text-muted-foreground">{label}</div>
      <div className={mono ? "truncate font-mono text-[13px]" : "truncate text-[13px]"}>{children}</div>
    </div>
  )
}

/** Read-only subscription details for a bill row (matches旧 subscription-view-dialog). */
export function SubscriptionViewDialog({
  open,
  onOpenChange,
  bill,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  bill: BillView | null
}) {
  const { t } = useTranslation()
  const nextRenewalDate = getNextMonthlyRenewalDate(bill?.account_opened_at ?? "")
  const primaryName = bill?.account_name || bill?.subscription_name || ""
  const customerLine = bill?.customer_email || bill?.subscription_name || ""

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{t("bills.viewTitle")}</DialogTitle>
          <DialogDescription>{t("bills.viewDesc")}</DialogDescription>
        </DialogHeader>

        {bill ? (
          <div className="grid gap-4">
            <div>
              <div className="text-base font-semibold">{primaryName}</div>
              <div className="mt-1.5 flex flex-wrap items-center gap-1.5">
                <Badge variant="secondary" className="font-normal">
                  {customerLine}
                  {bill.seat_name ? ` · ${bill.seat_name}` : ""}
                </Badge>
                {bill.is_resale ? (
                  <Badge variant="outline" className="font-normal">
                    {t("cards.resale")}
                  </Badge>
                ) : null}
                <Badge
                  variant={bill.archived ? "secondary" : "success"}
                  className="font-normal"
                >
                  {bill.status_label}
                </Badge>
              </div>
            </div>

            <div className="grid grid-cols-2 gap-x-4 gap-y-2.5 sm:grid-cols-3">
              <ViewItem label={t("bills.viewPrice")}>
                <span className="tabular-nums">¥{bill.price_yuan}</span>
              </ViewItem>
              <ViewItem label={t("bills.viewCost")}>
                <span className="tabular-nums">¥{bill.cost_yuan}</span>
              </ViewItem>
              {bill.is_resale ? (
                <ViewItem label={t("cards.agencyFee")}>
                  <span className="tabular-nums">¥{bill.agency_fee_yuan}</span>
                </ViewItem>
              ) : (
                <ViewItem label={t("bills.viewProfit")}>
                  <span className="tabular-nums">¥{bill.profit_yuan}</span>
                </ViewItem>
              )}
              <ViewItem label={t("bills.viewCycle")}>{bill.cycle_desc}</ViewItem>
              <ViewItem label={t("bills.viewCron")} mono>
                {bill.cron_expr}
              </ViewItem>
              <ViewItem label={t("bills.viewOffsets")}>{bill.offsets_text || "—"}</ViewItem>
              <ViewItem label={t("bills.viewBoarded")}>
                <span className="tabular-nums">{bill.boarded_at || "—"}</span>
              </ViewItem>
              {bill.archived && bill.archived_at_label ? (
                <ViewItem label={t("bills.viewArchivedAt")}>
                  <span className="tabular-nums">{bill.archived_at_label}</span>
                </ViewItem>
              ) : null}
              <ViewItem label={t("bills.viewChannels")}>{bill.channel_labels}</ViewItem>
              <ViewItem label={t("subscriptionDialog.customerEmail")} mono>
                {bill.customer_email || "—"}
              </ViewItem>
              <ViewItem label={t("subscriptionDialog.customerWechat")} mono>
                {bill.customer_wechat || "—"}
              </ViewItem>
              <ViewItem label={t("accounts.email")} mono>
                {bill.account_email || "—"}
              </ViewItem>
              <ViewItem label={t("accounts.spaceName")}>
                {bill.account_space_name || "—"}
              </ViewItem>
              <ViewItem label={t("accounts.openedAt")}>
                <span className="tabular-nums">{bill.account_opened_at || "—"}</span>
              </ViewItem>
              <ViewItem label={t("accounts.nextRenewalAt")}>
                <span className="tabular-nums">{nextRenewalDate || "—"}</span>
              </ViewItem>
            </div>

            <div>
              <div className="text-[11px] font-medium tracking-wide text-muted-foreground">
                {t("bills.viewRemark")}
              </div>
              <p className="mt-0.5 text-[13px] leading-relaxed whitespace-pre-wrap">
                {bill.remark || "—"}
              </p>
            </div>

            <div>
              <div className="text-[11px] font-medium tracking-wide text-muted-foreground">
                {t("bills.viewTradeUrl")}
              </div>
              {bill.trade_url ? (
                <a
                  href={bill.trade_url}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="mt-0.5 block truncate text-[13px] text-brand underline-offset-2 hover:underline"
                >
                  {bill.trade_url}
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
          {bill?.trade_url ? (
            <Button variant="ghost" asChild>
              <a href={bill.trade_url} target="_blank" rel="noopener noreferrer">
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
