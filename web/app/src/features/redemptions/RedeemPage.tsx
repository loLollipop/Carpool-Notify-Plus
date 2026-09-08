import * as React from "react"
import { zodResolver } from "@hookform/resolvers/zod"
import { useMutation, useQuery } from "@tanstack/react-query"
import {
  AlertTriangle,
  ArrowRight,
  CheckCircle2,
  ChevronRight,
  Clock3,
  Copy,
  CreditCard,
  Gauge,
  LoaderCircle,
  Mail,
  Megaphone,
  Moon,
  Sparkles,
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
  fetchRenewalStatus,
  lookupRenewalSubscriptions,
  submitRedemptionApplication,
  submitRenewalApplication,
} from "@/api/endpoints"
import type {
  RedeemPageSettings,
  RedemptionStatus,
  RenewalSubscriptionView,
} from "@/api/types"
import { APP_NAME, BrandIcon } from "@/components/brand"
import { WeChatIcon } from "@/components/icons/wechat-icon"
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
const RENEWAL_STORAGE_KEY = "carpool-notify:renewal-token"
const EMAIL_PATTERN = /^[^@\s]+@[^@\s]+\.[^@\s]+$/
const PRIVATE_WECHAT_ID_PATTERN = /^wxid_/i
const CONTACT_LABEL_PATTERN = /^(?:微信\s*\/\s*手机号|微信|手机号|手机)[：:]\s*/

