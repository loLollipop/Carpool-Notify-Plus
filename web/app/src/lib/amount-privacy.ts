export const AMOUNT_MASK = "¥••••"
export const VALUE_MASK = "••••"

export function maskAmount(hidden: boolean, value: string | number) {
  return hidden ? AMOUNT_MASK : value
}
