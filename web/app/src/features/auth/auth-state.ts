import * as React from "react"

export type AuthStatus = "loading" | "authenticated" | "unauthenticated"

interface AuthContextValue {
  status: AuthStatus
  markAuthenticated: () => void
  logout: () => Promise<void>
}

export const AuthContext = React.createContext<AuthContextValue | null>(null)

export function useAuth() {
  const context = React.useContext(AuthContext)
  if (!context) {
    throw new Error("useAuth must be used within AuthProvider")
  }
  return context
}
