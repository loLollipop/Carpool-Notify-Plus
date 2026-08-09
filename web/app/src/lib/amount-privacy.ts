export const AMOUNT_MASK = "¥****"
export const VALUE_MASK = "****"

export function maskAmount(hidden: boolean, value: string | number) {
  return hidden ? AMOUNT_MASK : value
}

export function maskValue(hidden: boolean, value: string | number) {
  return hidden ? VALUE_MASK : value
}
