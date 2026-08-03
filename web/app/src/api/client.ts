export class ApiError extends Error {
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
  const response = await fetch(path, {
    method,
    credentials: "same-origin",
    headers: body !== undefined ? { "Content-Type": "application/json" } : undefined,
    body: body !== undefined ? JSON.stringify(body) : undefined,
  })

  const payload = await response.json().then(
    (data) => data as Record<string, unknown>,
    () => null,
  )

  if (response.status === 401) {
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

function readError(payload: Record<string, unknown> | null): string | null {
  if (payload && typeof payload.error === "string" && payload.error !== "") {
    return payload.error
  }
  return null
}
