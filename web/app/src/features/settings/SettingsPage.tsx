import * as React from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import {
  BellRing,
  Download,
  ExternalLink,
  Eye,
  ImageUp,
  Mail,
  Megaphone,
  MessageSquare,
  Send,
  ShieldCheck,
  Trash2,
} from "lucide-react"

import { previewSettingsTemplate, saveSettings, testSettingsNotify } from "@/api/endpoints"
import { useAppMutation } from "@/api/mutations"
import { useSettings } from "@/api/queries"
import type {
  ChannelSetting,
  NotificationConfig,
  RedeemPageSettings,
  Settings,
  SettingsInput,
} from "@/api/types"
import { ConfirmDialog } from "@/components/confirm-dialog"
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
import { Skeleton } from "@/components/ui/skeleton"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Textarea } from "@/components/ui/textarea"
import { cn } from "@/lib/utils"

type TemplateKind = "notify" | "customer"
type SettingsSection = "templates" | "delivery" | "redemption" | "tools"

type TemplateFieldKey =
  | "customerEmail"
  | "customerWechat"
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
  | { status: "ok"; rendered: string; sampleName: string; subject: string }
  | { status: "error"; message: string }

const MAX_QR_UPLOAD_BYTES = 1024 * 1024

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
  const fields = TEMPLATE_FIELDS.filter((field) =>
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
    case "amountDue":
      return "本期应收：¥{{.AmountDue}}"
    case "cycleDesc":
      return "计费周期：{{.CycleDesc}}"
    case "nextDueDate":
      return "到期日期：{{.NextDueDate}}（{{.DueInText}}）"
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
}: {
  kind: TemplateKind
  draft: VisualTemplateDraft
  onDraftChange: (draft: VisualTemplateDraft) => void
  onPreview: () => void
}) {
  const { t } = useTranslation()
  const icon =
    kind === "customer" ? <Mail className="size-4" /> : <MessageSquare className="size-4" />

  const toggleField = (key: TemplateFieldKey, checked: boolean) => {
    const nextFields = checked
      ? TEMPLATE_FIELDS.map((field) => field.key).filter(
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
          <Label>{t(kind === "customer" ? "settings.customerTemplate" : "settings.notifyTemplate")}</Label>
        </div>
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
          {TEMPLATE_FIELDS.map((field) => (
            <label
              key={field.key}
              className="flex min-h-10 cursor-pointer items-center gap-2 rounded-md border bg-background px-3 py-2 text-sm"
            >
              <Checkbox
                checked={draft.fields.includes(field.key)}
                onCheckedChange={(value) => toggleField(field.key, value === true)}
              />
              <span>{t(`settings.templateFields.${field.key}`)}</span>
            </label>
          ))}
        </div>
      </div>

      <div className="grid gap-1.5">
        <Label>{t("settings.templateFooter")}</Label>
        <Textarea
          rows={kind === "customer" ? 3 : 2}
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
            <p className="text-xs text-muted-foreground">一行一条，最多 6 条；打开或刷新兑换页会自动弹窗。</p>
          </div>
        </Card>

        <Card className="content-start gap-5 p-5 sm:p-6">
          <div className="flex items-center gap-3 border-b pb-4">
            <span className="grid size-9 shrink-0 place-items-center rounded-md bg-brand/10 text-brand">
              <MessageSquare className="size-4" />
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
        <p className="text-xs leading-relaxed text-muted-foreground">{t("settings.channelsHint")}</p>
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
        <p className="text-xs leading-relaxed text-muted-foreground">
          {t("settings.channelsFootnote")}
        </p>
      </Card>
    </div>
  )
}

function SystemTools() {
  const { t } = useTranslation()
  const [testConfirmOpen, setTestConfirmOpen] = React.useState(false)
  const testMutation = useAppMutation(() => testSettingsNotify(), {
    onSuccess: () => setTestConfirmOpen(false),
  })

  return (
    <div className="grid gap-4 lg:grid-cols-2">
      <Card className="relative flex-col gap-4 overflow-hidden p-6 sm:flex-row sm:items-center sm:justify-between">
        <div className="flex items-start gap-3">
          <span className="grid size-9 shrink-0 place-items-center rounded-md bg-brand/10 text-brand">
            <Send className="size-4" />
          </span>
          <div>
            <h2 className="text-sm font-semibold">{t("settings.testTitle")}</h2>
            <p className="mt-1 text-xs leading-relaxed text-muted-foreground">
              {t("settings.testHint")}
            </p>
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
            <p className="mt-1 text-xs leading-relaxed text-muted-foreground">
              {t("settings.exportHint")}
            </p>
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

function SettingsForm({ settings }: { settings: Settings }) {
  const { t } = useTranslation()
  const [activeSection, setActiveSection] = React.useState<SettingsSection>("templates")

  const [notifyDraft, setNotifyDraft] = React.useState(() =>
    draftFromTemplate("notify", settings.notify_template),
  )
  const [customerDraft, setCustomerDraft] = React.useState(() =>
    draftFromTemplate("customer", settings.customer_email_template),
  )
  const [enabledChannels, setEnabledChannels] = React.useState<Set<string>>(
    () => new Set(settings.enabled_channels ?? []),
  )
  const [deliveryConfig, setDeliveryConfig] = React.useState(() =>
    initialNotificationConfigState(settings.notification_config),
  )
  const [redeemPage, setRedeemPage] = React.useState(() =>
    normalizeRedeemPageSettings(settings.redeem_page),
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
    previewSettingsTemplate(kind, kind === "notify" ? notifyTemplate : customerTemplate)
      .then((result) =>
        setPreview({
          status: "ok",
          rendered: result.rendered,
          sampleName: result.sample_name,
          subject: result.subject,
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
    if (notifyTemplate.trim() === "" || customerTemplate.trim() === "") {
      toast.error(t("settings.validation.templateRequired"))
      return
    }

    const smtpPort = Number(deliveryConfig.smtp.port)
    if (!Number.isInteger(smtpPort) || smtpPort < 1 || smtpPort > 65535) {
      toast.error(t("settings.validation.smtpPortInvalid"))
      return
    }

    saveMutation.mutate({
      notify_template: notifyTemplate,
      customer_email_template: customerTemplate,
      channels: Array.from(enabledChannels),
      redeem_page: normalizeRedeemPageSettings(redeemPage),
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
        <div className="rounded-lg border bg-card p-1 shadow-[0_1px_3px_color-mix(in_oklab,var(--foreground)_6%,transparent)]">
          <TabsList className="grid h-auto w-full grid-cols-2 gap-1 border-0 bg-transparent p-0 lg:grid-cols-4">
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
              kind="customer"
              draft={customerDraft}
              onDraftChange={setCustomerDraft}
              onPreview={() => openPreview("customer")}
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

        <TabsContent value="tools" className="mt-0 animate-fade-in">
          <SystemTools />
        </TabsContent>
      </Tabs>

      {activeSection !== "tools" ? (
        <div className="flex justify-end rounded-lg border border-[var(--sidebar-border)] bg-[var(--sidebar)] p-3 shadow-sm">
          <Button type="submit" className="w-full sm:w-auto" disabled={saveMutation.isPending}>
            {t("settings.saveSettings")}
          </Button>
        </div>
      ) : null}

      <Dialog
        open={previewKind !== null}
        onOpenChange={(open) => {
          if (!open) setPreviewKind(null)
        }}
      >
        <DialogContent className="sm:max-w-lg">
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
              {previewKind === "customer" ? (
                <div>
                  <div className="text-[11px] font-medium text-muted-foreground">
                    {t("settings.previewSubject")}
                  </div>
                  <div className="mt-0.5 text-[13px] font-medium">{preview.subject}</div>
                </div>
              ) : null}
              <pre className="max-h-80 overflow-auto rounded-lg border bg-muted/40 p-3.5 text-[13px] leading-relaxed whitespace-pre-wrap">
                {preview.rendered}
              </pre>
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
      <PageHeader title={t("settings.title")} description={t("settings.desc")} />

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
