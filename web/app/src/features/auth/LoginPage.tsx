import * as React from "react"
import { useLocation, useNavigate, Navigate } from "react-router-dom"
import { useTranslation } from "react-i18next"
import {
  CalendarClock,
  Database,
  Eye,
  EyeOff,
  LoaderCircle,
  LockKeyhole,
  ReceiptText,
  UsersRound,
} from "lucide-react"

import { login } from "@/api/endpoints"
import { BrandIcon } from "@/components/brand"
import { Button } from "@/components/ui/button"
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

  const ambientSignals = [
    { key: "users", label: t("nav.users"), icon: UsersRound, position: "is-top-left" },
    { key: "bills", label: t("nav.bills"), icon: ReceiptText, position: "is-bottom-left" },
    { key: "accounts", label: t("nav.accounts"), icon: Database, position: "is-top-right" },
    { key: "calendar", label: t("nav.calendar"), icon: CalendarClock, position: "is-bottom-right" },
  ]

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
    <div className="login-stage relative flex min-h-dvh items-center justify-center overflow-clip px-4 py-8 sm:px-8">
      <span className="login-stage-glow is-top" aria-hidden="true" />
      <span className="login-stage-glow is-bottom" aria-hidden="true" />

      <div className="login-ambient-network" aria-hidden="true">
        <svg
          className="login-network-map"
          viewBox="0 0 1600 900"
          preserveAspectRatio="none"
          focusable="false"
        >
          <g className="login-network-orbits">
            <ellipse cx="800" cy="450" rx="570" ry="350" />
            <ellipse cx="800" cy="450" rx="660" ry="410" />
          </g>

          <g className="login-network-lines">
            <path d="M0 188 H150 C238 188 252 282 354 282 H430" />
            <path d="M0 716 H170 C258 716 272 626 372 626 H432" />
            <path d="M1600 176 H1450 C1360 176 1348 270 1244 270 H1170" />
            <path d="M1600 724 H1434 C1350 724 1334 632 1238 632 H1168" />
          </g>

          <g className="login-network-lines is-dashed">
            <path d="M74 86 H300 L356 142" />
            <path d="M1526 86 H1300 L1244 142" />
            <path d="M74 814 H300 L356 758" />
            <path d="M1526 814 H1300 L1244 758" />
          </g>

          <g className="login-network-packets">
            <path pathLength="1" d="M0 188 H150 C238 188 252 282 354 282 H430" />
            <path pathLength="1" d="M0 716 H170 C258 716 272 626 372 626 H432" />
            <path pathLength="1" d="M1600 176 H1450 C1360 176 1348 270 1244 270 H1170" />
            <path pathLength="1" d="M1600 724 H1434 C1350 724 1334 632 1238 632 H1168" />
          </g>

          <g className="login-network-nodes">
            <circle cx="150" cy="188" r="4" />
            <circle cx="354" cy="282" r="4" />
            <circle cx="170" cy="716" r="4" />
            <circle cx="372" cy="626" r="4" />
            <circle cx="1450" cy="176" r="4" />
            <circle cx="1244" cy="270" r="4" />
            <circle cx="1434" cy="724" r="4" />
            <circle cx="1238" cy="632" r="4" />
          </g>
        </svg>

        {ambientSignals.map(({ key, label, icon: Icon, position }) => (
          <span key={key} className={`login-ambient-chip ${position}`}>
            <span className="login-ambient-chip-icon">
              <Icon className="size-3.5" />
            </span>
            <span>{label}</span>
            <span className="login-ambient-chip-pulse" />
          </span>
        ))}
      </div>

      <main className="relative z-10 grid w-full max-w-[820px] overflow-hidden rounded-2xl border border-border/80 bg-card shadow-lift animate-rise lg:min-h-[430px] lg:grid-cols-[300px_minmax(0,1fr)]">
        <aside className="login-surface relative hidden overflow-hidden p-8 text-[var(--login-panel-foreground)] lg:flex lg:flex-col">
          <div className="relative z-10 flex items-center gap-3">
            <BrandIcon className="size-11" />
            <div className="min-w-0">
              <h1 className="truncate text-base font-semibold">{t("common.appName")}</h1>
              <p className="mt-1 text-xs text-[var(--login-panel-muted)]">
                {t("auth.consoleLabel")}
              </p>
            </div>
          </div>

          <div className="relative flex flex-1 items-center justify-center" aria-hidden="true">
            <span className="absolute size-48 rounded-full border border-white/[0.07]" />
            <span className="absolute size-32 rounded-full border border-white/[0.10]" />
            <span className="absolute h-px w-56 bg-gradient-to-r from-transparent via-white/15 to-transparent" />
            <span className="absolute h-56 w-px bg-gradient-to-b from-transparent via-white/10 to-transparent" />
            <span className="absolute left-[19%] top-[32%] size-1.5 rounded-full bg-gold" />
            <span className="absolute bottom-[30%] right-[18%] size-1 rounded-full bg-brand" />
            <BrandIcon className="relative z-10 size-20 rounded-2xl shadow-[0_18px_44px_rgba(0,0,0,0.25)]" />
          </div>
        </aside>

        <section className="flex min-h-[420px] flex-col justify-center p-6 sm:p-10 lg:min-h-0 lg:px-12">
          <div className="mb-10 flex items-center gap-3 lg:hidden">
            <BrandIcon className="size-11" />
            <div className="min-w-0">
              <h1 className="truncate text-base font-semibold">{t("common.appName")}</h1>
              <p className="mt-1 text-xs text-muted-foreground">{t("auth.consoleLabel")}</p>
            </div>
          </div>

          <div className="mb-7 flex items-center gap-3">
            <span className="grid size-10 place-items-center rounded-lg border border-brand/15 bg-brand/[0.08] text-brand">
              <LockKeyhole className="size-[18px]" />
            </span>
            <h2 className="text-2xl font-semibold tracking-tight">{t("auth.adminEntry")}</h2>
          </div>

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
                  className="h-12 bg-background/60 pl-10 pr-11"
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
            <Button type="submit" size="lg" className="h-12 w-full" disabled={submitting}>
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
        </section>
      </main>
    </div>
  )
}
