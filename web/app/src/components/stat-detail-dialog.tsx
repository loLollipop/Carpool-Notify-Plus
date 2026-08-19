import * as React from "react"
import { ChevronLeft, ChevronRight, Search } from "lucide-react"
import { useTranslation } from "react-i18next"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { cn } from "@/lib/utils"

export interface StatDetailItem {
  id: React.Key
  title: string
  subtitle?: string
  meta?: Array<string | null | undefined>
  value?: React.ReactNode
  valueTone?: "default" | "success" | "warning" | "danger"
  searchText?: string
}

export interface StatDetailState {
  title: string
  description?: string
  items: StatDetailItem[]
  emptyText?: string
}

const ITEMS_PER_PAGE = 8

export function StatDetailDialog({
  open,
  onOpenChange,
  detail,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  detail: StatDetailState | null
}) {
  const { t } = useTranslation()
  const [search, setSearch] = React.useState("")
  const [page, setPage] = React.useState(1)
  const returnFocusRef = React.useRef<HTMLElement | null>(null)

  // These dialogs are opened by controlled statistic cards rather than a
  // DialogTrigger, so Radix cannot infer where focus should return. Remember
  // the last focused page control while closed and restore it on close.
  React.useEffect(() => {
    if (open) return
    const rememberFocus = (event: FocusEvent) => {
      const target = event.target
      if (target instanceof HTMLElement && !target.closest('[role="dialog"]')) {
        returnFocusRef.current = target
      }
    }
    document.addEventListener("focusin", rememberFocus, true)
    return () => document.removeEventListener("focusin", rememberFocus, true)
  }, [open])

  const items = React.useMemo(() => detail?.items ?? [], [detail?.items])
  const filteredItems = React.useMemo(() => {
    const query = search.trim().toLocaleLowerCase()
    if (!query) return items
    return items.filter((item) =>
      [item.title, item.subtitle, ...(item.meta ?? []), item.searchText]
        .filter(Boolean)
        .join(" ")
        .toLocaleLowerCase()
        .includes(query),
    )
  }, [items, search])
  const pageCount = Math.max(1, Math.ceil(filteredItems.length / ITEMS_PER_PAGE))
  const safePage = Math.min(page, pageCount)
  const pageItems = filteredItems.slice(
    (safePage - 1) * ITEMS_PER_PAGE,
    safePage * ITEMS_PER_PAGE,
  )

  return (
    <Dialog
      open={open}
      onOpenChange={(nextOpen) => {
        if (!nextOpen) {
          setSearch("")
          setPage(1)
        }
        onOpenChange(nextOpen)
      }}
    >
      <DialogContent
        className="grid max-h-[86vh] grid-rows-[auto_auto_minmax(0,1fr)_auto] gap-0 overflow-hidden p-0 sm:max-w-2xl"
        onCloseAutoFocus={(event) => {
          event.preventDefault()
          returnFocusRef.current?.focus()
        }}
      >
        <DialogHeader className="border-b px-5 py-4 pr-12">
          <div className="flex items-center gap-2">
            <DialogTitle>{detail?.title ?? t("statDetails.title")}</DialogTitle>
            <Badge variant="secondary" className="tabular-nums">
              {t("statDetails.count", { count: items.length })}
            </Badge>
          </div>
          {detail?.description ? (
            <DialogDescription>{detail.description}</DialogDescription>
          ) : null}
        </DialogHeader>

        {items.length > ITEMS_PER_PAGE ? (
          <div className="border-b bg-muted/20 px-5 py-3">
            <div className="relative">
              <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                value={search}
                onChange={(event) => {
                  setSearch(event.target.value)
                  setPage(1)
                }}
                className="h-9 pl-9"
                placeholder={t("statDetails.searchPlaceholder")}
                aria-label={t("statDetails.searchPlaceholder")}
              />
            </div>
          </div>
        ) : (
          <div />
        )}

        <div className="min-h-0 overflow-y-auto overscroll-contain">
          {pageItems.length > 0 ? (
            <div className="divide-y">
              {pageItems.map((item) => (
                <article
                  key={item.id}
                  className="grid min-w-0 gap-3 px-5 py-3.5 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-center"
                >
                  <div className="min-w-0">
                    <div className="truncate text-sm font-semibold" title={item.title}>
                      {item.title}
                    </div>
                    {item.subtitle ? (
                      <div className="mt-0.5 truncate text-xs text-muted-foreground" title={item.subtitle}>
                        {item.subtitle}
                      </div>
                    ) : null}
                    {(item.meta ?? []).filter(Boolean).length > 0 ? (
                      <div className="mt-1.5 flex flex-wrap gap-x-3 gap-y-1 text-[11px] text-muted-foreground">
                        {(item.meta ?? []).filter(Boolean).map((value, index) => (
                          <span key={`${item.id}:meta:${index}`}>{value}</span>
                        ))}
                      </div>
                    ) : null}
                  </div>
                  {item.value !== undefined ? (
                    <div
                      className={cn(
                        "shrink-0 text-sm font-semibold tabular-nums sm:text-right",
                        item.valueTone === "success" && "text-success",
                        item.valueTone === "warning" && "text-gold",
                        item.valueTone === "danger" && "text-destructive",
                      )}
                    >
                      {item.value}
                    </div>
                  ) : null}
                </article>
              ))}
            </div>
          ) : (
            <div className="grid min-h-44 place-items-center px-6 text-center text-sm text-muted-foreground">
              {search ? t("statDetails.searchEmpty") : detail?.emptyText ?? t("statDetails.empty")}
            </div>
          )}
        </div>

        <div className="flex items-center justify-between gap-3 border-t bg-muted/20 px-5 py-3 text-xs text-muted-foreground">
          <span className="tabular-nums">
            {t("statDetails.pageStatus", { page: safePage, pageCount })}
          </span>
          {pageCount > 1 ? (
            <div className="flex items-center gap-1.5">
              <Button
                type="button"
                variant="outline"
                size="icon-sm"
                disabled={safePage <= 1}
                aria-label={t("statDetails.prevPage")}
                onClick={() => setPage((current) => Math.max(1, current - 1))}
              >
                <ChevronLeft />
              </Button>
              <Button
                type="button"
                variant="outline"
                size="icon-sm"
                disabled={safePage >= pageCount}
                aria-label={t("statDetails.nextPage")}
                onClick={() => setPage((current) => Math.min(pageCount, current + 1))}
              >
                <ChevronRight />
              </Button>
            </div>
          ) : null}
        </div>
      </DialogContent>
    </Dialog>
  )
}
