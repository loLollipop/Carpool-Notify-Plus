import { Navigate, useLocation } from "react-router-dom"

import { APP_NAME, BrandIcon } from "@/components/brand"
import { useAuth } from "./auth-state"

function BootSplash() {
  return (
    <div className="flex min-h-dvh items-center justify-center">
      <div className="flex items-center gap-3 text-muted-foreground animate-fade-in">
        <BrandIcon className="size-8 animate-pulse" />
        <span className="text-xs font-semibold uppercase">{APP_NAME}</span>
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
