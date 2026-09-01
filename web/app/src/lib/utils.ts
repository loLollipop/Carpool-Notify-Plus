import { clsx, type ClassValue } from "clsx"
import { twMerge } from "tailwind-merge"

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

export function compareISODateStrings(left: string, right: string) {
  if (left === right) return 0
  if (!left) return 1
  if (!right) return -1
  return left.localeCompare(right)
}
