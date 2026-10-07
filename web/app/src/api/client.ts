import { isSandboxModeActive } from "@/lib/sandbox-mode"
import { clearCSRFToken, CSRFTokenError, getCSRFHeaders } from "@/lib/csrf"

class ApiError extends Error {
  status: number

  constructor(message: string, status: number) {
    super(message)
    this.name = "ApiError"
    this.status = status
  }
}

type UnauthorizedHandler = () => void

let unauthorizedHandler: UnauthorizedHandler | null = null

/** AuthProvider registers a callback so any 401 flips the app to the login screen. */
export function setUnauthorizedHandler(handler: UnauthorizedHandler | null) {
  unauthorizedHandler = handler
}

interface RequestOptions {
  method?: "GET" | "POST" | "PUT" | "DELETE"
  body?: unknown
  /** Skip the global 401 handler (used by the session bootstrap probe). */
  silent401?: boolean
}

export async function api<T>(path: string, options: RequestOptions = {}): Promise<T> {
  const { method = "GET", body, silent401 = false } = options

  const needsCSRF = method !== "GET" && !isPublicMutation(path)
  // Freeze the destination before any await. A storage event can switch
  // sandbox mode while a CSRF token is refreshed, but a retry must never
  // cross from the sandbox into live data (or the other way around).
  const requestPath = scopeBusinessPath(path)

  const send = async (forceRefresh = false) => {
    const headers: Record<string, string> = {}
    if (body !== undefined) {
      headers["Content-Type"] = "application/json"
    }

    if (needsCSRF) {
      const csrfHeaders = await getCSRFHeaders(forceRefresh)
      Object.assign(headers, csrfHeaders)
    }

    return fetch(requestPath, {
      method,
      cache: "no-store",
      credentials: "same-origin",
      headers: Object.keys(headers).length > 0 ? headers : undefined,
      body: body !== undefined ? JSON.stringify(body) : undefined,
    })
  }

  let response: Response
  try {
    response = await send()
  } catch (error) {
    if (error instanceof CSRFTokenError && error.status === 401) {
      clearCSRFToken()
      if (!silent401) {
        unauthorizedHandler?.()
      }
    }
    throw error
  }

  let payload = await response.json().then(
    (data) => data as Record<string, unknown>,
    () => null,
  )

  // A session can outlive its CSRF token. Refresh once on the specific
  // protection response so normal business 403s are not retried blindly.
  if (response.status === 403 && needsCSRF && isCSRFError(payload)) {
    clearCSRFToken()
    response = await send(true)
    payload = await response.json().then(
      (data) => data as Record<string, unknown>,
      () => null,
    )
  }

  if (response.status === 401) {
    clearCSRFToken()
    if (!silent401) {
      unauthorizedHandler?.()
    }
    throw new ApiError(readError(payload) ?? "未登录", 401)
  }
  if (!response.ok || payload?.ok === false) {
    throw new ApiError(readError(payload) ?? `请求失败 (${response.status})`, response.status)
  }
  return payload as T
}

const PUBLIC_MUTATION_PATHS = new Set([
  "/api/login",
  "/api/redeem",
  "/api/renewal/lookup",
  "/api/renewal",
])

function isPublicMutation(path: string) {
  const pathname = path.split("?", 1)[0]
  const unscopedPath = pathname.startsWith("/api/sandbox")
    ? `/api${pathname.slice("/api/sandbox".length)}`
    : pathname
  return PUBLIC_MUTATION_PATHS.has(unscopedPath)
}

function isCSRFError(payload: Record<string, unknown> | null) {
  const message = readError(payload)
  return message === "Missing CSRF token" || message === "Invalid CSRF token"
}

const SANDBOX_BUSINESS_PREFIXES = [
  "/api/calendar",
  "/api/dashboard",
  "/api/operations/overview",
  "/api/goals",
  "/api/redemptions",
  "/api/renewal-applications",
  "/api/redemption-codes",
  "/api/subscriptions",
  "/api/accounts",
  "/api/account-options",
  "/api/seats",
  "/api/after-sales",
  "/api/bills",
  "/api/operating-expenses",
  "/api/settings/test-notify",
  "/api/settings/test-customer-email",
]

function scopeBusinessPath(path: string) {
  if (!isSandboxModeActive()) return path
  const sandboxed = SANDBOX_BUSINESS_PREFIXES.some(
    (prefix) => path === prefix || path.startsWith(`${prefix}/`) || path.startsWith(`${prefix}?`),
  )
  return sandboxed ? `/api/sandbox${path.slice(4)}` : path
}

function readError(payload: Record<string, unknown> | null): string | null {
  if (payload && typeof payload.error === "string" && payload.error !== "") {
    return payload.error
  }
  return null
}
