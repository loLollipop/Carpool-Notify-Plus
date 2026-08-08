import * as React from "react"

import {
  getSandboxModeState,
  subscribeSandboxMode,
} from "@/lib/sandbox-mode"

export function useSandboxMode() {
  return React.useSyncExternalStore(
    subscribeSandboxMode,
    getSandboxModeState,
    getSandboxModeState,
  )
}
