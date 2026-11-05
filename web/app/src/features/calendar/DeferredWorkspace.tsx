import * as React from "react"
import { Eye, Pause, Play, Receipt, Search, UserRoundX } from "lucide-react"
import { useTranslation } from "react-i18next"

import { completeDeferredSubscriptions, resumeDeferredSubscription } from "@/api/endpoints"
import { useAppMutation } from "@/api/mutations"
import type { SubscriptionView } from "@/api/types"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import { Input } from "@/components/ui/input"
import { formatAccountLabel } from "@/lib/account-display"

export function DeferredWorkspace({
  subscriptions,
  onView,
  onPaid,
}: {
  subscriptions: SubscriptionView[]
  onView: (subscription: SubscriptionView) => void
  onPaid: (subscription: SubscriptionView) => void
}) {
  const { t } = useTranslation()
  const [search, setSearch] = React.useState("")
  const [selected, setSelected] = React.useState<Map<number, string>>(new Map())
  const [reviewed, setReviewed] = React.useState<SubscriptionView[] | null>(null)
  const [resumeTarget, setResumeTarget] = React.useState<SubscriptionView | null>(null)
  const completeMutation = useAppMutation(completeDeferredSubscriptions, {
    scope: "subscriptions",
    onSuccess: () => { setReviewed(null); setSelected(new Map()) },
  })
  const resumeMutation = useAppMutation(resumeDeferredSubscription, {
    scope: "subscriptions",
    onSuccess: () => setResumeTarget(null),
  })
  const filtered = subscriptions.filter((view) => [
    view.subscription.customer_email, view.subscription.customer_wechat,
    view.subscription.name, view.account_display_email, view.account_name,
    String(view.account_serial), view.seat_name,
  ].some((value) => value?.toLowerCase().includes(search.trim().toLowerCase())))
  // A payment or edit invalidates an earlier selection, even if the row remains.
  const selectedViews = filtered.filter((view) => selected.get(view.subscription.id) === view.subscription.updated_at)
  const allSelected = filtered.length > 0 && selectedViews.length === filtered.length
  const toggle = (view: SubscriptionView, checked: boolean) => setSelected((previous) => {
    const next = new Map(previous)
    if (checked) next.set(view.subscription.id, view.subscription.updated_at)
    else next.delete(view.subscription.id)
    return next
  })

  return (
    <div className="grid gap-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h2 className="text-sm font-semibold">{t("calendar.deferred.title", { count: subscriptions.length })}</h2>
          <p className="mt-1 text-xs leading-relaxed text-muted-foreground">{t("calendar.deferred.description")}</p>
        </div>
        <Button
          variant="destructive"
          size="sm"
          disabled={selectedViews.length === 0 || completeMutation.isPending || resumeMutation.isPending}
          onClick={() => setReviewed(selectedViews)}
        >
          <UserRoundX />{t("calendar.deferred.completeSelected", { count: selectedViews.length })}
        </Button>
      </div>
      {subscriptions.length > 0 ? (
        <div className="flex flex-wrap items-center gap-3">
          <label className="flex items-center gap-2 text-xs">
            <Checkbox
              checked={allSelected ? true : selectedViews.length > 0 ? "indeterminate" : false}
              aria-label={t("calendar.deferred.selectAll")}
              onCheckedChange={(checked) => {
                setSelected(new Map(checked === true ? filtered.map((view) => [view.subscription.id, view.subscription.updated_at]) : []))
              }}
            />
            {t("calendar.deferred.selectAll")}
          </label>
          <div className="relative ml-auto w-full sm:w-72">
            <Search className="absolute left-3 top-2.5 size-4 text-muted-foreground" />
            <Input className="pl-9" placeholder={t("calendar.deferred.search")} aria-label={t("calendar.deferred.search")} value={search} onChange={(event) => setSearch(event.target.value)} />
          </div>
        </div>
      ) : null}
      {filtered.length === 0 ? (
        <div className="grid min-h-60 place-items-center rounded-lg border border-dashed p-5 text-center">
          <div><Pause className="mx-auto size-7 text-muted-foreground" /><p className="mt-3 text-sm text-muted-foreground">{t("calendar.deferred.empty")}</p></div>
        </div>
      ) : filtered.map((view) => {
        const id = view.subscription.id
        const customer = view.subscription.customer_email || view.subscription.customer_wechat || view.subscription.name
        return (
          <div key={id} className="flex min-w-0 items-start gap-3 rounded-lg border bg-card p-4">
            <Checkbox className="mt-1" checked={selected.get(id) === view.subscription.updated_at} aria-label={t("calendar.deferred.selectCustomer", { name: customer })} onCheckedChange={(checked) => toggle(view, checked === true)} />
            <div className="min-w-0 flex-1">
              <div className="flex flex-wrap items-start justify-between gap-2">
                <button className="min-w-0 break-all text-left text-sm font-semibold hover:text-brand" onClick={() => onView(view)}>{customer}</button>
                <Badge variant={view.account_paused ? "warning" : "secondary"}>
                  {t(view.account_paused ? "accounts.statusPaused" : "calendar.deferred.accountAvailable")}
                </Badge>
              </div>
              <p className="mt-1 break-all text-xs text-muted-foreground">{formatAccountLabel(view.account_serial, view.account_display_email || view.account_name, "—", view.account_space_role)} · {view.seat_name}</p>
              <div className="mt-2 flex flex-wrap gap-x-5 gap-y-1 text-xs text-muted-foreground">
                <span>{t("calendar.deferred.originalDue", { date: view.subscription.renewal_deferred_due_date })}</span>
                <span>{t("calendar.deferred.since", { date: view.deferred_at_label })}</span>
                {view.subscription.customer_wechat ? <span>{view.subscription.customer_wechat}</span> : null}
              </div>
              <div className="mt-3 flex flex-wrap gap-2">
                <Button size="sm" variant="outline" onClick={() => onView(view)}><Eye />{t("calendar.viewInfo")}</Button>
                <Button size="sm" variant="outline" onClick={() => onPaid(view)}><Receipt />{t("calendar.work.recordPaid")}</Button>
                <Button size="sm" variant="ghost" disabled={resumeMutation.isPending} onClick={() => setResumeTarget(view)}><Play />{t("calendar.deferred.resume")}</Button>
              </div>
            </div>
          </div>
        )
      })}
      <ConfirmDialog
        open={reviewed !== null}
        onOpenChange={(open) => { if (!open && !completeMutation.isPending) setReviewed(null) }}
        title={t("calendar.deferred.completeTitle")}
        description={t("calendar.deferred.completeDescription", { count: reviewed?.length ?? 0 })}
        actionLabel={t("calendar.deferred.completeAction")}
        destructive
        pending={completeMutation.isPending}
        onConfirm={() => { if (reviewed) completeMutation.mutate(reviewed) }}
      />
      <ConfirmDialog
        open={resumeTarget !== null}
        onOpenChange={(open) => { if (!open && !resumeMutation.isPending) setResumeTarget(null) }}
        title={t("calendar.deferred.resumeTitle")}
        description={t("calendar.deferred.resumeDescription")}
        actionLabel={t("calendar.deferred.resume")}
        pending={resumeMutation.isPending}
        onConfirm={() => { if (resumeTarget) resumeMutation.mutate(resumeTarget.subscription.id) }}
      />
    </div>
  )
}
