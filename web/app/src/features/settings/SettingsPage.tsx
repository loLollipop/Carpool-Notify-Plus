import * as React from "react"
import { useMutation, useQuery } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import {
  BellRing,
  CircleCheckBig,
  Copy,
  Database,
  Download,
  ExternalLink,
  Eye,
  FlaskConical,
  Gauge,
  ImageUp,
  Mail,
  Megaphone,
  MessageSquare,
  Send,
  ShieldCheck,
  Snowflake,
  Sparkles,
  Trash2,
} from "lucide-react"

import {
  fetchSandboxStatus,
  previewSettingsTemplate,
  resetSandbox,
  saveSettings,
  testSettingsCustomerEmail,
  testSettingsNotify,
} from "@/api/endpoints"
import { useAppMutation } from "@/api/mutations"
import { useSettings } from "@/api/queries"
import type {
  ChannelSetting,
  NotificationConfig,
  RedeemPageSettings,
  SandboxStatus,
  Settings,
  SettingsInput,
} from "@/api/types"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { EmailPreview } from "@/components/email-preview"
import { WeChatIcon } from "@/components/icons/wechat-icon"
import { PageHeader } from "@/components/page-header"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { Checkbox } from "@/components/ui/checkbox"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Skeleton } from "@/components/ui/skeleton"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Textarea } from "@/components/ui/textarea"
import { cn } from "@/lib/utils"
import { enterSandboxMode, exitSandboxMode } from "@/lib/sandbox-mode"
import { useSandboxMode } from "@/hooks/use-sandbox-mode"

type TemplateKind =
  | "notify"
  | "customer"
  | "customer_price_increase"
  | "customer_price_decrease"
type CustomerTemplateKind = Exclude<TemplateKind, "notify">
type SettingsSection = "templates" | "delivery" | "redemption" | "operations" | "tools"

type TemplateFieldKey =
  | "customerEmail"
  | "customerWechat"
  | "previousPrice"
  | "amountDue"
  | "cycleDesc"
  | "nextDueDate"
  | "remark"
  | "tradeURL"

interface TemplateFieldOption {
  key: TemplateFieldKey
  signatures: string[]
}

const TEMPLATE_FIELDS: TemplateFieldOption[] = [
  { key: "customerEmail", signatures: [".CustomerEmail"] },
  { key: "customerWechat", signatures: [".CustomerWechat"] },
  { key: "previousPrice", signatures: [".PreviousPrice"] },
  { key: "amountDue", signatures: [".AmountDue", ".PricePerPerson"] },
  { key: "cycleDesc", signatures: [".CycleDesc"] },
  { key: "nextDueDate", signatures: [".NextDueDate", ".DueInText", ".DaysUntilDue"] },
  { key: "remark", signatures: [".Remark"] },
  { key: "tradeURL", signatures: [".TradeURL"] },
]

const DEFAULT_FIELDS: TemplateFieldKey[] = [
  "customerEmail",
  "customerWechat",
  "amountDue",
  "cycleDesc",
  "nextDueDate",
  "remark",
  "tradeURL",
]

const PRICE_CHANGE_FIELDS: TemplateFieldKey[] = [
  "customerEmail",
  "previousPrice",
  "amountDue",
  "cycleDesc",
  "nextDueDate",
  "remark",
  "tradeURL",
]

function templateFieldsForKind(kind: TemplateKind) {
  return TEMPLATE_FIELDS.filter(
    (field) =>
      field.key !== "previousPrice" ||
      kind === "customer_price_increase" ||
      kind === "customer_price_decrease",
  )
}

interface VisualTemplateDraft {
  title: string
  duePrefix: string
  dueSuffix: string
  fields: TemplateFieldKey[]
  footer: string
}

interface SMTPConfigState {
  host: string
  port: string
  username: string
  password: string
  from: string
  to: string
}

interface NotificationConfigState {
  smtp: SMTPConfigState
  iyuuToken: string
  gotifyURL: string
  gotifyToken: string
}

type TemplatePreviewState =
  | { status: "loading" }
  | { status: "ok"; rendered: string; sampleName: string; subject: string; html: string }
  | { status: "error"; message: string }

const MAX_QR_UPLOAD_BYTES = 1024 * 1024
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
  codex_plus_weekly_quota_usd: 150,
  codex_team_weekly_quota_usd: 200,
  web_primary_benefit_label: "GPT-5.6 sol 极高",
  web_plus_primary_benefit: "不支持",
  web_team_primary_benefit: "支持",
  web_secondary_benefit_label: "Pro 模型",
  web_plus_secondary_benefit: "—",
  web_team_secondary_benefit: "15 次/月",
}

function ChannelBadge({ channel }: { channel: ChannelSetting }) {
  const { t } = useTranslation()
  return channel.configured ? (
    <Badge variant="success">{t("settings.configured")}</Badge>
  ) : (
    <Badge variant="secondary">{t("settings.notConfigured")}</Badge>
  )
}

function channelLabel(channel: ChannelSetting, t: (key: string) => string) {
  if (channel.key === "smtp") return t("settings.channelSMTP")
  if (channel.key === "gotify") return t("settings.channelGotifyOptional")
  return channel.label
}

function isFixedRouteChannel(channel: ChannelSetting) {
  return channel.key === "iyuu" || channel.key === "smtp"
}

function isChannelChecked(channel: ChannelSetting, enabledChannels: Set<string>) {
  if (isFixedRouteChannel(channel)) {
    return channel.configured
  }
  return enabledChannels.has(channel.key)
}

function defaultTemplateDraft(kind: TemplateKind): VisualTemplateDraft {
  if (kind === "customer_price_increase" || kind === "customer_price_decrease") {
    const decrease = kind === "customer_price_decrease"
    return {
      title: "",
      duePrefix: decrease
        ? "您好，您的 ChatGPT Team 拼车续费价格将进行优惠调整，"
        : "您好，您的 ChatGPT Team 拼车续费价格已安排调整，",
      dueSuffix: "。",
      fields: PRICE_CHANGE_FIELDS,
      footer: decrease
        ? "为感谢您的支持并提升续费体验，我们将结合近期运营安排与客户关系维护，对本次续费价格进行优惠调整，自上述生效日期起按调整后的价格续费。\n当前已支付周期与历史账单不受影响；完成本次续费后，后续周期将以调整后的价格为基准。\n如有任何疑问，请联系管理员。"
        : "综合近期同类服务价格以及账号、售后维护成本，经重新核算，自上述生效日期起将按调整后的价格续费。\n当前已支付周期与历史账单不受影响；完成本次续费后，后续周期将以调整后的价格为基准。\n如对调整有疑问，或不准备继续续费，请在生效日前联系管理员确认。",
    }
  }
  if (kind === "customer") {
    return {
      title: "",
      duePrefix: "您好，您的 ChatGPT Team 拼车服务",
      dueSuffix: "，请留意续费时间。",
      fields: DEFAULT_FIELDS,
      footer: "为避免到期后影响使用，请在到期前完成续费。\n如需续费或售后协助，请联系管理员。",
    }
  }
  return {
    title: "【到期提醒】",
    duePrefix: "到期状态：",
    dueSuffix: "",
    fields: DEFAULT_FIELDS,
    footer: "",
  }
}

function inferFooterFromTemplate(templateBody: string) {
  const lines = templateBody.split("\n")
  const signatures = [
    ".DueInText",
    ...TEMPLATE_FIELDS.flatMap((field) => field.signatures),
  ]
  let lastManagedLine = -1
  lines.forEach((line, index) => {
    if (signatures.some((signature) => line.includes(signature))) {
      lastManagedLine = index
    }
  })
  if (lastManagedLine < 0 || lastManagedLine >= lines.length - 1) {
    return ""
  }
  return lines
    .slice(lastManagedLine + 1)
    .join("\n")
    .trim()
}

