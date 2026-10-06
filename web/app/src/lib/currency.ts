/**
 * Currency formatting utilities for CNY amounts stored in cents.
 */

/**
 * Format cents as CNY with proper localization and 2 decimal places.
 * @param cents - Amount in cents (e.g., 12345 = ¥123.45)
 * @returns Formatted string with ¥ symbol
 */
export function formatCents(cents: number): string {
  return `¥${(cents / 100).toLocaleString("zh-CN", {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  })}`
}

/**
 * Format cents as plain number string with 2 decimals (no currency symbol).
 * Used for exports or when currency symbol is added separately.
 */
export function formatCentsPlain(cents: number): string {
  return (cents / 100).toFixed(2)
}

/**
 * Format cents for chart axes - compact format without trailing zeros.
 * Shows K suffix for thousands.
 */
export function formatCentsAxis(cents: number): string {
  const value = cents / 100
  if (value >= 1000) {
    return `¥${(value / 1000).toFixed(1)}K`
  }
  return `¥${value.toFixed(0)}`
}

/**
 * Format cents only if non-zero, otherwise return empty string.
 * Useful for optional amount fields.
 */
export function formatCentsOptional(cents: number): string {
  return cents > 0 ? formatCentsPlain(cents) : ""
}
