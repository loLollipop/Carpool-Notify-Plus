import { Navigate, useLocation } from "react-router-dom"

import { useAuth } from "./auth-state"
import { AuthLoadingScreen } from "./AuthFrame"

export function RequireAuth({ children }: { children: React.ReactNode }) {
  const { status } = useAuth()
  const location = useLocation()

  if (status === "loading") {
    return <AuthLoadingScreen />
  }
  if (status === "unauthenticated") {
    return <Navigate to="/login" replace state={{ from: location }} />
  }
  return <>{children}</>
}