function draftFromTemplate(kind: TemplateKind, templateBody: string): VisualTemplateDraft {
  const draft = defaultTemplateDraft(kind)
  const fields = templateFieldsForKind(kind).filter((field) =>
    field.signatures.some((signature) => templateBody.includes(signature)),
  ).map((field) => field.key)

  if (fields.length > 0) {
    draft.fields = fields
  }
  const footer = inferFooterFromTemplate(templateBody)
  if (footer !== "") {
    draft.footer = footer
  }
  return draft
}

function fieldLine(key: TemplateFieldKey, kind: TemplateKind) {
  switch (key) {
    case "customerEmail":
      return "客户邮箱：{{.CustomerEmail}}"
    case "customerWechat":
      return "{{if .CustomerWechat}}客户微信：{{.CustomerWechat}}{{end}}"
    case "previousPrice":
      return "原每期价格：¥{{.PreviousPrice}}"
    case "amountDue":
      return kind === "customer_price_increase" || kind === "customer_price_decrease"
        ? "调整后每期价格：¥{{.AmountDue}}"
        : "本期应收：¥{{.AmountDue}}"
    case "cycleDesc":
      return "计费周期：{{.CycleDesc}}"
    case "nextDueDate":
      return kind === "customer_price_increase" || kind === "customer_price_decrease"
        ? "生效日期：{{.NextDueDate}}（{{.DueInText}}）"
        : "到期日期：{{.NextDueDate}}（{{.DueInText}}）"
    case "remark":
      return "{{if .Remark}}备注：{{.Remark}}{{end}}"
    case "tradeURL":
      return kind === "customer"
        ? "{{if .TradeURL}}续费链接：{{.TradeURL}}{{end}}"
        : "{{if .TradeURL}}链接：{{.TradeURL}}{{end}}"
  }
}

function renderVisualTemplate(kind: TemplateKind, draft: VisualTemplateDraft) {
  const lines: string[] = []
  const title = draft.title.trim()
  if (title !== "") {
    lines.push(title)
  }

  const dueLine = `${draft.duePrefix}{{.DueInText}}${draft.dueSuffix}`.trim()
  if (dueLine !== "") {
    lines.push(dueLine)
  }

  const detailLines = draft.fields.map((field) => fieldLine(field, kind))
  if (detailLines.length > 0) {
    if (lines.length > 0) {
      lines.push("")
    }
    lines.push(...detailLines)
  }

  const footer = draft.footer.trim()
  if (footer !== "") {
    lines.push("", footer)
  }

  return lines.join("\n").replace(/\n{3,}/g, "\n\n").trim()
}

function updateDraftField<T extends keyof VisualTemplateDraft>(
  draft: VisualTemplateDraft,
  key: T,
  value: VisualTemplateDraft[T],
) {
  return { ...draft, [key]: value }
}

function TemplateEditor({
  kind,
  draft,
  onDraftChange,
  onPreview,
  customerTemplateKind,
  onCustomerTemplateKindChange,
}: {
  kind: TemplateKind
  draft: VisualTemplateDraft
  onDraftChange: (draft: VisualTemplateDraft) => void
  onPreview: () => void
  customerTemplateKind?: CustomerTemplateKind
  onCustomerTemplateKindChange?: (kind: CustomerTemplateKind) => void
}) {
  const { t } = useTranslation()
  const availableFields = templateFieldsForKind(kind)
  const icon =
    kind === "notify" ? <MessageSquare className="size-4" /> : <Mail className="size-4" />

  const toggleField = (key: TemplateFieldKey, checked: boolean) => {
    const nextFields = checked
      ? availableFields.map((field) => field.key).filter(
          (fieldKey) => draft.fields.includes(fieldKey) || fieldKey === key,
        )
      : draft.fields.filter((fieldKey) => fieldKey !== key)
    onDraftChange(updateDraftField(draft, "fields", nextFields))
  }

  return (
    <div
      className={cn(
        "relative grid gap-4 overflow-hidden rounded-lg border bg-card p-4 shadow-[0_1px_2px_color-mix(in_oklab,var(--foreground)_3%,transparent)]",
      )}
    >
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-2">
          <span
            className={cn(
              "grid size-8 place-items-center rounded-md",
              "bg-brand/10 text-brand",
            )}
          >
            {icon}
          </span>
          <Label>{t(kind === "notify" ? "settings.notifyTemplate" : "settings.customerTemplate")}</Label>
        </div>
        <div className="flex min-w-0 items-center gap-2">
          {kind !== "notify" && customerTemplateKind && onCustomerTemplateKindChange ? (
            <Select
              value={customerTemplateKind}
              onValueChange={(value) =>
                onCustomerTemplateKindChange(value as CustomerTemplateKind)
              }
            >
              <SelectTrigger
                size="sm"
                className="w-[9.75rem] bg-background"
                aria-label={t("settings.customerTemplateType")}
              >
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="customer">
                  {t("settings.customerTemplateRegular")}
                </SelectItem>
                <SelectItem value="customer_price_increase">
                  {t("settings.customerTemplatePriceIncrease")}
                </SelectItem>
                <SelectItem value="customer_price_decrease">
                  {t("settings.customerTemplatePriceDecrease")}
                </SelectItem>
              </SelectContent>
            </Select>
          ) : null}
          <Button
            type="button"
            variant="ghost"
            size="xs"
            className="text-muted-foreground"
            onClick={onPreview}
          >
            <Eye data-slot="icon" />
            {t("settings.preview")}
          </Button>
        </div>
      </div>

      {kind === "notify" ? (
        <div className="grid gap-1.5">
          <Label htmlFor="notify-template-title">{t("settings.templateTitleLine")}</Label>
          <Input
            id="notify-template-title"
            value={draft.title}
            onChange={(event) =>
              onDraftChange(updateDraftField(draft, "title", event.target.value))
            }
          />
        </div>
      ) : null}

      <div className="grid gap-1.5">
        <Label>{t("settings.templateDueSentence")}</Label>
        <div className="grid gap-2 sm:grid-cols-[minmax(0,1fr)_auto_minmax(0,1fr)] sm:items-center">
          <Input
            value={draft.duePrefix}
            aria-label={t("settings.templateDuePrefix")}
            onChange={(event) =>
              onDraftChange(updateDraftField(draft, "duePrefix", event.target.value))
            }
          />
          <Badge variant="brand" className="justify-self-start sm:justify-self-center">
            {t("settings.templateFields.dueInText")}
          </Badge>
          <Input
            value={draft.dueSuffix}
            aria-label={t("settings.templateDueSuffix")}
            onChange={(event) =>
              onDraftChange(updateDraftField(draft, "dueSuffix", event.target.value))
            }
          />
        </div>
      </div>

      <div className="grid gap-2">
        <Label>{t("settings.templateFieldsTitle")}</Label>
        <div className="grid gap-2 sm:grid-cols-2">
          {availableFields.map((field) => (
            <label
              key={field.key}
              className="flex min-h-10 cursor-pointer items-center gap-2 rounded-md border bg-background px-3 py-2 text-sm"
            >
              <Checkbox
                checked={draft.fields.includes(field.key)}
                onCheckedChange={(value) => toggleField(field.key, value === true)}
              />
              <span>
                {t(
                  (kind === "customer_price_increase" || kind === "customer_price_decrease") && field.key === "amountDue"
                    ? "settings.templateFields.adjustedAmountDue"
                    : `settings.templateFields.${field.key}`,
                )}
              </span>
            </label>
          ))}
        </div>
      </div>

      <div className="grid gap-1.5">
        <Label>{t("settings.templateFooter")}</Label>
        <Textarea
          rows={kind === "notify" ? 2 : 3}
          value={draft.footer}
          placeholder={t("common.optional")}
          onChange={(event) =>
            onDraftChange(updateDraftField(draft, "footer", event.target.value))
          }
        />
      </div>
    </div>
  )
}

