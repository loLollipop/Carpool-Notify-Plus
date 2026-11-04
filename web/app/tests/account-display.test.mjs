import assert from 'node:assert/strict'
import test from 'node:test'
import { formatAccountLabel, formatAccountName } from '../src/lib/account-display.ts'

test('labels a secondary space with its shared account serial', () => {
  assert.equal(formatAccountName('help@npoolive.org', 'secondary'), 'help@npoolive.org · 副')
  assert.equal(formatAccountLabel(13, 'help@npoolive.org', '-', 'secondary'), '13 · help@npoolive.org · 副')
})

test('keeps primary and standalone account labels unchanged', () => {
  for (const role of ['primary', 'standalone', undefined]) {
    assert.equal(formatAccountLabel(13, 'help@npoolive.org', '-', role), '13 · help@npoolive.org')
  }
})
