import * as React from "react"
import { zodResolver } from "@hookform/resolvers/zod"
import { useForm } from "react-hook-form"
import { useTranslation } from "react-i18next"
import { z } from "zod"

import { updateBill } from "@/api/endpoints"
import { useAppMutation } from "@/api/mutations"
import type { BillView } from "@/api/types"
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
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/components/ui/form"
import { Input } from "@/components/ui/input"
import { Textarea } from "@/components/ui/textarea"

const MONEY_PATTERN = /^\d+(\.\d{1,2})?$/

export function BillEditDialog({
  open,
  onOpenChange,
  bill,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  bill: BillView | null
}) {
  const { t } = useTranslation()
  const billLabel = bill
    ? `${bill.account_name || bill.subscription_name} · ${
        bill.customer_email || bill.subscription_name
      } · ${bill.due_date} · `
    : ""

  const schema = React.useMemo(
    () =>
      z.object({
        amount_yuan: z
          .string()
          .trim()
          .min(1, t("bills.validation.amountRequired"))
          .regex(MONEY_PATTERN, t("subscriptionDialog.validation.priceInvalid")),
        note: z.string(),
      }),
    [t],
  )

  type FormValues = z.infer<typeof schema>

  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: { amount_yuan: "", note: "" },
  })
  const resetForm = form.reset

  React.useEffect(() => {
    if (open && bill) {
      resetForm({ amount_yuan: bill.amount_yuan, note: bill.note })
    }
  }, [open, bill, resetForm])

  const saveMutation = useAppMutation(
    ({ billId, values }: { billId: number; values: FormValues }) =>
      updateBill(billId, { amount_yuan: values.amount_yuan.trim(), note: values.note.trim() }),
    {
      onSuccess: () => onOpenChange(false),
    },
  )

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{t("bills.editTitle")}</DialogTitle>
          <DialogDescription>
            {billLabel}
            {t("bills.editDesc")}
          </DialogDescription>
        </DialogHeader>

        <Form {...form}>
          <form
            onSubmit={form.handleSubmit((values) => {
              if (bill) saveMutation.mutate({ billId: bill.id, values })
            })}
            className="grid gap-5"
          >
            <FormField
              control={form.control}
              name="amount_yuan"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t("bills.amount")}</FormLabel>
                  <FormControl>
                    <Input inputMode="decimal" autoComplete="off" {...field} />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name="note"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t("bills.note")}</FormLabel>
                  <FormControl>
                    <Textarea rows={3} placeholder={t("common.optional")} {...field} />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
                {t("common.cancel")}
              </Button>
              <Button type="submit" disabled={!bill || saveMutation.isPending}>
                {t("common.save")}
              </Button>
            </DialogFooter>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  )
}
