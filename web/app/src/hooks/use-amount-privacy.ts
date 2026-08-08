import * as React from "react"

const STORAGE_KEY = "carpool-notify:amounts-hidden"

function readStoredPreference() {
  try {
    return window.localStorage.getItem(STORAGE_KEY) === "true"
  } catch {
    return false
  }
}

export function useAmountPrivacy() {
  const [amountsHidden, setAmountsHidden] = React.useState(readStoredPreference)

  const toggleAmounts = React.useCallback(() => {
    setAmountsHidden((current) => {
      const next = !current
      try {
        window.localStorage.setItem(STORAGE_KEY, String(next))
      } catch {
        // Privacy mode still works for the current page when storage is unavailable.
      }
      return next
    })
  }, [])

  return { amountsHidden, toggleAmounts }
}