function initialNotificationConfigState(config: NotificationConfig): NotificationConfigState {
  return {
    smtp: {
      host: config.smtp.host,
      port: String(config.smtp.port || 587),
      username: config.smtp.username,
      password: "",
      from: config.smtp.from,
      to: config.smtp.to,
    },
    iyuuToken: "",
    gotifyURL: config.gotify.url,
    gotifyToken: "",
  }
}

function SecretHint({ configured }: { configured: boolean }) {
  const { t } = useTranslation()
  return (
    <span className="text-xs text-muted-foreground">
      {configured ? t("settings.secretKeepHint") : t("settings.secretMissingHint")}
    </span>
  )
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

function RedeemPageSettingsEditor({
  value,
  onChange,
}: {
  value: RedeemPageSettings
  onChange: (value: RedeemPageSettings) => void
}) {
  const fileInputRef = React.useRef<HTMLInputElement | null>(null)
  const announcementItemsText = value.announcement_items.join("\n")

  const update = (patch: Partial<RedeemPageSettings>) => {
    onChange({ ...value, ...patch })
  }

  const handleUpload = (event: React.ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0]
    event.target.value = ""
    if (!file) return
    if (!["image/png", "image/jpeg", "image/webp"].includes(file.type)) {
      toast.error("二维码仅支持 PNG、JPG 或 WebP")
      return
    }
    if (file.size > MAX_QR_UPLOAD_BYTES) {
      toast.error("二维码图片太大，请压缩到 1MB 以内")
      return
    }

    const reader = new FileReader()
    reader.onload = () => {
      const dataURL = typeof reader.result === "string" ? reader.result : ""
      if (!dataURL.startsWith("data:image/")) {
        toast.error("二维码读取失败，请换一张图片")
        return
      }
      update({ support_qr_data_url: dataURL })
      toast.success("二维码已载入，保存设置后生效")
    }
    reader.onerror = () => toast.error("二维码读取失败，请重试")
    reader.readAsDataURL(file)
  }

  return (
    <div className="grid gap-4 animate-fade-up" style={{ animationDelay: "90ms" }}>
      <div className="flex flex-wrap items-center justify-between gap-3 px-1">
        <div className="flex items-center gap-2">
          <span className="grid size-8 place-items-center rounded-md bg-brand/10 text-brand">
            <Megaphone className="size-4" />
          </span>
          <h2 className="panel-heading text-sm font-semibold">兑换页设置</h2>
        </div>
        <Button type="button" variant="outline" size="sm" asChild>
          <a href="/redeem" target="_blank" rel="noopener noreferrer">
            <ExternalLink data-slot="icon" />
            预览兑换页
          </a>
        </Button>
      </div>

      <div className="grid items-stretch gap-4 lg:grid-cols-2">
        <Card className="content-start gap-5 p-5 sm:p-6">
          <div className="flex items-center gap-3 border-b pb-4">
            <span className="grid size-9 shrink-0 place-items-center rounded-md bg-brand/10 text-brand">
              <Megaphone className="size-4" />
            </span>
            <h3 className="text-sm font-semibold">公告内容</h3>
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="redeem-announcement-title">公告标题</Label>
            <Input
              id="redeem-announcement-title"
              value={value.announcement_title}
              onChange={(event) => update({ announcement_title: event.target.value })}
            />
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="redeem-announcement-intro">公告说明</Label>
            <Textarea
              id="redeem-announcement-intro"
              rows={3}
              value={value.announcement_intro}
              onChange={(event) => update({ announcement_intro: event.target.value })}
            />
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="redeem-announcement-items">公告条目</Label>
            <Textarea
              id="redeem-announcement-items"
              rows={5}
              value={announcementItemsText}
              placeholder="一行一条公告"
              onChange={(event) =>
                update({
                  announcement_items: event.target.value
                    .split("\n")
                    .map((item) => item.trim())
                    .filter((item) => item !== ""),
                })
              }
            />
            <p className="text-xs text-muted-foreground">每行一条，最多 6 条</p>
          </div>
        </Card>

        <Card className="content-start gap-5 p-5 sm:p-6">
          <div className="flex items-center gap-3 border-b pb-4">
            <span className="grid size-9 shrink-0 place-items-center rounded-md bg-brand/10 text-brand">
              <WeChatIcon className="size-4" />
            </span>
            <h3 className="text-sm font-semibold">客服信息</h3>
          </div>

          <div className="grid items-start gap-5 xl:grid-cols-[minmax(0,1fr)_12rem]">
            <div className="grid content-start gap-4">
              <div className="grid gap-1.5">
                <Label htmlFor="redeem-support-title">客服标题</Label>
                <Input
                  id="redeem-support-title"
                  value={value.support_title}
                  onChange={(event) => update({ support_title: event.target.value })}
                />
              </div>
              <div className="grid gap-1.5">
                <Label htmlFor="redeem-support-description">客服说明</Label>
                <Input
                  id="redeem-support-description"
                  value={value.support_description}
                  onChange={(event) => update({ support_description: event.target.value })}
                />
              </div>
              <div className="grid gap-1.5">
                <Label htmlFor="redeem-support-wechat">客服微信号</Label>
                <Input
                  id="redeem-support-wechat"
                  value={value.support_wechat_id}
                  placeholder="不填写则不显示复制按钮"
                  onChange={(event) => update({ support_wechat_id: event.target.value })}
                />
              </div>
            </div>

            <div className="grid content-start gap-2">
              <Label>客服微信二维码</Label>
              {value.support_qr_data_url ? (
                <div className="grid aspect-square w-full max-w-48 place-items-center overflow-hidden rounded-lg border bg-white p-2">
                  <img
                    src={value.support_qr_data_url}
                    alt="客服微信二维码预览"
                    className="size-full rounded-md object-contain"
                  />
                </div>
              ) : (
                <div className="grid aspect-square w-full max-w-48 place-items-center rounded-lg border border-dashed bg-muted/35 px-4 text-center text-sm text-muted-foreground">
                  未上传二维码
                </div>
              )}
              <input
                ref={fileInputRef}
                type="file"
                accept="image/png,image/jpeg,image/webp"
                className="hidden"
                onChange={handleUpload}
              />
              <div className="flex flex-wrap gap-2">
                <Button type="button" variant="outline" size="sm" onClick={() => fileInputRef.current?.click()}>
                  <ImageUp data-slot="icon" />
                  上传图片
                </Button>
                {value.support_qr_data_url ? (
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon-sm"
                    className="text-destructive hover:text-destructive"
                    aria-label="移除二维码"
                    title="移除二维码"
                    onClick={() => update({ support_qr_data_url: "" })}
                  >
                    <Trash2 />
                  </Button>
                ) : null}
              </div>
              <p className="text-xs leading-relaxed text-muted-foreground">
                图片用于兑换页公开展示，请勿上传敏感截图。
              </p>
            </div>
          </div>
        </Card>
      </div>

      <Card className="gap-5 p-5 sm:p-6">
        <div className="flex items-start gap-3 border-b pb-4">
          <span className="grid size-9 shrink-0 place-items-center rounded-md bg-brand/10 text-brand">
            <Gauge className="size-4" />
          </span>
          <div>
            <h3 className="text-sm font-semibold">权益参考卡</h3>
            <p className="mt-1 text-xs leading-5 text-muted-foreground">
              保存后会同步到兑换页两侧的 Codex 额度与网页端模型权益卡。
            </p>
          </div>
        </div>

        <div className="grid gap-6 xl:grid-cols-[minmax(0,0.7fr)_minmax(0,1.3fr)]">
          <section className="grid content-start gap-4 rounded-lg border bg-muted/20 p-4">
            <div className="flex items-center gap-2">
              <Gauge className="size-4 text-brand" />
              <h4 className="text-sm font-semibold">Codex 周额度参考</h4>
            </div>
            <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-1 2xl:grid-cols-2">
              <div className="grid gap-1.5">
                <Label htmlFor="redeem-codex-plus-quota">Plus 周额度</Label>
                <div className="relative">
                  <Input
                    id="redeem-codex-plus-quota"
                    type="number"
                    min={1}
                    max={100000}
                    step={1}
                    inputMode="numeric"
                    value={value.codex_plus_weekly_quota_usd}
                    className="pr-14 tabular-nums"
                    onChange={(event) =>
                      update({ codex_plus_weekly_quota_usd: Number(event.target.value) })
                    }
                  />
                  <span className="pointer-events-none absolute top-1/2 right-3 -translate-y-1/2 text-xs text-muted-foreground">
                    美元
                  </span>
                </div>
              </div>
              <div className="grid gap-1.5">
                <Label htmlFor="redeem-codex-team-quota">Team 周额度</Label>
                <div className="relative">
                  <Input
                    id="redeem-codex-team-quota"
                    type="number"
                    min={1}
                    max={100000}
                    step={1}
                    inputMode="numeric"
                    value={value.codex_team_weekly_quota_usd}
                    className="pr-14 tabular-nums"
                    onChange={(event) =>
                      update({ codex_team_weekly_quota_usd: Number(event.target.value) })
                    }
                  />
                  <span className="pointer-events-none absolute top-1/2 right-3 -translate-y-1/2 text-xs text-muted-foreground">
                    美元
                  </span>
                </div>
              </div>
            </div>
          </section>

          <section className="grid content-start gap-4 rounded-lg border bg-muted/20 p-4">
            <div className="flex items-center gap-2">
              <Sparkles className="size-4 text-brand" />
              <h4 className="text-sm font-semibold">网页端模型权益</h4>
            </div>
            <div className="grid gap-4 md:grid-cols-3">
              <div className="grid gap-1.5">
                <Label htmlFor="redeem-web-primary-label">权益名称</Label>
                <Input
                  id="redeem-web-primary-label"
                  maxLength={80}
                  value={value.web_primary_benefit_label}
                  onChange={(event) => update({ web_primary_benefit_label: event.target.value })}
                />
              </div>
              <div className="grid gap-1.5">
                <Label htmlFor="redeem-web-plus-primary">Plus 权益</Label>
                <Input
                  id="redeem-web-plus-primary"
                  maxLength={80}
                  value={value.web_plus_primary_benefit}
                  onChange={(event) => update({ web_plus_primary_benefit: event.target.value })}
                />
              </div>
              <div className="grid gap-1.5">
                <Label htmlFor="redeem-web-team-primary">Team 权益</Label>
                <Input
                  id="redeem-web-team-primary"
                  maxLength={80}
                  value={value.web_team_primary_benefit}
                  onChange={(event) => update({ web_team_primary_benefit: event.target.value })}
                />
              </div>
              <div className="grid gap-1.5">
                <Label htmlFor="redeem-web-secondary-label">附加权益名称</Label>
                <Input
                  id="redeem-web-secondary-label"
                  maxLength={80}
                  value={value.web_secondary_benefit_label}
                  onChange={(event) => update({ web_secondary_benefit_label: event.target.value })}
                />
              </div>
              <div className="grid gap-1.5">
                <Label htmlFor="redeem-web-plus-secondary">Plus 附加权益</Label>
                <Input
                  id="redeem-web-plus-secondary"
                  maxLength={80}
                  value={value.web_plus_secondary_benefit}
                  onChange={(event) => update({ web_plus_secondary_benefit: event.target.value })}
                />
              </div>
              <div className="grid gap-1.5">
                <Label htmlFor="redeem-web-team-secondary">Team 附加权益</Label>
                <Input
                  id="redeem-web-team-secondary"
                  maxLength={80}
                  value={value.web_team_secondary_benefit}
                  onChange={(event) => update({ web_team_secondary_benefit: event.target.value })}
                />
              </div>
            </div>
          </section>
        </div>
      </Card>
    </div>
  )
}

