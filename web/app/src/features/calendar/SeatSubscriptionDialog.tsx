import * as React from "react"
import { useTranslation } from "react-i18next"
import { Link } from "react-router-dom"
import { ArrowUpRight, Bell, CalendarDays, Copy, ExternalLink, Mail, UserRound, UsersRound } from "lucide-react"
import { toast } from "sonner"

import { WeChatIcon } from "@/components/icons/wechat-icon"
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
import { useAmountPrivacy } from "@/hooks/use-amount-privacy"
import { maskAmount } from "@/lib/amount-privacy"
import { cn } from "@/lib/utils"
import type { SeatSubscriptionInfo } from "./seat-subscription-info"

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
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className={cn("mt-1 text-sm leading-6 [overflow-wrap:anywhere]", mono && "font-mono text-[13px]")}>
        {children}
      </dd>
    </div>
  )
}

function ContactItem({ label, value, icon: Icon }: {
  label: string
  value: string
  icon: React.ComponentType<{ className?: string; "aria-hidden"?: boolean }>
}) {
  const { t } = useTranslation()
  return (
    <div className="flex min-w-0 items-start gap-2.5 rounded-lg bg-muted/45 p-3">
      <Icon className="mt-0.5 size-4 shrink-0 text-muted-foreground" aria-hidden={true} />
      <div className="min-w-0 flex-1">
        <dt className="text-xs text-muted-foreground">{label}</dt>
        <dd className="mt-1 select-text text-sm leading-5 [overflow-wrap:anywhere]">{value || "—"}</dd>
      </div>
      {value ? (
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          className="-mr-1 -mt-1 text-muted-foreground"
          aria-label={t("calendar.preview.copy", { label })}
          title={t("calendar.preview.copy", { label })}
          onClick={async () => {
            try {
              await navigator.clipboard.writeText(value)
              toast.success(t("calendar.preview.copied", { label }))
            } catch {
              toast.error(t("calendar.preview.copyFailed"))
            }
          }}
        >
          <Copy className="size-3.5" />
        </Button>
      ) : null}
    </div>
  )
}

