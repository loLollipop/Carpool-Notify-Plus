import assert from 'node:assert/strict'
import test from 'node:test'
import { safePromotionURL, promotionIsVisible, DEFAULT_REDEEM_PROMOTION } from '../src/features/redemptions/redeem-promotion.ts'

const invalid = [
  'javascript:alert(1)', 'data:text/html,<script>', 'ftp://example.com/', '//example.com/', '/local',
  'https:example.com', 'https:///path', 'https://', 'https://?query=1', 'https://#fragment',
  'https://user:password@example.com/', 'https://user@example.com/', 'https://@example.com/',
  'https://example.com@evil.example/', 'https://example.com\\@evil.example/', 'https://example.com/path\\test',
  'https://example.com/path with space', 'https://example.com/path\nnext', 'https://example.com/path\tnext',
  'https://example.com/path\x00', 'https://example.com/path\u00a0next', 'https://example.com/<tag>',
  'https://example.com:bad/', 'https://example.com:/', 'https://example.com:0/', 'https://example.com:65536/',
  'https://example.com:-1/', 'https://./', 'https://..example.com/', 'https://example..com/',
  'https://-example.com/', 'https://example-.com/', 'https://example_com/', 'https://[example.com]/',
  'https://[::1', 'https://::1/', 'https://example.com/%zz', 'https://%65xample.com/', 'https://[not-an-ip]/',
  'https://example.com/' + '文'.repeat(677),
]
const valid = [
  'https://ikun.fuckoai.bond/', 'http://example.com/', 'https://example.com:65535/path',
  'https://example.com/体验?from=redeem&name=a%20b#详情', 'https://[2001:db8::1]:443/path?q=1#top',
  'https://127.0.0.1/', 'https://例子.中国/体验', 'https://localhost/path',
  'https://example.com/' + 'a'.repeat(2048 - 'https://example.com/'.length),
]
test('rejects unsafe or malformed promotion URLs', () => {
  for (const value of invalid) assert.equal(safePromotionURL(value), null, `accepted ${value}`)
})
test('accepts valid HTTP(S), IPv6, unicode and encoded query parameters', () => {
  for (const value of valid) assert.ok(safePromotionURL('  ' + value + '  '), `rejected ${value}`)
})
test('only shows an enabled promotion with copy and a safe link', () => {
  assert.equal(promotionIsVisible(DEFAULT_REDEEM_PROMOTION), true)
  assert.equal(promotionIsVisible({ ...DEFAULT_REDEEM_PROMOTION, promotion_enabled: false }), false)
  assert.equal(promotionIsVisible({ ...DEFAULT_REDEEM_PROMOTION, promotion_text: ' ' }), false)
  assert.equal(promotionIsVisible({ ...DEFAULT_REDEEM_PROMOTION, promotion_url: invalid[0] }), false)
})
