import * as React from "react"
import { useTranslation } from "react-i18next"

import {
  revokeGoalCustomerBenefitExtension,
  updateGoalCustomerBenefitExtension,
} from "@/api/endpoints"
import { useAppMutation } from "@/api/mutations"
import type { CustomerBenefitView } from "@/api/types"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"

function operationKey() {
  if (typeof globalThis.crypto?.randomUUID === "function") {
    return `extension-revision-${globalThis.crypto.randomUUID()}`
  }
  return `extension-revision-fallback-${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}`
}

export function ExtensionRevisionDialog({
  benefit,
  mode,
  onOpenChange,
}: {
  benefit: CustomerBenefitView | null
  mode: "edit" | "revoke"
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation()
  const [days, setDays] = React.useState(benefit?.extension_days ?? 1)
  const [reason, setReason] = React.useState("")
  const [key] = React.useState(operationKey)

  const mutation = useAppMutation(
    () => {
      if (!benefit) throw new Error(t("goals.care.extensionRevision.missing"))
      if (mode === "revoke") {
        return revokeGoalCustomerBenefitExtension(benefit.id, {
          reason: reason.trim(),
          operation_key: key,
        })
      }
      return updateGoalCustomerBenefitExtension(benefit.id, {
        extension_days: days,
        reason: reason.trim(),
        operation_key: key,
      })
    },
    { scope: "all", onSuccess: () => onOpenChange(false) },
  )

  const validDays = Number.isInteger(days) && days >= 1 && days <= 365
  const canSubmit = reason.trim().length > 0 && reason.trim().length <= 500 &&
    (mode === "revoke" || validDays) && !mutation.isPending

  return (
    <Dialog open={benefit !== null} onOpenChange={onOpenChange}>
      <DialogContent aria-describedby={undefined} className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>
            {t(`goals.care.extensionRevision.${mode}Title`)}
          </DialogTitle>
        </DialogHeader>
        <form
          aria-busy={mutation.isPending}
          className="grid gap-4"
          onSubmit={(event) => {
            event.preventDefault()
            if (canSubmit) mutation.mutate()
          }}
        >
          <p className="text-sm leading-6 text-muted-foreground">
            {t(`goals.care.extensionRevision.${mode}Summary`, {
              days: benefit?.extension_days ?? 0,
            })}
          </p>
          {mode === "edit" ? (
            <div className="grid gap-2">
              <Label htmlFor="extension-revision-days">
                {t("goals.care.extensionRevision.days")}
              </Label>
              <Input
                id="extension-revision-days"
                type="number"
                min={1}
                max={365}
                value={days}
                onChange={(event) => setDays(Number(event.target.value))}
              />
            </div>
          ) : null}
          <div className="grid gap-2">
            <Label htmlFor="extension-revision-reason">
              {t("goals.care.extensionRevision.reason")}
            </Label>
            <Textarea
              id="extension-revision-reason"
              rows={3}
              maxLength={500}
              value={reason}
              placeholder={t("goals.care.extensionRevision.reasonPlaceholder")}
              onChange={(event) => setReason(event.target.value)}
            />
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              {t("common.cancel")}
            </Button>
            <Button type="submit" variant={mode === "revoke" ? "destructive" : "default"} disabled={!canSubmit}>
              {mutation.isPending
                ? t("common.saving")
                : t(`goals.care.extensionRevision.${mode}Submit`)}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
