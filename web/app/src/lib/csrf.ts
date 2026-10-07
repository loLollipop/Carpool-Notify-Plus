/**
 * CSRF Token Management
 * Provides utilities for fetching and attaching CSRF tokens to API requests
 */

let cachedToken: string | null = null
let tokenRequest: Promise<string> | null = null

export class CSRFTokenError extends Error {
  status: number

  constructor(message: string, status: number) {
    super(message)
    this.name = "CSRFTokenError"
    this.status = status
  }
}

/**
 * Fetch CSRF token from the server
 */
export async function fetchCSRFToken(forceRefresh = false): Promise<string> {
  if (!forceRefresh && cachedToken) {
    return cachedToken
  }

  if (tokenRequest) {
    return tokenRequest
  }

  tokenRequest = (async () => {
    try {
      const response = await fetch("/api/csrf-token", {
        credentials: "include",
      })

      if (!response.ok) {
        throw new CSRFTokenError(`Failed to fetch CSRF token: ${response.status}`, response.status)
      }

      const data = await response.json()
      const token = data.csrf_token
      if (typeof token !== "string" || token === "") {
        throw new Error("Invalid CSRF token received")
      }
      cachedToken = token
      return token
    } catch (error) {
      console.error("CSRF token fetch failed:", error)
      throw error
    } finally {
      tokenRequest = null
    }
  })()

  return tokenRequest
}

/**
 * Clear cached CSRF token (useful after logout)
 */
export function clearCSRFToken(): void {
  cachedToken = null
}

/**
 * Get CSRF token for request headers
 * Fetches a new token if not cached
 */
export async function getCSRFHeaders(forceRefresh = false): Promise<Record<string, string>> {
  const token = await fetchCSRFToken(forceRefresh)
  return {
    "X-CSRF-Token": token,
  }
}

/**
 * Attach CSRF token to fetch request options
 */
export async function withCSRF(
  options: RequestInit = {},
): Promise<RequestInit> {
  const csrfHeaders = await getCSRFHeaders()

  return {
    ...options,
    headers: {
      ...options.headers,
      ...csrfHeaders,
    },
  }
}
