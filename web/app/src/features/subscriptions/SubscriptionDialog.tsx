import * as React from "react"
import { zodResolver } from "@hookform/resolvers/zod"
import { useForm, useWatch } from "react-hook-form"
import { useTranslation } from "react-i18next"
import { BadgeDollarSign, CalendarClock } from "lucide-react"
import { z } from "zod"

import { fetchCronPreview } from "@/api/endpoints"
import { createSubscription, updateSubscription } from "@/api/endpoints"
import { useAppMutation } from "@/api/mutations"
import { useAccountOptions } from "@/api/queries"
import type { SubscriptionInput } from "@/api/types"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Textarea } from "@/components/ui/textarea"
import { cn } from "@/lib/utils"
import { todayShanghai, type SubscriptionPrefill } from "./subscription-prefill"

const CYCLE_PRESETS = [
  { key: "presetMonthly", cron: "interval:30d" },
  { key: "presetQuarterly", cron: "interval:90d" },
  { key: "presetHalfYear", cron: "interval:180d" },
  { key: "presetYearly", cron: "interval:365d" },
] as const

const OFFSET_OPTIONS = [0, 1, 2, 3, 5, 7, 14]

const MONEY_PATTERN = /^\d+(\.\d{1,2})?$/
const EMAIL_PATTERN = /^[^@\s]+@[^@\s]+\.[^@\s]+$/

type CronPreviewState =
  | { status: "idle" }
  | { status: "loading" }
  | { status: "ok"; description: string; times: string[] }
  | { status: "error"; message: string }

/**
 * Debounced cron preview. State only holds the last fetched result; the visible
 * status (idle/loading) is derived by comparing it with the current expression.
 */
function useCronPreview(cronExpr: string, boardedAt: string, enabled: boolean): CronPreviewState {
  const [result, setResult] = React.useState<{
    expression: string
    boardedAt: string
    state: CronPreviewState
  } | null>(null)

  const expression = cronExpr.trim()
  const anchorDate = boardedAt.trim()

  React.useEffect(() => {
    if (!enabled || expression === "" || anchorDate === "") return
    let cancelled = false
    const timer = window.setTimeout(() => {
      fetchCronPreview(expression, anchorDate)
        .then((preview) => {
          if (cancelled) return
          setResult({
            expression,
            boardedAt: anchorDate,
            state: {
              status: "ok",
              description: preview.description,
              times: preview.times ?? [],
            },
          })
        })
        .catch((error: Error) => {
          if (cancelled) return
          setResult({
            expression,
            boardedAt: anchorDate,
            state: { status: "error", message: error.message },
          })
        })
    }, 400)
    return () => {
      cancelled = true
      window.clearTimeout(timer)
    }
  }, [expression, anchorDate, enabled])

  if (expression === "" || anchorDate === "") return { status: "idle" }
  if (result?.expression === expression && result.boardedAt === anchorDate) return result.state
  return { status: "loading" }
}

