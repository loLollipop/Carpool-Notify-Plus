import { useQuery } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"
import { Mail, Send } from "lucide-react"

import { fetchReminderPreview, sendCustomerEmail } from "@/api/endpoints"
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
import { Separator } from "@/components/ui/separator"
import { Skeleton } from "@/components/ui/skeleton"

export function ReminderPreviewDialog({
  open,
  onOpenChange,
  subscriptionId,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  subscriptionId: number | null
}) {
  const { t } = useTranslation()

  const previewQuery = useQuery({
    queryKey: ["reminder-preview", subscriptionId],
    queryFn: () => fetchReminderPreview(subscriptionId as number),
    enabled: open && subscriptionId !== null,
    staleTime: 0,
    gcTime: 0,
  })

  const sendMutation = useAppMutation(
    () => sendCustomerEmail(subscriptionId as number),
    {
      successMessage: t("reminder.sent"),
      onSuccess: () => onOpenChange(false),
    },
  )

  const preview = previewQuery.data

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{t("reminder.title")}</DialogTitle>
          <DialogDescription>{t("reminder.desc")}</DialogDescription>
        </DialogHeader>

        {previewQuery.isPending ? (
          <div className="grid gap-3">
            <Skeleton className="h-5 w-2/3" />
            <Skeleton className="h-5 w-1/2" />
            <Skeleton className="h-36 w-full" />
          </div>
        ) : previewQuery.isError ? (
          <p className="text-sm text-destructive">{(previewQuery.error as Error).message}</p>
        ) : preview ? (
          <div className="grid gap-3 text-sm">
            {preview.next_price_yuan ? (
              <div className="rounded-lg border border-brand/20 bg-brand/[0.05] px-3 py-2.5 text-xs leading-5">
                {t("reminder.scheduledPrice", {
                  current: preview.current_price_yuan,
                  next: preview.next_price_yuan,
                  date: preview.next_price_effective_due_date,
                })}
              </div>
            ) : null}
            <div className="flex items-center gap-2">
              <Mail className="size-4 text-muted-foreground" />
              <span className="text-muted-foreground">{t("reminder.to")}</span>
              <span className="font-medium">{preview.to}</span>
            </div>
            <div className="flex items-baseline gap-2">
              <span className="shrink-0 text-muted-foreground">{t("reminder.subject")}</span>
              <span className="font-medium">{preview.subject}</span>
            </div>
            <Separator />
            <pre className="max-h-72 overflow-auto rounded-lg border bg-muted/40 p-3.5 font-sans text-[13px] leading-relaxed whitespace-pre-wrap">
              {preview.body}
            </pre>
          </div>
        ) : null}

        <DialogFooter>
          <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
            {t("common.cancel")}
          </Button>
          <Button
            type="button"
            disabled={!preview || sendMutation.isPending}
            onClick={() => sendMutation.mutate(undefined)}
          >
            <Send data-slot="icon" />
            {t("reminder.send")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
