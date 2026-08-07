import * as React from "react"
import { useNavigate } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { CalendarDays, ShieldAlert } from "lucide-react"

import { banAccount } from "@/api/endpoints"
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
import { Textarea } from "@/components/ui/textarea"

export interface AccountBanTarget {
  id: number
  name: string
  activeCount: number
}

function shanghaiToday() {
  return new Intl.DateTimeFormat("en-CA", {
    timeZone: "Asia/Shanghai",
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
  }).format(new Date())
}

export function AccountBanDialog({
  target,
  onOpenChange,
}: {
  target: AccountBanTarget | null
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [bannedDate, setBannedDate] = React.useState(shanghaiToday)
  const [note, setNote] = React.useState("")

  const handleOpenChange = (open: boolean) => {
    if (!open) {
      setBannedDate(shanghaiToday())
      setNote("")
    }
    onOpenChange(open)
  }

  const mutation = useAppMutation(
    (input: { id: number; bannedDate: string; note: string }) =>
      banAccount(input.id, { banned_date: input.bannedDate, note: input.note }),
    {
      onSuccess: (_data, variables) => {
        handleOpenChange(false)
        navigate(`/after-sales?account=${variables.id}`)
      },
    },
  )

  return (
    <Dialog open={target !== null} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <ShieldAlert className="size-5 text-destructive" />
            {t("accountBan.title")}
          </DialogTitle>
          <DialogDescription>
            {t("accountBan.desc", { name: target?.name ?? "", count: target?.activeCount ?? 0 })}
          </DialogDescription>
        </DialogHeader>

        <div className="rounded-md border border-destructive/20 bg-destructive/[0.05] px-3 py-2.5 text-xs leading-5 text-muted-foreground">
          {t("accountBan.refundRule")}
        </div>

        <div className="grid gap-4 py-1">
          <div className="grid gap-2">
            <Label htmlFor="account-ban-date">{t("accountBan.date")}</Label>
            <div className="relative">
              <CalendarDays className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                id="account-ban-date"
                type="date"
                className="pl-9"
                value={bannedDate}
                max={shanghaiToday()}
                onChange={(event) => setBannedDate(event.target.value)}
              />
            </div>
          </div>
          <div className="grid gap-2">
            <Label htmlFor="account-ban-note">{t("accountBan.note")}</Label>
            <Textarea
              id="account-ban-note"
              rows={3}
              value={note}
              placeholder={t("accountBan.notePlaceholder")}
              onChange={(event) => setNote(event.target.value)}
            />
          </div>
        </div>

        <DialogFooter>
          <Button variant="outline" disabled={mutation.isPending} onClick={() => handleOpenChange(false)}>
            {t("common.cancel")}
          </Button>
          <Button
            variant="destructive"
            disabled={mutation.isPending || !bannedDate}
            onClick={() => {
              if (!target) return
              mutation.mutate({ id: target.id, bannedDate, note })
            }}
          >
            <ShieldAlert data-slot="icon" />
            {t("accountBan.confirm")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
