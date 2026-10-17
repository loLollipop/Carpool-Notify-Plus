import assert from 'node:assert/strict'
import test from 'node:test'

globalThis.window = {
  localStorage: { getItem: () => null, setItem() {}, removeItem() {} },
  addEventListener() {},
}

const { api } = await import('../src/api/client.ts')
const { clearCSRFToken } = await import('../src/lib/csrf.ts')
const { enterSandboxMode, exitSandboxMode } = await import('../src/lib/sandbox-mode.ts')

const sandbox = { access_token: 'sandbox-token', seeded_at: '', redemption_codes: [] }
const json = (payload, status = 200) => Response.json(payload, { status })

function setup(t, fetch) {
  const originalFetch = globalThis.fetch
  globalThis.fetch = fetch
  clearCSRFToken()
  exitSandboxMode()
  t.after(() => {
    globalThis.fetch = originalFetch
    clearCSRFToken()
    exitSandboxMode()
  })
}

test('sandbox acknowledgements and overview use sandbox API with CSRF protection', async (t) => {
  const requests = []
  setup(t, async (path, options) => {
    requests.push({ path, options })
    return path === '/api/csrf-token' ? json({ csrf_token: 'csrf-token' }) : json({ ok: true })
  })
  enterSandboxMode(sandbox)
  await api('/api/operations/tasks/acknowledge', { method: 'POST', body: { task_ids: ['team:1'] } })
  await api('/api/operations/overview')
  assert.deepEqual(requests.map(({ path }) => path), [
    '/api/csrf-token',
    '/api/sandbox/operations/tasks/acknowledge',
    '/api/sandbox/operations/overview',
  ])
  assert.equal(requests[1].options.headers['X-CSRF-Token'], 'csrf-token')
})

test('live acknowledgements keep the live API destination', async (t) => {
  const requests = []
  setup(t, async (path) => {
    requests.push(path)
    return path === '/api/csrf-token' ? json({ csrf_token: 'csrf-token' }) : json({ ok: true })
  })
  await api('/api/operations/tasks/acknowledge', { method: 'POST', body: { task_ids: ['team:1'] } })
  assert.equal(requests.at(-1), '/api/operations/tasks/acknowledge')
})

test('switching modes during CSRF refresh and retry never changes the write destination', async (t) => {
  const writes = []
  let tokenRequests = 0
  setup(t, async (path) => {
    if (path === '/api/csrf-token') {
      tokenRequests++
      exitSandboxMode()
      return json({ csrf_token: `csrf-${tokenRequests}` })
    }
    writes.push(path)
    return writes.length === 1
      ? json({ ok: false, error: 'Invalid CSRF token' }, 403)
      : json({ ok: true })
  })
  enterSandboxMode(sandbox)
  await api('/api/operations/tasks/acknowledge', { method: 'POST', body: { task_ids: ['team:1'] } })
  assert.equal(tokenRequests, 2)
  assert.deepEqual(writes, [
    '/api/sandbox/operations/tasks/acknowledge',
    '/api/sandbox/operations/tasks/acknowledge',
  ])
})

test('public sandbox redemption remains scoped once and does not request CSRF', async (t) => {
  const requests = []
  setup(t, async (path) => {
    requests.push(path)
    return json({ ok: true })
  })
  enterSandboxMode(sandbox)
  const path = '/api/sandbox/redeem?sandbox_token=sandbox-token'
  await api(path, { method: 'POST', body: { code: 'test-code' } })
  assert.deepEqual(requests, [path])
})