function NotificationConfigEditor({
  settings,
  value,
  onChange,
  enabledChannels,
  onToggleChannel,
}: {
  settings: Settings
  value: NotificationConfigState
  onChange: (value: NotificationConfigState) => void
  enabledChannels: Set<string>
  onToggleChannel: (key: string, checked: boolean) => void
}) {
  const { t } = useTranslation()

  const updateSMTP = (patch: Partial<SMTPConfigState>) => {
    onChange({ ...value, smtp: { ...value.smtp, ...patch } })
  }

  return (
    <div className="grid gap-4 animate-fade-up" style={{ animationDelay: "60ms" }}>
      <div className="flex items-center gap-2 px-1">
        <span className="grid size-8 place-items-center rounded-md bg-brand/10 text-brand">
          <ShieldCheck className="size-4" />
        </span>
        <h2 className="panel-heading text-sm font-semibold">{t("settings.deliveryConfig")}</h2>
      </div>

      <Card className="gap-5 p-5 sm:p-6">
        <div className="flex items-center justify-between gap-3 border-b pb-4">
          <div className="flex items-center gap-3">
            <span className="grid size-9 shrink-0 place-items-center rounded-md bg-brand/10 text-brand">
              <Mail className="size-4" />
            </span>
            <h3 className="text-sm font-semibold">{t("settings.smtpConfig")}</h3>
          </div>
          <Badge variant={settings.notification_config.smtp.password_set ? "success" : "secondary"}>
            {settings.notification_config.smtp.password_set
              ? t("settings.configured")
              : t("settings.notConfigured")}
          </Badge>
        </div>
        <div className="grid gap-4 md:grid-cols-2">
          <div className="grid gap-1.5">
            <Label htmlFor="smtp-host">{t("settings.smtpHost")}</Label>
            <Input
              id="smtp-host"
              value={value.smtp.host}
              placeholder="smtp.qq.com"
              onChange={(event) => updateSMTP({ host: event.target.value })}
            />
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="smtp-port">{t("settings.smtpPort")}</Label>
            <Input
              id="smtp-port"
              inputMode="numeric"
              value={value.smtp.port}
              placeholder="465"
              onChange={(event) => updateSMTP({ port: event.target.value })}
            />
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="smtp-username">{t("settings.smtpUsername")}</Label>
            <Input
              id="smtp-username"
              value={value.smtp.username}
              autoComplete="off"
              onChange={(event) => updateSMTP({ username: event.target.value })}
            />
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="smtp-password">{t("settings.smtpPassword")}</Label>
            <Input
              id="smtp-password"
              type="password"
              value={value.smtp.password}
              autoComplete="new-password"
              placeholder={settings.notification_config.smtp.password_set ? "••••••••" : ""}
              onChange={(event) => updateSMTP({ password: event.target.value })}
            />
            <SecretHint configured={settings.notification_config.smtp.password_set} />
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="smtp-from">{t("settings.smtpFrom")}</Label>
            <Input
              id="smtp-from"
              type="email"
              value={value.smtp.from}
              onChange={(event) => updateSMTP({ from: event.target.value })}
            />
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="smtp-to">{t("settings.smtpTo")}</Label>
            <Input
              id="smtp-to"
              value={value.smtp.to}
              placeholder={t("settings.smtpToPlaceholder")}
              onChange={(event) => updateSMTP({ to: event.target.value })}
            />
          </div>
        </div>
      </Card>

      <div className="grid items-stretch gap-4 lg:grid-cols-2">
        <Card className="content-start gap-5 p-5 sm:p-6">
          <div className="flex items-center justify-between gap-3 border-b pb-4">
            <div className="flex items-center gap-3">
              <span className="grid size-9 shrink-0 place-items-center rounded-md bg-brand/10 text-brand">
                <Send className="size-4" />
              </span>
              <h3 className="text-sm font-semibold">IYUU</h3>
            </div>
            <Badge variant={settings.notification_config.iyuu.token_set ? "success" : "secondary"}>
              {settings.notification_config.iyuu.token_set
                ? t("settings.configured")
                : t("settings.notConfigured")}
            </Badge>
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="iyuu-token">{t("settings.iyuuToken")}</Label>
            <Input
              id="iyuu-token"
              type="password"
              value={value.iyuuToken}
              autoComplete="new-password"
              placeholder={settings.notification_config.iyuu.token_set ? "••••••••" : ""}
              onChange={(event) => onChange({ ...value, iyuuToken: event.target.value })}
            />
            <SecretHint configured={settings.notification_config.iyuu.token_set} />
          </div>
        </Card>

        <Card className="content-start gap-5 p-5 sm:p-6">
          <div className="flex items-center justify-between gap-3 border-b pb-4">
            <div className="flex items-center gap-3">
              <span className="grid size-9 shrink-0 place-items-center rounded-md bg-brand/10 text-brand">
                <BellRing className="size-4" />
              </span>
              <h3 className="text-sm font-semibold">Gotify</h3>
            </div>
            <Badge variant={settings.notification_config.gotify.token_set ? "success" : "secondary"}>
              {settings.notification_config.gotify.token_set
                ? t("settings.configured")
                : t("settings.notConfigured")}
            </Badge>
          </div>
          <div className="grid gap-4">
            <div className="grid gap-1.5">
              <Label htmlFor="gotify-url">{t("settings.gotifyURL")}</Label>
              <Input
                id="gotify-url"
                value={value.gotifyURL}
                placeholder="https://gotify.example.com"
                onChange={(event) => onChange({ ...value, gotifyURL: event.target.value })}
              />
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="gotify-token">{t("settings.gotifyToken")}</Label>
              <Input
                id="gotify-token"
                type="password"
                value={value.gotifyToken}
                autoComplete="new-password"
                placeholder={settings.notification_config.gotify.token_set ? "••••••••" : ""}
                onChange={(event) => onChange({ ...value, gotifyToken: event.target.value })}
              />
              <SecretHint configured={settings.notification_config.gotify.token_set} />
            </div>
          </div>
        </Card>
      </div>

      <Card className="gap-3 p-5 sm:p-6">
        <Label>{t("settings.channels")}</Label>
        <div className="flex flex-wrap gap-2">
          {settings.channels.map((channel) => {
            const fixedRoute = isFixedRouteChannel(channel)
            const checked = isChannelChecked(channel, enabledChannels)
            return (
              <label
                key={channel.key}
                className={cn(
                  "flex items-center gap-2 rounded-lg border px-3 py-2 text-sm transition-colors select-none",
                  fixedRoute ? "cursor-default" : "cursor-pointer",
                  checked ? "border-brand/40 bg-brand/8" : "hover:bg-accent",
                )}
              >
                <Checkbox
                  checked={checked}
                  disabled={fixedRoute}
                  className="disabled:opacity-100"
                  onCheckedChange={(checkedValue) =>
                    onToggleChannel(channel.key, checkedValue === true)
                  }
                />
                {channelLabel(channel, t)}
                <ChannelBadge channel={channel} />
              </label>
            )
          })}
        </div>
      </Card>
    </div>
  )
}

