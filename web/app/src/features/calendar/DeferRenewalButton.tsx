import * as React from "react"
import { Pause } from "lucide-react"
import { useTranslation } from "react-i18next"

import { deferSubscription } from "@/api/endpoints"
import { useAppMutation } from "@/api/mutations"
import type { SubscriptionView } from "@/api/types"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { Button } from "@/components/ui/button"

export function DeferRenewalButton({
  subscription,
  onSuccess,
}: {
  subscription: SubscriptionView
  onSuccess?: () => void
}) {
  const { t } = useTranslation()
  const [reviewed, setReviewed] = React.useState<SubscriptionView | null>(null)
  const mutation = useAppMutation((view: SubscriptionView) => deferSubscription(view), {
    scope: "subscriptions",
    onSuccess: () => { setReviewed(null); onSuccess?.() },
  })
  if (subscription.subscription.business_type === "plus"
    || subscription.subscription.archived_at
    || subscription.cancellation_pending
    || subscription.renewal_deferred
    || subscription.days_remaining > 0) return null

  return (
    <>
      <Button variant="outline" size="sm" onClick={() => setReviewed(subscription)}>
        <Pause />{t("calendar.deferred.defer")}
      </Button>
      <ConfirmDialog
        open={reviewed !== null}
        onOpenChange={(open) => { if (!open && !mutation.isPending) setReviewed(null) }}
        title={t("calendar.deferred.confirmTitle")}
        description={t("calendar.deferred.confirmDescription", {
          name: reviewed?.subscription.customer_email || reviewed?.subscription.name,
          date: reviewed?.next_due_date,
        })}
        actionLabel={t("calendar.deferred.defer")}
        pending={mutation.isPending}
        onConfirm={() => { if (reviewed) mutation.mutate(reviewed) }}
      />
    </>
  )
}
