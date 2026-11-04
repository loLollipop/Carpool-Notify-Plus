export const maximumExtensionDays = 365
const dayMilliseconds = 24 * 60 * 60 * 1000

function dateTimestamp(value: string) {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(value)) return null
  const timestamp = Date.parse(`${value}T00:00:00Z`)
  return Number.isFinite(timestamp) && new Date(timestamp).toISOString().slice(0, 10) === value
    ? timestamp
    : null
}

function formatTimestamp(timestamp: number) {
  return new Date(timestamp).toISOString().slice(0, 10)
}

export function extensionDateRange(dueDates: string[]) {
  if (dueDates.length === 0) return null
  const timestamps = dueDates.map(dateTimestamp)
  if (timestamps.some((timestamp) => timestamp === null)) return null
  const dates = timestamps as number[]
  const earliest = Math.min(...dates)
  const latest = Math.max(...dates)
  const min = formatTimestamp(latest + dayMilliseconds)
  const max = formatTimestamp(earliest + maximumExtensionDays * dayMilliseconds)
  return { earliestDue: formatTimestamp(earliest), latestDue: formatTimestamp(latest), min, max }
}

export function extensionDaysUntil(dueDate: string, targetDate: string) {
  const due = dateTimestamp(dueDate)
  const target = dateTimestamp(targetDate)
  if (due === null || target === null) return null
  const days = (target - due) / dayMilliseconds
  return days >= 1 && days <= maximumExtensionDays ? days : null
}
