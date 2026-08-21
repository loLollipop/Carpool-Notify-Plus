import * as React from "react"
import { useTranslation } from "react-i18next"
import { CalendarClock, Snowflake } from "lucide-react"

import { updateSeatFreeze } from "@/api/endpoints"
import { useAppMutation } from "@/api/mutations"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"

export interface SeatFreezeTarget {
  seatId: number
  seatName: string
  customer: string
  frozenUntil: string
  frozenUntilLabel: string
}

function shanghaiDateTimeLocal(moment: Date) {
  const parts = new Intl.DateTimeFormat("en-CA", {
    timeZone: "Asia/Shanghai",
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    hourCycle: "h23",
  }).formatToParts(moment)
  const value = (type: Intl.DateTimeFormatPartTypes) =>
    parts.find((part) => part.type === type)?.value ?? ""
  return `${value("year")}-${value("month")}-${value("day")}T${value("hour")}:${value("minute")}`
}

function initialDeadline(target: SeatFreezeTarget | null) {
  if (!target?.frozenUntil) return ""
  const parsed = new Date(target.frozenUntil)
  return Number.isNaN(parsed.getTime()) ? "" : shanghaiDateTimeLocal(parsed)
}

export function SeatFreezeDialog({
  target,
  onOpenChange,
}: {
  target: SeatFreezeTarget | null
  onOpenChange: (open: boolean) => void
}) {
  if (!target) return null

  return (
    <SeatFreezeDialogContent
      key={`${target.seatId}:${target.frozenUntil}`}
      target={target}
      onOpenChange={onOpenChange}
    />
  )
}

function SeatFreezeDialogContent({
  target,
  onOpenChange,
}: {
  target: SeatFreezeTarget
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation()
  const [frozenUntil, setFrozenUntil] = React.useState(() => initialDeadline(target))
  const [openedAt] = React.useState(() => new Date())

  const mutation = useAppMutation(
    (input: { seatId: number; frozenUntil: string }) =>
      updateSeatFreeze(input.seatId, { frozen_until: input.frozenUntil }),
    { onSuccess: () => onOpenChange(false) },
  )

  const applyPreset = (days: number) => {
    const deadline = new Date(openedAt.getTime() + days * 24 * 60 * 60 * 1000)
    setFrozenUntil(shanghaiDateTimeLocal(deadline))
  }

  return (
    <Dialog open onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <span className="grid size-9 place-items-center rounded-lg bg-sky-500/10 text-sky-700 dark:text-sky-300">
              <Snowflake className="size-4" />
            </span>
            {t("accounts.freezeDialogTitle")}
          </DialogTitle>
          <DialogDescription>{t("accounts.freezeDialogDesc")}</DialogDescription>
        </DialogHeader>

        <div className="grid grid-cols-[auto_minmax(0,1fr)] gap-x-4 gap-y-2 rounded-xl border border-sky-500/15 bg-sky-500/[0.05] px-4 py-3 text-sm">
          <span className="text-muted-foreground">{t("accounts.freezeSeat")}</span>
          <span className="truncate font-medium">{target.seatName || "—"}</span>
          <span className="text-muted-foreground">{t("accounts.freezeCustomer")}</span>
          <span className="truncate font-medium">{target.customer || "—"}</span>
          <span className="text-muted-foreground">{t("accounts.freezeCurrentUntil")}</span>
          <span className="font-mono text-xs font-semibold text-sky-700 tabular-nums dark:text-sky-300">
            {target.frozenUntilLabel || "—"}
          </span>
        </div>

        <div className="grid gap-2">
          <Label htmlFor="seat-freeze-until">{t("accounts.freezeNewUntil")}</Label>
          <div className="relative">
            <CalendarClock className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              id="seat-freeze-until"
              type="datetime-local"
              value={frozenUntil}
              min={shanghaiDateTimeLocal(new Date(openedAt.getTime() + 60 * 1000))}
              onChange={(event) => setFrozenUntil(event.target.value)}
              className="pl-9 font-mono tabular-nums"
            />
          </div>
          <p className="text-xs leading-5 text-muted-foreground">
            {t("accounts.freezeDeadlineHint")}
          </p>
        </div>

        <div className="flex flex-wrap items-center gap-2">
          <span className="mr-1 text-xs text-muted-foreground">
            {t("accounts.freezePresets")}
          </span>
          {[1, 3, 7].map((days) => (
            <Button
              key={days}
              type="button"
              variant="outline"
              size="xs"
              onClick={() => applyPreset(days)}
            >
              {t("accounts.freezePresetDays", { days })}
            </Button>
          ))}
        </div>

        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            disabled={mutation.isPending}
            onClick={() => onOpenChange(false)}
          >
            {t("common.cancel")}
          </Button>
          <Button
            type="button"
            disabled={!target || !frozenUntil || mutation.isPending}
            onClick={() => {
              if (!target) return
              mutation.mutate({ seatId: target.seatId, frozenUntil })
            }}
          >
            <CalendarClock data-slot="icon" />
            {t("accounts.freezeSave")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
