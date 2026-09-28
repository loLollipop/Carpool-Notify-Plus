import type { AccountSpaceRole } from "@/api/types"

export function formatAccountName(name: string, role?: AccountSpaceRole, fallback = "-") {
  const label = name.trim()
  const resolved = label || fallback
  return role === "secondary" && resolved ? `${resolved} · 副` : resolved
}

export function formatAccountLabel(
  serial: number,
  name: string,
  fallback = "-",
  role?: AccountSpaceRole,
) {
  const label = formatAccountName(name, role, fallback)
  if (serial > 0) return label ? `${serial} · ${label}` : String(serial)
  return label
}

export function accountSerialSearchTerms(serial: number) {
  if (serial <= 0) return []
  return [String(serial), `${serial}号`, `#${serial}`, `no.${serial}`]
}
