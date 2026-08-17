import * as React from "react"
import { useLocation, useNavigate, Navigate } from "react-router-dom"
import { useTranslation } from "react-i18next"
import {
  ArrowRight,
  Eye,
  EyeOff,
  LoaderCircle,
  LockKeyhole,
  ShieldCheck,
} from "lucide-react"

import { login } from "@/api/endpoints"
import { BrandIcon } from "@/components/brand"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { useAuth } from "./auth-state"

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
    <div className="grid min-h-dvh bg-background lg:grid-cols-[minmax(400px,0.9fr)_minmax(560px,1.1fr)]">
      <aside className="login-surface relative hidden overflow-hidden border-r border-[var(--login-panel-border)] p-10 text-[var(--login-panel-foreground)] lg:flex lg:flex-col lg:justify-between xl:p-14">
        <BrandIcon
          className="pointer-events-none absolute -bottom-24 -right-24 size-80 opacity-[0.055] shadow-none"
        />

        <div className="relative flex items-center gap-3 animate-fade-up">
          <BrandIcon className="size-11" />
          <div className="min-w-0">
            <h1 className="truncate text-lg font-semibold">{t("common.appName")}</h1>
            <p className="mt-1 text-xs text-[var(--login-panel-muted)]">
              {t("auth.consoleLabel")}
            </p>
          </div>
        </div>

        <div className="relative max-w-[470px] animate-fade-up [animation-delay:90ms]">
          <div className="mb-6 flex items-center gap-3 text-[11px] font-semibold text-[var(--login-panel-muted)]">
            <span className="h-px w-8 bg-gold" aria-hidden="true" />
            {t("auth.eyebrow")}
          </div>
          <h2 className="max-w-[440px] text-[36px] font-semibold leading-[1.22] xl:text-[44px]">
            {t("auth.heroTitle")}
          </h2>
          <p className="mt-5 max-w-md text-[15px] leading-7 text-[var(--login-panel-muted)]">
            {t("auth.heroDescription")}
          </p>
          <p className="mt-8 border-l-2 border-gold pl-4 text-sm leading-6 text-[var(--login-panel-foreground)]/85">
            {t("auth.heroNote")}
          </p>
        </div>

        <div className="relative flex items-center justify-between border-t border-[var(--login-panel-border)] pt-5 text-[11px] text-[var(--login-panel-muted)]">
          <span className="font-semibold">{t("auth.privateConsole")}</span>
          <span className="flex items-center gap-2">
            <span className="size-1.5 rounded-full bg-gold" aria-hidden="true" />
            {t("auth.secureAccess")}
          </span>
        </div>
      </aside>

      <main className="relative flex min-h-dvh items-center justify-center px-4 py-8 sm:px-8 lg:px-12">
        <div className="absolute right-8 top-8 hidden items-center gap-2 text-xs text-muted-foreground lg:flex">
          <ShieldCheck className="size-4 text-brand" />
          {t("auth.secureAccess")}
        </div>

        <div className="w-full max-w-[440px]">
          <div className="mb-8 flex items-center gap-3 lg:hidden animate-fade-up">
            <BrandIcon className="size-11" />
            <div className="min-w-0">
              <h1 className="truncate text-base font-semibold">{t("common.appName")}</h1>
              <p className="mt-1 text-xs text-muted-foreground">{t("auth.consoleLabel")}</p>
            </div>
          </div>

          <Card
            className="relative gap-0 overflow-hidden border-border/90 p-6 shadow-lift animate-fade-up sm:p-9"
            style={{ animationDelay: "120ms" }}
          >
            <div>
              <div className="mb-5 flex items-center gap-2 text-xs font-medium text-brand">
                <span className="grid size-8 place-items-center rounded-md bg-brand/10">
                  <LockKeyhole className="size-4" />
                </span>
                {t("auth.adminEntry")}
              </div>
              <h2 className="text-2xl font-semibold">{t("auth.welcome")}</h2>
              <p className="mt-2 text-sm leading-6 text-muted-foreground">
                {t("auth.welcomeDescription")}
              </p>
            </div>

            <form onSubmit={handleSubmit} className="mt-8 grid gap-5">
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
                    className="h-12 pl-10 pr-11"
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
              <Button type="submit" size="lg" className="group h-12 w-full" disabled={submitting}>
                {submitting ? (
                  <>
                    <LoaderCircle className="size-4 animate-spin" />
                    {t("auth.submitting")}
                  </>
                ) : (
                  <>
                    {t("auth.submit")}
                    <ArrowRight className="size-4 transition-transform duration-300 group-hover:translate-x-1" />
                  </>
                )}
              </Button>
            </form>

            <div className="mt-7 flex items-center gap-2 border-t pt-5 text-xs text-muted-foreground">
              <ShieldCheck className="size-4 shrink-0 text-brand" />
              {t("auth.restrictedAccess")}
            </div>
          </Card>
        </div>
      </main>
    </div>
  )
}
