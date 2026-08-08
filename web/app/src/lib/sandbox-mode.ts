import type { SandboxStatus } from "@/api/types"

const STORAGE_KEY = "carpool-notify:sandbox-mode"

export interface SandboxModeState {
  enabled: boolean
  accessToken: string
  seededAt: string
  redemptionCodes: string[]
}

const EMPTY_STATE: SandboxModeState = {
  enabled: false,
  accessToken: "",
  seededAt: "",
  redemptionCodes: [],
}

function readState(): SandboxModeState {
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY)
    if (!raw) return EMPTY_STATE
    const parsed = JSON.parse(raw) as Partial<SandboxModeState>
    if (parsed.enabled !== true || typeof parsed.accessToken !== "string") return EMPTY_STATE
    return {
      enabled: true,
      accessToken: parsed.accessToken,
      seededAt: typeof parsed.seededAt === "string" ? parsed.seededAt : "",
      redemptionCodes: Array.isArray(parsed.redemptionCodes)
        ? parsed.redemptionCodes.filter((code): code is string => typeof code === "string")
        : [],
    }
  } catch {
    return EMPTY_STATE
  }
}

let currentState = readState()
const listeners = new Set<() => void>()

function publish(next: SandboxModeState) {
  currentState = next
  listeners.forEach((listener) => listener())
}

export function getSandboxModeState() {
  return currentState
}

export function isSandboxModeActive() {
  return currentState.enabled
}

export function enterSandboxMode(status: SandboxStatus) {
  const next: SandboxModeState = {
    enabled: true,
    accessToken: status.access_token,
    seededAt: status.seeded_at,
    redemptionCodes: status.redemption_codes ?? [],
  }
  try {
    window.localStorage.setItem(STORAGE_KEY, JSON.stringify(next))
  } catch {
    // The current tab can still use rehearsal mode when storage is unavailable.
  }
  publish(next)
}

export function exitSandboxMode() {
  try {
    window.localStorage.removeItem(STORAGE_KEY)
  } catch {
    // Keep the current page usable when storage is unavailable.
  }
  publish(EMPTY_STATE)
}

export function subscribeSandboxMode(listener: () => void) {
  listeners.add(listener)
  return () => listeners.delete(listener)
}

if (typeof window !== "undefined") {
  window.addEventListener("storage", (event) => {
    if (event.key === STORAGE_KEY) publish(readState())
  })
}
