import * as React from "react"
import { Link } from "react-router-dom"
import {
  Ban,
  CalendarClock,
  CheckCircle2,
  ChevronLeft,
  ChevronRight,
  Clock3,
  Copy,
  KeyRound,
  Mail,
  MessageCircle,
  Plus,
  RotateCcw,
  Search,
  Send,
  TicketCheck,
  Trash2,
} from "lucide-react"
import { toast } from "sonner"

import {
  deleteRedemptionCode,
  disableRedemptionCode,
  enableRedemptionCode,
  generateRedemptionCodes,
  inviteRedemption,
  rejectRedemption,
} from "@/api/endpoints"
import { useAppMutation } from "@/api/mutations"
import { useAccountOptions, useRedemptionCodes, useRedemptions } from "@/api/queries"
import type {
  AccountOption,
  RedemptionApplicationView,
  RedemptionCodeStatusValue,
  RedemptionCodeView,
  SeatOption,
} from "@/api/types"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { PageHeader } from "@/components/page-header"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { Checkbox } from "@/components/ui/checkbox"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Skeleton } from "@/components/ui/skeleton"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Textarea } from "@/components/ui/textarea"
import { cn } from "@/lib/utils"
import { todayShanghai } from "@/features/subscriptions/subscription-prefill"

type RedemptionFilter = "pending" | "invited" | "rejected" | "all"
type RedemptionSection = "applications" | "codes"

interface SelectableSeat {
  account: AccountOption
  seat: SeatOption
}

const APPLICATIONS_PER_PAGE = 1
const CODE_PAGE_SIZE = 4
const MONEY_PATTERN = /^\d+(\.\d{1,2})?$/
const CYCLE_PRESETS = [
  { label: "月付", cron: "interval:30d" },
  { label: "季付", cron: "interval:90d" },
  { label: "半年", cron: "interval:180d" },
  { label: "一年", cron: "interval:365d" },
] as const
const OFFSET_OPTIONS = [1, 2, 3, 5, 7, 14]
const EMPTY_REDEMPTIONS: RedemptionApplicationView[] = []
const EMPTY_CODES: RedemptionCodeView[] = []

function buildSeatOptions(accounts: AccountOption[]): SelectableSeat[] {
  return accounts.flatMap((account) =>
    (account.seats ?? []).map((seat) => ({
      account,
      seat,
    })),
  )
}

function statusBadge(status: RedemptionApplicationView["application"]["status"]) {
  if (status === "invited") {
    return (
      <Badge variant="success">
        <CheckCircle2 className="size-3.5" />
        已邀请
      </Badge>
    )
  }
  if (status === "rejected") {
    return (
      <Badge variant="destructive">
        <Ban className="size-3.5" />
        已驳回
      </Badge>
    )
  }
  return (
    <Badge variant="brand">
      <Clock3 className="size-3.5" />
      待处理
    </Badge>
  )
}

function redemptionCodeBadge(status: RedemptionCodeStatusValue) {
  if (status === "unused") {
    return (
      <Badge variant="success">
        <CheckCircle2 className="size-3.5" />
        可用
      </Badge>
    )
  }
  if (status === "disabled") {
    return (
      <Badge variant="warning">
        <Ban className="size-3.5" />
        已停用
      </Badge>
    )
  }
  return (
    <Badge variant="secondary">
      <Clock3 className="size-3.5" />
      已使用
    </Badge>
  )
}

async function copyRedemptionCode(code: string) {
  try {
    await navigator.clipboard.writeText(code)
    toast.success("已复制兑换码")
  } catch {
    toast.error("复制失败，请手动复制")
  }
}

function DetailPill({
  icon,
  label,
  value,
  mono = false,
}: {
  icon: React.ReactNode
  label: string
  value: string
  mono?: boolean
}) {
  if (!value) return null
  return (
    <span className="inline-flex min-w-0 max-w-full items-center gap-1.5 rounded-full border border-foreground/[0.06] bg-muted/60 px-2.5 py-1 text-xs text-muted-foreground">
      {icon}
      <span className="shrink-0">{label}</span>
      <span className={cn("min-w-0 truncate text-foreground", mono && "font-mono")}>{value}</span>
    </span>
  )
}

