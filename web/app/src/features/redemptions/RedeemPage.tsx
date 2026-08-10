import * as React from "react"
import { zodResolver } from "@hookform/resolvers/zod"
import { useMutation, useQuery } from "@tanstack/react-query"
import {
  AlertTriangle,
  ArrowRight,
  CheckCircle2,
  Clock3,
  Copy,
  LoaderCircle,
  Mail,
  Megaphone,
  MessageCircle,
  Moon,
  ShieldCheck,
  Sun,
  TicketCheck,
  Zap,
} from "lucide-react"
import { useTheme } from "next-themes"
import { useForm } from "react-hook-form"
import { useSearchParams } from "react-router-dom"
import { toast } from "sonner"
import { z } from "zod"

import {
  fetchRedeemPageSettings,
  fetchRedemptionStatus,
  submitRedemptionApplication,
} from "@/api/endpoints"
import type { RedeemPageSettings, RedemptionStatus } from "@/api/types"
import { APP_NAME, BrandIcon } from "@/components/brand"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
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
  announcement_title: "加入 ChatGPT Team 前请先确认",
  announcement_intro:
    "为保护工作空间中的内容与到期后的使用连续性，请先阅读以下说明。",
  announcement_items: [
    "工作空间与个人空间的记录相互独立，请及时备份工作空间中的重要对话、文件和资料。",
    "长期使用建议添加管理员微信，方便接收续费提醒、售后协助和异常通知。",
    "到期后若未及时续费，账号可能会被移出 Team；未备份的工作空间内容可能无法找回。",
  ],
  support_title: "客服微信",
  support_description: "续费提醒与售后协助",
  support_contact_label: "微信号",
  support_wechat_id: "",
  support_qr_data_url: "",
}
const TRUST_CHIPS = [
  { icon: Zap, label: "通常 1-2 分钟完成" },
  { icon: Mail, label: "邀请直达 GPT 邮箱" },
  { icon: ShieldCheck, label: "客服微信全程协助" },
] as const

const schema = z.object({
  customer_email: z
    .string()
    .trim()
    .min(1, "请填写邮箱")
    .regex(EMAIL_PATTERN, "邮箱格式不正确")
    .max(254, "邮箱太长"),
  redeem_code: z.string().trim().min(1, "请填写兑换码").max(120, "兑换码太长"),
  customer_contact: z.string().trim().min(1, "请填写微信号").max(80, "微信号太长"),
})

type FormValues = z.infer<typeof schema>

function redemptionTokenStorageKey(sandboxAccessToken: string) {
  return sandboxAccessToken ? `${STORAGE_KEY}:sandbox:${sandboxAccessToken}` : STORAGE_KEY
}

function readStoredToken(storageKey: string) {
  try {
    return window.localStorage.getItem(storageKey) ?? ""
  } catch {
    return ""
  }
}

