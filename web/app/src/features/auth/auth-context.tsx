import * as React from "react"
import { useQueryClient } from "@tanstack/react-query"

import { setUnauthorizedHandler } from "@/api/client"
import { getSession, logout as apiLogout } from "@/api/endpoints"
import { AuthContext, type AuthStatus } from "./auth-state"

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const [status, setStatus] = React.useState<AuthStatus>("loading")
  const queryClient = useQueryClient()

  React.useEffect(() => {
    let cancelled = false
    getSession()
      .then((session) => {
        if (!cancelled) {
          setStatus(session.authenticated ? "authenticated" : "unauthenticated")
        }
      })
      .catch(() => {
        if (!cancelled) {
          setStatus("unauthenticated")
        }
      })
    return () => {
      cancelled = true
    }
  }, [])

  React.useEffect(() => {
    setUnauthorizedHandler(() => setStatus("unauthenticated"))
    return () => setUnauthorizedHandler(null)
  }, [])

  const markAuthenticated = React.useCallback(() => setStatus("authenticated"), [])

  const logout = React.useCallback(async () => {
    try {
      await apiLogout()
    } finally {
      queryClient.clear()
      setStatus("unauthenticated")
    }
  }, [queryClient])

  const value = React.useMemo(
    () => ({ status, markAuthenticated, logout }),
    [status, markAuthenticated, logout],
  )

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}
