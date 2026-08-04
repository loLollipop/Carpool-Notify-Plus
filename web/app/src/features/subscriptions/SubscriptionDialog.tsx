import * as React from "react"
import { zodResolver } from "@hookform/resolvers/zod"
import { useForm, useWatch } from "react-hook-form"
import { useTranslation } from "react-i18next"
import { CalendarClock } from "lucide-react"
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Switch } from "@/components/ui/switch"
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
        cost_yuan: z
          .string()
          .trim()
          .refine((value) => value === "" || MONEY_PATTERN.test(value), {
            message: t("subscriptionDialog.validation.priceInvalid"),
          }),
        is_resale: z.boolean(),
        agency_fee_yuan: z
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
        trade_url: z.string(),
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
      cost_yuan: prefill?.costYuan ?? "",
      is_resale: prefill?.isResale ?? false,
      agency_fee_yuan: prefill?.agencyFeeYuan ?? "",
      cron_expr: prefill?.cronExpr ?? "",
      notify_offsets: prefill?.offsets ?? [],
      boarded_at: prefill?.boardedAt || todayShanghai(),
      customer_email: prefill?.customerEmail ?? "",
      customer_wechat: prefill?.customerWechat ?? "",
      trade_url: prefill?.tradeUrl ?? "",
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
  const isResale = useWatch({ control: form.control, name: "is_resale" })
  const accountId = useWatch({ control: form.control, name: "account_id" })
  const cronPreview = useCronPreview(cronExpr, boardedAt, open)
  const cronInputRef = React.useRef<HTMLInputElement | null>(null)

  const saveMutation = useAppMutation(
    (values: FormValues) => {
      const input: SubscriptionInput = {
        name: values.name.trim(),
        price_yuan: values.price_yuan.trim(),
        cost_yuan: isEdit ? values.cost_yuan.trim() : "",
        is_resale: values.is_resale,
        agency_fee_yuan: values.is_resale ? values.agency_fee_yuan.trim() : "0",
        cron_expr: values.cron_expr.trim(),
        notify_offsets: values.notify_offsets,
        remark: values.remark.trim(),
        trade_url: values.trade_url.trim(),
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
      <DialogContent className="sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>
            {isEdit ? t("subscriptionDialog.editTitle") : t("subscriptionDialog.createTitle")}
          </DialogTitle>
          <DialogDescription>{t("subscriptionDialog.desc")}</DialogDescription>
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
                    <FormDescription>{t("subscriptionDialog.nameHint")}</FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            ) : null}

            <input type="hidden" {...form.register("cost_yuan")} />
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
              name="is_resale"
              render={({ field }) => (
                <FormItem className="flex flex-row items-center justify-between rounded-lg border border-border/60 px-3 py-2.5">
                  <div className="space-y-0.5 pr-4">
                    <FormLabel>{t("subscriptionDialog.resale")}</FormLabel>
                    <FormDescription>{t("subscriptionDialog.resaleHint")}</FormDescription>
                  </div>
                  <FormControl>
                    <Switch checked={field.value} onCheckedChange={field.onChange} />
                  </FormControl>
                </FormItem>
              )}
            />

            {isResale ? (
              <FormField
                control={form.control}
                name="agency_fee_yuan"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t("subscriptionDialog.agencyFee")}</FormLabel>
                    <FormControl>
                      <Input inputMode="decimal" placeholder="0.00" {...field} />
                    </FormControl>
                    <FormDescription>{t("subscriptionDialog.agencyFeeHint")}</FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            ) : null}

            <FormField
              control={form.control}
              name="boarded_at"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t("subscriptionDialog.boardedAt")}</FormLabel>
                  <FormControl>
                    <Input type="date" {...field} />
                  </FormControl>
                  <FormDescription>{t("subscriptionDialog.boardedAtHint")}</FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name="cron_expr"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t("subscriptionDialog.cycle")}</FormLabel>
                  <div className="flex flex-wrap gap-2">
                    {CYCLE_PRESETS.map((preset) => (
                      <Button
                        key={preset.cron}
                        type="button"
                        variant={field.value.trim() === preset.cron ? "secondary" : "outline"}
                        size="sm"
                        onClick={() => field.onChange(preset.cron)}
                      >
                        {t(`subscriptionDialog.${preset.key}`)}
                      </Button>
                    ))}
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      onClick={() => cronInputRef.current?.focus()}
                    >
                      {t("subscriptionDialog.presetCustom")}
                    </Button>
                  </div>
                  <FormControl>
                    <Input
                      placeholder="interval:30d"
                      className="font-mono"
                      {...field}
                      ref={(element) => {
                        field.ref(element)
                        cronInputRef.current = element
                      }}
                    />
                  </FormControl>
                  <FormDescription>{t("subscriptionDialog.cronHint")}</FormDescription>
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
              name="trade_url"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t("subscriptionDialog.tradeUrl")}</FormLabel>
                  <FormControl>
                    <Input placeholder="https://" {...field} />
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
