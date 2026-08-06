import { Navigate, useLocation } from "react-router-dom"

import { useAuth } from "./auth-context"

function BootSplash() {
  return (
    <div className="flex min-h-dvh items-center justify-center">
      <div className="flex items-center gap-3 text-muted-foreground animate-fade-in">
        <span className="inline-block size-2 rounded-full bg-brand animate-pulse" />
        <span className="font-mono text-xs uppercase">Carpool Notify</span>
      </div>
    </div>
  )
}

export function RequireAuth({ children }: { children: React.ReactNode }) {
  const { status } = useAuth()
  const location = useLocation()

  if (status === "loading") {
    return <BootSplash />
  }
  if (status === "unauthenticated") {
    return <Navigate to="/login" replace state={{ from: location }} />
  }
  return <>{children}</>
}
