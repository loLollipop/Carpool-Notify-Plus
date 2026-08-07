import * as React from "react"
import { zodResolver } from "@hookform/resolvers/zod"
import { useMutation, useQuery } from "@tanstack/react-query"
import {
  AlertTriangle,
  CheckCircle2,
  Clock3,
  Copy,
  LoaderCircle,
  Mail,
  Megaphone,
  MessageCircle,
  Moon,
  Sun,
  TicketCheck,
} from "lucide-react"
import { useTheme } from "next-themes"
import { useForm, useWatch } from "react-hook-form"
import { toast } from "sonner"
import { z } from "zod"

import {
  fetchRedeemPageSettings,
  fetchRedemptionStatus,
  submitRedemptionApplication,
} from "@/api/endpoints"
import type { RedeemPageSettings } from "@/api/types"
import { APP_NAME, BrandIcon } from "@/components/brand"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
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
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { cn } from "@/lib/utils"

const STORAGE_KEY = "carpool-notify:redemption-token"
const EMAIL_PATTERN = /^[^@\s]+@[^@\s]+\.[^@\s]+$/
const DEFAULT_REDEEM_PAGE_SETTINGS: RedeemPageSettings = {
  announcement_title: "加入前请先确认",
  announcement_intro:
    "进入共享工作空间前，请先看完下面几点，避免工作空间记录、售后或续期提醒遗漏。",
  announcement_items: [
    "工作空间与个人空间记录相互独立，加入空间后请把工作空间里的重要对话、文件或资料及时备份。",
    "长期使用建议保存管理员联系方式，方便售后、续期提醒和异常通知。",
    "到期后如果没有及时续费，席位可能会被移出空间；移出前未备份的工作空间内容可能无法找回。",
  ],
  support_title: "客服微信",
  support_description: "售后与续期提醒",
  support_contact_label: "微信号",
  support_wechat_id: "",
  support_qr_data_url: "",
}
const CONTACT_OPTIONS = [
  { value: "wechat", label: "微信", placeholder: "字母开头，6-20 位", tone: "text-success" },
  { value: "qq", label: "QQ", placeholder: "请输入 QQ 号", tone: "text-brand" },
] as const

const schema = z.object({
  customer_email: z
    .string()
    .trim()
    .min(1, "请填写邮箱")
    .regex(EMAIL_PATTERN, "邮箱格式不正确")
    .max(254, "邮箱太长"),
  redeem_code: z.string().trim().min(1, "请填写兑换码").max(120, "兑换码太长"),
  contact_type: z.enum(["wechat", "qq"]),
  customer_contact: z.string().trim().min(1, "请填写联系方式").max(80, "联系方式太长"),
})

type FormValues = z.infer<typeof schema>
type ContactType = FormValues["contact_type"]

function readStoredToken() {
  try {
    return window.localStorage.getItem(STORAGE_KEY) ?? ""
  } catch {
    return ""
  }
}

function writeStoredToken(token: string) {
  try {
    if (token) {
      window.localStorage.setItem(STORAGE_KEY, token)
    } else {
      window.localStorage.removeItem(STORAGE_KEY)
    }
  } catch {
    // localStorage may be unavailable in private browsing.
  }
}

function contactLabel(value: ContactType) {
  return CONTACT_OPTIONS.find((option) => option.value === value)?.label ?? "微信"
}

function normalizeRedeemPageSettings(settings?: RedeemPageSettings | null): RedeemPageSettings {
  const merged = { ...DEFAULT_REDEEM_PAGE_SETTINGS, ...(settings ?? {}) }
  const items = (merged.announcement_items ?? [])
    .map((item) => item.trim())
    .filter((item) => item !== "")
  return {
    ...merged,
    announcement_items:
      items.length > 0 ? items : DEFAULT_REDEEM_PAGE_SETTINGS.announcement_items,
  }
}

function hasSupportContact(settings: RedeemPageSettings) {
  return settings.support_wechat_id.trim() !== "" || settings.support_qr_data_url.trim() !== ""
}

