import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'
import {
  BENEFIT_TEMPLATE_KINDS,
  businessScenarioBody,
  businessTemplateFields,
  defaultBusinessTemplate,
  parseBusinessTemplate,
  renderBusinessDraft,
  updateBusinessTemplate,
} from '../src/features/settings/business-email-template.ts'

// Read source defaults so changes on either side cannot silently break the form.
const model = readFileSync(new URL('../../../internal/model/model.go', import.meta.url), 'utf8')
const sourceDefault = (name) => {
  const match = model.match(new RegExp('const ' + name + ' = `([\\s\\S]*?)`'))
  assert.ok(match, name)
  return match[1].replace(/\r\n/g, '\n')
}
const renewal = sourceDefault('DefaultRenewalSuccessCustomerEmailTemplate')
const benefit = sourceDefault('DefaultCustomerBenefitEmailTemplate')

test('the existing renewal confirmation opens as copy and eight selected automatic fields', () => {
  const draft = parseBusinessTemplate(renewal, 'customer_renewal_success')
  assert.ok(draft)
  assert.match(draft.opening, /续费已确认/)
  assert.equal(draft.fields.length, 8)
  assert.match(draft.closing, /无需重复付款/)
  assert.equal(draft.postscript, '')
  assert.equal(renderBusinessDraft(draft, 'customer_renewal_success'), renewal)
})

test('all four existing benefit branches open visually with the right data and copy', () => {
  const expected = {
    customer_benefit_extension: ['已获赠延期福利', 'ExtensionDays', '本次延期已生效'],
    customer_benefit_discount: ['已获赠续费优惠', 'DiscountAmount', '当前已支付账期不受影响'],
    customer_benefit_revision: ['延期福利已修改', 'PreviousExtensionDays', '仅调整已赠送'],
    customer_benefit_revoke: ['延期福利已撤回', 'PreviousExtensionDays', '不代表退款或新增收费'],
  }
  for (const kind of BENEFIT_TEMPLATE_KINDS) {
    const draft = parseBusinessTemplate(benefit, kind)
    assert.ok(draft, kind)
    assert.ok(draft.opening.includes(expected[kind][0]))
    assert.ok(draft.fields.includes(expected[kind][1]))
    assert.ok(draft.closing.includes(expected[kind][2]))
    assert.ok(draft.fields.includes('OperationTime'))
    assert.match(draft.postscript, /后续如有变更/)
    assert.ok(!draft.opening.includes('{{'))
    assert.ok(!draft.closing.includes('{{'))
  }
})

test('editing one benefit scenario preserves the exact expanded bodies of the other three', () => {
  let body = benefit
  for (const kind of BENEFIT_TEMPLATE_KINDS) {
    const previous = new Map(BENEFIT_TEMPLATE_KINDS.map((scenario) => [scenario, businessScenarioBody(body, scenario)]))
    const draft = parseBusinessTemplate(body, kind)
    draft.opening = `开头修改 ${kind}`
    draft.closing = `结尾修改 ${kind}`
    body = updateBusinessTemplate(body, kind, draft)
    for (const scenario of BENEFIT_TEMPLATE_KINDS) {
      if (scenario !== kind) assert.equal(businessScenarioBody(body, scenario), previous.get(scenario))
    }
    assert.equal(parseBusinessTemplate(body, kind).opening, draft.opening)
  }
  for (const kind of BENEFIT_TEMPLATE_KINDS) assert.equal(parseBusinessTemplate(body, kind).closing, `结尾修改 ${kind}`)
})

test('field toggles preserve custom labels, field order, closing copy and the audit postscript', () => {
  const source = '自定义开头\n\n我的账期：{{.PeriodEnd}}\n实收人民币 {{.AmountPaid}} 元\n\n祝好！\n处理于 {{.OperationTime}}\n自定义补充说明'
  const draft = parseBusinessTemplate(source, 'customer_renewal_success')
  assert.ok(draft)
  assert.deepEqual(draft.fields, ['PeriodEnd', 'AmountPaid', 'OperationTime'])
  draft.fields = draft.fields.filter((key) => key !== 'AmountPaid')
  const removed = renderBusinessDraft(draft, 'customer_renewal_success')
  assert.ok(!removed.includes('.AmountPaid'))
  draft.fields.splice(1, 0, 'AmountPaid')
  assert.equal(renderBusinessDraft(draft, 'customer_renewal_success'), source)
})

test('empty field selections and intentionally empty copy stay empty', () => {
  const draft = parseBusinessTemplate(renewal, 'customer_renewal_success')
  draft.fields = []
  const body = renderBusinessDraft(draft, 'customer_renewal_success')
  assert.deepEqual(parseBusinessTemplate(body, 'customer_renewal_success').fields, [])
  assert.equal(renderBusinessDraft({ ...draft, opening: '', closing: '', postscript: '' }, 'customer_renewal_success'), '')
})

test('optional fields can be added with scenario-specific labels', () => {
  const kind = 'customer_benefit_revision'
  const draft = parseBusinessTemplate('自定义正文', kind)
  assert.deepEqual(draft.fields, [])
  assert.ok(businessTemplateFields(kind, draft).includes('PreviousExtensionDays'))
  draft.fields.push('ExtensionDays', 'NextDueDate')
  const body = renderBusinessDraft(draft, kind)
  assert.match(body, /调整后延期天数：{{.ExtensionDays}} 天/)
  assert.match(body, /调整后到期日期：{{.NextDueDate}}/)
})

test('advanced Go logic, inline multiple fields and unsupported layout stay opaque', () => {
  const examples = [
    '{{if .CustomerEmail}}您好 {{.CustomerEmail}}{{end}}',
    '{{printf "%s" .CustomerEmail}}',
    '{{- .CustomerEmail -}}',
    '{{.CustomerEmail}} + {{.SubscriptionID}}',
    '{{.CustomerEmail}}\n备注文字\n{{.SubscriptionID}}',
    '{{.CustomerEmail}}\n{{.CustomerEmail}}',
    '{{range .CustomerEmail}}{{.}}{{end}}',
    '{{if eq .BenefitType "extension"}}incomplete',
    '{{else}}bad{{end}}',
    '{{.Unknown}}',
  ]
  for (const body of examples) assert.equal(parseBusinessTemplate(body, 'customer_benefit_extension'), null, body)
})

test('standard replacements are editable and round-trip all five scenarios', () => {
  for (const kind of ['customer_renewal_success', ...BENEFIT_TEMPLATE_KINDS]) {
    const source = defaultBusinessTemplate(kind !== 'customer_renewal_success')
    const original = kind === 'customer_renewal_success' ? renewal : benefit
    const content = (body) => body.split('\n').map((line) => line.trim()).filter(Boolean).join('\n')
    assert.equal(content(businessScenarioBody(source, kind)), content(businessScenarioBody(original, kind)))
    const draft = parseBusinessTemplate(source, kind)
    assert.ok(draft, kind)
    const next = updateBusinessTemplate(source, kind, { ...draft, opening: '已编辑' })
    assert.equal(parseBusinessTemplate(next, kind).opening, '已编辑')
    assert.ok(!businessScenarioBody(next, kind).includes('{{if'))
  }
})