function RedemptionCodeManager() {
  const [count, setCount] = React.useState("1")
  const [note, setNote] = React.useState("")
  const [page, setPage] = React.useState(1)
  const [deleteTarget, setDeleteTarget] = React.useState<RedemptionCodeView | null>(null)
  const codesQuery = useRedemptionCodes()

  const codes = codesQuery.data?.codes ?? EMPTY_CODES
  const targetCode = deleteTarget?.code ?? null
  const pageCount = Math.max(1, Math.ceil(codes.length / CODE_PAGE_SIZE))
  const safePage = Math.min(page, pageCount)
  const pageStart = (safePage - 1) * CODE_PAGE_SIZE
  const paged = codes.slice(pageStart, pageStart + CODE_PAGE_SIZE)
  const pageEnd = pageStart + paged.length
  const parsedCount = Number(count)
  const countValid = Number.isInteger(parsedCount) && parsedCount >= 1 && parsedCount <= 20

  const generateMutation = useAppMutation(
    () =>
      generateRedemptionCodes({
        count: countValid ? parsedCount : 1,
        note: note.trim(),
      }),
    {
      successMessage: "兑换码已生成",
      onSuccess: (result) => {
        setNote("")
        setPage(1)
        const firstCode = result.codes[0]?.code.code
        if (firstCode) {
          void copyRedemptionCode(firstCode)
        }
      },
    },
  )
  const disableMutation = useAppMutation((id: number) => disableRedemptionCode(id), {
    successMessage: "兑换码已停用",
  })
  const enableMutation = useAppMutation((id: number) => enableRedemptionCode(id), {
    successMessage: "兑换码已启用",
  })
  const deleteMutation = useAppMutation((id: number) => deleteRedemptionCode(id), {
    successMessage: "兑换码已删除",
    onSuccess: () => setDeleteTarget(null),
  })

  return (
    <div className="flex h-full min-h-0 flex-col">
    <Card className="min-h-0 flex-1 gap-0 overflow-y-auto p-0">
      <div className="flex flex-col gap-4 border-b bg-muted/35 p-5 lg:flex-row lg:items-start lg:justify-between">
        <h2 className="flex items-center gap-2 text-lg font-semibold">
          <KeyRound className="size-4 text-muted-foreground" />
          生成兑换码
        </h2>
        <div className="grid gap-2 sm:grid-cols-[88px_minmax(220px,1fr)_auto] lg:w-[560px]">
          <Input
            type="number"
            min={1}
            max={20}
            value={count}
            onChange={(event) => setCount(event.target.value)}
            aria-label="生成数量"
            aria-invalid={count.trim() !== "" && !countValid}
          className="h-9"
          />
          <Input
            value={note}
            onChange={(event) => setNote(event.target.value)}
            placeholder="备注，比如客户来源 / 订单号"
            className="h-9"
          />
          <Button disabled={!countValid || generateMutation.isPending} onClick={() => generateMutation.mutate()}>
            <Plus data-slot="icon" />
            生成
          </Button>
        </div>
      </div>

      <div className="grid gap-3 border-b bg-muted/25 p-5 sm:grid-cols-3">
        <div className="relative overflow-hidden rounded-md border bg-card p-4">
          <div className="text-xs text-muted-foreground">可用兑换码</div>
          <div className="mt-2 text-2xl font-semibold text-success tabular-nums">
            {codesQuery.data?.available_count ?? 0}
          </div>
        </div>
        <div className="relative overflow-hidden rounded-md border bg-card p-4">
          <div className="text-xs text-muted-foreground">已使用</div>
          <div className="mt-2 text-2xl font-semibold text-brand tabular-nums">
            {codesQuery.data?.used_count ?? 0}
          </div>
        </div>
        <div className="relative overflow-hidden rounded-md border bg-card p-4">
          <div className="text-xs text-muted-foreground">已停用</div>
          <div className="mt-2 text-2xl font-semibold text-coral tabular-nums">
            {codesQuery.data?.disabled_count ?? 0}
          </div>
        </div>
      </div>

      {codesQuery.isPending ? (
        <div className="grid gap-2 p-5">
          {Array.from({ length: 3 }).map((_, index) => (
            <Skeleton key={index} className="h-14 rounded-lg" />
          ))}
        </div>
      ) : codesQuery.isError ? (
        <div className="flex items-center justify-between gap-3 p-5">
          <p className="text-sm text-muted-foreground">兑换码加载失败</p>
          <Button variant="outline" size="sm" onClick={() => codesQuery.refetch()}>
            重试
          </Button>
        </div>
      ) : codes.length === 0 ? (
        <div className="p-5 text-sm text-muted-foreground">
          还没有兑换码。先生成一个，再发给客户使用。
        </div>
      ) : (
        <div className="divide-y">
          {paged.map((view) => {
            const code = view.code
            const used = code.status === "used"
            const disabled = code.status === "disabled"
            return (
              <div
                key={code.id}
                className={cn(
                  "grid gap-3 border-l-2 p-5 transition-colors hover:bg-muted/25 md:grid-cols-[minmax(0,1fr)_auto] md:items-center",
                  used ? "border-l-brand" : disabled ? "border-l-coral" : "border-l-success",
                )}
              >
                <div className="min-w-0">
                  <div className="flex flex-wrap items-center gap-2">
                    {redemptionCodeBadge(code.status)}
                    <span className="truncate font-mono text-[15px] font-semibold">{code.code}</span>
                    <Button
                      type="button"
                      variant="ghost"
                      size="icon-sm"
                      aria-label="复制兑换码"
                      onClick={() => void copyRedemptionCode(code.code)}
                    >
                      <Copy />
                    </Button>
                  </div>
                  <div className="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-xs text-muted-foreground">
                    <span>生成：{view.created_at_label}</span>
                    {code.note ? <span>备注：{code.note}</span> : null}
                    {used ? <span>使用：{view.used_at_label || "-"}</span> : null}
                    {view.application_email ? <span>客户：{view.application_email}</span> : null}
                  </div>
                </div>
                <div className="flex items-center gap-2 md:justify-end">
                  {used ? null : disabled ? (
                    <>
                      <Button
                        variant="outline"
                        size="sm"
                        disabled={enableMutation.isPending}
                        onClick={() => enableMutation.mutate(code.id)}
                      >
                        <RotateCcw data-slot="icon" />
                        启用
                      </Button>
                      <Button
                        variant="outline"
                        size="sm"
                        className="text-destructive hover:text-destructive"
                        disabled={deleteMutation.isPending}
                        onClick={() => setDeleteTarget(view)}
                      >
                        <Trash2 data-slot="icon" />
                        删除
                      </Button>
                    </>
                  ) : (
                    <>
                      <Button
                        variant="outline"
                        size="sm"
                        disabled={disableMutation.isPending}
                        onClick={() => disableMutation.mutate(code.id)}
                      >
                        <Ban data-slot="icon" />
                        停用
                      </Button>
                      <Button
                        variant="outline"
                        size="sm"
                        className="text-destructive hover:text-destructive"
                        disabled={deleteMutation.isPending}
                        onClick={() => setDeleteTarget(view)}
                      >
                        <Trash2 data-slot="icon" />
                        删除
                      </Button>
                    </>
                  )}
                </div>
              </div>
            )
          })}

          {pageCount > 1 ? (
            <div className="flex flex-col items-center justify-between gap-3 p-5 pt-4 text-xs text-muted-foreground sm:flex-row">
              <span>
                第 {safePage} / {pageCount} 页 · {pageStart + 1}-{pageEnd} / {codes.length} 个
              </span>
              <div className="flex items-center gap-1.5">
                <Button
                  variant="outline"
                  size="icon-sm"
                  aria-label="上一页"
                  disabled={safePage <= 1}
                  onClick={() => setPage((current) => Math.max(1, current - 1))}
                >
                  <ChevronLeft />
                </Button>
                <Button
                  variant="outline"
                  size="icon-sm"
                  aria-label="下一页"
                  disabled={safePage >= pageCount}
                  onClick={() => setPage((current) => Math.min(pageCount, current + 1))}
                >
                  <ChevronRight />
                </Button>
              </div>
            </div>
          ) : null}
        </div>
      )}
    </Card>
    <ConfirmDialog
      open={deleteTarget !== null}
      onOpenChange={(open) => {
        if (!open && !deleteMutation.isPending) {
          setDeleteTarget(null)
        }
      }}
      title="删除兑换码？"
      description={
        targetCode
          ? `确认删除兑换码 ${targetCode.code}？删除后客户将无法再使用这个兑换码。已使用的兑换码会保留申请记录，不能删除。`
          : "确认删除这个兑换码？"
      }
      actionLabel="删除"
      destructive
      pending={deleteMutation.isPending}
      onConfirm={() => {
        if (targetCode) {
          deleteMutation.mutate(targetCode.id)
        }
      }}
    />
    </div>
  )
}

