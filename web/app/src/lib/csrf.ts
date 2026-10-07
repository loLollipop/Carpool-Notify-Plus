/**
 * CSRF Token Management
 * Provides utilities for fetching and attaching CSRF tokens to API requests
 */

let cachedToken: string | null = null

/**
 * Fetch CSRF token from the server
 */
export async function fetchCSRFToken(): Promise<string> {
  if (cachedToken) {
    return cachedToken
  }

  try {
    const response = await fetch("/api/csrf-token", {
      credentials: "include",
    })

    if (!response.ok) {
      throw new Error(`Failed to fetch CSRF token: ${response.status}`)
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
  }
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
export async function getCSRFHeaders(): Promise<Record<string, string>> {
  const token = await fetchCSRFToken()
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
