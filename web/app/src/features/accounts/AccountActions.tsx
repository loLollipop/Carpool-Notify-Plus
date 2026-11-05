import { Ellipsis, Pause, Pencil, Play, ShieldAlert, Trash2 } from "lucide-react"
import { useTranslation } from "react-i18next"

import type { AccountView } from "@/api/types"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { formatAccountName } from "@/lib/account-display"

/** Keep primary editing visible and state-changing actions in one consistent menu. */
export function AccountActions({
  view,
  onEdit,
  onToggleAvailability,
  availabilityPending,
  onBan,
  onDelete,
}: {
  view: AccountView
  onEdit: () => void
  onToggleAvailability: () => void
  availabilityPending: boolean
  onBan: () => void
  onDelete: () => void
}) {
  const { t } = useTranslation()
  const name = formatAccountName(view.display_email || view.account.email || view.account.name, view.space_role)
  const hasMoreActions = !view.account.banned_at || view.can_delete

  return (
    <div className="inline-flex shrink-0 items-center gap-1.5">
      <Button variant="outline" size="sm" className="border-brand/20 bg-brand/[0.04] text-brand hover:border-brand/35 hover:bg-brand/10 hover:text-brand" onClick={onEdit}>
        <Pencil aria-hidden="true" />
        {t("common.edit")}
      </Button>
      {hasMoreActions ? (
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="outline" size="icon-sm" aria-label={t("accounts.moreActions", { name })} title={t("accounts.moreActions", { name })}>
              <Ellipsis aria-hidden="true" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" sideOffset={6} className="w-64 rounded-xl p-1.5 shadow-lift">
            <DropdownMenuLabel className="px-2.5 py-2">{t("accounts.actionMenuTitle")}</DropdownMenuLabel>
            {!view.account.banned_at ? (
              <DropdownMenuItem disabled={availabilityPending} onSelect={onToggleAvailability} className="cursor-pointer items-start gap-3 rounded-md px-2.5 py-3">
                {view.account.paused ? <Play className="mt-0.5" /> : <Pause className="mt-0.5" />}
                <div className="grid gap-0.5">
                  <span className="font-medium">{t(view.account.paused ? "accounts.resumeUse" : "accounts.pauseUse")}</span>
                  <span className="text-xs leading-5 text-muted-foreground">{t(view.account.paused ? "accounts.resumeUseHint" : "accounts.pauseUseHint")}</span>
                </div>
              </DropdownMenuItem>
            ) : null}
            {!view.account.banned_at ? <DropdownMenuSeparator /> : null}
            {!view.account.banned_at ? (
              <DropdownMenuItem variant="destructive" onSelect={onBan} className="cursor-pointer rounded-md px-2.5 py-2.5">
                <ShieldAlert />
                {t("accounts.markBanned")}
              </DropdownMenuItem>
            ) : null}
            {view.can_delete ? (
              <DropdownMenuItem variant="destructive" onSelect={onDelete} className="cursor-pointer rounded-md px-2.5 py-2.5">
                <Trash2 />
                {t("common.delete")}
              </DropdownMenuItem>
            ) : null}
          </DropdownMenuContent>
        </DropdownMenu>
      ) : null}
    </div>
  )
}
