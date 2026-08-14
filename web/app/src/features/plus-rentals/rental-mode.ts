export const ONE_MONTH_RENTAL_CRON = "one-time:30d"

export function isOneMonthRentalCron(cronExpr: string) {
  return cronExpr.trim().toLowerCase() === ONE_MONTH_RENTAL_CRON
}

export function oneMonthRentalEndDate(startDate: string) {
  const matched = /^(\d{4})-(\d{2})-(\d{2})$/.exec(startDate.trim())
  if (!matched) return ""
  const date = new Date(
    Date.UTC(Number(matched[1]), Number(matched[2]) - 1, Number(matched[3])),
  )
  date.setUTCDate(date.getUTCDate() + 30)
  return date.toISOString().slice(0, 10)
}
