import * as React from "react"
import { useLocation, useNavigate, Navigate } from "react-router-dom"
import { useTranslation } from "react-i18next"
import {
  Eye,
  EyeOff,
  LoaderCircle,
  LockKeyhole,
} from "lucide-react"

import { login } from "@/api/endpoints"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { useAuth } from "./auth-state"
import { AuthFrame } from "./AuthFrame"

export function LoginPage() {
  const { t } = useTranslation()
  const { status, markAuthenticated } = useAuth()
  const navigate = useNavigate()
  const location = useLocation()
  const [password, setPassword] = React.useState("")
  const [error, setError] = React.useState("")
  const [submitting, setSubmitting] = React.useState(false)
  const [showPassword, setShowPassword] = React.useState(false)

  const from = (location.state as { from?: { pathname: string } } | null)?.from?.pathname ?? "/"

  if (status === "authenticated") {
    return <Navigate to={from} replace />
  }

  const handleSubmit = async (event: React.FormEvent) => {
    event.preventDefault()
    if (submitting) return
    setSubmitting(true)
    setError("")
    try {
      await login(password)
      markAuthenticated()
      navigate(from, { replace: true })
    } catch (loginError) {
      setError(loginError instanceof Error ? loginError.message : t("common.actionFailed"))
      setSubmitting(false)
    }
  }

  return (
    <AuthFrame>
      <main className="auth-form-card animate-rise">
        <div className="auth-form-kicker">{t("auth.privateConsole")}</div>
        <div className="auth-form-heading">
          <span className="auth-form-icon">
            <LockKeyhole className="size-5" />
          </span>
          <h1>{t("auth.adminEntry")}</h1>
        </div>
        <p className="auth-form-caption">{t("auth.consoleLabel")}</p>
        <span className="auth-form-rule" aria-hidden="true" />

        <form onSubmit={handleSubmit} className="grid gap-5">
          <div className="grid gap-2">
            <Label htmlFor="password">{t("auth.password")}</Label>
            <div className="relative">
              <LockKeyhole
                className="pointer-events-none absolute left-3.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground"
                aria-hidden="true"
              />
              <Input
                id="password"
                type={showPassword ? "text" : "password"}
                autoComplete="current-password"
                autoFocus
                required
                value={password}
                placeholder={t("auth.passwordPlaceholder")}
                onChange={(event) => setPassword(event.target.value)}
                aria-invalid={error !== ""}
                className="auth-password-input h-12 bg-transparent pl-10 pr-11"
              />
              <button
                type="button"
                className="absolute right-1.5 top-1/2 grid size-9 -translate-y-1/2 place-items-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                onClick={() => setShowPassword((visible) => !visible)}
                aria-label={showPassword ? t("auth.hidePassword") : t("auth.showPassword")}
              >
                {showPassword ? <EyeOff className="size-4" /> : <Eye className="size-4" />}
              </button>
            </div>
            {error ? (
              <p role="alert" className="text-sm text-destructive animate-fade-in">
                {error}
              </p>
            ) : null}
          </div>
          <Button type="submit" size="lg" className="auth-submit-button h-12 w-full" disabled={submitting}>
            {submitting ? (
              <>
                <LoaderCircle className="size-4 animate-spin" />
                {t("auth.submitting")}
              </>
            ) : (
              t("auth.submit")
            )}
          </Button>
        </form>
      </main>
    </AuthFrame>
  )
}
