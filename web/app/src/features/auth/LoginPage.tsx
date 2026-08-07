import * as React from "react"
import { useLocation, useNavigate, Navigate } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { ArrowRight, LockKeyhole } from "lucide-react"

import { login } from "@/api/endpoints"
import { BrandIcon } from "@/components/brand"
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
    <div className="grid min-h-dvh bg-background lg:grid-cols-[minmax(320px,0.8fr)_minmax(560px,1.2fr)]">
      <aside className="relative hidden overflow-hidden border-r border-[var(--login-panel-border)] bg-[var(--login-panel)] p-10 text-[var(--login-panel-foreground)] lg:flex lg:flex-col lg:justify-between xl:p-14">
        <div className="flex items-center gap-3 animate-fade-up">
          <BrandIcon className="size-10" />
          <div>
            <h1 className="text-lg font-semibold">{t("common.appName")}</h1>
            <p className="mt-1 text-xs text-[var(--login-panel-muted)]">Workspace Console</p>
          </div>
        </div>

        <div className="max-w-md animate-fade-up [animation-delay:90ms]">
          <div className="mb-7 flex items-center gap-2 text-xs font-medium text-[var(--login-panel-muted)]">
            <LockKeyhole className="size-3.5" />
            {t("auth.subtitle")}
          </div>
          <h2 className="text-4xl font-semibold leading-[1.18] xl:text-5xl">
            {t("auth.title")}
          </h2>
          <p className="mt-5 max-w-sm text-sm leading-7 text-[var(--login-panel-muted)]">
            {t("auth.hero1")}
          </p>
        </div>

        <div className="grid gap-3 opacity-60" aria-hidden="true">
          {Array.from({ length: 4 }).map((_, index) => (
            <span
              key={index}
              className="block h-px bg-[var(--login-panel-border)]"
              style={{ width: `${100 - index * 12}%` }}
            />
          ))}
        </div>
      </aside>

      <main className="grid min-h-dvh place-items-center px-4 py-10 sm:px-8">
        <div className="w-full max-w-[420px]">
          <div className="mb-7 flex items-center gap-3 lg:hidden animate-fade-up">
            <BrandIcon className="size-10" />
            <div>
              <h1 className="text-base font-semibold">{t("common.appName")}</h1>
              <p className="mt-1 flex items-center gap-1.5 text-xs text-muted-foreground">
                <LockKeyhole className="size-3.5" />
                {t("auth.subtitle")}
              </p>
            </div>
          </div>

          <Card className="relative gap-6 overflow-hidden p-6 animate-fade-up sm:p-8" style={{ animationDelay: "120ms" }}>
            <div>
              <div className="mb-4 grid size-10 place-items-center rounded-md bg-brand/10 text-brand">
                <LockKeyhole className="size-5" />
              </div>
              <h2 className="text-xl font-semibold">{t("auth.title")}</h2>
              <p className="mt-2 text-sm leading-6 text-muted-foreground">{t("auth.hero1")}</p>
            </div>
            <form onSubmit={handleSubmit} className="grid gap-5">
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
                  className="h-11"
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
      </main>
    </div>
  )
}