function sandboxRedeemPath(status: SandboxStatus | undefined) {
  if (!status?.ready || !status.access_token) return ""
  const code = status.redemption_codes?.[0] ?? ""
  return `/redeem?sandbox=${encodeURIComponent(status.access_token)}${
    code ? `&code=${encodeURIComponent(code)}` : ""
  }`
}

function SandboxToolCard() {
  const sandboxMode = useSandboxMode()
  const [resetConfirmOpen, setResetConfirmOpen] = React.useState(false)
  const statusQuery = useQuery({
    queryKey: ["sandbox-status"],
    queryFn: fetchSandboxStatus,
    retry: false,
  })
  const resetMutation = useMutation({
    mutationFn: resetSandbox,
    onSuccess: (result) => {
      statusQuery.refetch()
      setResetConfirmOpen(false)
      if (sandboxMode.enabled) enterSandboxMode(result.sandbox)
      toast.success(result.message ?? "演练环境已重置")
    },
    onError: (error: Error) => toast.error(error.message),
  })
  const status = statusQuery.data
  const ready = status?.ready === true
  const redeemPath = sandboxRedeemPath(status)
  const activeCode = status?.redemption_codes?.[0] ?? ""
  const scenarios = [
    ["兑换申请", "使用测试兑换码提交，再到兑换申请中分配沙盒主号"],
    ["提前退订", "在用户管理退订演练客户，再到售后处理完成退款"],
    ["账号封禁", "封禁沙盒封禁主号，检查批量生成的售后事项"],
    ["退款或换空间", "售后事项可选择退款，或转移到沙盒备用主号"],
  ]

  const enterSandbox = () => {
    if (!status?.ready) {
      toast.error("请先创建演练环境")
      return
    }
    enterSandboxMode(status)
    window.location.assign("/")
  }

  return (
    <Card className="overflow-hidden p-0 lg:col-span-2">
      <div className="flex flex-col gap-4 border-b bg-amber-500/[0.045] px-5 py-5 sm:flex-row sm:items-center sm:justify-between sm:px-6">
        <div className="flex items-start gap-3">
          <span className="grid size-10 shrink-0 place-items-center rounded-md bg-amber-500/12 text-amber-700 dark:text-amber-300">
            <FlaskConical className="size-5" />
          </span>
          <div>
            <div className="flex flex-wrap items-center gap-2">
              <h2 className="text-base font-semibold">业务演练沙盒</h2>
              <Badge variant={sandboxMode.enabled ? "default" : ready ? "secondary" : "outline"}>
                {sandboxMode.enabled ? "演练中" : ready ? "已准备" : "未创建"}
              </Badge>
            </div>
            <p className="mt-1 max-w-3xl text-xs leading-5 text-muted-foreground">
              使用独立测试数据库完整体验兑换、退订、封禁与售后流程。沙盒数据不会进入正式仪表盘、账单和利润统计，也不会发送真实通知。
            </p>
          </div>
        </div>
        <div className="flex flex-wrap gap-2 sm:justify-end">
          {ready ? (
            <Button type="button" variant="outline" onClick={() => setResetConfirmOpen(true)}>
              <Database data-slot="icon" />
              重置演练数据
            </Button>
          ) : (
            <Button type="button" onClick={() => setResetConfirmOpen(true)}>
              <FlaskConical data-slot="icon" />
              创建演练环境
            </Button>
          )}
          {sandboxMode.enabled ? (
            <Button
              type="button"
              variant="outline"
              onClick={() => {
                exitSandboxMode()
                window.location.assign("/")
              }}
            >
              退出演练
            </Button>
          ) : (
            <Button type="button" disabled={!ready} onClick={enterSandbox}>
              <FlaskConical data-slot="icon" />
              进入演练模式
            </Button>
          )}
        </div>
      </div>

      <div className="grid gap-6 px-5 py-5 sm:px-6 lg:grid-cols-[minmax(0,1.35fr)_minmax(280px,0.65fr)]">
        <div>
          <p className="text-xs font-semibold text-muted-foreground">可演练流程</p>
          <ul className="mt-3 grid gap-x-6 gap-y-4 sm:grid-cols-2">
            {scenarios.map(([title, description]) => (
              <li key={title} className="flex items-start gap-3">
                <CircleCheckBig className="mt-0.5 size-4 shrink-0 text-success" />
                <div>
                  <p className="text-sm font-medium">{title}</p>
                  <p className="mt-0.5 text-xs leading-5 text-muted-foreground">{description}</p>
                </div>
              </li>
            ))}
          </ul>
        </div>

        <div className="border-t pt-5 lg:border-l lg:border-t-0 lg:pl-6 lg:pt-0">
          <p className="text-xs font-semibold text-muted-foreground">测试兑换入口</p>
          {statusQuery.isPending ? (
            <div className="mt-3 grid gap-3">
              <Skeleton className="h-10 w-full" />
              <Skeleton className="h-9 w-full" />
            </div>
          ) : ready ? (
            <div className="mt-3 grid gap-3">
              <div className="flex items-center gap-2 rounded-md border bg-muted/35 px-3 py-2.5">
                <div className="min-w-0 flex-1">
                  <p className="text-[11px] text-muted-foreground">测试兑换码</p>
                  <p className="truncate font-mono text-sm font-semibold">{activeCode || "请重置生成新兑换码"}</p>
                </div>
                {activeCode ? (
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon-sm"
                    aria-label="复制测试兑换码"
                    onClick={() => {
                      void navigator.clipboard.writeText(activeCode)
                      toast.success("测试兑换码已复制")
                    }}
                  >
                    <Copy className="size-4" />
                  </Button>
                ) : null}
              </div>
              <Button variant="outline" disabled={!redeemPath} asChild={Boolean(redeemPath)}>
                {redeemPath ? (
                  <a href={redeemPath} target="_blank" rel="noreferrer">
                    <ExternalLink data-slot="icon" />
                    打开测试兑换页
                  </a>
                ) : (
                  <span>暂无可用兑换码</span>
                )}
              </Button>
            </div>
          ) : (
            <p className="mt-3 text-xs leading-5 text-muted-foreground">
              创建演练环境后，这里会显示专用兑换码和测试页面入口。
            </p>
          )}
        </div>
      </div>

      <ConfirmDialog
        open={resetConfirmOpen}
        onOpenChange={setResetConfirmOpen}
        title={ready ? "重置演练环境" : "创建演练环境"}
        description="此操作只会清空并重新生成沙盒数据，正式账号、订阅、账单和统计不会受到影响。"
        actionLabel={ready ? "确认重置" : "确认创建"}
        pending={resetMutation.isPending}
        onConfirm={() => resetMutation.mutate()}
      />
    </Card>
  )
}

