import * as React from "react"
import { useLocation, useNavigate, Navigate } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { ArrowRight, LockKeyhole } from "lucide-react"

import { login } from "@/api/endpoints"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { useAuth } from "./auth-context"

export function LoginPage() {
  const { t } = useTranslation()
  const { status, markAuthenticated } = useAuth()
  const navigate = useNavigate()
  const location = useLocation()
  const [password, setPassword] = React.useState("")
  const [error, setError] = React.useState("")
  const [submitting, setSubmitting] = React.useState(false)

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
    <div className="grid min-h-dvh place-items-center px-4 py-10">
      <div className="w-full max-w-sm">
        <div className="mb-8 flex flex-col items-center text-center animate-fade-up">
          <img
            src="/logo.png"
            alt=""
            aria-hidden="true"
            width={44}
            height={44}
            draggable={false}
            className="size-11"
          />
          <h1 className="mt-4 text-xl font-semibold tracking-tight">{t("common.appName")}</h1>
          <p className="mt-1.5 flex items-center gap-1.5 text-xs text-muted-foreground">
            <LockKeyhole className="size-3.5" />
            {t("auth.subtitle")}
          </p>
        </div>

        <Card className="gap-5 p-6 animate-fade-up" style={{ animationDelay: "120ms" }}>
          <div>
            <h2 className="text-base font-semibold tracking-tight">{t("auth.title")}</h2>
            <p className="mt-1 text-xs leading-relaxed text-muted-foreground">{t("auth.hero1")}</p>
          </div>
          <form onSubmit={handleSubmit} className="grid gap-4">
            <div className="grid gap-2">
              <Label htmlFor="password">{t("auth.password")}</Label>
              <Input
                id="password"
                type="password"
                autoComplete="current-password"
                autoFocus
                required
                value={password}
                placeholder={t("auth.passwordPlaceholder")}
                onChange={(event) => setPassword(event.target.value)}
                aria-invalid={error !== ""}
                className="h-10"
              />
              {error ? (
                <p className="text-sm text-destructive animate-fade-in">{error}</p>
              ) : null}
            </div>
            <Button type="submit" size="lg" className="group w-full" disabled={submitting}>
              {submitting ? t("auth.submitting") : t("auth.submit")}
              <ArrowRight className="size-4 transition-transform duration-300 group-hover:translate-x-1" />
            </Button>
          </form>
        </Card>
      </div>
    </div>
  )
}