function InvitePanel({
  view,
  seats,
}: {
  view: RedemptionApplicationView
  seats: SelectableSeat[]
}) {
  const [seatId, setSeatId] = React.useState("")
  const [priceYuan, setPriceYuan] = React.useState("")
  const [cronExpr, setCronExpr] = React.useState("interval:30d")
  const [boardedAt, setBoardedAt] = React.useState(todayShanghai)
  const [notifyOffsets, setNotifyOffsets] = React.useState<number[]>([3])
  const [remark, setRemark] = React.useState("")
  const [tradeURL, setTradeURL] = React.useState("")
  const [operatorNote, setOperatorNote] = React.useState("")
  const [rejectOpen, setRejectOpen] = React.useState(false)

  const selectedSeat = seats.find((option) => String(option.seat.id) === seatId) ?? null
  const priceValid = MONEY_PATTERN.test(priceYuan.trim())
  const canSubmit = selectedSeat !== null && priceValid && cronExpr.trim() !== ""

  const inviteMutation = useAppMutation(
    () =>
      inviteRedemption(view.application.id, {
        seat_id: Number(seatId),
        price_yuan: priceYuan.trim(),
        cron_expr: cronExpr.trim(),
        notify_offsets: notifyOffsets,
        boarded_at: boardedAt,
        remark: remark.trim(),
        trade_url: tradeURL.trim(),
        operator_note: operatorNote.trim(),
      }),
    { successMessage: "已邀请，并已自动创建订阅" },
  )
  const rejectMutation = useAppMutation(
    () => rejectRedemption(view.application.id, { reason: operatorNote.trim() }),
    {
      successMessage: "兑换申请已驳回，兑换码已恢复可用",
      onSuccess: () => setRejectOpen(false),
    },
  )

  return (
    <div className="mt-4 grid gap-4 border-t pt-4">
      <div className="grid gap-3 md:grid-cols-[1.35fr_0.7fr_0.75fr]">
        <div className="grid gap-2">
          <Label>分配母号空间 / 车位</Label>
          <Select value={seatId} onValueChange={setSeatId} disabled={seats.length === 0}>
            <SelectTrigger>
              <SelectValue placeholder={seats.length === 0 ? "暂无空闲车位" : "选择空闲车位"} />
            </SelectTrigger>
            <SelectContent>
              {seats.map((option) => (
                <SelectItem key={option.seat.id} value={String(option.seat.id)}>
                  <span className="flex min-w-0 items-center gap-2">
                    <span className="max-w-44 truncate font-medium">{option.account.name}</span>
                    <span className="text-xs text-muted-foreground">{option.seat.name}</span>
                    <span className="ml-auto text-xs tabular-nums text-muted-foreground">
                      ¥{option.account.cost_yuan}
                    </span>
                  </span>
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>

        <div className="grid gap-2">
          <Label>实收金额</Label>
          <Input
            inputMode="decimal"
            placeholder="20.00"
            value={priceYuan}
            onChange={(event) => setPriceYuan(event.target.value)}
            aria-invalid={priceYuan.trim() !== "" && !priceValid}
          />
        </div>

        <div className="grid gap-2">
          <Label>上车日期</Label>
          <Input type="date" value={boardedAt} onChange={(event) => setBoardedAt(event.target.value)} />
        </div>
      </div>

      {selectedSeat ? (
        <div className="flex flex-wrap gap-1.5 text-xs text-muted-foreground">
          <span className="rounded-md bg-muted px-2 py-1">母号：{selectedSeat.account.email || selectedSeat.account.name}</span>
          {selectedSeat.account.space_name ? (
            <span className="rounded-md bg-muted px-2 py-1">空间：{selectedSeat.account.space_name}</span>
          ) : null}
          <span className="rounded-md bg-muted px-2 py-1">
            占用：{selectedSeat.account.seat_used}/{selectedSeat.account.seat_total}
          </span>
        </div>
      ) : null}

      <div className="grid gap-3 md:grid-cols-[1fr_1fr]">
        <div className="grid gap-2">
          <Label>计费周期</Label>
          <div className="flex flex-wrap gap-1.5">
            {CYCLE_PRESETS.map((preset) => (
              <Button
                key={preset.cron}
                type="button"
                variant={cronExpr === preset.cron ? "secondary" : "outline"}
                size="sm"
                onClick={() => setCronExpr(preset.cron)}
              >
                {preset.label}
              </Button>
            ))}
          </div>
          <Input
            className="font-mono"
            value={cronExpr}
            onChange={(event) => setCronExpr(event.target.value)}
          />
        </div>

        <div className="grid gap-2">
          <Label>到期前邮件提醒</Label>
          <div className="flex flex-wrap gap-1.5">
            {OFFSET_OPTIONS.map((offset) => {
              const checked = notifyOffsets.includes(offset)
              return (
                <label
                  key={offset}
                  className={cn(
                    "flex cursor-pointer items-center gap-2 rounded-md border px-2.5 py-1.5 text-[13px] transition-colors select-none",
                    checked ? "border-brand/40 bg-brand/10" : "hover:bg-accent",
                  )}
                >
                  <Checkbox
                    checked={checked}
                    onCheckedChange={(value) => {
                      setNotifyOffsets((current) =>
                        value === true
                          ? [...current, offset].sort((a, b) => b - a)
                          : current.filter((item) => item !== offset),
                      )
                    }}
                  />
                  提前 {offset} 天
                </label>
              )
            })}
          </div>
        </div>
      </div>

      <div className="grid gap-3 md:grid-cols-2">
        <div className="grid gap-2">
          <Label>订阅备注</Label>
          <Textarea rows={2} value={remark} onChange={(event) => setRemark(event.target.value)} />
        </div>
        <div className="grid gap-2">
          <Label>交易链接</Label>
          <Input placeholder="https://" value={tradeURL} onChange={(event) => setTradeURL(event.target.value)} />
        </div>
      </div>

      <div className="grid gap-2">
        <Label>处理备注 / 驳回原因</Label>
        <Textarea
          rows={2}
          value={operatorNote}
          onChange={(event) => setOperatorNote(event.target.value)}
          placeholder="驳回时会显示给客户；留空则使用默认说明"
        />
      </div>

      <div className="flex flex-wrap items-center justify-end gap-2">
        {!priceValid && priceYuan.trim() !== "" ? (
          <span className="mr-auto text-xs text-destructive">实收金额格式不正确</span>
        ) : null}
        <Button
          type="button"
          variant="outline"
          className="text-destructive hover:text-destructive"
          disabled={inviteMutation.isPending || rejectMutation.isPending}
          onClick={() => setRejectOpen(true)}
        >
          <Ban data-slot="icon" />
          驳回
        </Button>
        <Button
          type="button"
          disabled={!canSubmit || inviteMutation.isPending || rejectMutation.isPending}
          onClick={() => inviteMutation.mutate()}
        >
          <Send data-slot="icon" />
          已邀请
        </Button>
      </div>
      <ConfirmDialog
        open={rejectOpen}
        onOpenChange={(open) => {
          if (!open && !rejectMutation.isPending) {
            setRejectOpen(false)
          }
        }}
        title="驳回兑换申请？"
        description={`确认驳回 ${view.application.customer_email} 的兑换申请？兑换码将恢复可用，客户可以修改资料后重新提交。`}
        actionLabel="确认驳回"
        destructive
        pending={rejectMutation.isPending}
        onConfirm={() => rejectMutation.mutate()}
      />
    </div>
  )
}

function RedemptionCard({
  view,
  index,
  seats,
}: {
  view: RedemptionApplicationView
  index: number
  seats: SelectableSeat[]
}) {
  const application = view.application
  const invited = application.status === "invited"
  const rejected = application.status === "rejected"

  return (
    <Card
      className={cn(
        "relative gap-0 overflow-hidden p-5 transition-colors hover:border-foreground/15 animate-fade-up",
        invited
          ? "border-l-4 border-l-success"
          : rejected
            ? "border-l-4 border-l-destructive"
            : "border-l-4 border-l-gold",
      )}
      style={{ animationDelay: `${Math.min(index * 35, 280)}ms` }}
    >
      <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            {statusBadge(application.status)}
            <span className="text-xs tabular-nums text-muted-foreground">{view.created_at_label}</span>
          </div>
          <h3 className="mt-3 truncate text-[16px] font-semibold">{application.customer_email}</h3>
          <div className="mt-2 flex flex-wrap gap-1.5">
            <DetailPill
              icon={<Mail className="size-3.5" />}
              label="邮箱"
              value={application.customer_email}
              mono
            />
            <DetailPill
              icon={<MessageCircle className="size-3.5" />}
              label="微信/QQ"
              value={application.customer_contact}
            />
            <DetailPill
              icon={<TicketCheck className="size-3.5" />}
              label="兑换码"
              value={application.redeem_code}
              mono
            />
          </div>
          {application.request_note ? (
            <p className="mt-3 line-clamp-2 text-sm leading-relaxed text-muted-foreground">
              {application.request_note}
            </p>
          ) : null}
        </div>

        {invited ? (
          <div className="min-w-0 rounded-md border border-success/20 bg-success/[0.06] p-3 text-xs">
            <div className="flex items-center gap-1.5 font-medium text-success">
              <CheckCircle2 className="size-3.5" />
              {view.invited_at_label || "已处理"}
            </div>
            <div className="mt-2 grid gap-1 text-muted-foreground">
              <span className="truncate">母号：{view.account_email || view.account_name || "-"}</span>
              <span className="truncate">空间：{view.account_space_name || "-"}</span>
              <span className="truncate">车位：{view.seat_name || "-"}</span>
            </div>
            {application.assigned_subscription_id > 0 ? (
              <Button asChild variant="outline" size="sm" className="mt-3 w-full">
                <Link to={`/users?subscription=${application.assigned_subscription_id}`}>
                  查看用户
                </Link>
              </Button>
            ) : null}
          </div>
        ) : rejected ? (
          <div className="min-w-0 rounded-md border border-destructive/20 bg-destructive/[0.06] p-3 text-xs">
            <div className="flex items-center gap-1.5 font-medium text-destructive">
              <Ban className="size-3.5" />
              已驳回
            </div>
            <p className="mt-2 max-w-sm leading-5 text-muted-foreground">
              {application.operator_note || "提交的信息有误，请修改后重新提交"}
            </p>
          </div>
        ) : null}
      </div>

      {application.status === "pending" ? <InvitePanel view={view} seats={seats} /> : null}
    </Card>
  )
}

export function RedemptionsPage() {
  const [section, setSection] = React.useState<RedemptionSection>("applications")
  const [filter, setFilter] = React.useState<RedemptionFilter>("pending")
  const [search, setSearch] = React.useState("")
  const [page, setPage] = React.useState(1)
  const redemptionsQuery = useRedemptions(filter)
  const accountOptionsQuery = useAccountOptions(0, true)

  const seats = React.useMemo(
    () => buildSeatOptions(accountOptionsQuery.data ?? []),
    [accountOptionsQuery.data],
  )

  const redemptions = redemptionsQuery.data?.redemptions ?? EMPTY_REDEMPTIONS
  const filtered = React.useMemo(() => {
    const query = search.trim().toLowerCase()
    if (!query) return redemptions
    return redemptions.filter((view) =>
      [
        view.application.customer_email,
        view.application.customer_contact,
        view.application.redeem_code,
        view.application.request_note,
        view.application.operator_note,
        view.account_name,
        view.account_email,
        view.account_space_name,
        view.seat_name,
        view.subscription_name,
      ].some((field) => field?.toLowerCase().includes(query)),
    )
  }, [redemptions, search])

  const pageCount = Math.max(1, Math.ceil(filtered.length / APPLICATIONS_PER_PAGE))
  const safePage = Math.min(page, pageCount)
  const pageStart = (safePage - 1) * APPLICATIONS_PER_PAGE
  const paged = filtered.slice(pageStart, pageStart + APPLICATIONS_PER_PAGE)
  const pageEnd = pageStart + paged.length
  const pendingCount = redemptionsQuery.data?.pending_count ?? 0

  const updateSearch = (value: string) => {
    setSearch(value)
    setPage(1)
  }

  const updateFilter = (value: RedemptionFilter) => {
    setFilter(value)
    setPage(1)
  }

  return (
    <div className="flex flex-col xl:h-[calc(100dvh-7rem)] xl:min-h-0 xl:overflow-hidden">
      <PageHeader title="兑换申请" />

      <Tabs
        value={section}
        onValueChange={(value) => setSection(value as RedemptionSection)}
        className="flex min-h-0 flex-1 flex-col gap-5"
      >
        <TabsList className="h-10 w-full justify-start bg-muted p-1 sm:w-fit">
          <TabsTrigger value="applications" className="h-8 px-4 text-sm">
            <Clock3 data-slot="icon" />
            处理申请
            {pendingCount > 0 ? (
              <span className="ml-1 rounded-full bg-brand px-1.5 py-0.5 text-[11px] leading-none text-brand-foreground">
                {pendingCount}
              </span>
            ) : null}
          </TabsTrigger>
          <TabsTrigger value="codes" className="h-8 px-4 text-sm">
            <KeyRound data-slot="icon" />
            兑换码管理
          </TabsTrigger>
        </TabsList>

        <TabsContent value="applications" className="flex min-h-0 flex-1 flex-col gap-5">
          <div className="flex justify-end rounded-lg border bg-card p-4">
            <div className="flex w-full flex-col gap-2 sm:w-auto sm:flex-row sm:items-center">
              <div className="relative">
                <Search className="absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground" />
                <Input
                  value={search}
                  onChange={(event) => updateSearch(event.target.value)}
                  placeholder="搜索邮箱 / 微信 / 兑换码..."
                  className="h-9 w-full pl-8 text-[13px] sm:w-72"
                />
              </div>
              <Select value={filter} onValueChange={(value) => updateFilter(value as RedemptionFilter)}>
                <SelectTrigger className="h-9 w-full sm:w-32">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="pending">待处理</SelectItem>
                  <SelectItem value="invited">已邀请</SelectItem>
                  <SelectItem value="rejected">已驳回</SelectItem>
                  <SelectItem value="all">全部</SelectItem>
                </SelectContent>
              </Select>
            </div>
          </div>

          <div className="grid gap-3 sm:grid-cols-3">
            <div className="relative overflow-hidden rounded-lg border bg-card p-4">
              <div className="flex items-center gap-2 text-xs font-medium text-muted-foreground">
                <Clock3 className="size-3.5" />
                待处理
              </div>
              <div className="mt-2 text-2xl font-semibold text-gold tabular-nums">{pendingCount}</div>
            </div>
            <div className="relative overflow-hidden rounded-lg border bg-card p-4">
              <div className="flex items-center gap-2 text-xs font-medium text-muted-foreground">
                <TicketCheck className="size-3.5" />
                可用空位
              </div>
              <div className="mt-2 text-2xl font-semibold text-success tabular-nums">{seats.length}</div>
            </div>
            <div className="relative overflow-hidden rounded-lg border bg-card p-4">
              <div className="flex items-center gap-2 text-xs font-medium text-muted-foreground">
                <CalendarClock className="size-3.5" />
                当前筛选
              </div>
              <div className="mt-2 text-2xl font-semibold text-brand tabular-nums">{filtered.length}</div>
            </div>
          </div>

          {redemptionsQuery.isPending ? (
            <div className="grid min-h-0 flex-1 content-start gap-4 overflow-hidden">
              {Array.from({ length: 3 }).map((_, index) => (
                <Skeleton key={index} className="h-64 rounded-xl" />
              ))}
            </div>
          ) : redemptionsQuery.isError ? (
            <Card className="flex-1 items-center justify-center gap-3 py-16 text-center">
              <p className="text-sm text-muted-foreground">兑换申请加载失败</p>
              <Button variant="outline" onClick={() => redemptionsQuery.refetch()}>
                重试
              </Button>
            </Card>
          ) : filtered.length === 0 ? (
            <Card className="flex-1 items-center justify-center gap-3 py-16 text-center animate-fade-up">
              <TicketCheck className="size-8 text-muted-foreground" />
              <p className="text-sm text-muted-foreground">暂无兑换申请</p>
            </Card>
          ) : (
            <div className="flex min-h-0 flex-1 flex-col gap-4">
              <div className="min-h-0 flex-1 space-y-4 overflow-y-auto pr-1">
                {paged.map((view, index) => (
                  <RedemptionCard
                    key={view.application.id}
                    view={view}
                    index={index}
                    seats={seats}
                  />
                ))}
              </div>

              {pageCount > 1 ? (
                <div className="flex flex-col items-center justify-between gap-3 border-t pt-4 text-xs text-muted-foreground sm:flex-row">
                  <span>
                    第 {safePage} / {pageCount} 页 · {pageStart + 1}-{pageEnd} /{" "}
                    {filtered.length} 条
                  </span>
                  <div className="flex items-center gap-1.5">
                    <Button
                      variant="outline"
                      size="icon-sm"
                      aria-label="上一页"
                      disabled={safePage <= 1}
                      onClick={() => setPage((current) => Math.max(1, current - 1))}
                    >
                      <ChevronLeft />
                    </Button>
                    <Button
                      variant="outline"
                      size="icon-sm"
                      aria-label="下一页"
                      disabled={safePage >= pageCount}
                      onClick={() => setPage((current) => Math.min(pageCount, current + 1))}
                    >
                      <ChevronRight />
                    </Button>
                  </div>
                </div>
              ) : null}
            </div>
          )}
        </TabsContent>

        <TabsContent value="codes" className="min-h-0 flex-1">
          <RedemptionCodeManager />
        </TabsContent>
      </Tabs>
    </div>
  )
}
