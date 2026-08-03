import * as React from "react"
import { useQueryClient } from "@tanstack/react-query"

import { setUnauthorizedHandler } from "@/api/client"
import { getSession, logout as apiLogout } from "@/api/endpoints"

type AuthStatus = "loading" | "authenticated" | "unauthenticated"

interface AuthContextValue {
  status: AuthStatus
  markAuthenticated: () => void
  logout: () => Promise<void>
}

const AuthContext = React.createContext<AuthContextValue | null>(null)

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

export function useAuth() {
  const context = React.useContext(AuthContext)
  if (!context) {
    throw new Error("useAuth must be used within AuthProvider")
  }
  return context
}
