export const BENEFIT_TEMPLATE_KINDS = [
  "customer_benefit_extension",
  "customer_benefit_discount",
  "customer_benefit_revision",
  "customer_benefit_revoke",
] as const

export type BenefitTemplateKind = (typeof BENEFIT_TEMPLATE_KINDS)[number]
export type BusinessTemplateKind = "customer_renewal_success" | BenefitTemplateKind

export const BUSINESS_TEMPLATE_LABELS: Record<BusinessTemplateKind, string> = {
  customer_renewal_success: "settings.renewalSuccessTemplate",
  customer_benefit_extension: "settings.benefitExtensionPreview",
  customer_benefit_discount: "settings.benefitDiscountPreview",
  customer_benefit_revision: "settings.benefitRevisionPreview",
  customer_benefit_revoke: "settings.benefitRevokePreview",
}

const BENEFIT_TYPES: Record<BenefitTemplateKind, string> = {
  customer_benefit_extension: "extension",
  customer_benefit_discount: "price_discount",
  customer_benefit_revision: "extension_revision",
  customer_benefit_revoke: "extension_revoke",
}

const FIELD_LINES = {
  CustomerEmail: "客户邮箱：{{.CustomerEmail}}",
  SubscriptionID: "订阅编号：#{{.SubscriptionID}}",
  AmountPaid: "续费金额：¥{{.AmountPaid}}",
  CycleDesc: "计费周期：{{.CycleDesc}}",
  PeriodStart: "本期开始：{{.PeriodStart}}",
  PeriodEnd: "本期结束：{{.PeriodEnd}}",
  NextDueDate: "下次到期日期：{{.NextDueDate}}",
  BenefitType: "福利类型：{{.BenefitType}}",
  BenefitDate: "发放日期：{{.BenefitDate}}",
  ExtensionDays: "赠送天数：{{.ExtensionDays}} 天",
  PreviousExtensionDays: "原延期天数：{{.PreviousExtensionDays}} 天",
  PreviousDueDate: "原到期日期：{{.PreviousDueDate}}",
  PreviousPrice: "原每期价格：¥{{.PreviousPrice}}",
  DiscountAmount: "每期优惠：¥{{.DiscountAmount}}",
  NewPrice: "调整后每期价格：¥{{.NewPrice}}",
  EffectiveDate: "生效日期：{{.EffectiveDate}}",
  OperationTime: "操作时间：{{.OperationTime}}",
} as const

export type BusinessFieldKey = keyof typeof FIELD_LINES

export interface BusinessTemplateDraft {
  opening: string
  fields: BusinessFieldKey[]
  fieldLines: Partial<Record<BusinessFieldKey, string>>
  closing: string
  postscript: string
}

const COMMON_BENEFIT_FIELDS: BusinessFieldKey[] = ["CustomerEmail", "SubscriptionID", "BenefitDate"]
const SCENARIO_FIELDS: Record<BusinessTemplateKind, BusinessFieldKey[]> = {
  customer_renewal_success: ["CustomerEmail", "SubscriptionID", "AmountPaid", "CycleDesc", "PeriodStart", "PeriodEnd", "NextDueDate", "OperationTime"],
  customer_benefit_extension: [...COMMON_BENEFIT_FIELDS, "ExtensionDays", "PreviousDueDate", "NextDueDate", "OperationTime"],
  customer_benefit_discount: [...COMMON_BENEFIT_FIELDS, "PreviousPrice", "DiscountAmount", "NewPrice", "EffectiveDate", "OperationTime"],
  customer_benefit_revision: [...COMMON_BENEFIT_FIELDS, "PreviousExtensionDays", "ExtensionDays", "PreviousDueDate", "NextDueDate", "OperationTime"],
  customer_benefit_revoke: [...COMMON_BENEFIT_FIELDS, "PreviousExtensionDays", "ExtensionDays", "PreviousDueDate", "NextDueDate", "OperationTime"],
}