function CustomerEmailTestCard({ settings }: { settings: Settings }) {
  const [recipient, setRecipient] = React.useState("")
  const [templateKind, setTemplateKind] = React.useState<CustomerTemplateKind>("customer")
  const smtp = settings.notification_config.smtp
  const smtpConfigured =
    smtp.host.trim() !== "" &&
    smtp.port > 0 &&
    smtp.username.trim() !== "" &&
    smtp.from.trim() !== "" &&
    smtp.password_set
  const testMutation = useAppMutation(
    (input: { recipient: string; template_kind: CustomerTemplateKind }) =>
      testSettingsCustomerEmail(input),
    { successToast: true },
  )

  const handleSend = () => {
    const normalizedRecipient = recipient.trim()
    if (!EMAIL_PATTERN.test(normalizedRecipient) || normalizedRecipient.length > 254) {
      toast.error("请填写有效的测试邮箱")
      return
    }
    testMutation.mutate({ recipient: normalizedRecipient, template_kind: templateKind })
  }

  return (
    <Card className="gap-5 p-5 sm:p-6 lg:col-span-2">
      <div className="flex flex-wrap items-start justify-between gap-3 border-b pb-4">
        <div className="flex items-start gap-3">
          <span className="grid size-10 shrink-0 place-items-center rounded-md bg-brand/10 text-brand">
            <Mail className="size-5" />
          </span>
          <div>
            <div className="flex flex-wrap items-center gap-2">
              <h2 className="text-base font-semibold">客户邮件送达测试</h2>
              <Badge variant={smtpConfigured ? "success" : "secondary"}>
                {smtpConfigured ? "SMTP 已就绪" : "SMTP 未配置"}
              </Badge>
            </div>
            <p className="mt-1 max-w-3xl text-xs leading-5 text-muted-foreground">
              使用当前已保存的正式模板和 SMTP 通道发送，可在目标邮箱确认邮件进入收件箱还是垃圾邮件。测试内容使用虚拟客户数据。
            </p>
          </div>
        </div>
      </div>

      <div className="grid items-end gap-4 md:grid-cols-[minmax(0,1.4fr)_minmax(13rem,0.8fr)_auto]">
        <div className="grid gap-1.5">
          <Label htmlFor="customer-email-test-recipient">测试收件邮箱</Label>
          <Input
            id="customer-email-test-recipient"
            type="email"
            inputMode="email"
            autoComplete="email"
            maxLength={254}
            value={recipient}
            placeholder="name@example.com"
            onChange={(event) => setRecipient(event.target.value)}
          />
        </div>
        <div className="grid gap-1.5">
          <Label htmlFor="customer-email-test-template">邮件模板</Label>
          <Select
            value={templateKind}
            onValueChange={(value) => setTemplateKind(value as CustomerTemplateKind)}
          >
            <SelectTrigger id="customer-email-test-template" className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="customer">正常续费模板</SelectItem>
              <SelectItem value="customer_price_increase">调价后邮件模板</SelectItem>
              <SelectItem value="customer_price_decrease">降价优惠模板</SelectItem>
            </SelectContent>
          </Select>
        </div>
        <Button
          type="button"
          className="w-full md:w-auto"
          disabled={!smtpConfigured || testMutation.isPending}
          onClick={handleSend}
        >
          <Send data-slot="icon" />
          {testMutation.isPending ? "正在发送" : "发送测试邮件"}
        </Button>
      </div>
      {!smtpConfigured ? (
        <p className="text-xs text-amber-700 dark:text-amber-300">
          请先在“通知渠道”中完整填写并保存 SMTP 配置。
        </p>
      ) : null}
    </Card>
  )
}

function SystemTools({ settings }: { settings: Settings }) {
  const { t } = useTranslation()
  const [testConfirmOpen, setTestConfirmOpen] = React.useState(false)
  const testMutation = useAppMutation(() => testSettingsNotify(), {
    onSuccess: () => setTestConfirmOpen(false),
  })

  return (
    <div className="grid gap-4 lg:grid-cols-2">
      <SandboxToolCard />

      <CustomerEmailTestCard settings={settings} />

      <Card className="relative flex-col gap-4 overflow-hidden p-6 sm:flex-row sm:items-center sm:justify-between">
        <div className="flex items-start gap-3">
          <span className="grid size-9 shrink-0 place-items-center rounded-md bg-brand/10 text-brand">
            <Send className="size-4" />
          </span>
          <div>
            <h2 className="text-sm font-semibold">{t("settings.testTitle")}</h2>
          </div>
        </div>
        <Button
          type="button"
          variant="outline"
          className="w-full shrink-0 sm:w-auto"
          onClick={() => setTestConfirmOpen(true)}
        >
          <Send data-slot="icon" />
          {t("settings.testAction")}
        </Button>
      </Card>

      <Card className="relative flex-col gap-4 overflow-hidden p-6 sm:flex-row sm:items-center sm:justify-between">
        <div className="flex items-start gap-3">
          <span className="grid size-9 shrink-0 place-items-center rounded-md bg-brand/10 text-brand">
            <Download className="size-4" />
          </span>
          <div>
            <h2 className="text-sm font-semibold">{t("settings.exportTitle")}</h2>
          </div>
        </div>
        <Button variant="outline" className="w-full shrink-0 sm:w-auto" asChild>
          <a href="/export">
            <Download data-slot="icon" />
            {t("settings.exportAction")}
          </a>
        </Button>
      </Card>

      <ConfirmDialog
        open={testConfirmOpen}
        onOpenChange={setTestConfirmOpen}
        title={t("confirms.testNotifyTitle")}
        description={t("confirms.testNotifyDesc")}
        actionLabel={t("confirms.testNotifyAction")}
        pending={testMutation.isPending}
        onConfirm={() => testMutation.mutate(undefined)}
      />
    </div>
  )
}

