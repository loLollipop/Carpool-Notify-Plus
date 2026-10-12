import { ArrowUpRight } from "lucide-react"

import {
  DEFAULT_REDEEM_PROMOTION,
  promotionIsVisible,
  safePromotionURL,
  type RedeemPromotionSettings,
} from "./redeem-promotion"

export function RedeemPromotionBanner({ settings }: { settings: RedeemPromotionSettings }) {
  if (!promotionIsVisible(settings)) return null

  return (
    <a
      className="redeem-promotion"
      href={safePromotionURL(settings.promotion_url)!}
      target="_blank"
      rel="noopener noreferrer sponsored"
      aria-label={`${settings.promotion_text.trim()}，${settings.promotion_cta_label.trim() || DEFAULT_REDEEM_PROMOTION.promotion_cta_label}（新标签页打开）`}
    >
      <span className="redeem-promotion-badge">
        {settings.promotion_badge.trim() || DEFAULT_REDEEM_PROMOTION.promotion_badge}
      </span>
      <span className="redeem-promotion-copy" title={settings.promotion_text.trim()}>
        {settings.promotion_text.trim()}
      </span>
      <span className="redeem-promotion-cta">
        <span>{settings.promotion_cta_label.trim() || DEFAULT_REDEEM_PROMOTION.promotion_cta_label}</span>
        <ArrowUpRight aria-hidden="true" />
      </span>
    </a>
  )
}
