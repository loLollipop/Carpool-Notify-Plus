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
            loading="eager"
            decoding="sync"
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
  ready,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  settings: RedeemPageSettings
  ready: boolean
}) {
  const handleOpenChange = (nextOpen: boolean) => {
    if (!nextOpen && !ready) return
    onOpenChange(nextOpen)
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent
        showCloseButton={false}
        className="sm:max-w-[520px]"
        onEscapeKeyDown={(event) => {
          if (!ready) event.preventDefault()
        }}
        onInteractOutside={(event) => {
          if (!ready) event.preventDefault()
        }}
      >
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

        <Button
          type="button"
          className="h-11 w-full"
          disabled={!ready}
          onClick={() => onOpenChange(false)}
        >
          {ready ? (
            "我已了解，继续兑换"
          ) : (
            <>
              <LoaderCircle data-slot="icon" className="animate-spin" />
              正在准备兑换页
            </>
          )}
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
            <DialogTitle className="text-xl leading-tight sm:text-2xl">确认兑换信息</DialogTitle>
            <DialogDescription className="leading-6">请确认邮箱无误，邀请会发往该账号。</DialogDescription>
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
  const [preloadedSupportQR, setPreloadedSupportQR] = React.useState("")

  const settingsQuery = useQuery({
    queryKey: ["public-redeem-settings", sandboxAccessToken || "live"],
    queryFn: () => fetchRedeemPageSettings(sandboxAccessToken),
    staleTime: 5 * 60 * 1000,
  })
  const redeemSettings = normalizeRedeemPageSettings(settingsQuery.data)
  const supportQRDataURL = redeemSettings.support_qr_data_url.trim()

  React.useEffect(() => {
    if (!supportQRDataURL) return

    let active = true
    const image = new Image()
    const markReady = () => {
      if (active) setPreloadedSupportQR(supportQRDataURL)
    }
    image.onload = markReady
    image.onerror = markReady
    image.src = supportQRDataURL
    if (image.complete) markReady()

    return () => {
      active = false
      image.onload = null
      image.onerror = null
    }
  }, [supportQRDataURL])

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
  const supportAssetReady = supportQRDataURL === "" || preloadedSupportQR === supportQRDataURL
  const redeemPageReady = !settingsQuery.isPending && supportAssetReady

  return (
    <main className="redeem-console min-h-dvh overflow-hidden text-[var(--redeem-text)]">
      <RedeemSafetyNoticeDialog
        open={noticeOpen}
        onOpenChange={setNoticeOpen}
        settings={redeemSettings}
        ready={redeemPageReady}
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
              <div className="mt-1.5 flex items-center gap-2 whitespace-nowrap text-[10px] font-medium text-[var(--redeem-muted)]">
                <span className="hidden sm:inline">Team 席位兑换</span>
                <span className="redeem-status-dot" aria-hidden="true" />
                <span className="text-[var(--redeem-accent)]">在线</span>
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

      <section className="relative mx-auto w-full max-w-[1120px] px-4 pb-8 pt-6 sm:px-6 sm:pb-10 sm:pt-8 lg:px-8 lg:pb-12 lg:pt-10">
        <div
          className={cn(
            "redeem-workspace grid items-stretch gap-6 animate-fade-up",
            supportColumnVisible
              ? "lg:grid-cols-[minmax(0,1fr)_320px] xl:grid-cols-[minmax(0,1fr)_340px]"
              : "mx-auto max-w-[760px]",
          )}
        >
          <div className="redeem-workspace-decor" aria-hidden="true">
            <div className="redeem-telemetry-bar">
              <span className="redeem-telemetry-label">
                <i />
                REDEMPTION WORKSPACE
              </span>
              <span className="redeem-telemetry-track" />
              <span className="redeem-telemetry-label">CPN / ACCESS</span>
            </div>
            <span className="redeem-frame-corner is-top-left" />
            <span className="redeem-frame-corner is-top-right" />
            <span className="redeem-frame-corner is-bottom-left" />
            <span className="redeem-frame-corner is-bottom-right" />
            <div className="redeem-side-rail is-left">
              <span>INPUT CHANNEL</span>
            </div>
            <div className="redeem-side-rail is-right">
              <span>SUPPORT CHANNEL</span>
            </div>
            <span className="redeem-frame-node is-left" />
            <span className="redeem-frame-node is-right" />
          </div>

          <Card className="redeem-terminal overflow-hidden p-0">
            <div className="redeem-terminal-bar">
              <div className="flex items-center gap-2">
                <span className="redeem-window-dot bg-[#ff6b63]" />
                <span className="redeem-window-dot bg-[#e9bd4e]" />
                <span className="redeem-window-dot bg-[var(--redeem-accent)]" />
                <span className="ml-2 font-mono text-[10px] font-medium tracking-[0.08em] text-[var(--redeem-muted)] sm:text-[11px]">
                  TEAM / REDEEM
                </span>
              </div>
              <div className="flex items-center gap-2 text-[10px] font-semibold text-[var(--redeem-accent)] sm:text-xs">
                <span className="redeem-status-dot" />
                通道在线
              </div>
            </div>

            <div className="px-5 pt-6 sm:px-8 sm:pt-8 lg:px-9">
              <p className="font-mono text-[10px] font-semibold tracking-[0.16em] text-[var(--redeem-accent)]">
                ACCESS REQUEST
              </p>
              <h1 className="mt-2 text-2xl font-semibold tracking-[-0.02em] sm:text-[28px]">
                兑换 Team 席位
              </h1>
              <div className="redeem-trust-strip" aria-label="服务保障">
                <span><ShieldCheck />人工核验</span>
                <span><Clock3 />进度可查</span>
                <span><MessageCircle />售后可联系</span>
              </div>
            </div>

            <Form {...form}>
              <form
                onSubmit={form.handleSubmit(reviewSubmission)}
                className="flex flex-1 flex-col gap-5 px-5 pb-6 pt-6 sm:px-8 sm:pb-8 lg:px-9 lg:pb-9"
              >
                <FormField
                  control={form.control}
                  name="redeem_code"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel className="redeem-field-label">
                        <span className="redeem-field-index">01</span>
                        兑换码
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
                      <span>核对兑换信息</span>
                      <ArrowRight data-slot="icon" className="ml-auto" />
                    </>
                  )}
                </Button>

                <div className="redeem-flow-footer">
                  <div className="redeem-flow-heading">
                    <span>兑换流程</span>
                    <span>SUBMISSION FLOW</span>
                  </div>
                  <ol className="redeem-flow-rail" aria-label="提交后的兑换流程">
                    <li>
                      <span>01</span>
                      <strong>提交核验</strong>
                    </li>
                    <li>
                      <span>02</span>
                      <strong>发送邀请</strong>
                    </li>
                    <li>
                      <span>03</span>
                      <strong>邮箱加入</strong>
                    </li>
                  </ol>
                </div>
              </form>
            </Form>
          </Card>

          {settingsQuery.isPending ? (
            <aside className="hidden h-[500px] animate-pulse rounded-[8px] border border-[var(--redeem-line)] bg-[var(--redeem-panel)] lg:block" />
          ) : (
            <SupportWechatPanel settings={redeemSettings} />
          )}
        </div>
      </section>
    </main>
  )
}
