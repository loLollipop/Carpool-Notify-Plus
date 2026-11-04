import assert from 'node:assert/strict'
import test from 'node:test'
import { extensionDateRange, extensionDaysUntil } from '../src/features/goals/benefit-extension-date.ts'

test('selecting November 9 from November 4 means five extra days', () => {
  assert.equal(extensionDaysUntil('2026-11-04', '2026-11-09'), 5)
  assert.equal(extensionDaysUntil('2026-11-06', '2026-11-09'), 3)
})

test('a shared date range extends every selected seat by 1 to 365 days', () => {
  assert.deepEqual(extensionDateRange(['2026-11-04', '2026-11-06']), {
    earliestDue: '2026-11-04', latestDue: '2026-11-06', min: '2026-11-07', max: '2027-11-04',
  })
  assert.equal(extensionDaysUntil('2026-11-04', '2026-11-04'), null)
  assert.equal(extensionDaysUntil('2026-11-06', '2026-11-05'), null)
  assert.equal(extensionDaysUntil('2026-11-04', '2027-11-04'), 365)
  assert.equal(extensionDaysUntil('2026-11-04', '2027-11-05'), null)
})

test('calendar arithmetic handles leap days and daylight saving boundaries', () => {
  assert.equal(extensionDaysUntil('2028-02-28', '2028-03-01'), 2)
  assert.equal(extensionDaysUntil('2026-03-07', '2026-03-09'), 2)
  assert.equal(extensionDaysUntil('2026-10-31', '2026-11-02'), 2)
})

test('missing and invalid review dates cannot produce a calendar range', () => {
  assert.equal(extensionDateRange([]), null)
  assert.equal(extensionDateRange(['2026-11-04', '']), null)
  assert.equal(extensionDateRange(['2026-11-31']), null)
  for (const value of ['', '2026-11-9', '2026-11-31', '2026-11-09T00:00:00Z']) {
    assert.equal(extensionDaysUntil('2026-11-04', value), null)
  }
})