function contactValueForValidation(value: string) {
  return value.trim().replace(CONTACT_LABEL_PATTERN, "")
}
const DEFAULT_REDEEM_PAGE_SETTINGS: RedeemPageSettings = {
  announcement_title: "首次兑换前请确认",
  announcement_intro:
    "提交兑换前，请先了解工作空间数据与后续续费方式。",
  announcement_items: [
    "工作空间与个人空间的记录相互独立，请及时备份工作空间中的重要对话、文件和资料。",
    "长期使用建议添加管理员微信，方便接收续费提醒、售后协助和异常通知。",
    "兑换成功后，可在本页切换到“自助续费”，也可联系客服协助续费；到期仍未续费的席位将自动移出空间。",
  ],
  support_title: "客服微信",
  support_description: "续费提醒与售后协助",
  support_contact_label: "微信号",
  support_wechat_id: "",
  support_qr_data_url: "",
  renewal_announcement_title: "自助续费付款说明",
  renewal_announcement_intro: "付款前请核对页面账单，并按显示金额完成续费。",
  renewal_announcement_items: [
    "扫码付款时请务必备注订阅邮箱；忘记备注时请联系客服处理。",
    "付款金额必须与页面显示的本期应付金额完全一致，否则无法核对续费；付错金额请联系客服。",
    "付款后点击“提交续费审核”，管理员确认到账后会更新订阅状态。",
  ],
  payment_title: "续费收款码",
  payment_description: "请按左侧账单金额付款，并备注订阅邮箱",
  payment_qr_data_url: "",
  codex_plus_weekly_quota_usd: 150,
  codex_team_weekly_quota_usd: 200,
  web_primary_benefit_label: "GPT-5.6 sol 极高",
  web_plus_primary_benefit: "不支持",
  web_team_primary_benefit: "支持",
  web_secondary_benefit_label: "Pro 模型",
  web_plus_secondary_benefit: "—",
  web_team_secondary_benefit: "15 次/月",
}
const schema = z.object({
  customer_email: z
    .string()
    .trim()
    .min(1, "请填写邮箱")
    .regex(EMAIL_PATTERN, "邮箱格式不正确")
    .max(254, "邮箱太长"),
  redeem_code: z.string().trim().min(1, "请填写兑换码").max(120, "兑换码太长"),
  customer_contact: z
    .string()
    .trim()
    .min(1, "请填写微信或手机号")
    .max(80, "微信或手机号太长")
    .refine((value) => !PRIVATE_WECHAT_ID_PATTERN.test(contactValueForValidation(value)), {
      message: "这是微信隐私号，无法通过搜索添加，请填写手机号",
    })
    .refine((value) => !contactValueForValidation(value).includes("@"), {
      message: "这里请填写微信或手机号，不能填写邮箱",
    }),
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
  const renewalItems = (merged.renewal_announcement_items ?? [])
    .map((item) => item.trim())
    .filter((item) => item !== "")
  return {
    ...merged,
    announcement_items:
      items.length > 0 ? items : DEFAULT_REDEEM_PAGE_SETTINGS.announcement_items,
    renewal_announcement_items:
      renewalItems.length > 0
        ? renewalItems
        : DEFAULT_REDEEM_PAGE_SETTINGS.renewal_announcement_items,
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

function RedeemAmbientField() {
  return (
    <div className="redeem-ambient-field" aria-hidden="true">
      <span className="redeem-ambient-scan" />

      <div className="redeem-circuit-bank is-left">
        <span className="redeem-circuit-track is-a" />
        <span className="redeem-circuit-track is-b" />
        <span className="redeem-circuit-track is-c" />
        <span className="redeem-circuit-track is-d" />
        <span className="redeem-circuit-track is-e" />
        <span className="redeem-ambient-crosshair is-a" />
        <span className="redeem-ambient-crosshair is-b" />
      </div>

      <div className="redeem-circuit-bank is-right">
        <span className="redeem-circuit-track is-a" />
        <span className="redeem-circuit-track is-b" />
        <span className="redeem-circuit-track is-c" />
        <span className="redeem-circuit-track is-d" />
        <span className="redeem-circuit-track is-e" />
        <span className="redeem-ambient-crosshair is-a" />
        <span className="redeem-ambient-crosshair is-b" />
      </div>

      <span className="redeem-ambient-horizon" />
    </div>
  )
}

function RedeemAnnouncementButton({
  mode,
  onClick,
}: {
  mode: PublicWorkspaceMode
  onClick: () => void
}) {
  const label = mode === "renewal" ? "查看续费说明" : "查看兑换公告"
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          aria-label={label}
          className="redeem-nav-button px-3"
          onClick={onClick}
        >
          <Megaphone data-slot="icon" className="size-4" />
          <span className="hidden sm:inline">公告</span>
        </Button>
      </TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
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
    <div className="wechat-qr-block grid gap-4">
      {qrDataURL ? (
        <div className="wechat-qr-image mx-auto w-full max-w-[260px] self-center rounded-lg border bg-white p-2.5 shadow-sm">
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
        <div className="wechat-contact flex items-center justify-between gap-3 border-t pt-4">
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

      <div className="redeem-support-body flex flex-1 flex-col p-5 xl:p-6">
        <div className="redeem-side-heading">
          <span className="redeem-support-icon size-10">
            <WeChatIcon className="size-5" />
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

        <div className="redeem-support-meta mt-5 grid overflow-hidden rounded-md border border-[var(--redeem-line)] bg-[var(--redeem-panel-muted)] text-xs">
          <div className="flex items-center justify-between gap-3 border-b border-[var(--redeem-line)] px-3.5 py-2.5">
            <span className="text-[var(--redeem-muted)]">预计处理</span>
            <strong className="font-medium">通常 1–2 分钟</strong>
          </div>
          <div className="flex items-center justify-between gap-3 px-3.5 py-2.5">
            <span className="text-[var(--redeem-muted)]">信息用途</span>
            <strong className="font-medium">仅用于邀请与售后</strong>
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
          <WeChatIcon data-slot="icon" />
          <span className="hidden sm:inline">客服</span>
        </Button>
      </DialogTrigger>
      <DialogContent className="max-w-[390px]">
        <DialogHeader className="items-center text-center">
          <DialogTitle>{settings.support_title}</DialogTitle>
          <DialogDescription>{settings.support_description}</DialogDescription>
        </DialogHeader>
        <WechatQrBlock settings={settings} compact />
      </DialogContent>
    </Dialog>
  )
}

function PaymentQrBlock({
  settings,
  selected,
}: {
  settings: RedeemPageSettings
  selected?: RenewalSubscriptionView | null
}) {
  const qrDataURL = settings.payment_qr_data_url.trim()
  return (
    <div className="wechat-qr-block grid gap-4">
      {selected ? (
        <div className="grid grid-cols-2 gap-2 rounded-md border border-[var(--redeem-line)] bg-[var(--redeem-panel-muted)] p-3 text-xs">
          <div>
            <p className="text-[var(--redeem-muted)]">本期应付</p>
            <p className="mt-1 text-lg font-semibold tabular-nums text-[var(--redeem-accent)]">¥{selected.amount_yuan}</p>
          </div>
          <div className="text-right">
            <p className="text-[var(--redeem-muted)]">续费账期</p>
            <p className="mt-1 font-mono font-semibold">{selected.due_date}</p>
          </div>
        </div>
      ) : null}
      {qrDataURL ? (
        <div className="wechat-qr-image mx-auto w-full max-w-[260px] rounded-lg border bg-white p-2.5 shadow-sm">
          <img
            src={qrDataURL}
            alt="续费收款码"
            loading="eager"
            decoding="sync"
            className="aspect-square w-full object-contain"
          />
        </div>
      ) : (
        <div className="grid aspect-square w-full max-w-[260px] place-items-center rounded-lg border border-dashed border-[var(--redeem-line-strong)] bg-[var(--redeem-panel-muted)] px-6 text-center text-sm leading-6 text-[var(--redeem-muted)]">
          收款码暂未配置，请联系客服续费
        </div>
      )}
      <div className="rounded-md border border-amber-500/20 bg-amber-500/[0.07] px-3.5 py-3 text-xs leading-5 text-[var(--redeem-muted)]">
        付款时务必备注订阅邮箱，金额必须与页面账单完全一致。
      </div>
    </div>
  )
}

function PaymentPanel({
  settings,
  selected,
}: {
  settings: RedeemPageSettings
  selected?: RenewalSubscriptionView | null
}) {
  return (
    <aside className="redeem-support-panel hidden overflow-hidden lg:flex lg:flex-col">
      <div className="redeem-support-terminal-bar">
        <div className="flex items-center gap-2">
          <span className="redeem-window-dot bg-[#ff6b63]" />
          <span className="redeem-window-dot bg-[#e9bd4e]" />
          <span className="redeem-window-dot bg-[var(--redeem-accent)]" />
          <span className="ml-1 font-mono text-[10px] font-medium tracking-[0.08em] text-[var(--redeem-muted)]">payment.channel</span>
        </div>
        <span className="redeem-online-label">PAY</span>
      </div>
      <div className="redeem-support-body flex flex-1 flex-col p-5 xl:p-6">
        <div className="redeem-side-heading">
          <span className="redeem-support-icon size-10"><CreditCard className="size-5" /></span>
          <div className="min-w-0">
            <p className="font-mono text-[9px] font-semibold tracking-[0.16em] text-[var(--redeem-accent)]">RENEWAL PAYMENT</p>
            <h2 className="mt-1.5 text-lg font-semibold">{settings.payment_title}</h2>
            <p className="mt-1 text-xs leading-5 text-[var(--redeem-muted)]">{settings.payment_description}</p>
          </div>
        </div>
        <div className="redeem-support-qr-shell mt-5 flex-1">
          <PaymentQrBlock settings={settings} selected={selected} />
        </div>
      </div>
    </aside>
  )
}

function PaymentDialogButton({
  settings,
  selected,
}: {
  settings: RedeemPageSettings
  selected?: RenewalSubscriptionView | null
}) {
  return (
    <Dialog>
      <DialogTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          aria-label="查看续费收款码"
          className="redeem-nav-button lg:hidden"
        >
          <CreditCard data-slot="icon" />
          <span className="hidden sm:inline">收款码</span>
        </Button>
      </DialogTrigger>
      <DialogContent className="max-w-[390px]">
        <DialogHeader className="items-center text-center">
          <DialogTitle>{settings.payment_title}</DialogTitle>
          <DialogDescription>{settings.payment_description}</DialogDescription>
        </DialogHeader>
        <PaymentQrBlock settings={settings} selected={selected} />
      </DialogContent>
    </Dialog>
  )
}

function CodexQuotaReferenceCard({ settings }: { settings: RedeemPageSettings }) {
  const quotaMax = Math.max(
    settings.codex_plus_weekly_quota_usd,
    settings.codex_team_weekly_quota_usd,
    1,
  )
  const quotaWidth = (value: number) => `${Math.max(4, Math.round((value / quotaMax) * 100))}%`

  return (
    <Card className="redeem-reference-card redeem-reference-quota p-0" aria-labelledby="codex-quota-title">
      <header className="redeem-reference-card-header">
        <span className="redeem-reference-icon">
          <Gauge className="size-[18px]" />
        </span>
        <div>
          <p>CODEX CAPACITY</p>
          <h2 id="codex-quota-title">Codex 周额度参考</h2>
        </div>
        <span className="redeem-reference-badge">EST.</span>
      </header>

      <div className="redeem-quota-list">
        <div className="redeem-quota-row">
          <div className="redeem-quota-plan">
            <span>PLUS</span>
            <strong>≈ ${settings.codex_plus_weekly_quota_usd}</strong>
            <small>/ 周</small>
          </div>
          <span className="redeem-quota-meter" aria-hidden="true">
            <i
              className="is-plus"
              style={{ width: quotaWidth(settings.codex_plus_weekly_quota_usd) }}
            />
          </span>
        </div>
        <div className="redeem-quota-row is-team">
          <div className="redeem-quota-plan">
            <span>TEAM</span>
            <strong>≈ ${settings.codex_team_weekly_quota_usd}</strong>
            <small>/ 周</small>
          </div>
          <span className="redeem-quota-meter" aria-hidden="true">
            <i
              className="is-team"
              style={{ width: quotaWidth(settings.codex_team_weekly_quota_usd) }}
            />
          </span>
        </div>
      </div>

      <p className="redeem-reference-note">近期额度折算，仅作使用强度参考，并非现金余额。</p>
    </Card>
  )
}

function benefitUnavailable(value: string) {
  return /^(?:不支持|无|暂无|—|-|0|none|not supported)$/i.test(value.trim())
}

function BenefitValue({ value, primary = false }: { value: string; primary?: boolean }) {
  const unavailable = benefitUnavailable(value)
  return (
    <dd className={unavailable ? "is-muted" : primary ? "is-supported" : "is-highlighted"}>
      {!unavailable && primary ? <CheckCircle2 /> : null}
      {value}
    </dd>
  )
}

function WebModelReferenceCard({ settings }: { settings: RedeemPageSettings }) {
  return (
    <Card className="redeem-reference-card redeem-reference-model p-0" aria-labelledby="web-model-title">
      <header className="redeem-reference-card-header">
        <span className="redeem-reference-icon">
          <Sparkles className="size-[18px]" />
        </span>
        <div>
          <p>WEB MODEL ACCESS</p>
          <h2 id="web-model-title">网页端模型权益</h2>
        </div>
        <span className="redeem-reference-badge">WEB</span>
      </header>

      <div className="redeem-model-plans" aria-label="Plus 与 Team 网页端模型权益对比">
        <section className="redeem-model-plan">
          <div className="redeem-model-plan-heading">
            <strong>PLUS</strong>
            <span>标准权益</span>
          </div>
          <dl>
            <div>
              <dt>{settings.web_primary_benefit_label}</dt>
              <BenefitValue value={settings.web_plus_primary_benefit} primary />
            </div>
            <div>
              <dt>{settings.web_secondary_benefit_label}</dt>
              <BenefitValue value={settings.web_plus_secondary_benefit} />
            </div>
          </dl>
        </section>
        <section className="redeem-model-plan is-team">
          <div className="redeem-model-plan-heading">
            <strong>TEAM</strong>
            <span>增强权益</span>
          </div>
          <dl>
            <div>
              <dt>{settings.web_primary_benefit_label}</dt>
              <BenefitValue value={settings.web_team_primary_benefit} primary />
            </div>
            <div>
              <dt>{settings.web_secondary_benefit_label}</dt>
              <BenefitValue value={settings.web_team_secondary_benefit} />
            </div>
          </dl>
        </section>
      </div>

      <p className="redeem-reference-note">权益可能动态调整，以账号实际显示为准。</p>
    </Card>
  )
}

type RedeemReferencePanel = "quota" | "models"

function FloatingReferencePanel({
  side,
  eyebrow,
  title,
  icon,
  open,
  onToggle,
  children,
}: {
  side: "left" | "right"
  eyebrow: string
  title: string
  icon: React.ReactNode
  open: boolean
  onToggle: () => void
  children: React.ReactNode
}) {
  const panelID = `redeem-reference-${side}`

  return (
    <div
      className={cn("redeem-reference-float", `is-${side}`, open && "is-open")}
    >
      <button
        type="button"
        className="redeem-reference-trigger"
        aria-label={`${open ? "收起" : "展开"}${title}`}
        aria-expanded={open}
        aria-controls={panelID}
        onClick={onToggle}
      >
        <span className="redeem-reference-trigger-icon">{icon}</span>
        <span className="redeem-reference-trigger-copy">
          <small>{eyebrow}</small>
          <strong>{title}</strong>
        </span>
        <ChevronRight className="redeem-reference-trigger-chevron" aria-hidden="true" />
      </button>

      {open ? (
        <div id={panelID} className="redeem-reference-popover">
          {children}
        </div>
      ) : null}
    </div>
  )
}

function RedeemReferenceFloats({ settings }: { settings: RedeemPageSettings }) {
  const [activePanel, setActivePanel] = React.useState<RedeemReferencePanel | null>(null)
  const dockRef = React.useRef<HTMLDivElement>(null)

  React.useEffect(() => {
    if (!activePanel) return

    const closeOnPointerDown = (event: PointerEvent) => {
      if (!dockRef.current?.contains(event.target as Node)) {
        setActivePanel(null)
      }
    }
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape") setActivePanel(null)
    }

    document.addEventListener("pointerdown", closeOnPointerDown)
    document.addEventListener("keydown", closeOnEscape)
    return () => {
      document.removeEventListener("pointerdown", closeOnPointerDown)
      document.removeEventListener("keydown", closeOnEscape)
    }
  }, [activePanel])

  const togglePanel = (panel: RedeemReferencePanel) => {
    setActivePanel((current) => current === panel ? null : panel)
  }

  return (
    <div ref={dockRef} className="redeem-reference-dock" aria-label="套餐权益参考">
      <FloatingReferencePanel
        side="left"
        eyebrow="CODEX"
        title="额度参考"
        icon={<Gauge />}
        open={activePanel === "quota"}
        onToggle={() => togglePanel("quota")}
      >
        <CodexQuotaReferenceCard settings={settings} />
      </FloatingReferencePanel>
      <FloatingReferencePanel
        side="right"
        eyebrow="WEB"
        title="模型权益"
        icon={<Sparkles />}
        open={activePanel === "models"}
        onToggle={() => togglePanel("models")}
      >
        <WebModelReferenceCard settings={settings} />
      </FloatingReferencePanel>
    </div>
  )
}

type PublicWorkspaceMode = "redeem" | "renewal"

function RedeemSafetyNoticeDialog({
  open,
  onOpenChange,
  settings,
  ready,
  mode,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  settings: RedeemPageSettings
  ready: boolean
  mode: PublicWorkspaceMode
}) {
  const handleOpenChange = (nextOpen: boolean) => {
    if (!nextOpen && !ready) return
    onOpenChange(nextOpen)
  }

  const renewalMode = mode === "renewal"
  const title = renewalMode ? settings.renewal_announcement_title : settings.announcement_title
  const intro = renewalMode ? settings.renewal_announcement_intro : settings.announcement_intro
  const items = renewalMode ? settings.renewal_announcement_items : settings.announcement_items

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent
        showCloseButton={false}
        className="gap-4 sm:max-w-[480px] sm:p-6"
        onEscapeKeyDown={(event) => {
          if (!ready) event.preventDefault()
        }}
        onInteractOutside={(event) => {
          if (!ready) event.preventDefault()
        }}
      >
        <DialogHeader className="gap-2">
          <div className="flex items-center gap-3">
            <span className="grid size-9 shrink-0 place-items-center rounded-lg bg-amber-500/10 text-amber-600 dark:text-amber-400">
              <AlertTriangle className="size-5" />
            </span>
            <DialogTitle className="text-xl leading-tight">{title}</DialogTitle>
          </div>
          <DialogDescription className="text-sm leading-5">
            {intro}
          </DialogDescription>
        </DialogHeader>

        <div className="grid gap-2 text-sm leading-5">
          {items.map((item, index) => (
            <div key={item} className="flex gap-3 rounded-lg border bg-muted/20 px-3.5 py-3">
              <span className="grid size-5 shrink-0 place-items-center rounded-full bg-brand/10 text-[10px] font-semibold text-brand">
                {index + 1}
              </span>
              <p className="text-muted-foreground">{item}</p>
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
            renewalMode ? "我已了解，开始续费" : "我已了解，继续兑换"
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
              icon={<WeChatIcon className="size-4 text-success" />}
              label="微信 / 手机号"
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
            <Button
              type="button"
              className="group sm:min-w-44"
              disabled={submitting}
              onClick={onConfirm}
            >
              {submitting ? (
                <>
                  <LoaderCircle data-slot="icon" className="animate-spin" />
                  正在提交
                </>
              ) : (
                <>
                  确认无误，继续兑换
                  <ArrowRight
                    data-slot="icon"
                    className="transition-transform duration-300 group-hover:translate-x-0.5"
                  />
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

            {invited ? (
              <div className="rounded-lg border bg-muted/20 px-4 py-3.5">
                <p className="text-xs font-semibold text-muted-foreground">接下来三步完成加入</p>
                <ol className="mt-2.5 grid gap-2 text-sm leading-5">
                  {[
                    "查收邀请邮件，收件箱里没有就看一下垃圾邮件",
                    "点击邮件中的「Join Team / 接受邀请」按钮",
                    "登录 ChatGPT，在左侧切换到 Team 工作空间",
                  ].map((stepText, stepIndex) => (
                    <li key={stepText} className="flex items-start gap-2.5">
                      <span className="mt-0.5 grid size-5 shrink-0 place-items-center rounded-full bg-success/10 text-[11px] font-semibold text-success">
                        {stepIndex + 1}
                      </span>
                      <span>{stepText}</span>
                    </li>
                  ))}
                </ol>
                <p className="mt-3 border-t pt-2.5 text-xs leading-5 text-muted-foreground">
                  超过 10 分钟仍未收到？添加页面上的客服微信，请管理员补发邀请。
                </p>
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

function renewalTokenStorageKey(sandboxAccessToken: string) {
  return sandboxAccessToken
    ? `${RENEWAL_STORAGE_KEY}:sandbox:${sandboxAccessToken}`
    : RENEWAL_STORAGE_KEY
}

function renewalDueHint(daysRemaining: number) {
  if (daysRemaining > 0) return `还有 ${daysRemaining} 天到期`
  if (daysRemaining === 0) return "今天到期"
  return `已到期 ${Math.abs(daysRemaining)} 天`
}

function renewalSubscriptionCaption(item: RenewalSubscriptionView) {
  const parts: string[] = []
  if (item.business_type === "team" && item.account_serial > 0) {
    parts.push(`${item.account_serial}号母号`)
  }
  if (item.seat_name) parts.push(item.seat_name)
  return parts.join(" · ") || item.service_label
}

function RenewalWorkspace({
  sandboxAccessToken,
  paymentConfigured,
  onSelectionChange,
}: {
  sandboxAccessToken: string
  paymentConfigured: boolean
  onSelectionChange: (view: RenewalSubscriptionView | null) => void
}) {
  const sandboxMode = sandboxAccessToken !== ""
  const storageKey = renewalTokenStorageKey(sandboxAccessToken)
  const [email, setEmail] = React.useState(sandboxMode ? "sandbox-customer@example.com" : "")
  const [selectedID, setSelectedID] = React.useState(0)
  const [confirmOpen, setConfirmOpen] = React.useState(false)
  const [statusOpen, setStatusOpen] = React.useState(false)
  const [trackingToken, setTrackingToken] = React.useState(() => readStoredToken(storageKey))

  const lookupMutation = useMutation({
    mutationFn: (customerEmail: string) => lookupRenewalSubscriptions(customerEmail, sandboxAccessToken),
    onSuccess: (result) => {
      const preferred = result.subscriptions.find((item) => item.renewable) ?? result.subscriptions[0]
      setSelectedID(preferred?.subscription_id ?? 0)
    },
    onError: (error: Error) => toast.error(error.message),
  })
  const selected = lookupMutation.data?.subscriptions.find(
    (item) => item.subscription_id === selectedID,
  ) ?? null

  React.useEffect(() => {
    onSelectionChange(selected)
  }, [onSelectionChange, selected])

  const statusQuery = useQuery({
    queryKey: ["public-renewal-status", sandboxAccessToken || "live", trackingToken],
    queryFn: () => fetchRenewalStatus(trackingToken, sandboxAccessToken),
    enabled: trackingToken !== "",
    refetchInterval: (query) => query.state.data?.status === "pending" ? 5_000 : false,
    retry: false,
  })
  const submitMutation = useMutation({
    mutationFn: (item: RenewalSubscriptionView) => submitRenewalApplication({
      customer_email: lookupMutation.data?.customer_email ?? email.trim(),
      subscription_id: item.subscription_id,
    }, sandboxAccessToken),
    onSuccess: (result) => {
      setTrackingToken(result.tracking_token)
      writeStoredToken(storageKey, result.tracking_token)
      setConfirmOpen(false)
      setStatusOpen(true)
      toast.success(result.message ?? "续费审核已提交")
      void lookupMutation.mutateAsync(lookupMutation.data?.customer_email ?? email.trim()).catch(() => undefined)
    },
    onError: (error: Error) => toast.error(error.message),
  })

  const runLookup = () => {
    const value = email.trim()
    if (!EMAIL_PATTERN.test(value)) {
      toast.error("请输入有效的订阅邮箱")
      return
    }
    lookupMutation.mutate(value)
  }
  const status = statusQuery.data
  const approved = status?.status === "approved"
  const rejected = status?.status === "rejected"

  return (
    <>
      <div className="redeem-renewal-workspace flex flex-1 flex-col gap-5 px-5 pb-6 pt-6 sm:px-8 sm:pb-8 lg:px-9 lg:pb-9">
        <div className="grid gap-2 sm:grid-cols-[minmax(0,1fr)_auto]">
          <div className="relative">
            <Mail className="redeem-input-icon" />
            <Input
              type="email"
              autoComplete="email"
              value={email}
              placeholder="输入订阅时登记的邮箱"
              className="redeem-input h-14 pl-11"
              onChange={(event) => setEmail(event.target.value)}
              onKeyDown={(event) => {
                if (event.key === "Enter") {
                  event.preventDefault()
                  runLookup()
                }
              }}
            />
          </div>
          <Button
            type="button"
            variant="outline"
            className="h-14 px-6"
            disabled={lookupMutation.isPending}
            onClick={runLookup}
          >
            {lookupMutation.isPending ? <LoaderCircle data-slot="icon" className="animate-spin" /> : <TicketCheck data-slot="icon" />}
            查询我的订阅
          </Button>
        </div>

        {lookupMutation.data ? (
          <div className="redeem-renewal-result">
            {lookupMutation.data.subscriptions.length > 1 ? (
              <div className="redeem-renewal-selector" role="tablist" aria-label="选择需要续费的订阅">
                {lookupMutation.data.subscriptions.map((item, index) => {
                  const active = item.subscription_id === selectedID
                  return (
                    <button
                      key={item.subscription_id}
                      type="button"
                      role="tab"
                      aria-selected={active}
                      className={cn("redeem-renewal-selector-item", active && "is-active")}
                      onClick={() => setSelectedID(item.subscription_id)}
                    >
                      <span>{String(index + 1).padStart(2, "0")}</span>
                      <strong>{item.service_label}</strong>
                    </button>
                  )
                })}
              </div>
            ) : null}

            {selected ? (
              <section className="redeem-renewal-summary" aria-live="polite">
                <div className="redeem-renewal-summary-heading">
                  <div className="min-w-0">
                    <p>RENEWAL PROFILE</p>
                    <h2>续费信息</h2>
                    <span>{renewalSubscriptionCaption(selected)}</span>
                  </div>
                  <div className="redeem-renewal-summary-actions">
                    <span className={cn("redeem-renewal-status", selected.days_remaining <= 7 && "is-urgent")}>{selected.status_label}</span>
                    {trackingToken ? (
                      <button type="button" className="redeem-renewal-progress" onClick={() => setStatusOpen(true)}>
                        审核进度
                        <ChevronRight />
                      </button>
                    ) : null}
                  </div>
                </div>
                <dl className="redeem-renewal-data">
                  <div>
                    <dt>邮箱</dt>
                    <dd className="font-mono">{lookupMutation.data.customer_email}</dd>
                  </div>
                  <div>
                    <dt>本期应付</dt>
                    <dd className="tabular-nums text-[var(--redeem-accent)]">¥{selected.amount_yuan}</dd>
                  </div>
                  <div>
                    <dt>计费周期</dt>
                    <dd>{selected.cycle_desc || "—"}</dd>
                  </div>
                  <div>
                    <dt>到期日期</dt>
                    <dd>
                      <span className="font-mono">{selected.due_date}</span>
                      <small className={cn(selected.days_remaining <= 7 && "is-urgent")}>（{renewalDueHint(selected.days_remaining)}）</small>
                    </dd>
                  </div>
                </dl>
                {!selected.renewable ? (
                  <p className="redeem-renewal-unavailable">{selected.unavailable_reason}</p>
                ) : null}
              </section>
            ) : null}
          </div>
        ) : (
          <div className="redeem-renewal-empty grid min-h-40 flex-1 place-items-center rounded-lg border border-dashed border-[var(--redeem-line-strong)] bg-[var(--redeem-panel-muted)] px-5 text-center">
            <div>
              <span className="mx-auto grid size-11 place-items-center rounded-lg bg-[var(--redeem-accent-soft)] text-[var(--redeem-accent)]"><CreditCard className="size-5" /></span>
              <p className="mt-3 text-sm font-semibold">先查询，再按账单付款</p>
              <p className="mt-1 text-xs leading-5 text-[var(--redeem-muted)]">系统会显示应付金额、续费日期与席位状态。</p>
            </div>
          </div>
        )}

        <Button
          type="button"
          className="redeem-submit-button h-14 w-full"
          disabled={!selected?.renewable || !paymentConfigured || submitMutation.isPending}
          onClick={() => setConfirmOpen(true)}
        >
          {submitMutation.isPending ? <LoaderCircle data-slot="icon" className="animate-spin" /> : <CreditCard data-slot="icon" />}
          {submitMutation.isPending ? "正在提交" : "提交续费审核"}
        </Button>
      </div>

      <Dialog open={confirmOpen} onOpenChange={(open) => { if (!submitMutation.isPending) setConfirmOpen(open) }}>
        <DialogContent className="gap-5 sm:max-w-[520px]">
          <DialogHeader>
            <DialogTitle>确认已按账单完成付款</DialogTitle>
            <DialogDescription>提交后管理员会按邮箱备注、金额和账期核对到账记录。</DialogDescription>
          </DialogHeader>
          {selected ? (
            <div className="divide-y overflow-hidden rounded-lg border bg-muted/20 text-sm">
              <ReviewItem icon={<Mail className="size-4 text-brand" />} label="邮箱" value={lookupMutation.data?.customer_email ?? email.trim()} mono />
              <ReviewItem icon={<CreditCard className="size-4 text-brand" />} label="付款金额" value={`¥${selected.amount_yuan}`} />
              <ReviewItem
                icon={<Clock3 className="size-4 text-brand" />}
                label="续费账期"
                value={selected.period_end_date ? `${selected.due_date} 至 ${selected.period_end_date}` : selected.due_date}
                mono
              />
            </div>
          ) : null}
          <div className="rounded-lg border border-amber-500/25 bg-amber-500/[0.07] px-4 py-3 text-sm leading-6 text-muted-foreground">
            金额不一致、忘记备注邮箱或付错金额时，请先联系客服，不要重复提交。
          </div>
          <DialogFooter>
            <Button variant="outline" disabled={submitMutation.isPending} onClick={() => setConfirmOpen(false)}>返回核对</Button>
            <Button disabled={!selected || submitMutation.isPending} onClick={() => { if (selected) submitMutation.mutate(selected) }}>
              {submitMutation.isPending ? <LoaderCircle data-slot="icon" className="animate-spin" /> : <CheckCircle2 data-slot="icon" />}
              确认付款并提交审核
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={statusOpen} onOpenChange={setStatusOpen}>
        <DialogContent className="gap-5 sm:max-w-[540px]">
          <DialogHeader className="items-center text-center">
            <span className={cn(
              "mb-2 grid size-14 place-items-center rounded-xl",
              approved ? "bg-success/10 text-success" : rejected ? "bg-destructive/10 text-destructive" : "bg-brand/10 text-brand",
            )}>
              {approved ? <CheckCircle2 className="size-7" /> : rejected ? <AlertTriangle className="size-7" /> : <Clock3 className="size-7 animate-pulse" />}
            </span>
            <DialogTitle>{approved ? "续费已完成" : rejected ? "续费审核未通过" : "续费审核中"}</DialogTitle>
            <DialogDescription>
              {approved ? "管理员已确认到账，订阅状态和下一账期已更新。" : rejected ? status?.operator_note || "请核对付款信息后联系客服。" : "管理员正在核对款项，结果会自动更新。"}
            </DialogDescription>
          </DialogHeader>
          {statusQuery.isError ? (
            <p className="rounded-lg border bg-muted/20 px-4 py-3 text-center text-sm text-muted-foreground">没有找到这条续费申请，请重新查询订阅。</p>
          ) : (
            <div className="divide-y rounded-lg border px-4 text-sm">
              <div className="flex justify-between gap-3 py-3"><span className="text-muted-foreground">邮箱</span><strong className="truncate font-mono">{status?.customer_email || "加载中"}</strong></div>
              <div className="flex justify-between gap-3 py-3"><span className="text-muted-foreground">账期 / 金额</span><strong>{status ? `${status.due_date} · ¥${status.amount_yuan}` : "加载中"}</strong></div>
              <div className="flex justify-between gap-3 py-3"><span className="text-muted-foreground">提交时间</span><strong>{status?.created_at_label || "加载中"}</strong></div>
            </div>
          )}
          {(approved || rejected || statusQuery.isError) ? (
            <Button type="button" variant="outline" onClick={() => {
              setStatusOpen(false)
              setTrackingToken("")
              writeStoredToken(storageKey, "")
              runLookup()
            }}>返回续费页</Button>
          ) : null}
        </DialogContent>
      </Dialog>
    </>
  )
}

export function RedeemPage() {
  const [searchParams] = useSearchParams()
  const sandboxAccessToken = searchParams.get("sandbox")?.trim() ?? ""
  const initialRedeemCode = searchParams.get("code")?.trim() ?? ""
  const sandboxMode = sandboxAccessToken !== ""
  const [mode, setMode] = React.useState<PublicWorkspaceMode>("redeem")
  const [selectedRenewal, setSelectedRenewal] = React.useState<RenewalSubscriptionView | null>(null)
  const tokenStorageKey = redemptionTokenStorageKey(sandboxAccessToken)
  const [trackingToken, setTrackingToken] = React.useState(() => readStoredToken(tokenStorageKey))
  const [noticeOpen, setNoticeOpen] = React.useState(() => trackingToken === "")
  const [flowStep, setFlowStep] = React.useState<RedeemFlowStep>(() =>
    trackingToken ? "progress" : "review",
  )
  const [flowDialogOpen, setFlowDialogOpen] = React.useState(() => trackingToken !== "")
  const [reviewValues, setReviewValues] = React.useState<FormValues | null>(null)
  const [lastSubmission, setLastSubmission] = React.useState<FormValues | null>(null)
  const [preloadedSupportQR, setPreloadedSupportQR] = React.useState("")
  const [preloadedPaymentQR, setPreloadedPaymentQR] = React.useState("")
  const renewalNoticeSeenRef = React.useRef(false)

  const settingsQuery = useQuery({
    queryKey: ["public-redeem-settings", sandboxAccessToken || "live"],
    queryFn: () => fetchRedeemPageSettings(sandboxAccessToken),
    staleTime: 5 * 60 * 1000,
  })
  const redeemSettings = normalizeRedeemPageSettings(settingsQuery.data)
  const supportQRDataURL = redeemSettings.support_qr_data_url.trim()
  const paymentQRDataURL = redeemSettings.payment_qr_data_url.trim()

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

  React.useEffect(() => {
    if (!paymentQRDataURL) return
    let active = true
    const image = new Image()
    const markReady = () => { if (active) setPreloadedPaymentQR(paymentQRDataURL) }
    image.onload = markReady
    image.onerror = markReady
    image.src = paymentQRDataURL
    if (image.complete) markReady()
    return () => {
      active = false
      image.onload = null
      image.onerror = null
    }
  }, [paymentQRDataURL])

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
        customer_contact: values.customer_contact.trim(),
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
    const trimmedValues = {
      customer_email: values.customer_email.trim(),
      customer_contact: values.customer_contact.trim(),
      redeem_code: values.redeem_code.trim(),
    }
    setLastSubmission(trimmedValues)
    setReviewValues(trimmedValues)
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
    // 已成功邀请的兑换码会被消耗，重新兑换时不再预填；驳回或记录失效则保留，方便修正后重新提交
    const keepRedeemCode = statusQuery.data?.status !== "invited"
    setTrackingToken("")
    writeStoredToken(tokenStorageKey, "")
    setFlowDialogOpen(false)
    setFlowStep("review")
    setReviewValues(null)
    form.reset({
      customer_email:
        lastSubmission?.customer_email ?? (sandboxMode ? "sandbox-customer@example.com" : ""),
      redeem_code: keepRedeemCode ? (lastSubmission?.redeem_code ?? "") : "",
      customer_contact:
        lastSubmission?.customer_contact ?? (sandboxMode ? "sandbox_wechat" : ""),
    })
  }

  const statusLoadFailed = trackingToken !== "" && statusQuery.isError
  const supportConfigured = hasSupportContact(redeemSettings)
  const supportColumnVisible = mode === "renewal" || supportConfigured || settingsQuery.isPending
  const supportAssetReady = supportQRDataURL === "" || preloadedSupportQR === supportQRDataURL
  const paymentAssetReady = paymentQRDataURL === "" || preloadedPaymentQR === paymentQRDataURL
  const redeemPageReady = !settingsQuery.isPending && (mode === "renewal" ? paymentAssetReady : supportAssetReady)
  const handleModeChange = (nextMode: PublicWorkspaceMode) => {
    setMode(nextMode)
    if (nextMode === "renewal" && !renewalNoticeSeenRef.current) {
      renewalNoticeSeenRef.current = true
      setNoticeOpen(true)
    }
  }
  const handleRenewalSelectionChange = React.useCallback((view: RenewalSubscriptionView | null) => {
    setSelectedRenewal(view)
  }, [setSelectedRenewal])

  return (
    <main className="redeem-console min-h-dvh overflow-hidden text-[var(--redeem-text)]">
      <RedeemSafetyNoticeDialog
        open={noticeOpen}
        onOpenChange={setNoticeOpen}
        settings={redeemSettings}
        ready={redeemPageReady}
        mode={mode}
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

      <RedeemAmbientField />
      {mode === "redeem" ? <RedeemReferenceFloats settings={redeemSettings} /> : null}

      <header className="redeem-topbar">
        <div className="mx-auto flex h-16 w-full max-w-[1760px] items-center justify-between gap-3 px-4 sm:h-[68px] sm:px-6 lg:px-8">
          <div className="flex min-w-0 items-center gap-3">
            <BrandIcon className="size-8 rounded-md shadow-none sm:size-9" />
            <div className="min-w-0">
              <p className="truncate text-[13px] font-semibold leading-none sm:text-[15px]">{APP_NAME}</p>
              <div className="mt-1.5 flex items-center gap-2 whitespace-nowrap text-[10px] font-medium text-[var(--redeem-muted)]">
                <span className="hidden sm:inline">{mode === "renewal" ? "订阅自助续费" : "Team 席位兑换"}</span>
                <span className="redeem-status-dot" aria-hidden="true" />
                <span className="text-[var(--redeem-accent)]">在线</span>
              </div>
            </div>
            {sandboxMode ? <span className="redeem-sandbox-badge">SANDBOX</span> : null}
          </div>
          <div className="flex shrink-0 items-center gap-2">
            <RedeemAnnouncementButton mode={mode} onClick={() => setNoticeOpen(true)} />
            {mode === "renewal" ? (
              <PaymentDialogButton settings={redeemSettings} selected={selectedRenewal} />
            ) : (
              <SupportWechatDialogButton settings={redeemSettings} />
            )}
            <RedeemThemeToggle />
          </div>
        </div>
      </header>

      <section className="relative mx-auto w-full max-w-[1760px] px-4 pb-8 pt-6 sm:px-6 sm:pb-10 sm:pt-7 lg:px-8 lg:pb-12 lg:pt-8">
        <div
          className={cn(
            "redeem-workspace grid items-stretch gap-6 animate-fade-up",
            supportColumnVisible
              ? "has-support"
              : "is-single",
          )}
        >
          <div className="redeem-workspace-decor" aria-hidden="true">
            <div className="redeem-telemetry-bar">
              <span className="redeem-telemetry-label">
                <i />
                {mode === "renewal" ? "RENEWAL WORKSPACE" : "REDEMPTION WORKSPACE"}
              </span>
              <span className="redeem-telemetry-track" />
              <span className="redeem-telemetry-label">CPN / ACCESS</span>
            </div>
            <span className="redeem-frame-corner is-top-left" />
            <span className="redeem-frame-corner is-top-right" />
            <span className="redeem-frame-corner is-bottom-left" />
            <span className="redeem-frame-corner is-bottom-right" />
            <div className="redeem-side-rail is-left">
                <span>{mode === "renewal" ? "BILL CHANNEL" : "INPUT CHANNEL"}</span>
            </div>
            <div className="redeem-side-rail is-right">
                <span>{mode === "renewal" ? "PAYMENT CHANNEL" : "SUPPORT CHANNEL"}</span>
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
                  {mode === "renewal" ? "ACCOUNT / RENEW" : "TEAM / REDEEM"}
                </span>
              </div>
              <span className="redeem-channel-status">
                <span className="redeem-status-dot" />
                <span>通道在线</span>
              </span>
            </div>

            <div className="px-5 pt-6 sm:px-8 sm:pt-8 lg:px-9">
              <div className="redeem-portal-heading">
                <p className="redeem-portal-eyebrow">
                  {mode === "renewal" ? "RENEWAL REVIEW" : "ACCESS REQUEST"}
                </p>
                <h1 className="redeem-portal-title">
                  <span>ChatGPT</span>
                  <span className="redeem-portal-title-accent">自助兑换系统</span>
                </h1>
                <p className="redeem-service-caption">
                  {mode === "renewal" ? "查询订阅账单并提交续费审核" : "提交兑换信息并等待席位邀请"}
                </p>
              </div>
              <nav className="redeem-mode-switch" aria-label="自助服务入口">
                <button
                  type="button"
                  className={cn("redeem-mode-tab", mode === "redeem" && "is-active")}
                  aria-current={mode === "redeem" ? "page" : undefined}
                  onClick={() => handleModeChange("redeem")}
                >
                  <span className="redeem-mode-index">01</span>
                  <span className="redeem-mode-icon"><TicketCheck /></span>
                  <span className="redeem-mode-copy">
                    <strong>兑换申请</strong>
                    <small>REDEEM</small>
                  </span>
                </button>
                <button
                  type="button"
                  className={cn("redeem-mode-tab", mode === "renewal" && "is-active")}
                  aria-current={mode === "renewal" ? "page" : undefined}
                  onClick={() => handleModeChange("renewal")}
                >
                  <span className="redeem-mode-index">02</span>
                  <span className="redeem-mode-icon"><CreditCard /></span>
                  <span className="redeem-mode-copy">
                    <strong>自助续费</strong>
                    <small>RENEW</small>
                  </span>
                </button>
              </nav>
            </div>

            {mode === "redeem" ? <Form {...form}>
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
                          微信 / 手机号
                        </FormLabel>
                        <FormControl>
                          <div className="relative">
                            <WeChatIcon className="redeem-input-icon" />
                            <Input
                              autoComplete="off"
                              placeholder="请输入可搜索的微信号或手机号"
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
                  className="redeem-submit-button h-14 w-full justify-center"
                  disabled={submitMutation.isPending}
                >
                  {submitMutation.isPending ? (
                    <>
                      <LoaderCircle data-slot="icon" className="animate-spin" />
                      正在建立兑换请求
                    </>
                  ) : (
                    <span>开始兑换</span>
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
            </Form> : (
              <RenewalWorkspace
                sandboxAccessToken={sandboxAccessToken}
                paymentConfigured={paymentQRDataURL !== ""}
                onSelectionChange={handleRenewalSelectionChange}
              />
            )}
          </Card>

          {settingsQuery.isPending ? (
            <aside className="redeem-support-placeholder hidden h-[500px] animate-pulse rounded-[8px] border border-[var(--redeem-line)] bg-[var(--redeem-panel)] lg:block" />
          ) : mode === "renewal" ? (
            <PaymentPanel settings={redeemSettings} selected={selectedRenewal} />
          ) : (
            <SupportWechatPanel settings={redeemSettings} />
          )}
        </div>
      </section>
    </main>
  )
}
