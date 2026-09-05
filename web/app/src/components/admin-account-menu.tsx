import * as React from "react"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { ChevronUp, LoaderCircle, LogOut, PencilLine, ShieldCheck } from "lucide-react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"

import { updateAdminProfile } from "@/api/endpoints"
import { queryKeys, useAdminProfile } from "@/api/queries"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { cn } from "@/lib/utils"

const DEFAULT_DISPLAY_NAME = "Admin"

function getAvatarInitial(displayName: string) {
  return Array.from(displayName.trim())[0]?.toLocaleUpperCase() ?? "A"
}

export function AdminAccountMenu({
  collapsed = false,
  placement = "sidebar",
  onLogout,
}: {
  collapsed?: boolean
  placement?: "sidebar" | "header"
  onLogout: () => void
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const profileQuery = useAdminProfile()
  const displayName = profileQuery.data?.display_name || DEFAULT_DISPLAY_NAME
  const [editOpen, setEditOpen] = React.useState(false)
  const [draftName, setDraftName] = React.useState(displayName)

  const updateMutation = useMutation({
    mutationFn: updateAdminProfile,
    onSuccess: (result) => {
      queryClient.setQueryData(queryKeys.adminProfile, result.profile)
      setEditOpen(false)
      toast.success(result.message ?? t("profile.updated"))
    },
    onError: (error: Error) => toast.error(error.message),
  })

  const compact = collapsed || placement === "header"
  const avatar = (
    <span
      className={cn(
        "grid size-9 shrink-0 place-items-center rounded-full border border-brand/25 bg-brand/10 text-sm font-bold text-brand shadow-sm",
        compact && "size-9",
      )}
      aria-hidden="true"
    >
      {getAvatarInitial(displayName)}
    </span>
  )

  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            variant="ghost"
            className={cn(
              "group h-12 w-full justify-start gap-3 rounded-xl border border-transparent px-2 text-left text-[var(--sidebar-foreground)] hover:border-[var(--sidebar-border)] hover:bg-accent",
              compact && "size-10 justify-center rounded-full border-[var(--sidebar-border)] p-0",
              placement === "header" && "bg-card text-foreground",
            )}
            aria-label={t("profile.openMenu", { name: displayName })}
          >
            {avatar}
            {!compact ? (
              <>
                <span className="min-w-0 flex-1">
                  <span className="block truncate text-sm font-semibold">{displayName}</span>
                  <span className="block truncate text-[11px] font-normal text-[var(--sidebar-muted)]">
                    {t("profile.role")}
                  </span>
                </span>
                <ChevronUp className="size-4 shrink-0 text-[var(--sidebar-muted)] transition-transform group-data-[state=open]:rotate-180" />
              </>
            ) : null}
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent
          side={placement === "header" ? "bottom" : collapsed ? "right" : "top"}
          align={placement === "header" ? "end" : "start"}
          sideOffset={8}
          className="w-60 rounded-xl p-1.5 shadow-xl"
        >
          <DropdownMenuLabel className="flex items-center gap-3 px-2.5 py-2.5">
            {avatar}
            <span className="min-w-0">
              <span className="block truncate text-sm font-semibold text-foreground">
                {displayName}
              </span>
              <span className="mt-0.5 block text-[11px] font-normal text-muted-foreground">
                {t("profile.role")}
              </span>
            </span>
          </DropdownMenuLabel>
          <DropdownMenuSeparator />
          <DropdownMenuItem
            className="rounded-lg px-2.5 py-2.5"
            onSelect={() => {
              setDraftName(displayName)
              setEditOpen(true)
            }}
          >
            <PencilLine className="size-4" />
            {t("profile.edit")}
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuItem
            variant="destructive"
            className="rounded-lg px-2.5 py-2.5"
            onSelect={onLogout}
          >
            <LogOut className="size-4" />
            {t("nav.logout")}
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>

      <Dialog
        open={editOpen}
        onOpenChange={(open) => {
          if (!updateMutation.isPending) setEditOpen(open)
        }}
      >
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>{t("profile.title")}</DialogTitle>
            <DialogDescription>{t("profile.description")}</DialogDescription>
          </DialogHeader>
          <form
            className="space-y-5"
            onSubmit={(event) => {
              event.preventDefault()
              const nextName = draftName.trim()
              if (!nextName) return
              updateMutation.mutate({ display_name: nextName })
            }}
          >
            <div className="flex items-center gap-3 rounded-xl border bg-muted/35 p-3.5">
              <span className="grid size-11 shrink-0 place-items-center rounded-full border border-brand/25 bg-brand/10 text-base font-bold text-brand">
                {getAvatarInitial(draftName || displayName)}
              </span>
              <span className="min-w-0">
                <span className="flex items-center gap-1.5 text-sm font-semibold">
                  <ShieldCheck className="size-4 text-brand" />
                  {t("profile.role")}
                </span>
                <span className="mt-1 block text-xs text-muted-foreground">
                  {t("profile.authHint")}
                </span>
              </span>
            </div>
            <div className="space-y-2">
              <Label htmlFor="admin-display-name">{t("profile.displayName")}</Label>
              <Input
                id="admin-display-name"
                value={draftName}
                maxLength={40}
                autoComplete="nickname"
                autoFocus
                onChange={(event) => setDraftName(event.target.value)}
                placeholder={DEFAULT_DISPLAY_NAME}
              />
              <p className="text-xs text-muted-foreground">{t("profile.displayNameHint")}</p>
            </div>
            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                disabled={updateMutation.isPending}
                onClick={() => setEditOpen(false)}
              >
                {t("common.cancel")}
              </Button>
              <Button
                type="submit"
                disabled={updateMutation.isPending || draftName.trim() === ""}
              >
                {updateMutation.isPending ? <LoaderCircle className="animate-spin" /> : null}
                {updateMutation.isPending ? t("common.saving") : t("common.save")}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </>
  )
}
