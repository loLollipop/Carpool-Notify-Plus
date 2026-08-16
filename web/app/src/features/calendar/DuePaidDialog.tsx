import * as React from "react"
import { useQuery } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"

import { fetchDuePeriods, setDuePaid } from "@/api/endpoints"
import { useAppMutation } from "@/api/mutations"
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
import { Label } from "@/components/ui/label"
import { useAmountPrivacy } from "@/hooks/use-amount-privacy"
import { maskAmount } from "@/lib/amount-privacy"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"

export interface DuePaidTarget {
  subscriptionId: number
  name: string
  priceYuan: string
  cycleDesc: string
  dueDate: string
  kind?: "team" | "plus"
}

export function DuePaidDialog({
  open,
  onOpenChange,
  target,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  target: DuePaidTarget | null
}) {
  const { t } = useTranslation()
  const { amountsHidden } = useAmountPrivacy()
  const plusRental = target?.kind === "plus"
  // User pick overrides the derived default; cleared whenever the dialog closes.
  const [overrideStart, setOverrideStart] = React.useState("")

  const periodsQuery = useQuery({
    queryKey: ["due-periods", target?.subscriptionId, target?.dueDate],
    queryFn: () => fetchDuePeriods(target!.subscriptionId, target!.dueDate),
    enabled: open && target !== null,
    staleTime: 0,
    gcTime: 0,
  })

  const periods = periodsQuery.data ?? []
  const defaultStart =
    (
      periods.find((period) => period.preferred && !period.paid) ??
      periods.find((period) => !period.paid)
    )?.start_date ?? ""
  const selectedStart = overrideStart !== "" ? overrideStart : defaultStart
  const selectedPeriod = periods.find((period) => period.start_date === selectedStart)
  const selectedPriceYuan = selectedPeriod?.price_yuan || target?.priceYuan || ""

  const handleOpenChange = (next: boolean) => {
    if (!next) setOverrideStart("")
    onOpenChange(next)
  }

  const confirmMutation = useAppMutation(
    () => setDuePaid(target!.subscriptionId, selectedStart, true),
    {
      successMessage: t(plusRental ? "plusRentals.renewalRecorded" : "duePaid.markedPaid"),
      onSuccess: () => handleOpenChange(false),
    },
  )

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{t(plusRental ? "plusRentals.renewalTitle" : "duePaid.title")}</DialogTitle>
          <DialogDescription className="leading-relaxed">
            {t(plusRental ? "plusRentals.renewalDesc" : "duePaid.desc")}
          </DialogDescription>
        </DialogHeader>

        <div className="grid gap-4 text-sm">
          <div className="grid gap-1">
            <span className="text-xs font-medium text-muted-foreground">
              {t(plusRental ? "plusRentals.customerName" : "duePaid.subscription")}
            </span>
            <span className="font-medium">{target?.name ?? "—"}</span>
          </div>
          <div className="grid grid-cols-2 gap-4">
            <div className="grid gap-1">
              <span className="text-xs font-medium text-muted-foreground">
                {t(plusRental ? "plusRentals.renewalPrice" : "duePaid.renewalPrice")}
              </span>
              <div className="flex flex-wrap items-center gap-1.5">
                <span className="font-semibold tabular-nums">
                  {selectedPriceYuan
                    ? maskAmount(amountsHidden, `¥${selectedPriceYuan}`)
                    : "—"}
                </span>
                {selectedPeriod?.price_change_applies ? (
                  <Badge variant="secondary" className="font-normal text-brand">
                    {t("duePaid.nextPriceBadge")}
                  </Badge>
                ) : null}
              </div>
            </div>
            <div className="grid gap-1">
              <span className="text-xs font-medium text-muted-foreground">
                {t("duePaid.cycle")}
              </span>
              <span>{target?.cycleDesc || "—"}</span>
            </div>
          </div>

          <div className="grid gap-2">
            <Label>{t("duePaid.period")}</Label>
            <Select
              value={selectedStart}
              onValueChange={setOverrideStart}
              disabled={periodsQuery.isPending}
            >
              <SelectTrigger className="w-full">
                <SelectValue
                  placeholder={
                    periodsQuery.isPending
                      ? t("common.loading")
                      : periods.length === 0
                        ? t("duePaid.emptyPeriods")
                        : t("duePaid.selectPeriod")
                  }
                />
              </SelectTrigger>
              <SelectContent>
                {periods.map((period) => (
                  <SelectItem
                    key={period.start_date}
                    value={period.start_date}
                    disabled={period.paid}
                  >
                    {period.label || `${period.start_date} ${t("duePaid.to")} ${period.end_date}`}
                    {` · ${maskAmount(amountsHidden, `¥${period.price_yuan}`)}`}
                    {period.price_change_applies ? ` · ${t("duePaid.nextPriceBadge")}` : ""}
                    {period.paid ? t("duePaid.paidSuffix") : ""}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <p className="text-xs leading-relaxed text-muted-foreground">
              {periodsQuery.isError
                ? t("duePaid.loadPeriodsFailed")
                : selectedPeriod
                  ? t("duePaid.periodHintSelected", {
                      start: selectedPeriod.start_date,
                      end: selectedPeriod.end_date,
                      price: maskAmount(amountsHidden, selectedPeriod.price_yuan),
                    })
                  : t("duePaid.periodHintDefault")}
            </p>
          </div>
        </div>

        <DialogFooter>
          <Button type="button" variant="outline" onClick={() => handleOpenChange(false)}>
            {t("common.cancel")}
          </Button>
          <Button
            type="button"
            disabled={
              !selectedStart ||
              selectedPeriod?.paid === true ||
              periodsQuery.isPending ||
              confirmMutation.isPending
            }
            onClick={() => confirmMutation.mutate(undefined)}
          >
            {t(plusRental ? "plusRentals.confirmRenewal" : "duePaid.confirm")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