async function copySupportWechatId(wechatId: string) {
  const value = wechatId.trim()
  if (!value) {
    toast.error("暂未配置客服微信号")
    return
  }
  try {
    await navigator.clipboard.writeText(value)
    toast.success("已复制客服微信号")
  } catch {
    toast.error(`复制失败，请手动输入 ${value}`)
  }
}

function RedeemThemeToggle() {
  const { resolvedTheme, setTheme } = useTheme()

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          aria-label="切换深浅色"
          className="border bg-card text-muted-foreground hover:bg-accent hover:text-foreground"
          onClick={() => setTheme(resolvedTheme === "dark" ? "light" : "dark")}
        >
          <Sun className="size-4 scale-100 rotate-0 transition-all duration-300 dark:scale-0 dark:-rotate-90" />
          <Moon className="absolute size-4 scale-0 rotate-90 transition-all duration-300 dark:scale-100 dark:rotate-0" />
        </Button>
      </TooltipTrigger>
      <TooltipContent>切换深浅色</TooltipContent>
    </Tooltip>
  )
}

function RedeemAnnouncementButton({ onClick }: { onClick: () => void }) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          type="button"
          variant="outline"
          size="sm"
          aria-label="查看公告"
          className="h-8 bg-card px-3"
          onClick={onClick}
        >
          <Megaphone data-slot="icon" className="size-4" />
          公告
        </Button>
      </TooltipTrigger>
      <TooltipContent>查看加入空间说明</TooltipContent>
    </Tooltip>
  )
}

function WechatQrBlock({
  settings,
  compact = false,
}: {
  settings: RedeemPageSettings
  compact?: boolean
}) {
  const wechatId = settings.support_wechat_id.trim()
  const qrDataURL = settings.support_qr_data_url.trim()

  return (
    <div className="grid gap-4">
      {qrDataURL ? (
        <div className="mx-auto w-full max-w-[260px] bg-white p-2">
          <img
            src={qrDataURL}
            alt="客服微信二维码"
            className={cn("aspect-square w-full object-contain", compact ? "max-h-80" : "")}
          />
        </div>
      ) : null}
      {wechatId ? (
        <div className="flex items-center justify-between gap-3 border-t pt-4">
          <div className="min-w-0">
            <p className="text-xs font-medium text-muted-foreground">
              {settings.support_contact_label || "微信号"}
            </p>
            <p className="truncate font-mono text-sm font-semibold">{wechatId}</p>
          </div>
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="shrink-0"
            onClick={() => void copySupportWechatId(wechatId)}
          >
            <Copy data-slot="icon" />
            复制
          </Button>
        </div>
      ) : null}
    </div>
  )
}

function SupportWechatPanel({ settings }: { settings: RedeemPageSettings }) {
  if (!hasSupportContact(settings)) {
    return null
  }

  return (
    <aside className="hidden overflow-hidden rounded-lg border bg-card p-5 shadow-[0_1px_2px_color-mix(in_oklab,var(--foreground)_4%,transparent)] lg:block lg:sticky lg:top-6">
      <div className="mb-5 flex items-center gap-3 border-b pb-4">
        <div className="grid size-9 place-items-center rounded-md bg-success/10 text-success">
          <MessageCircle className="size-5" />
        </div>
        <div className="min-w-0">
          <p className="text-base font-semibold">{settings.support_title}</p>
          <p className="mt-0.5 text-sm leading-5 text-muted-foreground">
            {settings.support_description}
          </p>
        </div>
      </div>
      <WechatQrBlock settings={settings} />
    </aside>
  )
}

function SupportWechatDialogButton({ settings }: { settings: RedeemPageSettings }) {
  if (!hasSupportContact(settings)) {
    return null
  }

  return (
    <Dialog>
      <DialogTrigger asChild>
        <Button type="button" variant="outline" size="sm" aria-label="客服微信" className="h-8 lg:hidden">
          <MessageCircle data-slot="icon" />
          客服
        </Button>
      </DialogTrigger>
      <DialogContent className="max-w-[390px]">
        <DialogHeader>
          <DialogTitle>{settings.support_title}</DialogTitle>
          <DialogDescription>{settings.support_description}</DialogDescription>
        </DialogHeader>
        <WechatQrBlock settings={settings} compact />
      </DialogContent>
    </Dialog>
  )
}

