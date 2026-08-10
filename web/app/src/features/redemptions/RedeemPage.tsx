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
          className="redeem-nav-button"
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
          className="redeem-nav-button px-3"
          onClick={onClick}
        >
          <Megaphone data-slot="icon" className="size-4" />
          <span className="hidden sm:inline">公告</span>
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
    <aside className="redeem-support-panel hidden overflow-hidden lg:flex lg:flex-col">
      <div className="redeem-support-terminal-bar">
        <div className="flex items-center gap-2">
          <span className="redeem-window-dot bg-[#ff6b63]" />
          <span className="redeem-window-dot bg-[#e9bd4e]" />
          <span className="redeem-window-dot bg-[var(--redeem-accent)]" />
          <span className="ml-1 font-mono text-[10px] font-medium tracking-[0.08em] text-[var(--redeem-muted)]">
            support.channel
          </span>
        </div>
        <span className="redeem-online-label">ONLINE</span>
      </div>

      <div className="flex flex-1 flex-col p-5 xl:p-6">
        <div className="flex items-start gap-3">
          <span className="redeem-support-icon size-10">
            <MessageCircle className="size-5" />
          </span>
          <div className="min-w-0">
            <p className="font-mono text-[9px] font-semibold tracking-[0.16em] text-[var(--redeem-accent)]">
              HUMAN SUPPORT
            </p>
            <h2 className="mt-1.5 text-lg font-semibold">{settings.support_title}</h2>
            <p className="mt-1 text-xs leading-5 text-[var(--redeem-muted)]">
              {settings.support_description}
            </p>
          </div>
        </div>

        <div className="redeem-support-qr-shell mt-5">
          <WechatQrBlock settings={settings} compact />
        </div>

        <div className="mt-5 flex items-start gap-2.5 border-t border-[var(--redeem-line)] pt-4 text-xs leading-5 text-[var(--redeem-muted)]">
          <ShieldCheck className="mt-0.5 size-4 shrink-0 text-[var(--redeem-accent)]" />
          <p>兑换异常、续期提醒或邮箱核对，都可以通过客服微信获得协助。</p>
        </div>
      </div>
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
          className="redeem-nav-button lg:hidden"
        >
          <MessageCircle data-slot="icon" />
          <span className="hidden sm:inline">客服</span>
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
    <main className="redeem-console min-h-dvh overflow-hidden text-[var(--redeem-text)]">
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

      <header className="redeem-topbar">
        <div className="mx-auto flex h-16 w-full max-w-[1200px] items-center justify-between gap-4 px-4 sm:h-[72px] sm:px-6 lg:px-8">
          <div className="flex min-w-0 items-center gap-3">
            <BrandIcon className="size-8 rounded-md shadow-none sm:size-9" />
            <div className="min-w-0">
              <p className="truncate text-sm font-semibold leading-none sm:text-[15px]">{APP_NAME}</p>
              <div className="mt-1.5 flex items-center gap-2 whitespace-nowrap font-mono text-[9px] font-semibold tracking-[0.14em] text-[var(--redeem-muted)] sm:text-[10px]">
                <span className="hidden sm:inline">REDEEM GATEWAY</span>
                <span className="redeem-status-dot" aria-hidden="true" />
                <span className="text-[var(--redeem-accent)]">ONLINE</span>
              </div>
            </div>
            {sandboxMode ? (
              <span className="redeem-sandbox-badge">SANDBOX</span>
            ) : null}
          </div>
          <div className="flex shrink-0 items-center gap-2">
            <RedeemAnnouncementButton onClick={() => setNoticeOpen(true)} />
            <SupportWechatDialogButton settings={redeemSettings} />
            <RedeemThemeToggle />
          </div>
        </div>
      </header>

      <section className="relative mx-auto w-full max-w-[1200px] px-4 pb-8 pt-7 sm:px-6 sm:pb-10 sm:pt-10 lg:px-8 lg:pb-12 lg:pt-12">
        <div className="redeem-overview grid gap-8 animate-fade-up lg:grid-cols-[minmax(300px,0.78fr)_minmax(0,1.22fr)] lg:items-end lg:gap-12">
          <div>
            <div className="flex items-center gap-2 font-mono text-[10px] font-semibold tracking-[0.18em] text-[var(--redeem-accent)]">
              <span className="h-px w-7 bg-[var(--redeem-accent)]" aria-hidden="true" />
              ACCESS / REDEEM
            </div>
            <h1 className="mt-3 max-w-[560px] text-[30px] font-semibold leading-[1.14] tracking-[-0.035em] sm:text-[38px] lg:text-[40px]">
              兑换你的 <span className="text-[var(--redeem-accent)]">Team 席位</span>
            </h1>
            <p className="mt-3 max-w-[560px] text-sm leading-6 text-[var(--redeem-muted)] sm:text-[15px] sm:leading-7">
              填写兑换凭证与接收账号，提交后处理状态会自动同步，无需重复刷新。
            </p>
            <div className="mt-4 flex flex-wrap gap-2">
              <span className="redeem-mini-chip">
                <Clock3 className="size-3.5" /> 约 1–2 分钟
              </span>
              <span className="redeem-mini-chip">
                <ShieldCheck className="size-3.5" /> 提交前可核对
              </span>
            </div>
          </div>

          <div className="hidden lg:block">
            <div className="mb-3 flex items-center justify-between font-mono text-[10px] font-semibold tracking-[0.16em] text-[var(--redeem-muted)]">
              <span>PROCESS SEQUENCE</span>
              <span>03 STEPS</span>
            </div>
            <ol className="redeem-process-grid">
              {[
                ["01", "验证兑换凭证", "填写兑换码与账号信息", "READY"],
                ["02", "进入处理队列", "管理员核验并发送邀请", "QUEUE"],
                ["03", "获取空间权限", "前往邮箱确认加入 Team", "ACCESS"],
              ].map(([number, title, description, state], index) => (
                <li key={number} className={cn("redeem-process-card", index === 0 && "is-active")}>
                  <div className="flex items-center justify-between gap-3">
                    <span className="redeem-process-number">{number}</span>
                    <span className="font-mono text-[9px] tracking-[0.12em] text-[var(--redeem-muted)]">
                      {state}
                    </span>
                  </div>
                  <p className="mt-3 text-sm font-semibold">{title}</p>
                  <p className="mt-1 text-[11px] leading-5 text-[var(--redeem-muted)]">
                    {description}
                  </p>
                </li>
              ))}
            </ol>
          </div>
        </div>

        <div
          className={cn(
            "mt-7 grid items-start gap-6 sm:mt-8",
            supportColumnVisible
              ? "lg:grid-cols-[minmax(0,1fr)_320px] xl:grid-cols-[minmax(0,1fr)_350px]"
              : "mx-auto max-w-[880px]",
          )}
        >
        <Card className="redeem-terminal animate-fade-up overflow-hidden p-0 [animation-delay:80ms]">
          <div className="redeem-terminal-bar">
            <div className="flex items-center gap-2">
              <span className="redeem-window-dot bg-[#ff6b63]" />
              <span className="redeem-window-dot bg-[#e9bd4e]" />
              <span className="redeem-window-dot bg-[var(--redeem-accent)]" />
              <span className="ml-2 font-mono text-[10px] font-medium tracking-[0.08em] text-[var(--redeem-muted)] sm:text-[11px]">
                redemption.request
              </span>
            </div>
            <div className="flex items-center gap-2 font-mono text-[9px] font-semibold tracking-[0.14em] text-[var(--redeem-accent)] sm:text-[10px]">
              <span className="redeem-status-dot" />
              SYSTEM READY
            </div>
          </div>

          <div className="px-5 pb-1 pt-6 sm:px-8 sm:pt-8 lg:px-10 lg:pt-10">
            <div className="flex items-start justify-between gap-4">
              <div>
                <p className="font-mono text-[10px] font-semibold tracking-[0.18em] text-[var(--redeem-accent)]">
                  NEW ACCESS REQUEST
                </p>
                <h2 className="mt-2 text-xl font-semibold tracking-[-0.02em] sm:text-2xl">
                  提交兑换申请
                </h2>
                <p className="mt-2 text-xs leading-5 text-[var(--redeem-muted)] sm:text-sm">
                  三项信息填写完成后，你还可以在提交前进行最终核对。
                </p>
              </div>
              <span className="redeem-required-badge hidden sm:inline-flex">3 REQUIRED</span>
            </div>
          </div>

          <Form {...form}>
            <form
              onSubmit={form.handleSubmit(reviewSubmission)}
              className="grid gap-6 px-5 pb-6 pt-6 sm:px-8 sm:pb-8 lg:gap-7 lg:px-10 lg:pb-10"
            >
              <FormField
                control={form.control}
                name="redeem_code"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel className="redeem-field-label">
                      <span className="redeem-field-index">01</span>
                      兑换码
                      <span className="redeem-field-code">REDEEM TOKEN</span>
                    </FormLabel>
                    <FormControl>
                      <div className="relative">
                        <TicketCheck className="redeem-input-icon" />
                        <Input
                          autoComplete="off"
                          placeholder="CPN-XXXX-XXXX-XXXX"
                          className="redeem-input h-14 pl-11 font-mono tracking-[0.04em]"
                          {...field}
                        />
                      </div>
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <div className="grid gap-6 md:grid-cols-2">
                <FormField
                  control={form.control}
                  name="customer_email"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel className="redeem-field-label">
                        <span className="redeem-field-index">02</span>
                        GPT 邮箱
                        <span className="redeem-field-code">INVITE TARGET</span>
                      </FormLabel>
                      <FormControl>
                        <div className="relative">
                          <Mail className="redeem-input-icon" />
                          <Input
                            type="email"
                            autoComplete="email"
                            placeholder="name@example.com"
                            className="redeem-input h-[52px] pl-11"
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
                      <FormLabel className="redeem-field-label">
                        <span className="redeem-field-index">03</span>
                        微信号
                        <span className="redeem-field-code">SUPPORT ID</span>
                      </FormLabel>
                      <FormControl>
                        <div className="relative">
                          <MessageCircle className="redeem-input-icon" />
                          <Input
                            autoComplete="username"
                            placeholder="请输入常用微信号"
                            className="redeem-input h-[52px] pl-11"
                            {...field}
                          />
                        </div>
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </div>

              <div className="redeem-form-notice">
                <CheckCircle2 className="mt-0.5 size-4 shrink-0 text-[var(--redeem-accent)]" />
                <p>
                  提交前会再次展示全部信息供你核对；确认后请保持页面打开，进度会自动更新。
                </p>
              </div>

              <Button
                type="submit"
                className="redeem-submit-button h-14 w-full"
                disabled={submitMutation.isPending}
              >
                {submitMutation.isPending ? (
                  <>
                    <LoaderCircle data-slot="icon" className="animate-spin" />
                    正在建立兑换请求
                  </>
                ) : (
                  <>
                    <span>开始兑换</span>
                    <span className="hidden font-mono text-[10px] font-semibold tracking-[0.12em] opacity-70 sm:inline">
                      INITIATE REQUEST
                    </span>
                    <ArrowRight data-slot="icon" className="ml-auto" />
                  </>
                )}
              </Button>
            </form>
          </Form>

          <div className="redeem-terminal-footer">
            <span className="flex items-center gap-2">
              <ShieldCheck className="size-3.5" /> SECURE INPUT
            </span>
            <span>STATUS SYNC · 5S</span>
          </div>
        </Card>

          {settingsQuery.isPending ? (
            <aside className="hidden h-[560px] animate-pulse rounded-[14px] border border-[var(--redeem-line)] bg-[var(--redeem-panel)] lg:block" />
          ) : (
            <SupportWechatPanel settings={redeemSettings} />
          )}
        </div>
      </section>

      <footer className="relative mx-auto flex w-full max-w-[1200px] items-center justify-between gap-4 border-t border-[var(--redeem-line)] px-4 py-5 font-mono text-[9px] tracking-[0.12em] text-[var(--redeem-muted)] sm:px-6 lg:px-8">
        <span>CARPOOL NOTIFY PLUS / REDEMPTION SERVICE</span>
        <span className="hidden sm:inline">PRIVACY MODE · SYSTEM OPERATIONAL</span>
      </footer>
    </main>
  )
}
