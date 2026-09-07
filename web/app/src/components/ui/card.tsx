import * as React from "react"

import { cn } from "@/lib/utils"

function Card({ className, ...props }: React.ComponentProps<"div">) {
  return (
    <div
      data-slot="card"
      className={cn(
        "bg-card text-card-foreground flex flex-col gap-5 rounded-xl border py-5 shadow-card",
        className,
      )}
      {...props}
    />
  )
}

export { Card }
