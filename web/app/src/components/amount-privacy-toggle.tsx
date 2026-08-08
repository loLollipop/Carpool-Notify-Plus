import { useTranslation } from "react-i18next"
import { Eye, EyeOff } from "lucide-react"

import { Button } from "@/components/ui/button"

export function AmountPrivacyToggle({
  amountsHidden,
  onToggle,
}: {
  amountsHidden: boolean
  onToggle: () => void
}) {
  const { t } = useTranslation()
  const label = amountsHidden ? t("privacy.showAmounts") : t("privacy.hideAmounts")

  return (
    <Button
      type="button"
      variant="ghost"
      size="icon-sm"
      className="text-muted-foreground hover:bg-accent hover:text-foreground"
      aria-label={label}
      aria-pressed={amountsHidden}
      title={label}
      onClick={onToggle}
    >
      {amountsHidden ? <EyeOff className="size-4" /> : <Eye className="size-4" />}
    </Button>
  )
}
