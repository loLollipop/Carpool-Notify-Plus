import * as React from "react"
import { ChevronLeft, ChevronRight, Megaphone, Pencil, Trash2 } from "lucide-react"
import { useTranslation } from "react-i18next"

import {
  createOperatingExpense,
  deleteOperatingExpense,
  updateOperatingExpense,
} from "@/api/endpoints"
import { useAppMutation } from "@/api/mutations"
import type {
  BillsSummary,
  OperatingExpenseInput,
  OperatingExpenseView,
} from "@/api/types"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { maskAmount } from "@/lib/amount-privacy"

const EXPENSES_PER_PAGE = 6

function todayInShanghai() {
  return new Intl.DateTimeFormat("en-CA", {
    timeZone: "Asia/Shanghai",
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
  }).format(new Date())
}

export function OperatingExpenseDialog({
  open,
  onOpenChange,
  expenses,
  summary,
  amountsHidden,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  expenses: OperatingExpenseView[]
  summary: BillsSummary
  amountsHidden: boolean
}) {
  const { t } = useTranslation()
  const today = todayInShanghai()
  const [editingID, setEditingID] = React.useState<number | null>(null)
  const [occurredOn, setOccurredOn] = React.useState(today)
  const [amountYuan, setAmountYuan] = React.useState("")
  const [note, setNote] = React.useState("")
  const [page, setPage] = React.useState(1)
  const [deleteTarget, setDeleteTarget] = React.useState<OperatingExpenseView | null>(null)

  const resetForm = React.useCallback(() => {
    setEditingID(null)
    setOccurredOn(todayInShanghai())
    setAmountYuan("")
    setNote("")
  }, [])

  const saveMutation = useAppMutation(
    (input: OperatingExpenseInput) =>
      editingID === null
        ? createOperatingExpense(input)
        : updateOperatingExpense(editingID, input),
    { onSuccess: resetForm },
  )
  const deleteMutation = useAppMutation((id: number) => deleteOperatingExpense(id), {
    onSuccess: () => {
      if (deleteTarget?.id === editingID) resetForm()
      setDeleteTarget(null)
    },
  })

  const pageCount = Math.max(1, Math.ceil(expenses.length / EXPENSES_PER_PAGE))
  const safePage = Math.min(page, pageCount)
  const pagedExpenses = expenses.slice(
    (safePage - 1) * EXPENSES_PER_PAGE,
    safePage * EXPENSES_PER_PAGE,
  )

  const startEdit = (expense: OperatingExpenseView) => {
    setEditingID(expense.id)
    setOccurredOn(expense.occurred_on)
    setAmountYuan(expense.amount_yuan)
    setNote(expense.note)
  }

  const submit = (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    saveMutation.mutate({
      occurred_on: occurredOn,
      amount_yuan: amountYuan.trim(),
      note: note.trim(),
    })
  }

  const handleOpenChange = (nextOpen: boolean) => {
    if (!nextOpen) {
      resetForm()
      setPage(1)
    }
    onOpenChange(nextOpen)
  }

  return (
    <>
      <Dialog open={open} onOpenChange={handleOpenChange}>
        <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-3xl">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <span className="grid size-8 place-items-center rounded-md bg-brand/10 text-brand">
                <Megaphone className="size-4" />
              </span>
              {t("bills.promotion.title")}
            </DialogTitle>
            <DialogDescription>{t("bills.promotion.description")}</DialogDescription>
          </DialogHeader>

          <div className="grid grid-cols-3 gap-2">
            {[
              [t("bills.promotion.total"), summary.operating_expense_yuan],
              [t("bills.promotion.thisMonth"), summary.this_month_operating_expense_yuan],
              [t("bills.promotion.monthlyAverage"), summary.operating_expense_monthly_average_yuan],
            ].map(([label, value]) => (
              <div key={label} className="rounded-lg border bg-muted/25 px-3 py-3">
                <div className="text-[11px] text-muted-foreground">{label}</div>
                <div className="display-numeral mt-1.5 text-lg leading-none text-foreground">
                  {maskAmount(amountsHidden, `¥${value}`)}
                </div>
              </div>
            ))}
          </div>

          <form onSubmit={submit} className="rounded-xl border bg-card p-4">
            <div className="mb-3 flex items-center justify-between gap-3">
              <h3 className="text-sm font-semibold">
                {t(editingID === null ? "bills.promotion.addTitle" : "bills.promotion.editTitle")}
              </h3>
              {editingID !== null ? (
                <Button type="button" variant="ghost" size="sm" onClick={resetForm}>
                  {t("common.cancel")}
                </Button>
              ) : null}
            </div>
            <div className="grid gap-4 sm:grid-cols-[150px_150px_minmax(0,1fr)]">
              <div className="grid gap-1.5">
                <Label htmlFor="promotion-date">{t("bills.promotion.date")}</Label>
                <Input
                  id="promotion-date"
                  type="date"
                  value={occurredOn}
                  max={today}
                  required
                  onChange={(event) => setOccurredOn(event.target.value)}
                />
              </div>
              <div className="grid gap-1.5">
                <Label htmlFor="promotion-amount">{t("bills.promotion.amount")}</Label>
                <Input
                  id="promotion-amount"
                  inputMode="decimal"
                  autoComplete="off"
                  placeholder="0.00"
                  value={amountYuan}
                  required
                  onChange={(event) => setAmountYuan(event.target.value)}
                />
              </div>
              <div className="grid gap-1.5">
                <Label htmlFor="promotion-note">{t("bills.promotion.note")}</Label>
                <Textarea
                  id="promotion-note"
                  rows={1}
                  className="min-h-9 resize-none"
                  placeholder={t("bills.promotion.notePlaceholder")}
                  value={note}
                  onChange={(event) => setNote(event.target.value)}
                />
              </div>
            </div>
            <div className="mt-3 flex justify-end">
              <Button type="submit" size="sm" disabled={saveMutation.isPending}>
                {t(editingID === null ? "bills.promotion.addAction" : "common.save")}
              </Button>
            </div>
          </form>

          <div>
            <div className="mb-2 flex items-center justify-between">
              <h3 className="text-sm font-semibold">{t("bills.promotion.history")}</h3>
              <span className="text-xs text-muted-foreground">
                {t("bills.promotion.recordCount", { count: expenses.length })}
              </span>
            </div>
            {expenses.length === 0 ? (
              <div className="rounded-xl border border-dashed py-8 text-center text-sm text-muted-foreground">
                {t("bills.promotion.empty")}
              </div>
            ) : (
              <div className="overflow-hidden rounded-xl border">
                {pagedExpenses.map((expense, index) => (
                  <div
                    key={expense.id}
                    className={`flex items-center gap-3 px-3 py-3 ${index > 0 ? "border-t" : ""}`}
                  >
                    <div className="grid size-9 shrink-0 place-items-center rounded-md bg-brand/8 text-brand">
                      <Megaphone className="size-4" />
                    </div>
                    <div className="min-w-0 flex-1">
                      <div className="flex flex-wrap items-center gap-x-2 gap-y-0.5">
                        <span className="text-sm font-medium">
                          {t("bills.promotion.categoryLabel")}
                        </span>
                        <span className="text-xs tabular-nums text-muted-foreground">
                          {expense.occurred_on}
                        </span>
                      </div>
                      <p className="truncate text-xs text-muted-foreground">
                        {expense.note || t("bills.promotion.noNote")}
                      </p>
                    </div>
                    <div className="display-numeral shrink-0 text-sm text-warning">
                      {maskAmount(amountsHidden, `-¥${expense.amount_yuan}`)}
                    </div>
                    <div className="flex shrink-0 items-center gap-1">
                      <Button
                        type="button"
                        variant="ghost"
                        size="icon-sm"
                        aria-label={t("common.edit")}
                        onClick={() => startEdit(expense)}
                      >
                        <Pencil />
                      </Button>
                      <Button
                        type="button"
                        variant="ghost"
                        size="icon-sm"
                        className="text-destructive hover:text-destructive"
                        aria-label={t("common.delete")}
                        onClick={() => setDeleteTarget(expense)}
                      >
                        <Trash2 />
                      </Button>
                    </div>
                  </div>
                ))}
                {pageCount > 1 ? (
                  <div className="flex items-center justify-between border-t px-3 py-2 text-xs text-muted-foreground">
                    <span>{t("bills.pageStatus", { page: safePage, pageCount })}</span>
                    <div className="flex gap-1">
                      <Button
                        type="button"
                        variant="outline"
                        size="icon-sm"
                        disabled={safePage <= 1}
                        aria-label={t("cards.prevPage")}
                        onClick={() => setPage(safePage - 1)}
                      >
                        <ChevronLeft />
                      </Button>
                      <Button
                        type="button"
                        variant="outline"
                        size="icon-sm"
                        disabled={safePage >= pageCount}
                        aria-label={t("cards.nextPage")}
                        onClick={() => setPage(safePage + 1)}
                      >
                        <ChevronRight />
                      </Button>
                    </div>
                  </div>
                ) : null}
              </div>
            )}
          </div>

          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => handleOpenChange(false)}>
              {t("common.close")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        open={deleteTarget !== null}
        onOpenChange={(nextOpen) => {
          if (!nextOpen) setDeleteTarget(null)
        }}
        title={t("bills.promotion.deleteTitle")}
        description={t("bills.promotion.deleteDescription")}
        actionLabel={t("common.delete")}
        destructive
        pending={deleteMutation.isPending}
        onConfirm={() => {
          if (deleteTarget) deleteMutation.mutate(deleteTarget.id)
        }}
      />
    </>
  )
}
