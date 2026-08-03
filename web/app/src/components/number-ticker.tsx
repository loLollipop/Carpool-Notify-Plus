import * as React from "react"

/**
 * Animates the leading numeric portion of a value string (e.g. "468.00", "12", "31.5%")
 * from 0 to its final value, preserving prefix/suffix and decimal places.
 */
export function NumberTicker({
  value,
  duration = 800,
  className,
}: {
  value: string | number
  duration?: number
  className?: string
}) {
  const raw = String(value)
  const [display, setDisplay] = React.useState(raw)

  React.useEffect(() => {
    const match = raw.match(/^([^0-9-]*)(-?\d+(?:\.\d+)?)(.*)$/)
    const skipAnimation =
      !match || window.matchMedia("(prefers-reduced-motion: reduce)").matches

    let frame: number
    if (skipAnimation) {
      frame = requestAnimationFrame(() => setDisplay(raw))
      return () => cancelAnimationFrame(frame)
    }

    const [, prefix, numberText, suffix] = match
    const target = parseFloat(numberText)
    const decimals = numberText.includes(".") ? numberText.split(".")[1].length : 0
    const start = performance.now()

    const tick = (now: number) => {
      const progress = Math.min((now - start) / duration, 1)
      const eased = 1 - Math.pow(1 - progress, 3)
      const current = target * eased
      setDisplay(`${prefix}${current.toFixed(decimals)}${suffix}`)
      if (progress < 1) {
        frame = requestAnimationFrame(tick)
      }
    }
    frame = requestAnimationFrame(tick)
    return () => cancelAnimationFrame(frame)
  }, [raw, duration])

  return (
    <span className={className} style={{ fontVariantNumeric: "tabular-nums" }}>
      {display}
    </span>
  )
}
