import assert from 'node:assert/strict'
import test from 'node:test'

const storage = new Map()
let onStorage
let reloads = 0
globalThis.window = {
  localStorage: {
    getItem: (key) => storage.get(key) ?? null,
    setItem: (key, value) => storage.set(key, value),
    removeItem: (key) => storage.delete(key),
  },
  location: { reload: () => { reloads++ } },
  addEventListener: (event, listener) => {
    if (event === 'storage') onStorage = listener
  },
}

const { enterSandboxMode, exitSandboxMode, isSandboxModeActive } =
  await import('../src/lib/sandbox-mode.ts')
const key = 'carpool-notify:sandbox-mode'
const sandbox = { access_token: 'sandbox-token', seeded_at: '', redemption_codes: [] }

function setup(t) {
  exitSandboxMode()
  storage.clear()
  reloads = 0
  t.after(() => exitSandboxMode())
}

test('entering sandbox in another tab reloads before changing the API scope', (t) => {
  setup(t)
  storage.set(key, JSON.stringify({ enabled: true, accessToken: sandbox.access_token }))
  onStorage({ key })
  assert.equal(reloads, 1)
  // Until navigation clears the old queries and forms, actions keep their scope.
  assert.equal(isSandboxModeActive(), false)
})

test('leaving or resetting sandbox in another tab discards old page state', (t) => {
  setup(t)
  enterSandboxMode(sandbox)
  storage.delete(key)
  onStorage({ key })
  assert.equal(reloads, 1)
  assert.equal(isSandboxModeActive(), true)

  storage.set(key, JSON.stringify({ enabled: true, accessToken: 'replacement-token' }))
  onStorage({ key })
  assert.equal(reloads, 2)
})

test('clearing local storage also reloads an active sandbox page', (t) => {
  setup(t)
  enterSandboxMode(sandbox)
  storage.clear()
  onStorage({ key: null })
  assert.equal(reloads, 1)
})

test('unrelated storage updates and unchanged mode do not reload the page', (t) => {
  setup(t)
  enterSandboxMode(sandbox)
  onStorage({ key: 'theme' })
  onStorage({ key })
  onStorage({ key: null })
  assert.equal(reloads, 0)
  assert.equal(isSandboxModeActive(), true)
})
