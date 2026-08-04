interface DateParts {
  year: number
  month: number
  day: number
}

const DATE_ONLY_PATTERN = /^(\d{4})-(\d{2})-(\d{2})$/

function parseDateOnly(value: string): DateParts | null {
  const match = DATE_ONLY_PATTERN.exec(value.trim())
  if (!match) return null

  const year = Number(match[1])
  const month = Number(match[2])
  const day = Number(match[3])
  if (!Number.isInteger(year) || month < 1 || month > 12) return null
  if (day < 1 || day > daysInMonth(year, month)) return null
  return { year, month, day }
}

function daysInMonth(year: number, month: number) {
  return new Date(Date.UTC(year, month, 0)).getUTCDate()
}

function compareDateParts(left: DateParts, right: DateParts) {
  if (left.year !== right.year) return left.year - right.year
  if (left.month !== right.month) return left.month - right.month
  return left.day - right.day
}

function addMonth(parts: DateParts): DateParts {
  if (parts.month === 12) {
    return { ...parts, year: parts.year + 1, month: 1 }
  }
  return { ...parts, month: parts.month + 1 }
}

function monthlyAnniversary(year: number, month: number, openedDay: number): DateParts {
  return {
    year,
    month,
    day: Math.min(openedDay, daysInMonth(year, month)),
  }
}

function formatDateParts(parts: DateParts) {
  const year = String(parts.year).padStart(4, "0")
  const month = String(parts.month).padStart(2, "0")
  const day = String(parts.day).padStart(2, "0")
  return `${year}-${month}-${day}`
}

export function todayShanghai(): string {
  return new Date().toLocaleDateString("en-CA", { timeZone: "Asia/Shanghai" })
}

export function getNextMonthlyRenewalDate(openedAt: string, fromDate = todayShanghai()) {
  const opened = parseDateOnly(openedAt)
  const from = parseDateOnly(fromDate)
  if (!opened || !from) return ""

  const base = compareDateParts(from, opened) > 0 ? from : opened
  let candidate = monthlyAnniversary(base.year, base.month, opened.day)

  while (
    compareDateParts(candidate, opened) <= 0 ||
    compareDateParts(candidate, from) < 0
  ) {
    const nextMonth = addMonth(candidate)
    candidate = monthlyAnniversary(nextMonth.year, nextMonth.month, opened.day)
  }

  return formatDateParts(candidate)
}
