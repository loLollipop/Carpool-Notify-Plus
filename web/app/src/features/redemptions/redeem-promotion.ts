import type { RedeemPageSettings } from "@/api/types"

export type RedeemPromotionSettings = Pick<
  RedeemPageSettings,
  | "promotion_enabled"
  | "promotion_badge"
  | "promotion_text"
  | "promotion_url"
  | "promotion_cta_label"
>

export const DEFAULT_REDEEM_PROMOTION: RedeemPromotionSettings = {
  promotion_enabled: true,
  promotion_badge: "推广",
  promotion_text: "个人自营纯血中转 · 欢迎对接～",
  promotion_url: "https://ikun.fuckoai.bond/",
  promotion_cta_label: "前往体验",
}

// Validate again in the browser: saved settings or a stale server response must
// never turn this public link into executable code or a credential-bearing URL.
export function safePromotionURL(value: string): string | null {
  const trimmed = value.trim()
  const authority = /^https?:\/\/([^/?#]+)/i.exec(trimmed)?.[1]
  if (!authority || new TextEncoder().encode(trimmed).length > 2048 ||
    /[\s\\<>\p{Cc}]/u.test(trimmed) || /%(?![\da-f]{2})/i.test(trimmed) || authority.includes("@")) return null
  try {
    const url = new URL(trimmed)
    if (!url.hostname || url.username || url.password) return null
    if (authority.startsWith("[")) {
      const ipv6 = /^\[[^\]]+\](?::(\d+))?$/u.exec(authority)
      if (!ipv6 || (ipv6[1] && (Number(ipv6[1]) < 1 || Number(ipv6[1]) > 65535))) return null
    } else {
      const hostPort = /^([^:]+)(?::(\d+))?$/u.exec(authority)
      if (!hostPort || (hostPort[2] && (Number(hostPort[2]) < 1 || Number(hostPort[2]) > 65535))) return null
      const host = hostPort[1]
      if (new TextEncoder().encode(host).length > 253 ||
        host.replace(/\.$/u, "").split(".").some((label) =>
          new TextEncoder().encode(label).length > 63 ||
          !/^[\p{L}\p{Nd}](?:[\p{L}\p{Nd}-]*[\p{L}\p{Nd}])?$/u.test(label))) return null
    }
    return url.href
  } catch {
    return null
  }
}

export function promotionIsVisible(settings: RedeemPromotionSettings): boolean {
  return settings.promotion_enabled === true && settings.promotion_text.trim() !== "" &&
    safePromotionURL(settings.promotion_url) !== null
}
