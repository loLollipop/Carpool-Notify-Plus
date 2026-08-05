import * as React from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { Download, Eye, Send } from "lucide-react"

import { previewSettingsTemplate, saveSettings, testSettingsNotify } from "@/api/endpoints"
import { useAppMutation } from "@/api/mutations"
import { useSettings } from "@/api/queries"
import type { ChannelSetting, Settings } from "@/api/types"
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
import { Label } from "@/components/ui/label"
import { Skeleton } from "@/components/ui/skeleton"
import { Textarea } from "@/components/ui/textarea"
import { cn } from "@/lib/utils"

const TEMPLATE_VARS =
  "{{.Name}} {{.CustomerEmail}} {{.CustomerWechat}} {{.AccountName}} {{.SeatName}} {{.SubscriptionName}} {{.AmountDue}} {{.PricePerPerson}} {{.CycleDesc}} {{.NextDueDate}} {{.Remark}} {{.TradeURL}}"

function ChannelBadge({ channel }: { channel: ChannelSetting }) {
  const { t } = useTranslation()
  if (channel.key === "smtp") {
    if (channel.operator_configured) {
      return <Badge variant="success">{t("settings.configured")}</Badge>
    }
    if (channel.configured) {
      return <Badge variant="warning">{t("settings.missingRecipient")}</Badge>
    }
    return <Badge variant="secondary">{t("settings.notConfigured")}</Badge>
  }
  return channel.configured ? (
    <Badge variant="success">{t("settings.configured")}</Badge>
  ) : (
    <Badge variant="secondary">{t("settings.notConfigured")}</Badge>
  )
}

/** 旧版 UI 的渠道显示名(server 只回短名,如 "SMTP")。 */
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

type TemplatePreviewState =
  | { status: "loading" }
  | { status: "ok"; rendered: string; sampleName: string; subject: string }
  | { status: "error"; message: string }

function SettingsForm({ settings }: { settings: Settings }) {
  const { t } = useTranslation()

  // Lazily seeded from the loaded settings; local edits stay authoritative afterwards.
  const [notifyTemplate, setNotifyTemplate] = React.useState(settings.notify_template)
  const [customerTemplate, setCustomerTemplate] = React.useState(settings.customer_email_template)
  const [enabledChannels, setEnabledChannels] = React.useState<Set<string>>(
    () => new Set(settings.enabled_channels ?? []),
  )
  const [previewKind, setPreviewKind] = React.useState<"notify" | "customer" | null>(null)
  const [preview, setPreview] = React.useState<TemplatePreviewState>({ status: "loading" })

  const saveMutation = useAppMutation(
    (input: { notify_template: string; customer_email_template: string; channels: string[] }) =>
      saveSettings(input),
  )

  const openPreview = (kind: "notify" | "customer") => {
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
    saveMutation.mutate({
      notify_template: notifyTemplate,
      customer_email_template: customerTemplate,
      channels: Array.from(enabledChannels),
    })
  }

  return (
    <Card className="p-6 animate-fade-up">
      <form onSubmit={handleSave} className="grid gap-6">
        <div className="grid gap-2">
          <div className="flex items-center justify-between">
            <Label htmlFor="notify-template">{t("settings.notifyTemplate")}</Label>
            <Button
              type="button"
              variant="ghost"
              size="xs"
              className="text-muted-foreground"
              onClick={() => openPreview("notify")}
            >
              <Eye data-slot="icon" />
              {t("settings.preview")}
            </Button>
          </div>
          <p className="text-xs leading-relaxed break-all text-muted-foreground">
            {t("settings.notifyTemplateHint", { vars: TEMPLATE_VARS })}
          </p>
          <Textarea
            id="notify-template"
            rows={10}
            className="font-mono text-[13px]"
            value={notifyTemplate}
            onChange={(event) => setNotifyTemplate(event.target.value)}
          />
        </div>

        <div className="grid gap-2">
          <div className="flex items-center justify-between">
            <Label htmlFor="customer-template">{t("settings.customerTemplate")}</Label>
            <Button
              type="button"
              variant="ghost"
              size="xs"
              className="text-muted-foreground"
              onClick={() => openPreview("customer")}
            >
              <Eye data-slot="icon" />
              {t("settings.preview")}
            </Button>
          </div>
          <p className="text-xs text-muted-foreground">{t("settings.customerTemplateHint")}</p>
          <Textarea
            id="customer-template"
            rows={10}
            className="font-mono text-[13px]"
            value={customerTemplate}
            onChange={(event) => setCustomerTemplate(event.target.value)}
          />
        </div>

        <div className="grid gap-2">
          <Label>{t("settings.channels")}</Label>
          <p className="text-xs leading-relaxed text-muted-foreground">
            {t("settings.channelsHint")}
          </p>
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
                    onCheckedChange={(value) => toggleChannel(channel.key, value === true)}
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

        <div>
          <Button type="submit" disabled={saveMutation.isPending}>
            {t("settings.saveSettings")}
          </Button>
        </div>
      </form>

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
    </Card>
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
    <div className="mx-auto max-w-3xl">
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
            style={{ animationDelay: "60ms" }}
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
            style={{ animationDelay: "120ms" }}
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