/** Expand only the supported scenario conditions; leave arbitrary Go logic untouched. */
export function businessScenarioBody(template: string, kind: BusinessTemplateKind): string | null {
  const benefitType = kind === "customer_renewal_success" ? "" : BENEFIT_TYPES[kind]
  const stack: { parent: boolean; matched: boolean; hasElse: boolean }[] = []
  let active = true
  let offset = 0
  let result = ""
  for (const match of template.matchAll(/{{([\s\S]*?)}}/g)) {
    const literal = template.slice(offset, match.index)
    if (literal.includes("{{") || literal.includes("}}")) return null
    if (active) result += literal
    const action = match[1].trim()
    const condition = /^(if|else\s+if)\s+eq\s+\.BenefitType\s+"([^"]+)"$/.exec(action)
    if (condition) {
      const matches = condition[2] === benefitType
      if (condition[1] === "if") {
        stack.push({ parent: active, matched: matches, hasElse: false })
        active = active && matches
      } else {
        const frame = stack.at(-1)
        if (!frame || frame.hasElse) return null
        active = frame.parent && !frame.matched && matches
        frame.matched ||= matches
      }
    } else if (action === "else") {
      const frame = stack.at(-1)
      if (!frame || frame.hasElse) return null
      active = frame.parent && !frame.matched
      frame.matched = true
      frame.hasElse = true
    } else if (action === "end") {
      const frame = stack.pop()
      if (!frame) return null
      active = frame.parent
    } else if (/^\.\w+$/.test(action) && Object.hasOwn(FIELD_LINES, action.slice(1))) {
      if (active) result += match[0]
    } else {
      return null
    }
    offset = match.index + match[0].length
  }
  const tail = template.slice(offset)
  if (stack.length || tail.includes("{{") || tail.includes("}}")) return null
  return result + (active ? tail : "")
}

export function parseBusinessTemplate(template: string, kind: BusinessTemplateKind): BusinessTemplateDraft | null {
  const body = businessScenarioBody(template, kind)
  if (body === null) return null
  const lines = body.replace(/\r\n/g, "\n").split("\n")
  const fields: BusinessFieldKey[] = []
  const fieldLines: BusinessTemplateDraft["fieldLines"] = {}
  const opening: string[] = []
  const closing: string[] = []
  const postscript: string[] = []
  let section: "opening" | "fields" | "closing" | "postscript" = "opening"
  for (const line of lines) {
    const tokens = [...line.matchAll(/{{\s*\.(\w+)\s*}}/g)]
    if (tokens.length) {
      // The form represents whole field lines, not inline code or repeated values.
      if (tokens.length !== 1 || line.replace(tokens[0][0], "").includes("{{")) return null
      const key = tokens[0][1] as BusinessFieldKey
      if (fields.includes(key) || section === "postscript") return null
      if (key === "OperationTime") {
        section = "postscript"
      } else {
        if (section === "closing") return null
        section = "fields"
      }
      fields.push(key)
      fieldLines[key] = line
    } else if (line.includes("{{") || line.includes("}}")) {
      return null
    } else if (section === "opening") {
      opening.push(line)
    } else if (section === "postscript") {
      postscript.push(line)
    } else {
      if (line.trim()) section = "closing"
      closing.push(line)
    }
  }
  return { opening: opening.join("\n").trim(), fields, fieldLines, closing: closing.join("\n").trim(), postscript: postscript.join("\n").trim() }
}

export function businessTemplateFields(kind: BusinessTemplateKind, draft: BusinessTemplateDraft): BusinessFieldKey[] {
  return [...new Set([...SCENARIO_FIELDS[kind], ...draft.fields])]
}

function defaultFieldLine(kind: BusinessTemplateKind, key: BusinessFieldKey): string {
  if (key === "NextDueDate" && kind !== "customer_renewal_success") return "调整后到期日期：{{.NextDueDate}}"
  if (key === "ExtensionDays" && (kind === "customer_benefit_revision" || kind === "customer_benefit_revoke")) return "调整后延期天数：{{.ExtensionDays}} 天"
  return FIELD_LINES[key]
}