export function SubscriptionDialog({
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
  const accountOptionsQuery = useAccountOptions(prefill?.seatId ?? 0, open)

  const schema = React.useMemo(
    () =>
      z.object({
        account_id: z.string().min(1, t("subscriptionDialog.validation.accountRequired")),
        name: z.string(),
        price_yuan: z
          .string()
          .trim()
          .min(1, t("subscriptionDialog.validation.priceRequired"))
          .regex(MONEY_PATTERN, t("subscriptionDialog.validation.priceInvalid")),
        next_price_yuan: z
          .string()
          .trim()
          .refine((value) => value === "" || MONEY_PATTERN.test(value), {
            message: t("subscriptionDialog.validation.priceInvalid"),
          }),
        cost_yuan: z
          .string()
          .trim()
          .refine((value) => value === "" || MONEY_PATTERN.test(value), {
            message: t("subscriptionDialog.validation.priceInvalid"),
          }),
        cron_expr: z.string().trim().min(1, t("subscriptionDialog.validation.cronRequired")),
        notify_offsets: z.array(z.number()),
        boarded_at: z.string().min(1, t("subscriptionDialog.validation.boardedAtRequired")),
        customer_email: z
          .string()
          .trim()
          .refine((value) => value === "" || EMAIL_PATTERN.test(value), {
            message: t("subscriptionDialog.validation.emailInvalid"),
          }),
        customer_wechat: z.string().trim(),
        remark: z.string(),
      }),
    [t],
  )

  type FormValues = z.infer<typeof schema>

  const defaultValues = React.useCallback(
    (): FormValues => ({
      account_id: prefill && prefill.accountId > 0 ? String(prefill.accountId) : "",
      name: prefill?.name ?? "",
      price_yuan: prefill?.priceYuan ?? "",
      next_price_yuan: prefill?.nextPriceYuan ?? "",
      cost_yuan: prefill?.costYuan ?? "",
      cron_expr: prefill?.cronExpr || "interval:30d",
      notify_offsets: prefill?.offsets ?? [],
      boarded_at: prefill?.boardedAt || todayShanghai(),
      customer_email: prefill?.customerEmail ?? "",
      customer_wechat: prefill?.customerWechat ?? "",
      remark: prefill?.remark ?? "",
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

  const cronExpr = useWatch({ control: form.control, name: "cron_expr" })
  const boardedAt = useWatch({ control: form.control, name: "boarded_at" })
  const accountId = useWatch({ control: form.control, name: "account_id" })
  const currentPriceYuan = useWatch({ control: form.control, name: "price_yuan" })
  const nextPriceYuan = useWatch({ control: form.control, name: "next_price_yuan" })
  const scheduleChanged =
    form.formState.dirtyFields.boarded_at === true ||
    form.formState.dirtyFields.cron_expr === true
  const cronPreview = useCronPreview(cronExpr, boardedAt, open)

  const saveMutation = useAppMutation(
    (values: FormValues) => {
      const input: SubscriptionInput = {
        name: values.name.trim(),
        business_type: "team",
        price_yuan: values.price_yuan.trim(),
        next_price_yuan: isEdit ? values.next_price_yuan.trim() : "",
        cost_yuan: isEdit ? values.cost_yuan.trim() : "",
        cron_expr: values.cron_expr.trim(),
        notify_offsets: values.notify_offsets,
        remark: values.remark.trim(),
        trade_url: "",
        customer_email: values.customer_email.trim(),
        customer_wechat: values.customer_wechat.trim(),
        account_id: Number(values.account_id),
        seat_id: prefill?.seatId ?? 0,
        boarded_at: values.boarded_at,
      }
      return isEdit ? updateSubscription(prefill.id, input) : createSubscription(input)
    },
    {
      onSuccess: () => onOpenChange(false),
    },
  )

  const accounts = React.useMemo(() => accountOptionsQuery.data ?? [], [accountOptionsQuery.data])
  // Backend only lists free seats (plus the seat being edited). Hide accounts with no free seats.
  const selectableAccounts = React.useMemo(
    () => accounts.filter((account) => (account.seats?.length ?? 0) > 0),
    [accounts],
  )
  const selectedAccount = React.useMemo(
    () => selectableAccounts.find((account) => String(account.id) === accountId) ?? null,
    [accountId, selectableAccounts],
  )

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent aria-describedby={undefined} className="max-h-[90vh] overflow-y-auto sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>
            {isEdit ? t("subscriptionDialog.editTitle") : t("subscriptionDialog.createTitle")}
          </DialogTitle>
        </DialogHeader>

        <Form {...form}>
          <form
            onSubmit={form.handleSubmit((values) => saveMutation.mutate(values))}
            className="grid gap-5"
          >
            <FormField
              control={form.control}
              name="account_id"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t("subscriptionDialog.account")}</FormLabel>
                  <Select value={field.value} onValueChange={field.onChange}>
                    <FormControl>
                      <SelectTrigger>
                        <SelectValue placeholder={t("subscriptionDialog.accountPlaceholder")} />
                      </SelectTrigger>
                    </FormControl>
                    <SelectContent>
                      {selectableAccounts.map((account) => (
                        <SelectItem key={account.id} value={String(account.id)}>
                          <span className="flex min-w-0 flex-1 items-baseline gap-2">
                            <span className="max-w-44 shrink-0 truncate font-medium">
                              {account.name}
                            </span>
                            {account.remark ? (
                              <span
                                className="min-w-0 truncate text-xs text-muted-foreground"
                                title={account.remark}
                              >
                                {account.remark}
                              </span>
                            ) : null}
                            <span className="shrink-0 text-xs tabular-nums text-muted-foreground">
                              ¥{account.cost_yuan}
                            </span>
                            <span className="ml-auto shrink-0 text-xs tabular-nums text-muted-foreground">
                              {account.seat_used}/{account.seat_total}
                            </span>
                          </span>
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                  <FormDescription>
                    {selectedAccount
                      ? selectedAccount.zero_renewal_next_month
                        ? t("subscriptionDialog.accountCostZeroHint", {
                            cost: selectedAccount.cost_yuan,
                          })
                        : t("subscriptionDialog.accountCostHint", {
                            cost: selectedAccount.cost_yuan,
                          })
                      : t("subscriptionDialog.accountHint")}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            {isEdit ? (
              <FormField
                control={form.control}
                name="name"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t("subscriptionDialog.name")}</FormLabel>
                    <FormControl>
                      <Input {...field} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            ) : null}

            <input type="hidden" {...form.register("cost_yuan")} />
            <div className="grid items-start gap-4 sm:grid-cols-2">
              <FormField
                control={form.control}
                name="price_yuan"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t("subscriptionDialog.price")}</FormLabel>
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
                    <FormLabel>{t("subscriptionDialog.boardedAt")}</FormLabel>
                    <FormControl>
                      <Input type="date" {...field} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>

            {isEdit ? (
              <section className="grid gap-3 rounded-xl border border-brand/20 bg-brand/[0.045] p-4">
                <div className="flex items-center gap-2">
                  <span className="grid size-8 place-items-center rounded-lg bg-brand/10 text-brand">
                    <BadgeDollarSign className="size-4" />
                  </span>
                  <div>
                    <h3 className="text-sm font-semibold">
                      {t("subscriptionDialog.nextPriceTitle")}
                    </h3>
                    <p className="text-xs text-muted-foreground">
                      {t("subscriptionDialog.nextPriceSummary")}
                    </p>
                  </div>
                </div>
                <div className="grid items-start gap-3 sm:grid-cols-[1fr_auto]">
                  <FormField
                    control={form.control}
                    name="next_price_yuan"
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t("subscriptionDialog.nextRenewalPrice")}</FormLabel>
                        <FormControl>
                          <Input
                            inputMode="decimal"
                            placeholder={t("subscriptionDialog.nextPricePlaceholder")}
                            {...field}
                          />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                  {nextPriceYuan.trim() ? (
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      className="self-end"
                      onClick={() =>
                        form.setValue("next_price_yuan", "", {
                          shouldDirty: true,
                          shouldValidate: true,
                        })
                      }
                    >
                      {t("subscriptionDialog.cancelPriceChange")}
                    </Button>
                  ) : null}
                </div>
                <p className="text-xs leading-5 text-muted-foreground">
                  {nextPriceYuan.trim()
                    ? scheduleChanged
                      ? t("subscriptionDialog.nextPriceRecalculate", {
                          current: currentPriceYuan || "--",
                          next: nextPriceYuan,
                        })
                      : t("subscriptionDialog.nextPriceEffective", {
                          current: currentPriceYuan || "--",
                          next: nextPriceYuan,
                          date:
                            prefill?.nextPriceEffectiveDueDate || prefill?.nextDueDate || "--",
                        })
                    : t("subscriptionDialog.nextPriceHint")}
                </p>
              </section>
            ) : null}

            <FormField
              control={form.control}
              name="cron_expr"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t("subscriptionDialog.cycle")}</FormLabel>
                  <FormControl>
                    <div className="grid grid-cols-4 gap-1.5" role="radiogroup">
                      {CYCLE_PRESETS.map((preset) => (
                        <Button
                          key={preset.cron}
                          type="button"
                          role="radio"
                          aria-checked={field.value.trim() === preset.cron}
                          variant={field.value.trim() === preset.cron ? "secondary" : "outline"}
                          size="sm"
                          className={cn(
                            "px-2 text-xs",
                            field.value.trim() === preset.cron &&
                              "border-brand/25 bg-brand/10 text-brand",
                          )}
                          onClick={() => field.onChange(preset.cron)}
                        >
                          {t(`subscriptionDialog.${preset.key}`)}
                        </Button>
                      ))}
                    </div>
                  </FormControl>
                  <FormMessage />

                  <div className="rounded-lg border bg-muted/40 px-3.5 py-3">
                    <div className="flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
                      <CalendarClock className="size-3.5" />
                      {t("subscriptionDialog.cronPreviewTitle")}
                      {cronPreview.status === "ok" && cronPreview.description ? (
                        <span className="ml-auto rounded-sm bg-brand/12 px-1.5 py-0.5 text-brand">
                          {cronPreview.description}
                        </span>
                      ) : null}
                    </div>
                    <div className="mt-2 text-xs leading-relaxed">
                      {cronPreview.status === "idle" ? (
                        <span className="text-muted-foreground">
                          {t("subscriptionDialog.cronPreviewHint")}
                        </span>
                      ) : cronPreview.status === "loading" ? (
                        <span className="text-muted-foreground">{t("common.loading")}</span>
                      ) : cronPreview.status === "error" ? (
                        <span className="text-destructive">{cronPreview.message}</span>
                      ) : (
                        <ol className="grid gap-1 font-mono tabular-nums sm:grid-cols-2">
                          {cronPreview.times.map((time, index) => (
                            <li key={time} className="flex items-center gap-2">
                              <span className="text-muted-foreground/60">{index + 1}.</span>
                              {time}
                            </li>
                          ))}
                        </ol>
                      )}
                    </div>
                  </div>
                </FormItem>
              )}
            />

            <FormField
                control={form.control}
                name="notify_offsets"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t("subscriptionDialog.offsets")}</FormLabel>
                    <div className="flex flex-wrap gap-1.5">
                      {OFFSET_OPTIONS.map((offset) => {
                        const checked = field.value.includes(offset)
                        return (
                          <label
                            key={offset}
                            className={cn(
                              "flex cursor-pointer items-center gap-2 rounded-md border px-2.5 py-1.5 text-[13px] transition-colors select-none",
                              checked
                                ? "border-brand/40 bg-brand/10 text-foreground"
                                : "hover:bg-accent",
                            )}
                          >
                            <Checkbox
                              checked={checked}
                              onCheckedChange={(value) => {
                                field.onChange(
                                  value === true
                                    ? [...field.value, offset].sort((a, b) => a - b)
                                    : field.value.filter((item) => item !== offset),
                                )
                              }}
                            />
                            {offset === 0
                              ? t("subscriptionDialog.offsetToday")
                              : t("subscriptionDialog.offsetDays", { count: offset })}
                          </label>
                        )
                      })}
                    </div>
                    <FormDescription>{t("subscriptionDialog.offsetsHint")}</FormDescription>
                  </FormItem>
                )}
            />

            <FormField
                control={form.control}
                name="customer_email"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t("subscriptionDialog.customerEmail")}</FormLabel>
                    <FormControl>
                      <Input
                        type="email"
                        autoComplete="email"
                        placeholder={t("common.optional")}
                        {...field}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
            />

            <FormField
              control={form.control}
              name="customer_wechat"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t("subscriptionDialog.customerWechat")}</FormLabel>
                  <FormControl>
                    <Input
                      autoComplete="off"
                      placeholder={t("subscriptionDialog.customerWechatPlaceholder")}
                      {...field}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name="remark"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t("subscriptionDialog.remark")}</FormLabel>
                  <FormControl>
                    <Textarea rows={2} {...field} />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
                {t("common.cancel")}
              </Button>
              <Button type="submit" disabled={saveMutation.isPending}>
                {t("common.save")}
              </Button>
            </DialogFooter>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  )
}
