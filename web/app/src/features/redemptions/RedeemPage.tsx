import * as React from "react"
import { zodResolver } from "@hookform/resolvers/zod"
import { useMutation, useQuery } from "@tanstack/react-query"
import {
  AlertTriangle,
  Car,
  CheckCircle2,
  Clock3,
  Copy,
  LoaderCircle,
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

import { fetchRedemptionStatus, submitRedemptionApplication } from "@/api/endpoints"
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
const NOTICE_STORAGE_KEY = "carpool-notify:redeem-notice-confirmed"
const SUPPORT_WECHAT_ID = "Jerrylove_Bom"
const SUPPORT_WECHAT_QR_SRC = "/wechat-support-qr.png"
const EMAIL_PATTERN = /^[^@\s]+@[^@\s]+\.[^@\s]+$/
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

function readNoticeConfirmed() {
  try {
    return window.localStorage.getItem(NOTICE_STORAGE_KEY) === "1"
  } catch {
    return false
  }
}

function writeNoticeConfirmed() {
  try {
    window.localStorage.setItem(NOTICE_STORAGE_KEY, "1")
  } catch {
    // localStorage may be unavailable in private browsing.
  }
}

function contactLabel(value: ContactType) {
  return CONTACT_OPTIONS.find((option) => option.value === value)?.label ?? "微信"
}

async function copySupportWechatId() {
  try {
    await navigator.clipboard.writeText(SUPPORT_WECHAT_ID)
    toast.success("已复制客服微信号")
  } catch {
    toast.error(`复制失败，请手动输入 ${SUPPORT_WECHAT_ID}`)
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
          className="rounded-full bg-card/70 shadow-sm backdrop-blur hover:bg-card"
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
          className="h-8 rounded-full bg-card/70 px-3 shadow-sm backdrop-blur hover:bg-card"
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

function WechatQrBlock({ compact = false }: { compact?: boolean }) {
  return (
    <div className="grid gap-3">
      <div className="rounded-lg border bg-white p-2.5">
        <img
          src={SUPPORT_WECHAT_QR_SRC}
          alt="客服微信二维码"
          className={cn("aspect-square w-full rounded-md object-contain", compact ? "max-h-72" : "")}
        />
      </div>
      <div className="flex items-center justify-between gap-3">
        <div className="min-w-0">
          <p className="text-xs font-medium text-muted-foreground">微信号</p>
          <p className="truncate font-mono text-sm font-semibold">{SUPPORT_WECHAT_ID}</p>
        </div>
        <Button
          type="button"
          variant="outline"
          size="sm"
          className="shrink-0"
          onClick={() => void copySupportWechatId()}
        >
          <Copy data-slot="icon" />
          复制
        </Button>
      </div>
    </div>
  )
}

function SupportWechatFloating() {
  return (
    <>
      <aside className="fixed right-6 bottom-6 z-40 hidden w-[280px] rounded-lg border bg-card/95 p-4 shadow-xl shadow-black/10 backdrop-blur lg:block dark:shadow-black/30">
        <div className="mb-3 flex items-center gap-2">
          <div className="grid size-9 place-items-center rounded-lg bg-success/10 text-success">
            <MessageCircle className="size-5" />
          </div>
          <div className="min-w-0">
            <p className="text-base font-semibold">客服微信</p>
            <p className="truncate text-sm text-muted-foreground">售后与续期提醒</p>
          </div>
        </div>
        <WechatQrBlock />
      </aside>

      <Dialog>
        <DialogTrigger asChild>
          <Button
            type="button"
            variant="brand"
            className="fixed right-4 bottom-4 z-40 h-12 rounded-full px-4 shadow-lg lg:hidden"
          >
            <MessageCircle data-slot="icon" />
            客服微信
          </Button>
        </DialogTrigger>
        <DialogContent className="max-w-[390px]">
          <DialogHeader>
            <DialogTitle>添加客服微信</DialogTitle>
            <DialogDescription>
              长期拼车、续费提醒或售后问题，可以扫码添加微信联系。
            </DialogDescription>
          </DialogHeader>
          <WechatQrBlock compact />
        </DialogContent>
      </Dialog>
    </>
  )
}

function RedeemSafetyNoticeDialog({
  open,
  onOpenChange,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const noticeItems = [
    "工作空间与个人空间记录相互独立，加入空间后请把工作空间里的重要对话、文件或资料及时备份。",
    `长期拼车用户建议添加客服微信 ${SUPPORT_WECHAT_ID}，方便售后、续期提醒和异常通知。`,
    "到期后如果没有及时续费，席位可能会被移出空间；移出前未备份的工作空间内容可能无法找回。",
  ]

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent showCloseButton={false} className="sm:max-w-[520px]">
        <DialogHeader>
          <div className="mb-2 grid size-11 place-items-center rounded-lg bg-amber-500/10 text-amber-600 dark:text-amber-400">
            <AlertTriangle className="size-6" />
          </div>
          <DialogTitle className="text-2xl leading-tight">加入前请先确认</DialogTitle>
          <DialogDescription className="leading-6">
            进入共享工作空间前，请先看完下面几点，避免工作空间记录、售后或续期提醒遗漏。
          </DialogDescription>
        </DialogHeader>

        <div className="grid gap-3 text-sm leading-6">
          {noticeItems.map((item, index) => (
            <div key={item} className="flex gap-3 rounded-lg border bg-muted/30 p-3">
              <span className="grid size-6 shrink-0 place-items-center rounded-full bg-background text-xs font-semibold text-muted-foreground">
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
  const [noticeOpen, setNoticeOpen] = React.useState(() => !readNoticeConfirmed())

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
  const handleNoticeOpenChange = (open: boolean) => {
    if (!open) {
      writeNoticeConfirmed()
    }
    setNoticeOpen(open)
  }

  return (
    <main className="min-h-dvh bg-[#f6f7f9] px-4 py-6 pb-24 text-foreground dark:bg-background sm:px-6 lg:pb-6">
      <RedeemSafetyNoticeDialog open={noticeOpen} onOpenChange={handleNoticeOpenChange} />
      <SupportWechatFloating />

      <div className="mx-auto flex min-h-[calc(100dvh-3rem)] w-full max-w-[636px] flex-col justify-center">
        <div className="mb-8 flex justify-end gap-2">
          <RedeemAnnouncementButton onClick={() => setNoticeOpen(true)} />
          <RedeemThemeToggle />
        </div>

        <section className="text-center animate-fade-up">
          <div className="inline-flex items-center gap-3 text-xs font-semibold uppercase tracking-[0.32em] text-muted-foreground">
            <span className="size-2 rounded-full bg-brand" />
            Team Access
          </div>

          <div className="mt-6 flex items-center justify-center gap-4">
            <div className="grid size-12 shrink-0 place-items-center rounded-lg bg-brand text-brand-foreground shadow-sm shadow-brand/25">
              <Car className="size-7" />
            </div>
            <h1 className="text-4xl font-semibold tracking-normal sm:text-5xl">车位兑换</h1>
          </div>

          <p className="mx-auto mt-5 max-w-[560px] text-lg leading-8 text-muted-foreground">
            填写兑换码与账号信息，提交后会同步到后台处理，邀请会发送到你的 GPT 邮箱。
          </p>
        </section>

        <Card className="mt-10 overflow-hidden rounded-lg border bg-card p-0 shadow-sm animate-fade-up [animation-delay:80ms]">
          {trackingToken ? (
            <div className="grid min-h-[360px] gap-6 p-6 sm:p-8">
              <div className="mx-auto grid size-16 place-items-center rounded-lg bg-muted">
                {statusLoadFailed ? (
                  <TicketCheck className="size-8 text-muted-foreground" />
                ) : invited ? (
                  <CheckCircle2 className="size-8 text-success" />
                ) : (
                  <Clock3 className="size-8 text-brand" />
                )}
              </div>

              <div className="text-center">
                <p className="text-xs font-semibold uppercase tracking-[0.28em] text-muted-foreground">
                  {statusLoadFailed ? "Application" : invited ? "Invitation Sent" : "Processing"}
                </p>
                <h2 className="mt-3 text-2xl font-semibold tracking-normal sm:text-3xl">
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
                <div className="grid gap-3 rounded-lg border bg-muted/30 p-4 text-sm">
                  <div className="flex items-center justify-between gap-4">
                    <span className="text-muted-foreground">GPT 邮箱</span>
                    <span className="min-w-0 truncate font-mono">
                      {statusQuery.data?.customer_email || "加载中"}
                    </span>
                  </div>
                  <div className="flex items-center justify-between gap-4">
                    <span className="text-muted-foreground">提交时间</span>
                    <span className="tabular-nums">
                      {statusQuery.data?.created_at_label || "加载中"}
                    </span>
                  </div>
                  {invited ? (
                    <div className="flex items-center justify-between gap-4">
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
                <p className="text-lg leading-8 text-muted-foreground">
                  请填写用于加入 Team 的 GPT 邮箱，以及方便联系的微信或 QQ。
                </p>

                <FormField
                  control={form.control}
                  name="redeem_code"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel className="text-base">兑换码</FormLabel>
                      <FormControl>
                        <Input
                          autoComplete="off"
                          placeholder="CPN-XXXX-XXXX-XXXX"
                          className="h-14 rounded-lg px-5 text-base"
                          {...field}
                        />
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
                        <Input
                          type="email"
                          autoComplete="email"
                          placeholder="name@example.com"
                          className="h-14 rounded-lg px-5 text-base"
                          {...field}
                        />
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
                                  "flex h-14 items-center justify-center gap-2 rounded-lg border bg-muted/35 text-base font-semibold transition-all",
                                  "hover:border-brand/50 hover:bg-brand/5 focus-visible:border-ring focus-visible:ring-ring/40 focus-visible:ring-[3px] focus-visible:outline-none",
                                  selected &&
                                    "border-brand bg-brand/10 text-foreground shadow-sm shadow-brand/10",
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
                        <Input
                          autoComplete="off"
                          placeholder={activeContact?.placeholder ?? "方便核对订单"}
                          className="h-14 rounded-lg px-5 text-base"
                          {...field}
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                <Button
                  type="submit"
                  className="mt-2 h-14 text-base"
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

        <p className="mx-auto mt-7 max-w-md text-center text-sm leading-7 text-muted-foreground">
          兑换码仅限本人使用，提交后请保持本页打开等待邀请状态更新。
        </p>
      </div>
    </main>
  )
}
