import { Link } from "react-router-dom"
import { Mail, Receipt, UserRound } from "lucide-react"
import { useTranslation } from "react-i18next"

import type { OperationTask, SubscriptionView } from "@/api/types"
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
import { Skeleton } from "@/components/ui/skeleton"
import { formatAccountLabel } from "@/lib/account-display"
import { maskAmount } from "@/lib/amount-privacy"

function DetailItem({
  label,
  mono = false,
  children,
}: {
  label: string
  mono?: boolean
  children: React.ReactNode
}) {
  return (
    <div className="min-w-0">
      <div className="text-[11px] font-medium text-muted-foreground">{label}</div>
      <div className={mono ? "break-all font-mono text-[13px]" : "break-words text-[13px]"}>
        {children}
      </div>
    </div>
  )
}

function visibleRemark(remark: string) {
  return remark
    .split(/\r?\n/)
    .filter((line) => !/^兑换码\s*[：:]/.test(line.trim()))
    .join("\n")
    .trim()
}

export function OperationSubscriptionDialog({
  open,
  onOpenChange,
  task,
  subscription,
  loading,
  loadFailed,
  amountsHidden,
  onRecordPayment,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  task: OperationTask | null
  subscription: SubscriptionView | null
  loading: boolean
  loadFailed: boolean
  amountsHidden: boolean
  onRecordPayment: () => void
}) {
  const { t } = useTranslation()
  const detail = subscription?.subscription
  const remark = visibleRemark(detail?.remark ?? "")
  const accountLabel = subscription
    ? formatAccountLabel(
        subscription.account_serial,
        subscription.account_display_email || subscription.account_name || detail?.name || "",
        "—",
        subscription.account_space_role,
      )
    : task
      ? formatAccountLabel(
          task.account_serial,
          task.account_display_email || task.account_name,
          "—",
          task.account_space_role,
        )
      : "—"
  const customerEmail = detail?.customer_email || task?.customer_email || ""
  const customerWechat = detail?.customer_wechat || task?.customer_wechat || ""
  const seatName = subscription?.seat_name || task?.seat_name || ""
  const priceYuan = subscription?.price_yuan || task?.amount_yuan || ""
  const amountDueYuan = task?.amount_yuan || priceYuan
  const cycleDesc = subscription?.cycle_desc || task?.cycle_desc || ""
  const nextDueDate = subscription?.next_due_date || task?.due_date || ""
  const userPath = task?.subscription_id ? `/users?subscription=${task.subscription_id}` : "/users"

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="grid max-h-[88dvh] grid-rows-[auto_minmax(0,1fr)_auto] gap-0 overflow-hidden p-0 sm:max-w-xl">
        <DialogHeader className="border-b px-5 py-4 pr-12 sm:px-6">
          <div className="flex flex-wrap items-center gap-2">
            <DialogTitle>{t("dash.workbench.userInfoTitle")}</DialogTitle>
            <Badge variant="outline" className="font-normal">
              {task?.kind
                ? t(`dash.workbench.taskKinds.${task.kind}`)
                : t("dash.workbench.taskKinds.team_due")}
            </Badge>
          </div>
          <DialogDescription>{t("dash.workbench.userInfoDesc")}</DialogDescription>
        </DialogHeader>

        <div className="min-h-0 overflow-y-auto overscroll-contain px-5 py-5 sm:px-6">
          {loading ? (
            <div className="grid gap-4" aria-label={t("common.loading")}>
              <Skeleton className="h-16 rounded-lg" />
              <Skeleton className="h-28 rounded-lg" />
              <Skeleton className="h-20 rounded-lg" />
            </div>
          ) : (
            <div className="grid gap-4">
              {loadFailed ? (
                <div className="rounded-lg border border-gold/25 bg-gold/[0.06] px-3 py-2.5">
                  <p className="text-xs font-medium text-gold">
                    {t("dash.workbench.userInfoLoadFailed")}
                  </p>
                  <p className="mt-0.5 text-[11px] leading-relaxed text-muted-foreground">
                    {t("dash.workbench.userInfoLoadFailedHint")}
                  </p>
                </div>
              ) : null}
              <section className="rounded-lg border bg-muted/25 p-4">
                <div className="flex min-w-0 items-start gap-3">
                  <span className="grid size-10 shrink-0 place-items-center rounded-lg bg-brand/10 text-brand">
                    <UserRound className="size-5" aria-hidden="true" />
                  </span>
                  <div className="min-w-0">
                    <h3 className="break-words text-sm font-semibold">{accountLabel}</h3>
                    <div className="mt-1.5 flex flex-wrap gap-1.5">
                      {seatName ? <Badge variant="secondary">{seatName}</Badge> : null}
                      <Badge variant={task?.kind === "team_overdue" ? "destructive" : "warning"}>
                        {task?.kind
                          ? t(`dash.workbench.taskKinds.${task.kind}`)
                          : t("dash.workbench.taskKinds.team_due")}
                      </Badge>
                      {subscription?.next_price_yuan ? (
                        <Badge
                          variant="outline"
                          className="border-gold/25 bg-gold/[0.07] font-normal text-gold"
                        >
                          {t("cards.nextPrice", {
                            price: maskAmount(
                              amountsHidden,
                              `¥${subscription.next_price_yuan}`,
                            ),
                          })}
                          {subscription.next_price_effective_due_date
                            ? ` · ${subscription.next_price_effective_due_date}`
                            : ""}
                        </Badge>
                      ) : null}
                    </div>
                  </div>
                </div>
              </section>

              <section>
                <h4 className="text-xs font-semibold text-muted-foreground">
                  {t("dash.workbench.contactInfo")}
                </h4>
                <div className="mt-2 grid gap-2 sm:grid-cols-2">
                  <div className="flex min-w-0 items-start gap-2 rounded-lg border border-brand/10 bg-brand/[0.05] p-3">
                    <Mail className="mt-0.5 size-4 shrink-0 text-brand" aria-hidden="true" />
                    <DetailItem label={t("subscriptionDialog.customerEmail")} mono>
                      {customerEmail || t("cards.contactMissing")}
                    </DetailItem>
                  </div>
                  <div className="flex min-w-0 items-start gap-2 rounded-lg border border-success/10 bg-success/[0.05] p-3">
                    <WeChatIcon className="mt-0.5 size-4 shrink-0 text-success" />
                    <DetailItem label={t("subscriptionDialog.customerWechat")}>
                      {customerWechat || t("cards.contactMissing")}
                    </DetailItem>
                  </div>
                </div>
              </section>

              <section className="grid grid-cols-2 gap-x-4 gap-y-3 rounded-lg border p-4 sm:grid-cols-3">
                <DetailItem label={t("cards.perPerson")}>
                  <span className="font-medium tabular-nums">
                    {priceYuan ? maskAmount(amountsHidden, `¥${priceYuan}`) : "—"}
                  </span>
                </DetailItem>
                <DetailItem label={t("dash.workbench.amountDue")}>
                  <span className="font-semibold text-brand tabular-nums">
                    {amountDueYuan ? maskAmount(amountsHidden, `¥${amountDueYuan}`) : "—"}
                  </span>
                </DetailItem>
                <DetailItem label={t("cards.cost")}>
                  <span className="font-medium text-gold tabular-nums">
                    {subscription
                      ? maskAmount(
                          amountsHidden,
                          `¥${subscription.allocated_cost_yuan || subscription.cost_yuan}`,
                        )
                      : "—"}
                  </span>
                </DetailItem>
                <DetailItem label={t("cards.profit")}>
                  <span className="font-medium text-success tabular-nums">
                    {subscription
                      ? maskAmount(
                          amountsHidden,
                          `¥${subscription.allocated_profit_yuan || subscription.profit_yuan}`,
                        )
                      : "—"}
                  </span>
                </DetailItem>
                <DetailItem label={t("cards.cycle")}>{cycleDesc || "—"}</DetailItem>
                <DetailItem label={t("cards.nextDue")}>
                  <span className="tabular-nums">{nextDueDate || "—"}</span>
                </DetailItem>
                <DetailItem label={t("cards.boardedAt")}>
                  <span className="tabular-nums">{subscription?.boarded_at || "—"}</span>
                </DetailItem>
              </section>

              <section>
                <h4 className="text-[11px] font-medium text-muted-foreground">
                  {t("dash.workbench.userRemark")}
                </h4>
                <p className="mt-1 min-h-10 whitespace-pre-wrap break-words rounded-lg border bg-muted/20 px-3 py-2.5 text-[13px] leading-relaxed">
                  {remark || "—"}
                </p>
              </section>
            </div>
          )}
        </div>

        <DialogFooter className="border-t bg-muted/20 px-5 py-4 sm:px-6">
          <Button variant="outline" asChild>
            <Link to={userPath} onClick={() => onOpenChange(false)}>
              {t("dash.workbench.openUserPage")}
            </Link>
          </Button>
          <Button type="button" onClick={onRecordPayment} disabled={!task?.subscription_id}>
            <Receipt data-slot="icon" />
            {t("dash.workbench.recordPaid")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