export function renderBusinessDraft(draft: BusinessTemplateDraft, kind: BusinessTemplateKind): string {
  const fields = draft.fields.filter((key) => key !== "OperationTime")
    .map((key) => draft.fieldLines[key] ?? defaultFieldLine(kind, key)).join("\n")
  const sections = [draft.opening.trim(), fields, draft.closing.trim()].filter(Boolean)
  let body = sections.join("\n\n")
  if (draft.fields.includes("OperationTime")) {
    body += `${body ? "\n" : ""}${draft.fieldLines.OperationTime ?? FIELD_LINES.OperationTime}`
  }
  if (draft.postscript.trim()) body += `${body ? "\n" : ""}${draft.postscript.trim()}`
  return body
}

/** Replace one scenario while retaining the other expanded bodies, even if opaque. */
export function updateBusinessTemplate(template: string, kind: BusinessTemplateKind, draft: BusinessTemplateDraft): string {
  if (kind === "customer_renewal_success") return renderBusinessDraft(draft, kind)
  const bodies = BENEFIT_TEMPLATE_KINDS.map((scenario) => scenario === kind
    ? renderBusinessDraft(draft, kind)
    : businessScenarioBody(template, scenario))
  if (bodies.some((body) => body === null)) throw new Error("Unsupported benefit template logic")
  return bodies.map((body, index) => `{{${index === 0 ? "if" : "else if"} eq .BenefitType "${BENEFIT_TYPES[BENEFIT_TEMPLATE_KINDS[index]]}"}}${body}`).join("") + "{{end}}"
}

const DEFAULT_OPENING: Record<BusinessTemplateKind, string> = {
  customer_renewal_success: "您好，您的 ChatGPT Team 服务续费已确认，感谢您的支持。",
  customer_benefit_extension: "您好，为感谢您的支持，您的 ChatGPT Team 服务已获赠延期福利。",
  customer_benefit_discount: "您好，为感谢您的支持，您的 ChatGPT Team 服务已获赠续费优惠。",
  customer_benefit_revision: "您好，您的延期福利已修改，请以本邮件中的到期日期为准。",
  customer_benefit_revoke: "您好，您的延期福利已撤回，请以本邮件中的到期日期为准。",
}

const DEFAULT_CLOSING: Record<BusinessTemplateKind, string> = {
  customer_renewal_success: "本期款项已登记，无需重复付款。祝您使用愉快！\n如有疑问，请联系管理员。",
  customer_benefit_extension: "本次延期已生效，无需额外付款。\n如有疑问，请联系管理员。",
  customer_benefit_discount: "本次优惠福利无需支付额外费用。优惠价从上述日期对应的续费账期开始使用，此后每期均按优惠价续费，直至另行调整。当前已支付账期不受影响。\n如有疑问，请联系管理员。",
  customer_benefit_revision: "本次仅调整已赠送的延期天数，无需额外付款。\n如有疑问，请联系管理员。",
  customer_benefit_revoke: "本次撤回的是已赠送的延期福利，不代表退款或新增收费；如有疑问，请联系管理员确认服务安排。\n如有疑问，请联系管理员。",
}

function defaultDraft(kind: BusinessTemplateKind): BusinessTemplateDraft {
  return {
    opening: DEFAULT_OPENING[kind], fields: [...SCENARIO_FIELDS[kind]], fieldLines: {},
    closing: DEFAULT_CLOSING[kind],
    postscript: kind === "customer_renewal_success" ? "" : "本邮件记录上述时间的福利操作，后续如有变更，请以更新的通知为准。",
  }
}

export function defaultBusinessTemplate(benefit: boolean): string {
  if (!benefit) return renderBusinessDraft(defaultDraft("customer_renewal_success"), "customer_renewal_success")
  return BENEFIT_TEMPLATE_KINDS.map((kind, index) => `{{${index === 0 ? "if" : "else if"} eq .BenefitType "${BENEFIT_TYPES[kind]}"}}${renderBusinessDraft(defaultDraft(kind), kind)}`).join("") + "{{end}}"
}