function writeStoredToken(storageKey: string, token: string) {
  try {
    if (token) {
      window.localStorage.setItem(storageKey, token)
    } else {
      window.localStorage.removeItem(storageKey)
    }
  } catch {
    // localStorage may be unavailable in private browsing.
  }
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
          className="border border-white/15 bg-white/[0.06] text-white hover:bg-white/10 hover:text-white"
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
          variant="ghost"
          size="sm"
          aria-label="查看公告"
          className="h-8 border border-white/15 bg-white/[0.06] px-3 text-white hover:bg-white/10 hover:text-white"
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
  stretched = false,
}: {
  settings: RedeemPageSettings
  compact?: boolean
  stretched?: boolean
}) {
  const wechatId = settings.support_wechat_id.trim()
  const qrDataURL = settings.support_qr_data_url.trim()

  return (
    <div className={cn("grid gap-4", stretched && "min-h-0 flex-1 grid-rows-[1fr_auto]")}>
      {qrDataURL ? (
        <div className="mx-auto w-full max-w-[260px] self-center rounded-lg border bg-white p-2.5 shadow-sm">
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
    <aside className="hidden h-full overflow-hidden rounded-xl border bg-card p-5 shadow-card lg:flex lg:flex-col">
      <div className="mb-5 flex items-center gap-3 border-b pb-4">
        <div className="grid size-9 place-items-center rounded-lg bg-success/10 text-success">
          <MessageCircle className="size-5" />
        </div>
        <div className="min-w-0">
          <p className="text-base font-semibold">{settings.support_title}</p>
          <p className="mt-0.5 text-sm leading-5 text-muted-foreground">
            {settings.support_description}
          </p>
        </div>
      </div>
      <WechatQrBlock settings={settings} stretched />
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
        <Button
          type="button"
          variant="ghost"
          size="sm"
          aria-label="客服微信"
          className="h-8 border border-white/15 bg-white/[0.06] text-white hover:bg-white/10 hover:text-white lg:hidden"
        >
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

type RedeemFlowStep = "review" | "progress"

function ReviewItem({
  icon,
  label,
  value,
  mono = false,
}: {
  icon: React.ReactNode
  label: string
  value: string
  mono?: boolean
}) {
  return (
    <div className="grid gap-2 px-4 py-3.5 sm:grid-cols-[130px_minmax(0,1fr)] sm:items-center sm:px-5">
      <div className="flex items-center gap-2 text-xs font-medium text-muted-foreground">
        {icon}
        {label}
      </div>
      <div className={cn("break-all text-sm font-semibold sm:text-right", mono && "font-mono")}>
        {value}
      </div>
    </div>
  )
}

function RedemptionFlowDialog({
  open,
  step,
  reviewValues,
  submitting,
  status,
  statusLoadFailed,
  onOpenChange,
  onConfirm,
  onRestart,
}: {
  open: boolean
  step: RedeemFlowStep
  reviewValues: FormValues | null
  submitting: boolean
  status: RedemptionStatus | undefined
  statusLoadFailed: boolean
  onOpenChange: (open: boolean) => void
  onConfirm: () => void
  onRestart: () => void
}) {
  const resolvedStatus = status?.status ?? "pending"
  const invited = resolvedStatus === "invited"
  const rejected = resolvedStatus === "rejected"
  const pending = !statusLoadFailed && !invited && !rejected
  const rejectionReason = status?.rejection_reason?.trim() ?? ""
  const canRestartApplication = statusLoadFailed || invited || rejected
  const statusEyebrow = statusLoadFailed
    ? "Application"
    : invited
      ? "Invitation Sent"
      : rejected
        ? "Rejected"
        : "Processing"
  const statusHeadline = statusLoadFailed
    ? "没有找到这条申请，请重新提交"
    : invited
      ? "已成功发送邀请，请在邮箱中点击确认加入空间"
      : rejected
        ? "申请已驳回，请修改信息后重新提交"
        : "申请已提交，管理员正在处理"
  const statusDescription = statusLoadFailed
    ? "可能是本地保存的旧记录已经失效，重新提交兑换信息即可。"
    : invited
      ? "如果收件箱没看到邀请，可以检查垃圾邮件或稍等邮箱同步。"
      : rejected
        ? rejectionReason
          ? `管理员说明：${rejectionReason}`
          : "提交的信息有误，请检查兑换资料后重新提交。"
        : "进度会在这里自动更新，通常需要 1-2 分钟。"
  const statusActionLabel = statusLoadFailed
    ? "重新提交兑换"
    : invited
      ? "继续兑换"
      : rejected
        ? "重新提交兑换"
        : "等待管理员处理中"

  if (step === "review" && reviewValues === null) {
    return null
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      {step === "review" && reviewValues ? (
        <DialogContent
          key="review"
          className="gap-5 sm:max-w-[600px] sm:p-7"
          showCloseButton={!submitting}
          onInteractOutside={(event) => {
            if (submitting) event.preventDefault()
          }}
          onEscapeKeyDown={(event) => {
            if (submitting) event.preventDefault()
          }}
        >
          <DialogHeader className="gap-2 pr-8">
            <div className="mb-1 grid size-11 place-items-center rounded-lg border border-brand/15 bg-brand/10 text-brand">
              <TicketCheck className="size-5" />
            </div>
            <DialogTitle className="text-xl leading-tight sm:text-2xl">请核对兑换信息</DialogTitle>
            <DialogDescription className="leading-6">
              管理员会按照以下资料发送 Team 邀请，提交前请确认信息准确无误。
            </DialogDescription>
          </DialogHeader>

          <div className="divide-y overflow-hidden rounded-lg border bg-muted/20">
            <ReviewItem
              icon={<TicketCheck className="size-4 text-brand" />}
              label="兑换码"
              value={reviewValues.redeem_code}
              mono
            />
            <ReviewItem
              icon={<Mail className="size-4 text-brand" />}
              label="GPT 邮箱"
              value={reviewValues.customer_email}
              mono
            />
            <ReviewItem
              icon={<MessageCircle className="size-4 text-success" />}
              label="微信号"
              value={reviewValues.customer_contact}
            />
          </div>

          <div className="flex items-start gap-3 rounded-lg border border-warning/25 bg-warning/[0.07] px-4 py-3 text-sm leading-6">
            <AlertTriangle className="mt-0.5 size-4 shrink-0 text-warning" />
            <p className="text-muted-foreground">
              请重点核对 GPT 邮箱。邮箱填写错误会导致邀请发送到错误账号。
            </p>
          </div>

          <DialogFooter className="border-t pt-5 sm:justify-between">
            <Button
              type="button"
              variant="outline"
              disabled={submitting}
              onClick={() => onOpenChange(false)}
            >
              返回修改
            </Button>
            <Button type="button" className="sm:min-w-44" disabled={submitting} onClick={onConfirm}>
              {submitting ? (
                <>
                  <LoaderCircle data-slot="icon" className="animate-spin" />
                  正在提交
                </>
              ) : (
                <>
                  确认无误，继续兑换
                  <ArrowRight data-slot="icon" />
                </>
              )}
            </Button>
          </DialogFooter>
        </DialogContent>
      ) : (
        <DialogContent
          key="progress"
          className="gap-6 sm:max-w-[680px] sm:p-7"
          showCloseButton={false}
          onInteractOutside={(event) => event.preventDefault()}
          onEscapeKeyDown={(event) => event.preventDefault()}
        >
          <div aria-live="polite" className="grid gap-6">
            <DialogHeader className="items-center gap-2 text-center">
              <div
                className={cn(
                  "mb-2 grid size-16 place-items-center rounded-xl border",
                  statusLoadFailed
                    ? "border-muted bg-muted"
                    : rejected
                      ? "border-destructive/20 bg-destructive/10"
                      : invited
                        ? "border-success/20 bg-success/10"
                        : "border-brand/20 bg-brand/10",
                )}
              >
                {statusLoadFailed ? (
                  <TicketCheck className="size-8 text-muted-foreground" />
                ) : rejected ? (
                  <AlertTriangle className="size-8 text-destructive" />
                ) : invited ? (
                  <CheckCircle2 className="size-8 text-success" />
                ) : (
                  <Clock3 className="size-8 text-brand" />
                )}
              </div>
              <p className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                {statusEyebrow}
              </p>
              <DialogTitle className="text-xl leading-tight sm:text-2xl">
                {statusHeadline}
              </DialogTitle>
              <DialogDescription className="max-w-lg text-center leading-6">
                {statusDescription}
              </DialogDescription>
            </DialogHeader>

            {pending ? (
              <div className="flex items-start gap-3 rounded-lg border border-brand/20 bg-brand/[0.06] px-4 py-3.5">
                <span className="mt-0.5 grid size-8 shrink-0 place-items-center rounded-full bg-brand/10 text-brand">
                  <Clock3 className="size-4 animate-pulse" />
                </span>
                <div>
                  <p className="text-sm font-semibold">请不要关闭此页面</p>
                  <p className="mt-1 text-xs leading-5 text-muted-foreground">
                    管理员正在处理兑换申请，请耐心等待，处理结果会自动显示在此弹窗中。
                  </p>
                </div>
              </div>
            ) : null}

            {!statusLoadFailed ? (
              <div className="divide-y border-y text-sm">
                <div className="flex items-center justify-between gap-4 py-3">
                  <span className="text-muted-foreground">GPT 邮箱</span>
                  <span className="min-w-0 truncate font-mono">
                    {status?.customer_email || "加载中"}
                  </span>
                </div>
                <div className="flex items-center justify-between gap-4 py-3">
                  <span className="text-muted-foreground">提交时间</span>
                  <span className="tabular-nums">{status?.created_at_label || "加载中"}</span>
                </div>
                {invited ? (
                  <div className="flex items-center justify-between gap-4 py-3">
                    <span className="text-muted-foreground">邀请时间</span>
                    <span className="tabular-nums">{status?.invited_at_label || "刚刚"}</span>
                  </div>
                ) : null}
              </div>
            ) : null}

            <Button
              type="button"
              variant="outline"
              className="h-11 w-full"
              disabled={!canRestartApplication}
              onClick={canRestartApplication ? onRestart : undefined}
            >
              {statusActionLabel}
            </Button>
          </div>
        </DialogContent>
      )}
    </Dialog>
  )
}

export function RedeemPage() {
  const [searchParams] = useSearchParams()
  const sandboxAccessToken = searchParams.get("sandbox")?.trim() ?? ""
  const initialRedeemCode = searchParams.get("code")?.trim() ?? ""
  const sandboxMode = sandboxAccessToken !== ""
  const tokenStorageKey = redemptionTokenStorageKey(sandboxAccessToken)
  const [trackingToken, setTrackingToken] = React.useState(() => readStoredToken(tokenStorageKey))
  const [noticeOpen, setNoticeOpen] = React.useState(() => trackingToken === "")
  const [flowStep, setFlowStep] = React.useState<RedeemFlowStep>(() =>
    trackingToken ? "progress" : "review",
  )
  const [flowDialogOpen, setFlowDialogOpen] = React.useState(() => trackingToken !== "")
  const [reviewValues, setReviewValues] = React.useState<FormValues | null>(null)

  const settingsQuery = useQuery({
    queryKey: ["public-redeem-settings", sandboxAccessToken || "live"],
    queryFn: () => fetchRedeemPageSettings(sandboxAccessToken),
    staleTime: 5 * 60 * 1000,
  })
  const redeemSettings = normalizeRedeemPageSettings(settingsQuery.data)

  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: {
      customer_email: sandboxMode ? "sandbox-customer@example.com" : "",
      redeem_code: initialRedeemCode,
      customer_contact: sandboxMode ? "sandbox_wechat" : "",
    },
  })

  const statusQuery = useQuery({
    queryKey: ["public-redemption-status", sandboxAccessToken || "live", trackingToken],
    queryFn: () => fetchRedemptionStatus(trackingToken, sandboxAccessToken),
    enabled: trackingToken !== "",
    refetchInterval: (query) => (query.state.data?.status === "pending" ? 5_000 : false),
    retry: false,
  })

  const submitMutation = useMutation({
    mutationFn: (values: FormValues) =>
      submitRedemptionApplication({
        customer_email: values.customer_email.trim(),
        customer_contact: `微信：${values.customer_contact.trim()}`,
        redeem_code: values.redeem_code.trim(),
        request_note: "",
      }, sandboxAccessToken),
    onSuccess: (result) => {
      setTrackingToken(result.tracking_token)
      writeStoredToken(tokenStorageKey, result.tracking_token)
      setFlowStep("progress")
      setFlowDialogOpen(true)
      setReviewValues(null)
      form.reset()
      toast.success(result.message ?? "申请已提交")
    },
    onError: (error: Error) => toast.error(error.message),
  })

  const reviewSubmission = (values: FormValues) => {
    setReviewValues({
      customer_email: values.customer_email.trim(),
      customer_contact: values.customer_contact.trim(),
      redeem_code: values.redeem_code.trim(),
    })
    setFlowStep("review")
    setFlowDialogOpen(true)
  }

  const confirmSubmission = () => {
    if (reviewValues) {
      submitMutation.mutate(reviewValues)
    }
  }

  const handleFlowDialogOpenChange = (open: boolean) => {
    if (!open && (submitMutation.isPending || flowStep === "progress")) return
    setFlowDialogOpen(open)
  }

  const resetApplication = () => {
    setTrackingToken("")
    writeStoredToken(tokenStorageKey, "")
    setFlowDialogOpen(false)
    setFlowStep("review")
    setReviewValues(null)
    form.reset({
      customer_email: sandboxMode ? "sandbox-customer@example.com" : "",
      redeem_code: "",
      customer_contact: sandboxMode ? "sandbox_wechat" : "",
    })
  }

  const statusLoadFailed = trackingToken !== "" && statusQuery.isError
  const supportConfigured = hasSupportContact(redeemSettings)
  const supportColumnVisible = supportConfigured || settingsQuery.isPending

  return (
    <main className="min-h-dvh bg-background text-foreground">
      <RedeemSafetyNoticeDialog
        open={noticeOpen}
        onOpenChange={setNoticeOpen}
        settings={redeemSettings}
      />
      <RedemptionFlowDialog
        open={flowDialogOpen}
        step={flowStep}
        reviewValues={reviewValues}
        submitting={submitMutation.isPending}
        status={statusQuery.data}
        statusLoadFailed={statusLoadFailed}
        onOpenChange={handleFlowDialogOpenChange}
        onConfirm={confirmSubmission}
        onRestart={resetApplication}
      />

      <section className="login-surface relative overflow-hidden border-b border-[var(--login-panel-border)] text-[var(--login-panel-foreground)]">
        <BrandIcon className="pointer-events-none absolute -right-24 -top-24 size-80 opacity-[0.05] shadow-none" />
        <div className="relative mx-auto w-full max-w-[1160px] px-4 pb-24 pt-6 sm:px-6 sm:pb-28 sm:pt-7">
          <div className="flex items-center justify-between gap-4">
            <div className="flex min-w-0 items-center gap-2.5 text-[11px] font-semibold text-[var(--login-panel-muted)]">
              <BrandIcon className="size-8" />
              <span className="hidden text-sm font-semibold tracking-wide text-[var(--login-panel-foreground)] sm:block">
                {APP_NAME}
              </span>
              <span
                className="hidden h-4 w-px bg-[var(--login-panel-border)] sm:block"
                aria-hidden="true"
              />
              <TicketCheck className="size-4 text-gold" />
              CHATGPT TEAM ACCESS
              {sandboxMode ? (
                <span className="rounded-sm border border-gold/30 bg-gold/10 px-2 py-0.5 text-gold">
                  业务演练
                </span>
              ) : null}
            </div>
            <div className="flex items-center gap-2">
              <RedeemAnnouncementButton onClick={() => setNoticeOpen(true)} />
              <SupportWechatDialogButton settings={redeemSettings} />
              <RedeemThemeToggle />
            </div>
          </div>

          <div className="mt-12 grid gap-10 lg:grid-cols-[minmax(0,1fr)_minmax(420px,0.8fr)] lg:items-end">
            <div className="max-w-[680px] animate-fade-up">
              <div className="mb-5 flex items-center gap-3 text-[11px] font-semibold tracking-wider text-[var(--login-panel-muted)]">
                <span className="h-px w-8 bg-gold" aria-hidden="true" />
                自助兑换通道
              </div>
              <h1 className="text-[32px] font-semibold leading-[1.18] sm:text-[44px]">
                ChatGPT Team 兑换中心
              </h1>
              <p className="mt-4 max-w-[620px] text-sm leading-7 text-[var(--login-panel-muted)] sm:text-base">
                提交兑换码与账号资料，管理员处理完成后，Team 邀请会直接发送到你的 GPT 邮箱。
              </p>
              <div className="mt-7 flex flex-wrap gap-2.5">
                {TRUST_CHIPS.map((chip) => (
                  <span
                    key={chip.label}
                    className="inline-flex items-center gap-1.5 rounded-full border border-white/15 bg-white/[0.06] px-3.5 py-1.5 text-xs font-medium backdrop-blur-sm"
                  >
                    <chip.icon className="size-3.5 text-gold" />
                    {chip.label}
                  </span>
                ))}
              </div>
            </div>

            <ol className="grid gap-y-3 border-t border-[var(--login-panel-border)] pt-5 sm:grid-cols-3 sm:gap-x-6 sm:gap-y-0">
              {[
                ["01", "填写资料", "兑换码、邮箱与微信"],
                ["02", "等待处理", "通常需要 1-2 分钟"],
                ["03", "确认邀请", "前往邮箱加入 Team"],
              ].map(([number, title, description]) => (
                <li
                  key={number}
                  className="grid grid-cols-[2rem_minmax(0,1fr)] gap-2 py-1 sm:block sm:border-l sm:border-[var(--login-panel-border)] sm:py-0 sm:pl-5 sm:first:border-l-0 sm:first:pl-0"
                >
                  <span className="font-mono text-[11px] font-semibold tracking-wider text-gold">
                    {number}
                  </span>
                  <div>
                    <p className="text-sm font-semibold sm:mt-1.5">{title}</p>
                    <p className="mt-1 text-xs leading-5 text-[var(--login-panel-muted)]">
                      {description}
                    </p>
                  </div>
                </li>
              ))}
            </ol>
          </div>
        </div>
      </section>

      <div className="relative z-10 mx-auto w-full max-w-[1160px] px-4 pb-12 sm:px-6">
        <div
          className={cn(
            "-mt-14 grid items-start gap-6 sm:-mt-16 lg:items-stretch",
            supportColumnVisible ? "lg:grid-cols-[minmax(0,1fr)_340px]" : "mx-auto max-w-[760px]",
          )}
        >
        <Card className="overflow-hidden p-0 shadow-lift animate-fade-up [animation-delay:80ms]">
          <div className="flex items-center justify-between gap-4 border-b bg-muted/30 px-5 py-4 sm:px-7 sm:py-5">
            <div className="flex min-w-0 items-center gap-3">
              <span className="grid size-9 shrink-0 place-items-center rounded-lg bg-brand/10 text-brand">
                <TicketCheck className="size-4" />
              </span>
              <div className="min-w-0">
                <p className="text-sm font-semibold sm:text-base">填写兑换信息</p>
                <p className="mt-0.5 truncate text-xs text-muted-foreground">
                  请确认 GPT 邮箱可以正常收信，管理员会通过微信协助核对订单
                </p>
              </div>
            </div>
            <span className="hidden shrink-0 rounded-full border bg-background px-2.5 py-1 text-[11px] font-medium text-muted-foreground sm:inline-block">
              共 3 项
            </span>
          </div>
            <Form {...form}>
              <form
                onSubmit={form.handleSubmit(reviewSubmission)}
                className="grid gap-6 p-6 sm:p-8"
              >
                <FormField
                  control={form.control}
                  name="redeem_code"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>兑换码</FormLabel>
                      <FormControl>
                        <div className="relative">
                          <TicketCheck className="pointer-events-none absolute left-3.5 top-1/2 size-4 -translate-y-1/2 text-brand" />
                          <Input
                            autoComplete="off"
                            placeholder="CPN-XXXX-XXXX-XXXX"
                            className="h-12 pl-10"
                            {...field}
                          />
                        </div>
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                <div className="grid gap-5 md:grid-cols-2">
                  <FormField
                    control={form.control}
                    name="customer_email"
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>GPT 邮箱</FormLabel>
                        <FormControl>
                          <div className="relative">
                            <Mail className="pointer-events-none absolute left-3.5 top-1/2 size-4 -translate-y-1/2 text-brand" />
                            <Input
                              type="email"
                              autoComplete="email"
                              placeholder="name@example.com"
                              className="h-12 pl-10"
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
                    name="customer_contact"
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>微信号</FormLabel>
                        <FormControl>
                          <div className="relative">
                            <MessageCircle className="pointer-events-none absolute left-3.5 top-1/2 size-4 -translate-y-1/2 text-success" />
                            <Input
                              autoComplete="username"
                              placeholder="请输入常用微信号"
                              className="h-12 pl-10"
                              {...field}
                            />
                          </div>
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                </div>

                <div className="flex items-start gap-2 rounded-lg border border-brand/15 bg-brand/[0.045] px-3.5 py-3 text-xs leading-5 text-muted-foreground">
                  <CheckCircle2 className="mt-0.5 size-4 shrink-0 text-brand" />
                  确认提交后，处理进度会显示在弹窗中，请保持页面打开并留意邮箱邀请。
                </div>

                <Button
                  type="submit"
                  className="h-12"
                  disabled={submitMutation.isPending}
                >
                  {submitMutation.isPending ? (
                    <>
                      <LoaderCircle data-slot="icon" className="animate-spin" />
                      提交中
                    </>
                  ) : (
                    <>
                      提交兑换申请
                      <ArrowRight data-slot="icon" />
                    </>
                  )}
                </Button>
              </form>
            </Form>
        </Card>
        {settingsQuery.isPending ? (
          <aside className="hidden h-[430px] animate-pulse rounded-xl border bg-muted/45 lg:block" />
        ) : (
          <SupportWechatPanel settings={redeemSettings} />
        )}
        </div>

        <p className="mx-auto mt-8 max-w-[720px] text-center text-xs leading-6 text-muted-foreground">
          <ShieldCheck className="mr-1.5 inline-block size-3.5 align-[-2px] text-brand" />
          兑换码仅限本人使用，确认提交后请保持本页打开，处理进度会在弹窗中自动更新。
        </p>
      </div>
    </main>
  )
}
