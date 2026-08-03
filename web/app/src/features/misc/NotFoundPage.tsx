import { Link } from "react-router-dom"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"

export function NotFoundPage() {
  const { t } = useTranslation()

  return (
    <div className="flex flex-col items-center justify-center py-24 text-center animate-fade-up">
      <div className="display-numeral text-8xl font-semibold text-brand">404</div>
      <h1 className="mt-6 text-xl font-semibold">{t("notFound.title")}</h1>
      <p className="mt-2 text-sm text-muted-foreground">{t("notFound.desc")}</p>
      <Button asChild className="mt-8">
        <Link to="/">{t("notFound.backHome")}</Link>
      </Button>
    </div>
  )
}