/** Read-only subscription details shared by task, calendar and deferred views. */
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
  const { amountsHidden } = useAmountPrivacy()
  const manageHref = React.useMemo(() => {
    if (!info) return "/users"
    if (info.businessType === "plus") {
      const params = new URLSearchParams()
      params.set("q", info.customerEmail || info.customerWechat || info.name)
      if (info.archived) params.set("filter", "archived")
      return `/plus-rentals?${params.toString()}`
    }
    const params = new URLSearchParams()
    params.set("subscription", String(info.subscriptionId))
    if (info.archived) params.set("filter", "archived")
    const query = info.customerEmail || info.customerWechat || info.accountName || info.name
    if (query) params.set("q", query)
    return `/users?${params.toString()}`
  }, [info])

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="grid max-h-[90dvh] grid-rows-[auto_minmax(0,1fr)_auto] gap-0 overflow-hidden p-0 sm:max-w-2xl sm:p-0">
        <DialogHeader className="border-b px-5 py-4 pr-12 sm:px-6">
          <DialogTitle>{t("calendar.viewSeatTitle")}</DialogTitle>
          <DialogDescription>{t("calendar.viewSeatDesc")}</DialogDescription>
        </DialogHeader>

        <div className="min-h-0 overflow-y-auto overscroll-contain px-5 py-4 sm:px-6">
          {info ? (
            <div className="grid gap-4">
              <section>
                <div className="flex min-w-0 items-start gap-3">
                  <span className="grid size-11 shrink-0 place-items-center rounded-xl bg-brand/10 text-brand">
                    <UserRound className="size-5" aria-hidden="true" />
                  </span>
                  <div className="min-w-0 flex-1">
                    <div className="mb-1 flex flex-wrap items-center gap-2">
                      <span className="text-xs text-muted-foreground">{t("calendar.preview.customer")}</span>
                      <Badge variant={info.businessType === "plus" ? "brand" : "secondary"}>
                        {info.businessType === "plus" ? "Plus" : "Team"}
                      </Badge>
                      <Badge variant={info.statusTone} className="font-normal">{info.statusLabel}</Badge>
                    </div>
                    <h3 className="select-text text-base font-semibold leading-6 [overflow-wrap:anywhere]">
                      {info.customerEmail || info.customerWechat || info.name}
                    </h3>
                  </div>
                </div>
                <dl className="mt-4 grid gap-2 sm:grid-cols-2">
                  <ContactItem
                    label={t(info.businessType === "plus" ? "plusRentals.accountEmail" : "subscriptionDialog.customerEmail")}
                    value={info.customerEmail}
                    icon={Mail}
                  />
                  <ContactItem label={t("subscriptionDialog.customerWechat")} value={info.customerWechat} icon={WeChatIcon} />
                </dl>
                <div className="mt-3 flex items-start gap-2 text-sm">
                  <UsersRound className="mt-0.5 size-4 shrink-0 text-muted-foreground" aria-hidden="true" />
                  <div className="min-w-0 leading-6">
                    <span className="mr-2 text-xs text-muted-foreground">{t("calendar.preview.account")}</span>
                    <span className="select-text [overflow-wrap:anywhere]">
                      {info.businessType === "plus" ? info.name : info.accountName || info.name}
                    </span>
                    {info.seatName ? <Badge variant="outline" className="ml-2 font-normal">{info.seatName}</Badge> : null}
                  </div>
                </div>
              </section>

              <dl className="grid grid-cols-3 divide-x rounded-xl border border-brand/15 bg-brand/[0.035] py-4">
                {[
                  { label: t("bills.viewPrice"), value: info.priceYuan, tone: "" },
                  { label: t("bills.viewCost"), value: info.costYuan, tone: "text-muted-foreground" },
                  { label: t("bills.viewProfit"), value: info.profitYuan, tone: Number(info.profitYuan) < 0 ? "text-destructive" : "text-brand" },
                ].map((item) => (
                  <div key={item.label} className="min-w-0 px-3 sm:px-5">
                    <dt className="text-xs text-muted-foreground">{item.label}</dt>
                    <dd className={cn("mt-1.5 text-base font-semibold tabular-nums [overflow-wrap:anywhere] sm:text-xl", item.tone)}>
                      {item.value ? maskAmount(amountsHidden, `¥${item.value}`) : "—"}
                    </dd>
                  </div>
                ))}
              </dl>

              <section>
                <h3 className="mb-3 flex items-center gap-2 text-sm font-semibold">
                  <CalendarDays className="size-4 text-brand" aria-hidden="true" />
                  {t("calendar.preview.billing")}
                </h3>
                <dl className="grid grid-cols-2 gap-x-5 gap-y-3">
                  <ViewItem label={t("bills.viewBoarded")}><span className="tabular-nums">{info.boardedAt || "—"}</span></ViewItem>
                  <ViewItem label={info.extraDateLabel}><span className="font-medium tabular-nums">{info.extraDate || "—"}</span></ViewItem>
                  <ViewItem label={t("bills.viewCycle")}>{info.cycleDesc}</ViewItem>
                  <ViewItem label={t("bills.viewCron")} mono>{info.cronExpr}</ViewItem>
                </dl>
              </section>

              <section className="border-t pt-4">
                <h3 className="mb-3 flex items-center gap-2 text-sm font-semibold">
                  <Bell className="size-4 text-brand" aria-hidden="true" />
                  {t("calendar.preview.notifications")}
                </h3>
                <dl className="grid gap-x-5 gap-y-3 sm:grid-cols-[1fr_2fr]">
                  <ViewItem label={t("bills.viewOffsets")}>{info.offsetsText || "—"}</ViewItem>
                  <ViewItem label={t("bills.viewChannels")}>{info.channelLabels || "—"}</ViewItem>
                </dl>
              </section>

              <dl className="grid gap-3 border-t pt-4 sm:grid-cols-2">
                <ViewItem label={t("bills.viewRemark")}>
                  <p className="whitespace-pre-wrap text-muted-foreground">{info.remark || "—"}</p>
                </ViewItem>
                <ViewItem label={t("bills.viewTradeUrl")}>
                  {info.tradeUrl ? (
                    <a href={info.tradeUrl} target="_blank" rel="noopener noreferrer" className="inline-flex max-w-full items-start gap-1.5 text-brand underline-offset-4 hover:underline">
                      <span className="min-w-0 [overflow-wrap:anywhere]">{info.tradeUrl}</span>
                      <ExternalLink className="mt-1 size-3.5 shrink-0" aria-hidden="true" />
                    </a>
                  ) : <span className="text-muted-foreground">{t("bills.viewTradeEmpty")}</span>}
                </ViewItem>
              </dl>
            </div>
          ) : null}
        </div>

        <DialogFooter className="flex-row items-center justify-end border-t bg-muted/25 px-5 py-4 sm:px-6">
          <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>{t("common.close")}</Button>
          {info ? (
            <Button asChild>
              <Link to={manageHref} onClick={() => onOpenChange(false)}>
                {t("calendar.manageUser")}
                <ArrowUpRight data-slot="icon" />
              </Link>
            </Button>
          ) : null}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
