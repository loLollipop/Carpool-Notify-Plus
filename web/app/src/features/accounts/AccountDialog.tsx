import * as React from "react"
import { zodResolver } from "@hookform/resolvers/zod"
import { useForm, useWatch } from "react-hook-form"
import { useTranslation } from "react-i18next"
import { z } from "zod"

import { createAccount, updateAccount } from "@/api/endpoints"
import { useAppMutation } from "@/api/mutations"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
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
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/components/ui/form"
import { Input } from "@/components/ui/input"
import { getNextMonthlyRenewalDate } from "@/lib/account-renewal"

const MONEY_PATTERN = /^\d+(\.\d{1,2})?$/
const EMAIL_PATTERN = /^[^@\s]+@[^@\s]+\.[^@\s]+$/

export interface AccountPrefill {
  id: number
  name: string
  remark: string
  paymentMethod: string
  email: string
  spaceName: string
  openedAt: string
  costYuan: string
  zeroRenewalNextMonth: boolean
  seatCount: number
}

export function AccountDialog({
  open,
  onOpenChange,
  prefill,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  prefill: AccountPrefill | null
}) {
  const { t } = useTranslation()
  const isEdit = prefill !== null

  const schema = React.useMemo(
    () =>
      z.object({
        name: z
          .string()
          .trim()
          .min(1, t("accounts.validation.emailRequired"))
          .max(120)
          .refine((value) => EMAIL_PATTERN.test(value), {
            message: t("subscriptionDialog.validation.emailInvalid"),
          }),
        remark: z.string(),
        payment_method: z.string(),
        space_name: z.string(),
        opened_at: z.string(),
        cost_yuan: z
          .string()
          .trim()
          .refine((value) => value === "" || MONEY_PATTERN.test(value), {
            message: t("subscriptionDialog.validation.priceInvalid"),
          }),
        zero_renewal_next_month: z.boolean(),
        seat_count: z
          .string()
          .refine((value) => {
            const parsed = Number(value)
            return Number.isInteger(parsed) && parsed >= 1 && parsed <= 1000
          }, t("accounts.validation.seatCountRange")),
      }),
    [t],
  )

  type FormValues = z.infer<typeof schema>

  const defaultValues = React.useCallback(
    (): FormValues => ({
      name: prefill?.email || prefill?.name || "",
      remark: prefill?.remark ?? "",
      payment_method: prefill?.paymentMethod ?? "",
      space_name: prefill?.spaceName ?? "",
      opened_at: prefill?.openedAt ?? "",
      cost_yuan: prefill?.costYuan ?? "",
      zero_renewal_next_month: prefill?.zeroRenewalNextMonth ?? false,
      seat_count: String(prefill?.seatCount ?? 1),
    }),
    [prefill],
  )

  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: defaultValues(),
  })

  React.useEffect(() => {
    if (open) {
      form.reset(defaultValues())
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, prefill])

  const openedAt = useWatch({ control: form.control, name: "opened_at" })
  const nextRenewalDate = getNextMonthlyRenewalDate(openedAt ?? "")

  const saveMutation = useAppMutation(
    (values: FormValues) => {
      const ownerEmail = values.name.trim()
      const input = {
        name: ownerEmail,
        remark: values.remark.trim(),
        payment_method: values.payment_method.trim(),
        email: ownerEmail,
        space_name: values.space_name.trim(),
        opened_at: values.opened_at,
        cost_yuan: values.cost_yuan.trim(),
        zero_renewal_next_month: values.zero_renewal_next_month,
        seat_count: Number(values.seat_count),
      }
      return isEdit ? updateAccount(prefill.id, input) : createAccount(input)
    },
    {
      onSuccess: () => onOpenChange(false),
    },
  )

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>
            {isEdit ? t("accounts.dialogEditTitle") : t("accounts.dialogCreateTitle")}
          </DialogTitle>
          <DialogDescription>{t("accounts.dialogDesc")}</DialogDescription>
        </DialogHeader>

        <Form {...form}>
          <form
            onSubmit={form.handleSubmit((values) => saveMutation.mutate(values))}
            className="grid gap-5"
          >
            <div className="grid items-start gap-4 sm:grid-cols-2">
              <FormField
                control={form.control}
                name="name"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t("accounts.email")}</FormLabel>
                    <FormControl>
                      <Input
                        type="email"
                        autoComplete="email"
                        maxLength={120}
                        placeholder={t("accounts.emailPlaceholder")}
                        {...field}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name="space_name"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t("accounts.spaceName")}</FormLabel>
                    <FormControl>
                      <Input placeholder={t("accounts.spaceNamePlaceholder")} {...field} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>
            <div className="grid items-start gap-4 sm:grid-cols-2">
              <FormField
                control={form.control}
                name="opened_at"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t("accounts.openedAt")}</FormLabel>
                    <FormControl>
                      <Input type="date" {...field} />
                    </FormControl>
                    <FormDescription>
                      {nextRenewalDate
                        ? t("accounts.nextRenewalHint", { date: nextRenewalDate })
                        : t("accounts.openedAtHint")}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name="cost_yuan"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t("accounts.cost")}</FormLabel>
                    <FormControl>
                      <Input inputMode="decimal" placeholder="20.00" {...field} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>
            <FormField
              control={form.control}
              name="payment_method"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t("accounts.paymentMethod")}</FormLabel>
                  <FormControl>
                    <Input placeholder={t("accounts.paymentMethodPlaceholder")} {...field} />
                  </FormControl>
                  <FormDescription>{t("accounts.paymentMethodHint")}</FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name="zero_renewal_next_month"
              render={({ field }) => (
                <FormItem className="flex flex-row items-start gap-3 rounded-lg border border-border/60 px-3 py-2.5">
                  <FormControl>
                    <Checkbox
                      checked={field.value}
                      onCheckedChange={(value) => field.onChange(value === true)}
                    />
                  </FormControl>
                  <div className="space-y-0.5">
                    <FormLabel>{t("accounts.zeroRenewalNextMonth")}</FormLabel>
                    <FormDescription>{t("accounts.zeroRenewalNextMonthHint")}</FormDescription>
                  </div>
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name="remark"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t("accounts.remark")}</FormLabel>
                  <FormControl>
                    <Input placeholder={t("common.optional")} {...field} />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name="seat_count"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t("accounts.seatCount")}</FormLabel>
                  <FormControl>
                    <Input type="number" inputMode="numeric" min={1} max={1000} step={1} {...field} />
                  </FormControl>
                  <FormDescription>{t("accounts.seatCountHint")}</FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
                {t("common.cancel")}
              </Button>
              <Button type="submit" disabled={saveMutation.isPending}>
                {isEdit ? t("accounts.saveAction") : t("accounts.createAction")}
              </Button>
            </DialogFooter>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  )
}
