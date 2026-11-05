import * as React from "react"
import { useTranslation } from "react-i18next"

import { ConfirmDialog } from "@/components/confirm-dialog"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Textarea } from "@/components/ui/textarea"
import {
  BENEFIT_TEMPLATE_KINDS,
  BUSINESS_TEMPLATE_LABELS,
  businessTemplateFields,
  defaultBusinessTemplate,
  parseBusinessTemplate,
  updateBusinessTemplate,
  type BenefitTemplateKind,
  type BusinessTemplateDraft,
} from "./business-email-template"

export function BusinessEmailTemplateEditor({
  benefit = false,
  value,
  onChange,
  previewKind,
  onPreviewKindChange,
}: {
  benefit?: boolean
  value: string
  onChange: (value: string) => void
  previewKind: BenefitTemplateKind
  onPreviewKindChange: (kind: BenefitTemplateKind) => void
}) {
  const { t } = useTranslation()
  const [advanced, setAdvanced] = React.useState(false)
  const [confirmReplace, setConfirmReplace] = React.useState(false)
  const kind = benefit ? previewKind : "customer_renewal_success"
  const parsed = React.useMemo(() => parseBusinessTemplate(value, kind), [value, kind])
  // Keep the draft while typing: serialization trims copy, but inputs must retain spaces and newlines.
  const [editing, setEditing] = React.useState<{ source: string; base: string; kind: string; draft: BusinessTemplateDraft } | null>(null)
  const draft = editing?.source === value && editing.kind === kind ? editing.draft : parsed
  const inputId = benefit ? "customer-benefit-template" : "renewal-success-template"
  const updateDraft = (next: BusinessTemplateDraft) => {
    const base = editing?.source === value && editing.kind === kind ? editing.base : value
    const source = updateBusinessTemplate(base, kind, next)
    setEditing({ source, base, kind, draft: next })
    onChange(source)
  }

  return (
    <>
      {benefit ? (
        <div className="grid gap-1.5">
          <Label htmlFor="benefit-preview-kind">{t("settings.benefitPreviewKind")}</Label>
          <Select value={previewKind} onValueChange={(next) => {
            setAdvanced(false)
            onPreviewKindChange(next as BenefitTemplateKind)
          }}>
            <SelectTrigger id="benefit-preview-kind" className="w-full"><SelectValue /></SelectTrigger>
            <SelectContent>
              {BENEFIT_TEMPLATE_KINDS.map((scenario) => (
                <SelectItem key={scenario} value={scenario}>{t(BUSINESS_TEMPLATE_LABELS[scenario])}</SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      ) : null}

      {advanced ? (
        <div className="grid gap-1.5">
          <Label htmlFor={inputId}>{t("settings.businessEditor.source")}</Label>
          <Textarea
            id={inputId}
            rows={14}
            value={value}
            spellCheck={false}
            className="h-80 field-sizing-fixed resize-y font-mono text-xs leading-6"
            onChange={(event) => onChange(event.target.value)}
          />
          <p className="text-xs leading-5 text-muted-foreground">{t("settings.businessTemplateSyntax")}</p>
        </div>
      ) : draft ? (
        <>
          <div className="grid gap-1.5">
            <Label htmlFor={`${inputId}-opening`}>{t("settings.businessEditor.opening")}</Label>
            <Textarea id={`${inputId}-opening`} rows={2} value={draft.opening}
              onChange={(event) => updateDraft({ ...draft, opening: event.target.value })} />
          </div>
          <div className="grid gap-2">
            <Label>{t("settings.templateFieldsTitle")}</Label>
            <div className="grid gap-2 sm:grid-cols-2">
              {businessTemplateFields(kind, draft).map((field) => (
                <label key={field} className="flex min-h-10 cursor-pointer items-center gap-2 rounded-md border bg-background px-3 py-2 text-sm">
                  <Checkbox checked={draft.fields.includes(field)} onCheckedChange={(checked) => updateDraft({
                    ...draft,
                    fields: checked === true ? [...draft.fields, field] : draft.fields.filter((key) => key !== field),
                  })} />
                  <span>{t(`settings.businessTemplateFieldLabels.${
                    benefit && field === "NextDueDate" ? "AdjustedDueDate"
                      : (kind === "customer_benefit_revision" || kind === "customer_benefit_revoke") && field === "ExtensionDays" ? "AdjustedExtensionDays"
                        : field
                  }`)}</span>
                </label>
              ))}
            </div>
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor={`${inputId}-closing`}>{t("settings.templateFooter")}</Label>
            <Textarea id={`${inputId}-closing`} rows={3} value={draft.closing} placeholder={t("common.optional")}
              onChange={(event) => updateDraft({ ...draft, closing: event.target.value })} />
          </div>
          {benefit || draft.postscript ? (
            <div className="grid gap-1.5">
              <Label htmlFor={`${inputId}-postscript`}>{t("settings.businessEditor.postscript")}</Label>
              <Textarea id={`${inputId}-postscript`} rows={2} value={draft.postscript} placeholder={t("common.optional")}
                onChange={(event) => updateDraft({ ...draft, postscript: event.target.value })} />
            </div>
          ) : null}
        </>
      ) : (
        <div className="grid gap-3 rounded-md border bg-muted/35 p-3">
          <p className="text-sm leading-6 text-muted-foreground">{t("settings.businessEditor.customTemplate")}</p>
          <Button type="button" variant="outline" className="justify-self-start" onClick={() => setAdvanced(true)}>
            {t("settings.businessEditor.editCustom")}
          </Button>
        </div>
      )}

      <div className="flex flex-wrap items-center gap-x-4 gap-y-2">
        {advanced ? (
          <Button type="button" variant="link" size="xs" className="h-auto px-0" onClick={() => {
            if (draft) setAdvanced(false)
            else setConfirmReplace(true)
          }}>{t("settings.businessEditor.visual")}</Button>
        ) : draft ? (
          <Button type="button" variant="link" size="xs" className="h-auto px-0 text-muted-foreground" onClick={() => setAdvanced(true)}>
            {t("settings.businessEditor.advanced")}
          </Button>
        ) : (
          <Button type="button" variant="link" size="xs" className="h-auto px-0" onClick={() => setConfirmReplace(true)}>
            {t("settings.businessEditor.visual")}
          </Button>
        )}
      </div>

      <ConfirmDialog
        open={confirmReplace}
        onOpenChange={setConfirmReplace}
        title={t("settings.businessEditor.replaceTitle")}
        description={t(benefit ? "settings.businessEditor.replaceBenefitDesc" : "settings.businessEditor.replaceDesc")}
        actionLabel={t("settings.businessEditor.replaceAction")}
        onConfirm={() => {
          onChange(defaultBusinessTemplate(benefit))
          setAdvanced(false)
          setConfirmReplace(false)
        }}
      />
    </>
  )
}
