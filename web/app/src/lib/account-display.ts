export function formatAccountLabel(serial: number, name: string, fallback = "-") {
  const label = name.trim()
  if (serial > 0) return label ? `${serial} · ${label}` : String(serial)
  return label || fallback
}

export function accountSerialSearchTerms(serial: number) {
  if (serial <= 0) return []
  return [String(serial), `${serial}号`, `#${serial}`, `no.${serial}`]
}