function SeatFreezeSettingsEditor({
  value,
  onChange,
}: {
  value: string
  onChange: (value: string) => void
}) {
  const { t } = useTranslation()

  return (
    <Card className="gap-5 p-6">
      <div className="flex items-start gap-3">
        <span className="grid size-9 shrink-0 place-items-center rounded-md bg-brand/10 text-brand">
          <Snowflake className="size-4" />
        </span>
        <div>
          <h2 className="text-sm font-semibold">{t("settings.seatFreezeTitle")}</h2>
          <p className="mt-1 max-w-2xl text-xs leading-5 text-muted-foreground">
            {t("settings.seatFreezeDescription")}
          </p>
        </div>
      </div>

      <div className="grid gap-2 sm:max-w-xs">
        <Label htmlFor="seat-freeze-days">{t("settings.seatFreezeDays")}</Label>
        <div className="relative">
          <Input
            id="seat-freeze-days"
            type="number"
            min={1}
            max={90}
            step={1}
            inputMode="numeric"
            value={value}
            onChange={(event) => onChange(event.target.value)}
            className="pr-12 tabular-nums"
          />
          <span className="pointer-events-none absolute top-1/2 right-3 -translate-y-1/2 text-xs text-muted-foreground">
            {t("settings.daysUnit")}
          </span>
        </div>
        <p className="text-xs text-muted-foreground">{t("settings.seatFreezeHint")}</p>
      </div>
    </Card>
  )
}