function RedeemSafetyNoticeDialog({
  open,
  onOpenChange,
  settings,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  settings: RedeemPageSettings
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent showCloseButton={false} className="sm:max-w-[520px]">
        <DialogHeader>
          <div className="mb-2 grid size-11 place-items-center rounded-lg bg-amber-500/10 text-amber-600 dark:text-amber-400">
            <AlertTriangle className="size-6" />
          </div>
          <DialogTitle className="text-2xl leading-tight">{settings.announcement_title}</DialogTitle>
          <DialogDescription className="leading-6">
            {settings.announcement_intro}
          </DialogDescription>
        </DialogHeader>

        <div className="divide-y overflow-hidden rounded-lg border text-sm leading-6">
          {settings.announcement_items.map((item, index) => (
            <div key={item} className="flex gap-3 p-4">
              <span className="grid size-6 shrink-0 place-items-center rounded-full bg-muted text-xs font-semibold text-muted-foreground">
                {index + 1}
              </span>
              <p>{item}</p>
            </div>
          ))}
        </div>

        <Button type="button" className="h-11 w-full" onClick={() => onOpenChange(false)}>
          我已了解，继续兑换
        </Button>
      </DialogContent>
    </Dialog>
  )
}

export function RedeemPage() {
  const [trackingToken, setTrackingToken] = React.useState(readStoredToken)
  const [noticeOpen, setNoticeOpen] = React.useState(true)

  const settingsQuery = useQuery({
    queryKey: ["public-redeem-settings"],
    queryFn: fetchRedeemPageSettings,
    staleTime: 5 * 60 * 1000,
  })
  const redeemSettings = normalizeRedeemPageSettings(settingsQuery.data)

  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: {
      customer_email: "",
      redeem_code: "",
      contact_type: "wechat",
      customer_contact: "",
    },
  })
  const contactType = useWatch({ control: form.control, name: "contact_type" })

  const statusQuery = useQuery({
    queryKey: ["public-redemption-status", trackingToken],
    queryFn: () => fetchRedemptionStatus(trackingToken),
    enabled: trackingToken !== "",
    refetchInterval: (query) => (query.state.data?.status === "pending" ? 5_000 : false),
    retry: false,
  })

  const submitMutation = useMutation({
    mutationFn: (values: FormValues) =>
      submitRedemptionApplication({
        customer_email: values.customer_email.trim(),
        customer_contact: `${contactLabel(values.contact_type)}：${values.customer_contact.trim()}`,
        redeem_code: values.redeem_code.trim(),
        request_note: "",
      }),
    onSuccess: (result) => {
      setTrackingToken(result.tracking_token)
      writeStoredToken(result.tracking_token)
      form.reset()
      toast.success(result.message ?? "申请已提交")
    },
    onError: (error: Error) => toast.error(error.message),
  })

  const resetApplication = () => {
    setTrackingToken("")
    writeStoredToken("")
    form.reset()
  }

  const status = statusQuery.data?.status ?? (trackingToken ? "pending" : null)
  const invited = status === "invited"
  const statusLoadFailed = trackingToken !== "" && statusQuery.isError
  const activeContact = CONTACT_OPTIONS.find((option) => option.value === contactType)
  const supportConfigured = hasSupportContact(redeemSettings)
  const supportColumnVisible = supportConfigured || settingsQuery.isPending

  return (
    <main className="min-h-dvh bg-background text-foreground">
      <RedeemSafetyNoticeDialog
        open={noticeOpen}
        onOpenChange={setNoticeOpen}
        settings={redeemSettings}
      />

      <header className="border-b bg-card">
        <div className="mx-auto flex h-16 w-full max-w-[1100px] items-center px-4 sm:px-6">
          <div className="flex min-w-0 items-center gap-3">
            <BrandIcon />
            <div className="min-w-0 leading-none">
              <p className="truncate text-sm font-semibold">{APP_NAME}</p>
              <p className="mt-1 hidden text-[11px] text-muted-foreground sm:block">Team Access</p>
            </div>
          </div>
          <div className="ml-auto flex items-center gap-2">
            <RedeemAnnouncementButton onClick={() => setNoticeOpen(true)} />
            <SupportWechatDialogButton settings={redeemSettings} />
            <RedeemThemeToggle />
          </div>
        </div>
      </header>

      <div className="mx-auto w-full max-w-[1100px] px-4 py-8 sm:px-6">
        <section className="max-w-[720px] animate-fade-up">
          <div className="flex items-center gap-2 text-xs font-semibold text-brand">
            <TicketCheck className="size-3.5" />
            ChatGPT Team
          </div>
          <h1 className="mt-2 text-3xl font-semibold sm:text-[38px]">车位兑换</h1>
          <p className="mt-3 max-w-[620px] text-sm leading-6 text-muted-foreground sm:text-base">
            填写兑换码与账号信息，提交后会同步到后台处理，邀请会发送到你的 GPT 邮箱。
          </p>
        </section>

        <div
          className={cn(
            "mt-7 grid items-start gap-6",
            supportColumnVisible ? "lg:grid-cols-[minmax(0,1fr)_320px]" : "max-w-[720px]",
          )}
        >
        <Card className="overflow-hidden p-0 animate-fade-up [animation-delay:80ms]">
          <div className="flex items-center justify-between border-b bg-muted/35 px-5 py-3 sm:px-7">
            <div className="flex items-center gap-2 text-sm font-semibold">
              <span className="grid size-7 place-items-center rounded-md bg-brand/10 text-brand">
                <TicketCheck className="size-4" />
              </span>
              {trackingToken ? "申请进度" : "兑换信息"}
            </div>
            <span className="text-[11px] font-medium text-muted-foreground">SEATFLOW / TEAM</span>
          </div>
          {trackingToken ? (
            <div className="grid min-h-[360px] gap-6 p-6 sm:p-8">
              <div
                className={cn(
                  "mx-auto grid size-16 place-items-center rounded-lg border",
                  statusLoadFailed
                    ? "border-muted bg-muted"
                    : invited
                      ? "border-success/20 bg-success/10"
                      : "border-brand/20 bg-brand/10",
                )}
              >
                {statusLoadFailed ? (
                  <TicketCheck className="size-8 text-muted-foreground" />
                ) : invited ? (
                  <CheckCircle2 className="size-8 text-success" />
                ) : (
                  <Clock3 className="size-8 text-brand" />
                )}
              </div>

              <div className="text-center">
                <p className="text-xs font-semibold uppercase text-muted-foreground">
                  {statusLoadFailed ? "Application" : invited ? "Invitation Sent" : "Processing"}
                </p>
                <h2 className="mt-3 text-2xl font-semibold sm:text-3xl">
                  {statusLoadFailed
                    ? "没有找到这条申请，请重新提交"
                    : invited
                      ? "已成功发送邀请，请在邮箱中点击确认加入空间"
                      : "申请已提交，请耐心等待 1-2 分钟"}
                </h2>
                <p className="mx-auto mt-3 max-w-md text-sm leading-7 text-muted-foreground">
                  {statusLoadFailed
                    ? "可能是本地保存的旧记录已经失效，重新提交兑换信息即可。"
                    : invited
                      ? "如果收件箱没看到邀请，可以检查垃圾邮件或稍等邮箱同步。"
                      : "当前页面会自动同步处理进度，完成后会直接更新为邀请已发送。"}
                </p>
              </div>

              {!statusLoadFailed ? (
                <div className="divide-y border-y text-sm">
                  <div className="flex items-center justify-between gap-4 py-3">
                    <span className="text-muted-foreground">GPT 邮箱</span>
                    <span className="min-w-0 truncate font-mono">
                      {statusQuery.data?.customer_email || "加载中"}
                    </span>
                  </div>
                  <div className="flex items-center justify-between gap-4 py-3">
                    <span className="text-muted-foreground">提交时间</span>
                    <span className="tabular-nums">
                      {statusQuery.data?.created_at_label || "加载中"}
                    </span>
                  </div>
                  {invited ? (
                    <div className="flex items-center justify-between gap-4 py-3">
                      <span className="text-muted-foreground">邀请时间</span>
                      <span className="tabular-nums">
                        {statusQuery.data?.invited_at_label || "刚刚"}
                      </span>
                    </div>
                  ) : null}
                </div>
              ) : null}

              <Button
                type="button"
                variant="outline"
                className="h-11 w-full"
                onClick={resetApplication}
              >
                重新提交兑换
              </Button>
            </div>
          ) : (
            <Form {...form}>
              <form
                onSubmit={form.handleSubmit((values) => submitMutation.mutate(values))}
                className="grid gap-5 p-6 sm:p-8"
              >
                <p className="border-b pb-5 text-sm leading-6 text-muted-foreground sm:text-base">
                  请填写用于加入 Team 的 GPT 邮箱，以及方便联系的微信或 QQ。
                </p>

                <FormField
                  control={form.control}
                  name="redeem_code"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel className="text-base">兑换码</FormLabel>
                      <FormControl>
                        <div className="relative">
                          <TicketCheck className="pointer-events-none absolute top-1/2 left-4 size-5 -translate-y-1/2 text-brand" />
                          <Input
                            autoComplete="off"
                            placeholder="CPN-XXXX-XXXX-XXXX"
                            className="h-13 rounded-lg pr-5 pl-12 text-base"
                            {...field}
                          />
                        </div>
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                <FormField
                  control={form.control}
                  name="customer_email"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel className="text-base">GPT 邮箱</FormLabel>
                      <FormControl>
                        <div className="relative">
                          <Mail className="pointer-events-none absolute top-1/2 left-4 size-5 -translate-y-1/2 text-brand" />
                          <Input
                            type="email"
                            autoComplete="email"
                            placeholder="name@example.com"
                            className="h-13 rounded-lg pr-5 pl-12 text-base"
                            {...field}
                          />
                        </div>
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                <FormField
                  control={form.control}
                  name="contact_type"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel className="text-base">联系方式</FormLabel>
                      <FormControl>
                        <div className="grid grid-cols-2 gap-3">
                          {CONTACT_OPTIONS.map((option) => {
                            const selected = field.value === option.value
                            return (
                              <button
                                key={option.value}
                                type="button"
                                aria-pressed={selected}
                                className={cn(
                                  "flex h-13 items-center justify-center gap-2 rounded-lg border bg-card text-base font-semibold transition-[border-color,background-color,color,box-shadow]",
                                  "hover:border-brand/50 hover:bg-accent/50 focus-visible:border-ring focus-visible:ring-ring/40 focus-visible:ring-[3px] focus-visible:outline-none",
                                  selected &&
                                    "border-brand bg-brand/10 text-foreground",
                                )}
                                onClick={() => field.onChange(option.value)}
                              >
                                <MessageCircle className={cn("size-5", option.tone)} />
                                {option.label}
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
                  name="customer_contact"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel className="text-base">{contactLabel(contactType)}号</FormLabel>
                      <FormControl>
                        <div className="relative">
                          <MessageCircle className="pointer-events-none absolute top-1/2 left-4 size-5 -translate-y-1/2 text-success" />
                          <Input
                            autoComplete="off"
                            placeholder={activeContact?.placeholder ?? "方便核对订单"}
                            className="h-13 rounded-lg pr-5 pl-12 text-base"
                            {...field}
                          />
                        </div>
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                <Button
                  type="submit"
                  className="mt-1 h-13 text-base"
                  disabled={submitMutation.isPending}
                >
                  {submitMutation.isPending ? (
                    <>
                      <LoaderCircle data-slot="icon" className="animate-spin" />
                      提交中
                    </>
                  ) : (
                    <>
                      <TicketCheck data-slot="icon" />
                      提交申请
                    </>
                  )}
                </Button>
              </form>
            </Form>
          )}
        </Card>
        {settingsQuery.isPending ? (
          <aside className="hidden h-[430px] animate-pulse rounded-lg border bg-muted/45 lg:block" />
        ) : (
          <SupportWechatPanel settings={redeemSettings} />
        )}
        </div>

        <p className="mt-6 max-w-[720px] text-center text-xs leading-6 text-muted-foreground sm:text-sm">
          兑换码仅限本人使用，提交后请保持本页打开等待邀请状态更新。
        </p>
      </div>
    </main>
  )
}
