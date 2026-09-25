import * as React from "react"
import { useTranslation } from "react-i18next"
import { Gift } from "lucide-react"

import { recordGoalCustomerBenefits } from "@/api/endpoints"
import { useAppMutation } from "@/api/mutations"
import type { CustomerBenefitType, RecordCustomerBenefitsInput } from "@/api/types"
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Textarea } from "@/components/ui/textarea"

type SupportedBenefitType = Extract<CustomerBenefitType, "extension" | "price_discount">

type CustomerBenefitDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  subscriptionIds: number[]
  suggestedType?: CustomerBenefitType
  targetLabel?: string
  onSuccess?: () => void
}

function shanghaiToday() {
  const parts = new Intl.DateTimeFormat("en-US", {
    timeZone: "Asia/Shanghai",
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
  }).formatToParts(new Date())
  const value = (type: "year" | "month" | "day") =>
    parts.find((part) => part.type === type)?.value ?? ""
  return `${value("year")}-${value("month")}-${value("day")}`
}

function supportedBenefitType(type?: CustomerBenefitType): SupportedBenefitType {
  return type === "price_discount" || type === "price_increase_thanks"
    ? "price_discount"
    : "extension"
}

function CustomerBenefitForm({
  onOpenChange,
  subscriptionIds,
  suggestedType,
  targetLabel,
  onSuccess,
}: Omit<CustomerBenefitDialogProps, "open">) {
  const { t } = useTranslation()
  const fieldID = React.useId()
  const initialBenefitType = supportedBenefitType(suggestedType)
  const [benefitType, setBenefitType] = React.useState<SupportedBenefitType>(initialBenefitType)
  const [benefitName, setBenefitName] = React.useState(() =>
    t(`goals.care.defaultBenefitName.${initialBenefitType}`),
  )
  const [actualCost, setActualCost] = React.useState("")
  const [perceivedValue, setPerceivedValue] = React.useState("")
  const [benefitDate, setBenefitDate] = React.useState(shanghaiToday)
  const [note, setNote] = React.useState("")

  const mutation = useAppMutation(
    (input: RecordCustomerBenefitsInput) => recordGoalCustomerBenefits(input),
    {
      scope: "goals",
      onSuccess: () => {
        onOpenChange(false)
        onSuccess?.()
      },
    },
  )

  const summary = targetLabel && subscriptionIds.length === 1
    ? t("goals.care.dialog.singleSummary", { name: targetLabel })
    : t("goals.care.dialog.summary", { count: subscriptionIds.length })

  return (
    <DialogContent aria-describedby={undefined} className="sm:max-w-xl">
        <DialogHeader>
          <div className="flex items-center gap-3">
            <span className="grid size-9 shrink-0 place-items-center rounded-lg border border-gold/20 bg-gold/[0.08] text-gold shadow-sm">
              <Gift className="size-4" aria-hidden="true" />
            </span>
            <DialogTitle>{t("goals.care.dialog.title")}</DialogTitle>
          </div>
        </DialogHeader>
        <form
          className="grid gap-4"
          onSubmit={(event) => {
            event.preventDefault()
            mutation.mutate({
              subscription_ids: subscriptionIds,
              benefit_type: benefitType,
              benefit_name: benefitName,
              actual_cost_yuan: actualCost,
              perceived_value_yuan: perceivedValue,
              benefit_date: benefitDate,
              note,
            })
          }}
        >
          <div className="rounded-md border border-gold/15 bg-gold/[0.045] p-3 text-xs leading-5 text-muted-foreground">
            {summary}
          </div>
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="grid gap-2">
              <Label htmlFor={`${fieldID}-type`}>{t("goals.care.dialog.type")}</Label>
              <Select
                value={benefitType}
                onValueChange={(value) => {
                  const nextType = value as SupportedBenefitType
                  setBenefitType(nextType)
                  setBenefitName(t(`goals.care.defaultBenefitName.${nextType}`))
                }}
              >
                <SelectTrigger id={`${fieldID}-type`}>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {(["extension", "price_discount"] as SupportedBenefitType[]).map((type) => (
                    <SelectItem key={type} value={type}>
                      {t(`goals.care.benefitType.${type}`)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="grid gap-2">
              <Label htmlFor={`${fieldID}-date`}>{t("goals.care.dialog.date")}</Label>
              <Input
                id={`${fieldID}-date`}
                type="date"
                max={shanghaiToday()}
                value={benefitDate}
                onChange={(event) => setBenefitDate(event.target.value)}
                required
              />
            </div>
          </div>
          <div className="grid gap-2">
            <Label htmlFor={`${fieldID}-name`}>{t("goals.care.dialog.name")}</Label>
            <Input
              id={`${fieldID}-name`}
              value={benefitName}
              onChange={(event) => setBenefitName(event.target.value)}
              placeholder={t(`goals.care.dialog.namePlaceholder.${benefitType}`)}
              required
            />
          </div>
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="grid gap-2">
              <Label htmlFor={`${fieldID}-cost`}>{t("goals.care.dialog.actualCost")}</Label>
              <div className="relative">
                <span className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-sm text-muted-foreground">¥</span>
                <Input
                  id={`${fieldID}-cost`}
                  className="pl-7 tabular-nums"
                  inputMode="decimal"
                  value={actualCost}
                  onChange={(event) => setActualCost(event.target.value)}
                  placeholder="0.00"
                />
              </div>
              <p className="text-[11px] text-muted-foreground">
                {t("goals.care.dialog.actualCostHint")}
              </p>
            </div>
            <div className="grid gap-2">
              <Label htmlFor={`${fieldID}-value`}>{t("goals.care.dialog.perceivedValue")}</Label>
              <div className="relative">
                <span className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-sm text-muted-foreground">¥</span>
                <Input
                  id={`${fieldID}-value`}
                  className="pl-7 tabular-nums"
                  inputMode="decimal"
                  value={perceivedValue}
                  onChange={(event) => setPerceivedValue(event.target.value)}
                  placeholder="0.00"
                />
              </div>
              <p className="text-[11px] text-muted-foreground">
                {t("goals.care.dialog.perceivedValueHint")}
              </p>
            </div>
          </div>
          <div className="grid gap-2">
            <Label htmlFor={`${fieldID}-note`}>{t("goals.care.dialog.note")}</Label>
            <Textarea
              id={`${fieldID}-note`}
              value={note}
              onChange={(event) => setNote(event.target.value)}
              placeholder={t("goals.care.dialog.notePlaceholder")}
              rows={3}
            />
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              {t("common.cancel")}
            </Button>
            <Button
              type="submit"
              disabled={mutation.isPending || subscriptionIds.length === 0 || !benefitName.trim()}
            >
              <Gift />
              {mutation.isPending ? t("common.saving") : t("goals.care.dialog.confirm")}
            </Button>
          </DialogFooter>
        </form>
    </DialogContent>
  )
}

export function CustomerBenefitDialog({
  open,
  onOpenChange,
  subscriptionIds,
  suggestedType,
  targetLabel,
  onSuccess,
}: CustomerBenefitDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      {open ? (
        <CustomerBenefitForm
          onOpenChange={onOpenChange}
          subscriptionIds={subscriptionIds}
          suggestedType={suggestedType}
          targetLabel={targetLabel}
          onSuccess={onSuccess}
        />
      ) : null}
    </Dialog>
  )
}
