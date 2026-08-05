import * as React from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { BellRing, Download, Eye, Mail, MessageSquare, Send, ShieldCheck } from "lucide-react"

import { previewSettingsTemplate, saveSettings, testSettingsNotify } from "@/api/endpoints"
import { useAppMutation } from "@/api/mutations"
import { useSettings } from "@/api/queries"
import type { ChannelSetting, NotificationConfig, Settings, SettingsInput } from "@/api/types"
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
import { Separator } from "@/components/ui/separator"
import { Skeleton } from "@/components/ui/skeleton"
import { Textarea } from "@/components/ui/textarea"
import { cn } from "@/lib/utils"

type TemplateKind = "notify" | "customer"

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
      duePrefix: "您好，您的拼车服务",
      dueSuffix: "，请及时续费，以免影响正常使用。",
      fields: DEFAULT_FIELDS,
      footer: "如需续费或有疑问，请添加 / 联系微信：Jerrylove_Bom\n谢谢。",
    }
  }
  return {
    title: "【拼车收钱】",
    duePrefix: "到期状态：",
    dueSuffix: "",
    fields: DEFAULT_FIELDS,
    footer: "",
  }
}

function draftFromTemplate(kind: TemplateKind, templateBody: string): VisualTemplateDraft {
  const draft = defaultTemplateDraft(kind)
  const fields = TEMPLATE_FIELDS.filter((field) =>
    field.signatures.some((signature) => templateBody.includes(signature)),
  ).map((field) => field.key)

  if (fields.length > 0) {
    draft.fields = fields
  }
  if (kind === "customer" && templateBody.includes("Jerrylove_Bom")) {
    draft.footer = "如需续费或有疑问，请添加 / 联系微信：Jerrylove_Bom\n谢谢。"
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
    <div className="grid gap-4 rounded-lg border bg-muted/20 p-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-2">
          {icon}
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
    <Card className="gap-5 p-6 animate-fade-up" style={{ animationDelay: "60ms" }}>
      <div className="flex items-center gap-2">
        <ShieldCheck className="size-4" />
        <h2 className="text-sm font-semibold">{t("settings.deliveryConfig")}</h2>
      </div>

      <div className="grid gap-4">
        <div className="grid gap-3 rounded-lg border bg-muted/20 p-4">
          <div className="flex items-center justify-between gap-3">
            <Label>{t("settings.smtpConfig")}</Label>
            <Badge variant={settings.notification_config.smtp.password_set ? "success" : "secondary"}>
              {settings.notification_config.smtp.password_set
                ? t("settings.configured")
                : t("settings.notConfigured")}
            </Badge>
          </div>
          <div className="grid gap-3 md:grid-cols-2">
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
        </div>

        <div className="grid gap-3 rounded-lg border bg-muted/20 p-4 md:grid-cols-2">
          <div className="grid gap-1.5">
            <div className="flex items-center justify-between gap-3">
              <Label htmlFor="iyuu-token">{t("settings.iyuuToken")}</Label>
              <Badge variant={settings.notification_config.iyuu.token_set ? "success" : "secondary"}>
                {settings.notification_config.iyuu.token_set
                  ? t("settings.configured")
                  : t("settings.notConfigured")}
              </Badge>
            </div>
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

          <div className="grid gap-3">
            <div className="grid gap-1.5">
              <div className="flex items-center justify-between gap-3">
                <Label htmlFor="gotify-url">{t("settings.gotifyURL")}</Label>
                <Badge variant={settings.notification_config.gotify.token_set ? "success" : "secondary"}>
                  {settings.notification_config.gotify.token_set
                    ? t("settings.configured")
                    : t("settings.notConfigured")}
                </Badge>
              </div>
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
        </div>
      </div>

      <Separator />

      <div className="grid gap-2">
        <Label>{t("settings.channels")}</Label>
        <p className="text-xs leading-relaxed text-muted-foreground">{t("settings.channelsHint")}</p>
        <div className="mt-1 flex flex-wrap gap-2">
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
      </div>
    </Card>
  )
}

function SettingsForm({ settings }: { settings: Settings }) {
  const { t } = useTranslation()

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
      <Card className="gap-5 p-6 animate-fade-up">
        <div className="flex items-center gap-2">
          <BellRing className="size-4" />
          <h2 className="text-sm font-semibold">{t("settings.templateBuilder")}</h2>
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
      </Card>

      <NotificationConfigEditor
        settings={settings}
        value={deliveryConfig}
        onChange={setDeliveryConfig}
        enabledChannels={enabledChannels}
        onToggleChannel={toggleChannel}
      />

      <div>
        <Button type="submit" disabled={saveMutation.isPending}>
          {t("settings.saveSettings")}
        </Button>
      </div>

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
                  <div className="text-[11px] font-medium tracking-wide text-muted-foreground">
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
  const [testConfirmOpen, setTestConfirmOpen] = React.useState(false)

  const testMutation = useAppMutation(() => testSettingsNotify(), {
    onSuccess: () => setTestConfirmOpen(false),
  })

  const settings = settingsQuery.data

  return (
    <div className="mx-auto max-w-5xl">
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
        <div className="grid gap-4">
          <SettingsForm settings={settings} />

          <Card
            className="flex-row items-center justify-between gap-4 p-6 animate-fade-up"
            style={{ animationDelay: "120ms" }}
          >
            <div>
              <h2 className="text-sm font-semibold">{t("settings.testTitle")}</h2>
              <p className="mt-1 text-xs leading-relaxed text-muted-foreground">
                {t("settings.testHint")}
              </p>
            </div>
            <Button variant="outline" className="shrink-0" onClick={() => setTestConfirmOpen(true)}>
              <Send data-slot="icon" />
              {t("settings.testAction")}
            </Button>
          </Card>

          <Card
            className="flex-row items-center justify-between gap-4 p-6 animate-fade-up"
            style={{ animationDelay: "180ms" }}
          >
            <div>
              <h2 className="text-sm font-semibold">{t("settings.exportTitle")}</h2>
              <p className="mt-1 text-xs leading-relaxed text-muted-foreground">
                {t("settings.exportHint")}
              </p>
            </div>
            <Button variant="outline" className="shrink-0" asChild>
              <a href="/export">
                <Download data-slot="icon" />
                {t("settings.exportAction")}
              </a>
            </Button>
          </Card>
        </div>
      ) : null}

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
