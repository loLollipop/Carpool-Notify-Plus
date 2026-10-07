import * as React from "react"
import { useTranslation } from "react-i18next"
import { Gift } from "lucide-react"

import { recordGoalCustomerBenefits } from "@/api/endpoints"
import { useAppMutation } from "@/api/mutations"
import type {
  CustomerBenefitType,
  ExtensionReviewSnapshot,
  RecordCustomerBenefitsInput,
} from "@/api/types"
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

const maximumExtensionDays = 365

type CustomerBenefitDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  subscriptionIds: number[]
  suggestedType?: CustomerBenefitType
  targetLabel?: string
  currentPriceCents?: number
  extensionReviewSnapshots: ExtensionReviewSnapshot[]
  onSuccess?: () => void
}

function formatCents(cents: number) {
  return (cents / 100).toFixed(2)
}

function parsePositiveYuanToCents(value: string) {
  if (!/^(?:\d+|\d*\.\d{1,2})$/.test(value)) return null
  const cents = Math.round(Number(value) * 100)
  return Number.isSafeInteger(cents) && cents > 0 ? cents : null
}

function createBenefitOperationKey() {
  if (typeof globalThis.crypto?.randomUUID === "function") {
    return globalThis.crypto.randomUUID()
  }
  return `fallback-${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}`
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
  currentPriceCents,
  extensionReviewSnapshots,
  onSuccess,
}: Omit<CustomerBenefitDialogProps, "open">) {
  const { t } = useTranslation()
  const fieldID = React.useId()
  const [operationKey] = React.useState(createBenefitOperationKey)
  const [reviewedSubscriptionIds] = React.useState(subscriptionIds)
  const [reviewSnapshots] = React.useState(extensionReviewSnapshots)
  const initialBenefitType = supportedBenefitType(suggestedType)
  const [benefitType, setBenefitType] = React.useState<SupportedBenefitType>(initialBenefitType)
  const [extensionDays, setExtensionDays] = React.useState("")
  const [extensionDaysTouched, setExtensionDaysTouched] = React.useState(false)
  const [priceDiscount, setPriceDiscount] = React.useState("")
  const [priceDiscountTouched, setPriceDiscountTouched] = React.useState(false)
  const [actualCost, setActualCost] = React.useState("")
  const [perceivedValue, setPerceivedValue] = React.useState("")
  const [benefitDate, setBenefitDate] = React.useState(shanghaiToday)
  const [note, setNote] = React.useState("")

  const mutation = useAppMutation(
    (input: RecordCustomerBenefitsInput) => recordGoalCustomerBenefits(input),
    {
      scope: "all",
      onSuccess: () => {
        onOpenChange(false)
        onSuccess?.()
      },
    },
  )

  const summaryKey = targetLabel && reviewedSubscriptionIds.length === 1
    ? `goals.care.dialog.singleSummary.${benefitType}`
    : `goals.care.dialog.summary.${benefitType}`
  const summary = t(summaryKey, { name: targetLabel, count: reviewedSubscriptionIds.length })
  const parsedExtensionDays = Number(extensionDays)
  const hasValidExtensionDays = /^\d+$/.test(extensionDays)
    && Number.isInteger(parsedExtensionDays)
    && parsedExtensionDays >= 1
    && parsedExtensionDays <= maximumExtensionDays
  const extensionBenefitName = hasValidExtensionDays
    ? t("goals.care.dialog.extensionBenefitName", { days: parsedExtensionDays })
    : ""
  const priceDiscountCents = parsePositiveYuanToCents(priceDiscount)
  const discountExceedsKnownPrice = priceDiscountCents !== null
    && currentPriceCents !== undefined
    && priceDiscountCents >= currentPriceCents
  const hasValidPriceDiscount = priceDiscountCents !== null && !discountExceedsKnownPrice

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
            if (benefitType === "extension" && !hasValidExtensionDays) {
              setExtensionDaysTouched(true)
              return
            }
            if (benefitType === "price_discount" && !hasValidPriceDiscount) {
              setPriceDiscountTouched(true)
              return
            }
            mutation.mutate({
              subscription_ids: reviewedSubscriptionIds,
              benefit_type: benefitType,
              benefit_name: benefitType === "extension" ? extensionBenefitName : "",
              operation_key: operationKey,
              extension_days: benefitType === "extension" ? parsedExtensionDays : 0,
              price_discount_yuan: benefitType === "price_discount" ? priceDiscount : "",
              actual_cost_yuan: actualCost,
              perceived_value_yuan: perceivedValue,
              benefit_date: benefitDate,
              note,
              extension_review_snapshots: benefitType === "extension"
                ? reviewSnapshots
                : undefined,
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
                  setExtensionDaysTouched(false)
                  setPriceDiscountTouched(false)
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
          {benefitType === "extension" ? (
            <div className="grid gap-2">
              <Label htmlFor={`${fieldID}-extension-days`}>
                {t("goals.care.dialog.extensionDays")}
              </Label>
              <div className="flex">
                <Input
                  id={`${fieldID}-extension-days`}
                  className="rounded-r-none border-r-0 tabular-nums focus-visible:z-10"
                  type="number"
                  inputMode="numeric"
                  min={1}
                  max={maximumExtensionDays}
                  step={1}
                  value={extensionDays}
                  onChange={(event) => setExtensionDays(event.target.value)}
                  onBlur={() => setExtensionDaysTouched(true)}
                  aria-invalid={extensionDaysTouched && !hasValidExtensionDays}
                  aria-describedby={`${fieldID}-extension-days-help`}
                  placeholder="7"
                  required
                  autoFocus
                />
                <span className="inline-flex h-9 shrink-0 items-center rounded-r-md border border-input bg-muted/40 px-3 text-sm text-muted-foreground">
                  {t("goals.care.dialog.daysUnit")}
                </span>
              </div>
              <p
                id={`${fieldID}-extension-days-help`}
                className={extensionDaysTouched && !hasValidExtensionDays
                  ? "text-xs text-destructive"
                  : "text-xs text-muted-foreground"}
              >
                {extensionDaysTouched && !hasValidExtensionDays
                  ? t("goals.care.dialog.extensionDaysError", { max: maximumExtensionDays })
                  : hasValidExtensionDays
                    ? t("goals.care.dialog.extensionDaysPreview", { days: parsedExtensionDays })
                    : t("goals.care.dialog.extensionDaysHint", { max: maximumExtensionDays })}
              </p>
            </div>
          ) : (
            <div className="grid gap-2">
              <Label htmlFor={`${fieldID}-price-discount`}>
                {t("goals.care.dialog.priceDiscount")}
              </Label>
              <div className="relative">
                <span className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-sm text-muted-foreground">¥</span>
                <Input
                  id={`${fieldID}-price-discount`}
                  className="pl-7 tabular-nums"
                  inputMode="decimal"
                  value={priceDiscount}
                  onChange={(event) => setPriceDiscount(event.target.value)}
                  onBlur={() => setPriceDiscountTouched(true)}
                  aria-invalid={priceDiscountTouched && !hasValidPriceDiscount}
                  aria-describedby={`${fieldID}-price-discount-help`}
                  placeholder="10.00"
                  required
                  autoFocus
                />
              </div>
              <p
                id={`${fieldID}-price-discount-help`}
                className={priceDiscountTouched && !hasValidPriceDiscount
                  ? "text-xs text-destructive"
                  : "text-xs text-muted-foreground"}
              >
                {priceDiscountTouched && discountExceedsKnownPrice
                  ? t("goals.care.dialog.priceDiscountTooLarge")
                  : priceDiscountTouched && priceDiscountCents === null
                    ? t("goals.care.dialog.priceDiscountError")
                    : hasValidPriceDiscount && currentPriceCents !== undefined
                      ? t("goals.care.dialog.priceDiscountKnownPreview", {
                          current: formatCents(currentPriceCents),
                          next: formatCents(currentPriceCents - priceDiscountCents),
                        })
                      : hasValidPriceDiscount
                        ? t("goals.care.dialog.priceDiscountPreview", {
                            discount: formatCents(priceDiscountCents),
                          })
                        : t("goals.care.dialog.priceDiscountHint")}
              </p>
            </div>
          )}
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
                {t(benefitType === "price_discount"
                  ? "goals.care.dialog.actualCostDiscountHint"
                  : "goals.care.dialog.actualCostHint")}
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
          <p className="text-xs text-muted-foreground">
            {t("goals.care.dialog.emailNotice")}
          </p>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              {t("common.cancel")}
            </Button>
            <Button
              type="submit"
              disabled={mutation.isPending
                || reviewedSubscriptionIds.length === 0
                || (benefitType === "extension"
                  && reviewSnapshots.length !== reviewedSubscriptionIds.length)
                || (benefitType === "extension" && !hasValidExtensionDays)
                || (benefitType === "price_discount" && !hasValidPriceDiscount)}
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
  currentPriceCents,
  extensionReviewSnapshots,
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
          currentPriceCents={currentPriceCents}
          extensionReviewSnapshots={extensionReviewSnapshots}
          onSuccess={onSuccess}
        />
      ) : null}
    </Dialog>
  )
}
