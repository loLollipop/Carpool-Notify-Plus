import * as React from "react"
import { useTranslation } from "react-i18next"

import { BrandIcon } from "@/components/brand"

export function AuthFrame({ children }: { children: React.ReactNode }) {
  const { t } = useTranslation()

  return (
    <div className="auth-frame">
      <aside className="auth-brand-panel" aria-label={t("common.appName")}>
        <div className="auth-brand-lockup">
          <BrandIcon className="size-11 rounded-xl shadow-none" />
          <span className="min-w-0">
            <strong className="auth-brand-name">{t("common.appName")}</strong>
            <span className="auth-brand-caption">{t("auth.consoleLabel")}</span>
          </span>
        </div>

        <div className="auth-wordmark" aria-hidden="true">
          <span>CARPOOL</span>
          <span className="is-muted">NOTIFY</span>
          <span className="is-accent">PLUS</span>
        </div>

        <div className="auth-brand-rail" aria-hidden="true">
          <span className="auth-brand-rail-line" />
          <span>PRIVATE OPERATIONS</span>
          <span className="auth-brand-rail-dot" />
        </div>
      </aside>

      <section className="auth-content">
        <div className="auth-content-logo" aria-hidden="true">
          <BrandIcon className="size-[4.5rem] rounded-[1.15rem] shadow-[0_18px_32px_rgba(15,118,110,0.22)]" />
        </div>
        {children}
      </section>
    </div>
  )
}

export function AuthLoadingScreen() {
  const { t } = useTranslation()

  return (
    <AuthFrame>
      <div className="auth-loading-panel" role="status" aria-live="polite" aria-busy="true">
        <p className="auth-loading-title">{t("auth.adminEntry")}</p>
        <p className="auth-loading-status">{t("common.loading")}</p>
        <span className="auth-loading-line" aria-hidden="true">
          <span />
        </span>
      </div>
    </AuthFrame>
  )
}
