import * as React from "react"
import { zodResolver } from "@hookform/resolvers/zod"
import { useForm, useWatch } from "react-hook-form"
import { useTranslation } from "react-i18next"
import { Mail, MessageCircle, WalletCards } from "lucide-react"
import { z } from "zod"

import { createSubscription, updateSubscription } from "@/api/endpoints"
import { useAppMutation } from "@/api/mutations"
import type { SubscriptionInput } from "@/api/types"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
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
import { Textarea } from "@/components/ui/textarea"
import { todayShanghai, type SubscriptionPrefill } from "@/features/subscriptions/subscription-prefill"
import { cn } from "@/lib/utils"
import {
  isOneMonthRentalCron,
  ONE_MONTH_RENTAL_CRON,
  oneMonthRentalEndDate,
} from "./rental-mode"

const MONEY_PATTERN = /^\d+(\.\d{1,2})?$/
const EMAIL_PATTERN = /^[^@\s]+@[^@\s]+\.[^@\s]+$/
const CYCLE_PRESETS = [
  { key: "monthly", cron: "interval:30d" },
  { key: "quarterly", cron: "interval:90d" },
  { key: "halfYear", cron: "interval:180d" },
  { key: "yearly", cron: "interval:365d" },
] as const

export function PlusRentalDialog({
  open,
  onOpenChange,
  prefill,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  prefill: SubscriptionPrefill | null
}) {
  const { t } = useTranslation()
  const isEdit = prefill !== null

  const schema = React.useMemo(
    () =>
      z.object({
        name: z.string().trim().min(1, t("plusRentals.validation.nameRequired")),
        account_email: z
          .string()
          .trim()
          .min(1, t("plusRentals.validation.emailRequired"))
          .regex(EMAIL_PATTERN, t("plusRentals.validation.emailInvalid")),
        contact: z.string().trim().min(1, t("plusRentals.validation.contactRequired")),
        price_yuan: z
          .string()
          .trim()
          .min(1, t("plusRentals.validation.rentRequired"))
          .regex(MONEY_PATTERN, t("plusRentals.validation.amountInvalid")),
        cost_yuan: z
          .string()
          .trim()
          .refine((value) => value === "" || MONEY_PATTERN.test(value), {
            message: t("plusRentals.validation.amountInvalid"),
          }),
        boarded_at: z.string().min(1, t("plusRentals.validation.startRequired")),
        rental_mode: z.enum(["recurring", "one_month"]),
        cron_expr: z.string().trim().min(1, t("plusRentals.validation.cycleRequired")),
        remark: z.string(),
      }),
    [t],
  )

  type FormValues = z.infer<typeof schema>

  const defaultValues = React.useCallback(
    (): FormValues => ({
      name: prefill?.name ?? "",
      account_email: prefill?.customerEmail ?? "",
      contact: prefill?.customerWechat ?? "",
      price_yuan: prefill?.priceYuan ?? "",
      cost_yuan: prefill?.costYuan === "0.00" ? "" : (prefill?.costYuan ?? ""),
      boarded_at: prefill?.boardedAt || todayShanghai(),
      rental_mode: isOneMonthRentalCron(prefill?.cronExpr ?? "") ? "one_month" : "recurring",
      cron_expr: prefill?.cronExpr || "interval:30d",
      remark: prefill?.remark ?? "",
    }),
    [prefill],
  )

  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: defaultValues(),
  })

  React.useEffect(() => {
    if (open) form.reset(defaultValues())
  }, [defaultValues, form, open])

  const rentalMode = useWatch({ control: form.control, name: "rental_mode" })
  const boardedAt = useWatch({ control: form.control, name: "boarded_at" })
  const oneMonthEndDate = oneMonthRentalEndDate(boardedAt)

  const saveMutation = useAppMutation(
    (values: FormValues) => {
      const input: SubscriptionInput = {
        name: values.name.trim(),
        business_type: "plus",
        price_yuan: values.price_yuan.trim(),
        cost_yuan: values.cost_yuan.trim(),
        cron_expr:
          values.rental_mode === "one_month" ? ONE_MONTH_RENTAL_CRON : values.cron_expr.trim(),
        notify_offsets: [],
        remark: values.remark.trim(),
        trade_url: "",
        customer_email: values.account_email.trim(),
        customer_wechat: values.contact.trim(),
        account_id: 0,
        seat_id: 0,
        boarded_at: values.boarded_at,
      }
      return isEdit ? updateSubscription(prefill.id, input) : createSubscription(input)
    },
    {
      successMessage: t(isEdit ? "plusRentals.updated" : "plusRentals.created"),
      onSuccess: () => onOpenChange(false),
    },
  )

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent aria-describedby={undefined} className="max-h-[92vh] overflow-y-auto sm:max-w-2xl">
        <DialogHeader>
          <div className="mb-1 flex size-10 items-center justify-center rounded-xl bg-brand/10 text-brand">
            <WalletCards className="size-5" />
          </div>
          <DialogTitle>
            {t(isEdit ? "plusRentals.editTitle" : "plusRentals.createTitle")}
          </DialogTitle>
        </DialogHeader>

        <Form {...form}>
          <form
            className="grid gap-6"
            onSubmit={form.handleSubmit((values) => saveMutation.mutate(values))}
          >
            <section className="grid items-start gap-4 rounded-xl border border-border/70 bg-muted/20 p-4 sm:grid-cols-2">
              <div className="flex items-center gap-2 text-sm font-semibold sm:col-span-2">
                <MessageCircle className="size-4 text-brand" />
                {t("plusRentals.customerSection")}
              </div>
              <FormField
                control={form.control}
                name="name"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t("plusRentals.customerName")}</FormLabel>
                    <FormControl>
                      <Input autoComplete="off" placeholder={t("plusRentals.customerNamePlaceholder")} {...field} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name="contact"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t("plusRentals.contact")}</FormLabel>
                    <FormControl>
                      <Input autoComplete="off" placeholder={t("plusRentals.contactPlaceholder")} {...field} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name="account_email"
                render={({ field }) => (
                  <FormItem className="sm:col-span-2">
                    <FormLabel className="flex items-center gap-1.5">
                      <Mail className="size-3.5" />
                      {t("plusRentals.accountEmail")}
                    </FormLabel>
                    <FormControl>
                      <Input type="email" autoComplete="off" placeholder="plus-account@example.com" {...field} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </section>

            <section className="grid items-start gap-4 sm:grid-cols-2">
              <FormField
                control={form.control}
                name="rental_mode"
                render={({ field }) => (
                  <FormItem className="sm:col-span-2">
                    <FormLabel>{t("plusRentals.rentalMode")}</FormLabel>
                    <FormControl>
                      <div className="grid gap-2 sm:grid-cols-2" role="radiogroup">
                        {(["recurring", "one_month"] as const).map((mode) => {
                          const selected = field.value === mode
                          const oneMonth = mode === "one_month"
                          return (
                            <button
                              key={mode}
                              type="button"
                              role="radio"
                              aria-checked={selected}
                              className={cn(
                                "rounded-xl border px-3.5 py-3 text-left transition-colors",
                                selected
                                  ? "border-brand/40 bg-brand/[0.07] ring-1 ring-brand/15"
                                  : "border-border/70 bg-background hover:border-brand/25 hover:bg-muted/30",
                              )}
                              onClick={() => {
                                field.onChange(mode)
                                if (
                                  mode === "recurring" &&
                                  isOneMonthRentalCron(form.getValues("cron_expr"))
                                ) {
                                  form.setValue("cron_expr", "interval:30d", {
                                    shouldDirty: true,
                                    shouldValidate: true,
                                  })
                                }
                              }}
                            >
                              <span className="block text-sm font-semibold">
                                {t(
                                  oneMonth
                                    ? "plusRentals.oneMonthMode"
                                    : "plusRentals.recurringMode",
                                )}
                              </span>
                              <span className="mt-1 block text-xs leading-5 text-muted-foreground">
                                {t(
                                  oneMonth
                                    ? "plusRentals.oneMonthModeHint"
                                    : "plusRentals.recurringModeHint",
                                )}
                              </span>
                            </button>
                          )
                        })}
                      </div>
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name="price_yuan"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t("plusRentals.rent")}</FormLabel>
                    <FormControl>
                      <Input inputMode="decimal" placeholder="68.00" {...field} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name="cost_yuan"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t("plusRentals.cost")}</FormLabel>
                    <FormControl>
                      <Input inputMode="decimal" placeholder="20.00" {...field} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name="boarded_at"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t("plusRentals.startedAt")}</FormLabel>
                    <FormControl>
                      <Input type="date" {...field} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name="cron_expr"
                render={({ field }) => (
                  rentalMode === "one_month" ? (
                    <FormItem>
                      <FormLabel>{t("plusRentals.oneMonthEnd")}</FormLabel>
                      <div className="flex h-9 items-center rounded-md border border-brand/20 bg-brand/[0.045] px-3 text-sm font-semibold tabular-nums">
                        {oneMonthEndDate || "--"}
                      </div>
                      <FormDescription>{t("plusRentals.oneMonthEndHint")}</FormDescription>
                      <FormMessage />
                    </FormItem>
                  ) : (
                    <FormItem>
                      <FormLabel>{t("plusRentals.cycle")}</FormLabel>
                      <FormControl>
                        <div className="grid grid-cols-4 gap-1.5" role="radiogroup">
                          {CYCLE_PRESETS.map((preset) => (
                            <Button
                              key={preset.cron}
                              type="button"
                              role="radio"
                              aria-checked={field.value === preset.cron}
                              size="sm"
                              variant={field.value === preset.cron ? "secondary" : "outline"}
                              className={cn("px-2 text-xs", field.value === preset.cron && "border-brand/25 bg-brand/10 text-brand")}
                              onClick={() => field.onChange(preset.cron)}
                            >
                              {t(`plusRentals.cycles.${preset.key}`)}
                            </Button>
                          ))}
                        </div>
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )
                )}
              />
            </section>

            <section>
              <FormField
                control={form.control}
                name="remark"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t("plusRentals.remark")}</FormLabel>
                    <FormControl>
                      <Textarea rows={2} placeholder={t("plusRentals.remarkPlaceholder")} {...field} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </section>

            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
                {t("common.cancel")}
              </Button>
              <Button type="submit" disabled={saveMutation.isPending}>
                {saveMutation.isPending ? t("common.loading") : t("common.save")}
              </Button>
            </DialogFooter>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  )
}