function SettingsForm({ settings }: { settings: Settings }) {
  const { t } = useTranslation()
  const [activeSection, setActiveSection] = React.useState<SettingsSection>("templates")

  const [notifyDraft, setNotifyDraft] = React.useState(() =>
    draftFromTemplate("notify", settings.notify_template),
  )
  const [customerDraft, setCustomerDraft] = React.useState(() =>
    draftFromTemplate("customer", settings.customer_email_template),
  )
  const [priceIncreaseCustomerDraft, setPriceIncreaseCustomerDraft] = React.useState(() =>
    draftFromTemplate(
      "customer_price_increase",
      settings.price_increase_customer_email_template,
    ),
  )
  const [priceDecreaseCustomerDraft, setPriceDecreaseCustomerDraft] = React.useState(() =>
    draftFromTemplate(
      "customer_price_decrease",
      settings.price_decrease_customer_email_template ?? "",
    ),
  )
  const [customerTemplateKind, setCustomerTemplateKind] =
    React.useState<CustomerTemplateKind>("customer")
  const [enabledChannels, setEnabledChannels] = React.useState<Set<string>>(
    () => new Set(settings.enabled_channels ?? []),
  )
  const [deliveryConfig, setDeliveryConfig] = React.useState(() =>
    initialNotificationConfigState(settings.notification_config),
  )
  const [redeemPage, setRedeemPage] = React.useState(() =>
    normalizeRedeemPageSettings(settings.redeem_page),
  )
  const [seatFreezeDays, setSeatFreezeDays] = React.useState(() =>
    String(settings.seat_freeze_days || 7),
  )
  const [previewKind, setPreviewKind] = React.useState<TemplateKind | null>(null)
  const [preview, setPreview] = React.useState<TemplatePreviewState>({ status: "loading" })

  const notifyTemplate = React.useMemo(
    () => renderVisualTemplate("notify", notifyDraft),
    [notifyDraft],
  )
  const customerTemplate = React.useMemo(
    () => renderVisualTemplate("customer", customerDraft),
    [customerDraft],
  )
  const priceIncreaseCustomerTemplate = React.useMemo(
    () => renderVisualTemplate("customer_price_increase", priceIncreaseCustomerDraft),
    [priceIncreaseCustomerDraft],
  )
  const priceDecreaseCustomerTemplate = React.useMemo(
    () => renderVisualTemplate("customer_price_decrease", priceDecreaseCustomerDraft),
    [priceDecreaseCustomerDraft],
  )

  const saveMutation = useAppMutation((input: SettingsInput) => saveSettings(input), {
    onSuccess: () =>
      setDeliveryConfig((previous) => ({
        ...previous,
        smtp: { ...previous.smtp, password: "" },
        iyuuToken: "",
        gotifyToken: "",
      })),
  })

  const openPreview = (kind: TemplateKind) => {
    setPreviewKind(kind)
    setPreview({ status: "loading" })
    const template =
      kind === "notify"
        ? notifyTemplate
        : kind === "customer_price_increase"
          ? priceIncreaseCustomerTemplate
          : kind === "customer_price_decrease"
            ? priceDecreaseCustomerTemplate
            : customerTemplate
    previewSettingsTemplate(kind, template)
      .then((result) =>
        setPreview({
          status: "ok",
          rendered: result.rendered,
          sampleName: result.sample_name,
          subject: result.subject,
          html: result.html,
        }),
      )
      .catch((error: Error) => setPreview({ status: "error", message: error.message }))
  }

  const toggleChannel = (key: string, checked: boolean) => {
    setEnabledChannels((previous) => {
      const next = new Set(previous)
      if (checked) {
        next.add(key)
      } else {
        next.delete(key)
      }
      return next
    })
  }

  const handleSave = (event: React.FormEvent) => {
    event.preventDefault()
    if (
      notifyTemplate.trim() === "" ||
      customerTemplate.trim() === "" ||
      priceIncreaseCustomerTemplate.trim() === "" ||
      priceDecreaseCustomerTemplate.trim() === ""
    ) {
      toast.error(t("settings.validation.templateRequired"))
      return
    }

    if (
      !Number.isInteger(redeemPage.codex_plus_weekly_quota_usd) ||
      !Number.isInteger(redeemPage.codex_team_weekly_quota_usd) ||
      redeemPage.codex_plus_weekly_quota_usd < 1 ||
      redeemPage.codex_team_weekly_quota_usd < 1 ||
      redeemPage.codex_plus_weekly_quota_usd > 100000 ||
      redeemPage.codex_team_weekly_quota_usd > 100000
    ) {
      toast.error("Codex 周额度必须是 1～100000 的整数")
      return
    }

    const smtpPort = Number(deliveryConfig.smtp.port)
    if (!Number.isInteger(smtpPort) || smtpPort < 1 || smtpPort > 65535) {
      toast.error(t("settings.validation.smtpPortInvalid"))
      return
    }

    const parsedSeatFreezeDays = Number(seatFreezeDays)
    if (
      !Number.isInteger(parsedSeatFreezeDays) ||
      parsedSeatFreezeDays < 1 ||
      parsedSeatFreezeDays > 90
    ) {
      toast.error(t("settings.validation.seatFreezeDaysInvalid"))
      return
    }

    saveMutation.mutate({
      notify_template: notifyTemplate,
      customer_email_template: customerTemplate,
      price_increase_customer_email_template: priceIncreaseCustomerTemplate,
      price_decrease_customer_email_template: priceDecreaseCustomerTemplate,
      channels: Array.from(enabledChannels),
      redeem_page: normalizeRedeemPageSettings(redeemPage),
      seat_freeze_days: parsedSeatFreezeDays,
      notification_config: {
        smtp: {
          host: deliveryConfig.smtp.host,
          port: smtpPort,
          username: deliveryConfig.smtp.username,
          password: deliveryConfig.smtp.password,
          from: deliveryConfig.smtp.from,
          to: deliveryConfig.smtp.to,
        },
        iyuu: {
          token: deliveryConfig.iyuuToken,
        },
        gotify: {
          url: deliveryConfig.gotifyURL,
          token: deliveryConfig.gotifyToken,
        },
      },
    })
  }

  return (
    <form onSubmit={handleSave} className="grid gap-4">
      <Tabs
        value={activeSection}
        onValueChange={(value) => setActiveSection(value as SettingsSection)}
        className="gap-5"
      >
        <div className="sticky top-[72px] z-20 rounded-lg border bg-card/95 p-1 shadow-card backdrop-blur-md">
          <TabsList className="grid h-auto w-full grid-cols-2 gap-1 border-0 bg-transparent p-0 sm:grid-cols-3 lg:grid-cols-5">
            <TabsTrigger
              value="templates"
              className="h-11 justify-center px-3 text-sm data-[state=active]:border-brand/20 data-[state=active]:bg-brand/[0.09] data-[state=active]:text-brand data-[state=active]:shadow-none"
            >
              <MessageSquare />
              {t("settings.sections.templates")}
            </TabsTrigger>
            <TabsTrigger
              value="delivery"
              className="h-11 justify-center px-3 text-sm data-[state=active]:border-brand/20 data-[state=active]:bg-brand/[0.09] data-[state=active]:text-brand data-[state=active]:shadow-none"
            >
              <BellRing />
              {t("settings.sections.delivery")}
            </TabsTrigger>
            <TabsTrigger
              value="redemption"
              className="h-11 justify-center px-3 text-sm data-[state=active]:border-brand/20 data-[state=active]:bg-brand/[0.09] data-[state=active]:text-brand data-[state=active]:shadow-none"
            >
              <Megaphone />
              {t("settings.sections.redemption")}
            </TabsTrigger>
            <TabsTrigger
              value="operations"
              className="h-11 justify-center px-3 text-sm data-[state=active]:border-brand/20 data-[state=active]:bg-brand/[0.09] data-[state=active]:text-brand data-[state=active]:shadow-none"
            >
              <Snowflake />
              {t("settings.sections.operations")}
            </TabsTrigger>
            <TabsTrigger
              value="tools"
              className="h-11 justify-center px-3 text-sm data-[state=active]:border-brand/20 data-[state=active]:bg-brand/[0.09] data-[state=active]:text-brand data-[state=active]:shadow-none"
            >
              <Download />
              {t("settings.sections.tools")}
            </TabsTrigger>
          </TabsList>
        </div>

        <TabsContent value="templates" className="mt-0 grid gap-4 animate-fade-in">
          <div className="flex items-center gap-2 px-1">
            <span className="grid size-8 place-items-center rounded-md bg-brand/10 text-brand">
              <MessageSquare className="size-4" />
            </span>
            <h2 className="panel-heading text-sm font-semibold">{t("settings.templateBuilder")}</h2>
          </div>
          <div className="grid gap-4 lg:grid-cols-2">
            <TemplateEditor
              kind="notify"
              draft={notifyDraft}
              onDraftChange={setNotifyDraft}
              onPreview={() => openPreview("notify")}
            />
            <TemplateEditor
              kind={customerTemplateKind}
              draft={
                customerTemplateKind === "customer"
                  ? customerDraft
                  : customerTemplateKind === "customer_price_increase"
                    ? priceIncreaseCustomerDraft
                    : priceDecreaseCustomerDraft
              }
              onDraftChange={
                customerTemplateKind === "customer"
                  ? setCustomerDraft
                  : customerTemplateKind === "customer_price_increase"
                    ? setPriceIncreaseCustomerDraft
                    : setPriceDecreaseCustomerDraft
              }
              onPreview={() => openPreview(customerTemplateKind)}
              customerTemplateKind={customerTemplateKind}
              onCustomerTemplateKindChange={setCustomerTemplateKind}
            />
          </div>
        </TabsContent>

        <TabsContent value="delivery" className="mt-0 animate-fade-in">
          <NotificationConfigEditor
            settings={settings}
            value={deliveryConfig}
            onChange={setDeliveryConfig}
            enabledChannels={enabledChannels}
            onToggleChannel={toggleChannel}
          />
        </TabsContent>

        <TabsContent value="redemption" className="mt-0 animate-fade-in">
          <RedeemPageSettingsEditor value={redeemPage} onChange={setRedeemPage} />
        </TabsContent>

        <TabsContent value="operations" className="mt-0 animate-fade-in">
          <SeatFreezeSettingsEditor value={seatFreezeDays} onChange={setSeatFreezeDays} />
        </TabsContent>

        <TabsContent value="tools" className="mt-0 animate-fade-in">
          <SystemTools settings={settings} />
        </TabsContent>
      </Tabs>

      {activeSection !== "tools" ? (
        <div className="sticky bottom-3 z-20 flex items-center justify-between gap-4 rounded-lg border border-brand/15 bg-card/95 p-3 shadow-lift backdrop-blur-md">
          <p className="hidden text-xs text-muted-foreground sm:block">{t("settings.saveHint")}</p>
          <Button type="submit" className="w-full sm:w-auto" disabled={saveMutation.isPending}>
            {saveMutation.isPending ? t("common.saving") : t("settings.saveSettings")}
          </Button>
        </div>
      ) : null}

      <Dialog
        open={previewKind !== null}
        onOpenChange={(open) => {
          if (!open) setPreviewKind(null)
        }}
      >
        <DialogContent className="gap-3 sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle>{t("settings.previewTitle")}</DialogTitle>
            <DialogDescription>
              {preview.status === "ok"
                ? t("settings.previewSample", { name: preview.sampleName })
                : t("settings.previewDesc")}
            </DialogDescription>
          </DialogHeader>

          {preview.status === "loading" ? (
            <div className="flex h-32 items-center justify-center text-sm text-muted-foreground">
              {t("common.loading")}
            </div>
          ) : preview.status === "error" ? (
            <p className="text-sm leading-relaxed text-destructive">{preview.message}</p>
          ) : (
            <div className="grid gap-3">
              {previewKind !== "notify" ? (
                <div className="flex items-start gap-3 rounded-xl border bg-muted/30 px-3.5 py-3">
                  <span className="grid size-8 shrink-0 place-items-center rounded-lg bg-brand/10 text-brand">
                    <Mail className="size-4" />
                  </span>
                  <div className="min-w-0">
                    <div className="text-[11px] font-medium text-muted-foreground">
                      {t("settings.previewSubject")}
                    </div>
                    <div className="mt-0.5 truncate text-[13px] font-semibold">
                      {preview.subject}
                    </div>
                  </div>
                </div>
              ) : null}
              <EmailPreview
                html={previewKind === "notify" ? "" : preview.html}
                plainText={preview.rendered}
                title={t("settings.previewTitle")}
              />
            </div>
          )}

          <DialogFooter>
            <Button type="button" onClick={() => setPreviewKind(null)}>
              {t("common.close")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </form>
  )
}

export function SettingsPage() {
  const { t } = useTranslation()
  const settingsQuery = useSettings()

  const settings = settingsQuery.data

  return (
    <div className="mx-auto max-w-6xl">
      <PageHeader title={t("settings.title")} />

      {settingsQuery.isPending ? (
        <div className="grid gap-4">
          <Skeleton className="h-[560px] rounded-xl" />
          <Skeleton className="h-28 rounded-xl" />
          <Skeleton className="h-28 rounded-xl" />
        </div>
      ) : settingsQuery.isError ? (
        <Card className="items-center gap-3 py-16 text-center">
          <p className="text-sm text-muted-foreground">{t("common.loadFailed")}</p>
          <Button variant="outline" onClick={() => settingsQuery.refetch()}>
            {t("common.retry")}
          </Button>
        </Card>
      ) : settings ? (
        <SettingsForm settings={settings} />
      ) : null}
    </div>
  )
}
