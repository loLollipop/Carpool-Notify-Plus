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
} from "lucide-react"
import { toast } from "sonner"

import {
  disableRedemptionCode,
  enableRedemptionCode,
  generateRedemptionCodes,
  inviteRedemption,
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
import { Switch } from "@/components/ui/switch"
import { Textarea } from "@/components/ui/textarea"
import { cn } from "@/lib/utils"
import { todayShanghai } from "@/features/subscriptions/subscription-prefill"

type RedemptionFilter = "pending" | "invited" | "all"

interface SelectableSeat {
  account: AccountOption
  seat: SeatOption
}

const PAGE_SIZE = 9
const CODE_PAGE_SIZE = 6
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
    <span className="inline-flex min-w-0 max-w-full items-center gap-1.5 rounded-md bg-muted px-2 py-1 text-xs text-muted-foreground">
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
  const codesQuery = useRedemptionCodes()

  const codes = codesQuery.data?.codes ?? EMPTY_CODES
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

  return (
    <Card className="mb-5 gap-0 overflow-hidden p-0">
      <div className="flex flex-col gap-4 border-b p-5 lg:flex-row lg:items-start lg:justify-between">
        <div>
          <div className="flex items-center gap-2 text-xs font-medium text-muted-foreground">
            <KeyRound className="size-3.5" />
            兑换码管理
          </div>
          <h2 className="mt-2 text-lg font-semibold tracking-tight">生成客户可用的一次性兑换码</h2>
          <p className="mt-1 text-sm text-muted-foreground">
            客户提交的兑换码必须和这里的可用码一致，提交成功后该码会自动标记为已使用。
          </p>
        </div>
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

      <div className="grid gap-3 border-b bg-muted/20 p-5 sm:grid-cols-3">
        <div>
          <div className="text-xs text-muted-foreground">可用兑换码</div>
          <div className="mt-1 text-2xl font-semibold tabular-nums">
            {codesQuery.data?.available_count ?? 0}
          </div>
        </div>
        <div>
          <div className="text-xs text-muted-foreground">已使用</div>
          <div className="mt-1 text-2xl font-semibold tabular-nums">
            {codesQuery.data?.used_count ?? 0}
          </div>
        </div>
        <div>
          <div className="text-xs text-muted-foreground">已停用</div>
          <div className="mt-1 text-2xl font-semibold tabular-nums">
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
                className="grid gap-3 p-5 md:grid-cols-[minmax(0,1fr)_auto] md:items-center"
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
                    <Button
                      variant="outline"
                      size="sm"
                      disabled={enableMutation.isPending}
                      onClick={() => enableMutation.mutate(code.id)}
                    >
                      <RotateCcw data-slot="icon" />
                      启用
                    </Button>
                  ) : (
                    <Button
                      variant="outline"
                      size="sm"
                      disabled={disableMutation.isPending}
                      onClick={() => disableMutation.mutate(code.id)}
                    >
                      <Ban data-slot="icon" />
                      停用
                    </Button>
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
  const [isResale, setIsResale] = React.useState(false)
  const [agencyFeeYuan, setAgencyFeeYuan] = React.useState("")
  const [cronExpr, setCronExpr] = React.useState("interval:30d")
  const [boardedAt, setBoardedAt] = React.useState(todayShanghai)
  const [notifyOffsets, setNotifyOffsets] = React.useState<number[]>([3])
  const [remark, setRemark] = React.useState("")
  const [tradeURL, setTradeURL] = React.useState("")
  const [operatorNote, setOperatorNote] = React.useState("")

  const selectedSeat = seats.find((option) => String(option.seat.id) === seatId) ?? null
  const priceValid = MONEY_PATTERN.test(priceYuan.trim())
  const agencyFeeValid = !isResale || agencyFeeYuan.trim() === "" || MONEY_PATTERN.test(agencyFeeYuan.trim())
  const canSubmit = selectedSeat !== null && priceValid && agencyFeeValid && cronExpr.trim() !== ""

  const mutation = useAppMutation(
    () =>
      inviteRedemption(view.application.id, {
        seat_id: Number(seatId),
        price_yuan: priceYuan.trim(),
        is_resale: isResale,
        agency_fee_yuan: isResale ? agencyFeeYuan.trim() : "0",
        cron_expr: cronExpr.trim(),
        notify_offsets: notifyOffsets,
        boarded_at: boardedAt,
        remark: remark.trim(),
        trade_url: tradeURL.trim(),
        operator_note: operatorNote.trim(),
      }),
    { successMessage: "已邀请，并已自动创建订阅" },
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

      <div className="flex items-center justify-between rounded-lg border px-3 py-2.5">
        <div className="space-y-0.5 pr-4">
          <Label>串货</Label>
          <p className="text-xs text-muted-foreground">只统计中介费时打开。</p>
        </div>
        <Switch checked={isResale} onCheckedChange={setIsResale} />
      </div>

      {isResale ? (
        <div className="grid gap-2">
          <Label>中介费</Label>
          <Input
            inputMode="decimal"
            placeholder="0.00"
            value={agencyFeeYuan}
            onChange={(event) => setAgencyFeeYuan(event.target.value)}
            aria-invalid={!agencyFeeValid}
          />
        </div>
      ) : null}

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
        <Label>处理备注</Label>
        <Textarea
          rows={2}
          value={operatorNote}
          onChange={(event) => setOperatorNote(event.target.value)}
        />
      </div>

      <div className="flex flex-wrap items-center justify-end gap-2">
        {!priceValid && priceYuan.trim() !== "" ? (
          <span className="mr-auto text-xs text-destructive">实收金额格式不正确</span>
        ) : null}
        <Button disabled={!canSubmit || mutation.isPending} onClick={() => mutation.mutate()}>
          <Send data-slot="icon" />
          已邀请
        </Button>
      </div>
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

  return (
    <Card
      className="gap-0 p-5 transition-colors hover:border-foreground/15 animate-fade-up"
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
          <div className="min-w-0 rounded-lg border bg-muted/30 p-3 text-xs">
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
        ) : null}
      </div>

      {!invited ? <InvitePanel view={view} seats={seats} /> : null}
    </Card>
  )
}

export function RedemptionsPage() {
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

  const pageCount = Math.max(1, Math.ceil(filtered.length / PAGE_SIZE))
  const safePage = Math.min(page, pageCount)
  const pageStart = (safePage - 1) * PAGE_SIZE
  const paged = filtered.slice(pageStart, pageStart + PAGE_SIZE)
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
    <>
      <PageHeader
        title="兑换申请"
        description="客户提交兑换后会出现在这里；分配母号空间并确认已邀请后，会自动创建订阅和首期账单。"
        actions={
          <>
            <div className="relative">
              <Search className="absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                value={search}
                onChange={(event) => updateSearch(event.target.value)}
                placeholder="搜索邮箱 / 微信 / 兑换码…"
                className="h-9 w-56 pl-8 text-[13px] sm:w-72"
              />
            </div>
            <Select value={filter} onValueChange={(value) => updateFilter(value as RedemptionFilter)}>
              <SelectTrigger className="h-9 w-32">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="pending">待处理</SelectItem>
                <SelectItem value="invited">已邀请</SelectItem>
                <SelectItem value="all">全部</SelectItem>
              </SelectContent>
            </Select>
          </>
        }
      />

      <RedemptionCodeManager />

      <div className="mb-5 grid gap-3 sm:grid-cols-3">
        <div className="rounded-lg border bg-card p-4">
          <div className="flex items-center gap-2 text-xs font-medium text-muted-foreground">
            <Clock3 className="size-3.5" />
            待处理
          </div>
          <div className="mt-2 text-2xl font-semibold tabular-nums">{pendingCount}</div>
        </div>
        <div className="rounded-lg border bg-card p-4">
          <div className="flex items-center gap-2 text-xs font-medium text-muted-foreground">
            <TicketCheck className="size-3.5" />
            可用空位
          </div>
          <div className="mt-2 text-2xl font-semibold tabular-nums">{seats.length}</div>
        </div>
        <div className="rounded-lg border bg-card p-4">
          <div className="flex items-center gap-2 text-xs font-medium text-muted-foreground">
            <CalendarClock className="size-3.5" />
            当前筛选
          </div>
          <div className="mt-2 text-2xl font-semibold tabular-nums">{filtered.length}</div>
        </div>
      </div>

      {redemptionsQuery.isPending ? (
        <div className="grid gap-4">
          {Array.from({ length: 3 }).map((_, index) => (
            <Skeleton key={index} className="h-64 rounded-xl" />
          ))}
        </div>
      ) : redemptionsQuery.isError ? (
        <Card className="items-center gap-3 py-16 text-center">
          <p className="text-sm text-muted-foreground">兑换申请加载失败</p>
          <Button variant="outline" onClick={() => redemptionsQuery.refetch()}>
            重试
          </Button>
        </Card>
      ) : filtered.length === 0 ? (
        <Card className="items-center gap-3 py-16 text-center animate-fade-up">
          <TicketCheck className="size-8 text-muted-foreground" />
          <p className="text-sm text-muted-foreground">暂无兑换申请</p>
        </Card>
      ) : (
        <div className="space-y-4">
          {paged.map((view, index) => (
            <RedemptionCard key={view.application.id} view={view} index={index} seats={seats} />
          ))}

          {pageCount > 1 ? (
            <div className="flex flex-col items-center justify-between gap-3 border-t pt-4 text-xs text-muted-foreground sm:flex-row">
              <span>
                第 {safePage} / {pageCount} 页 · {pageStart + 1}-{pageEnd} / {filtered.length} 条
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
    </>
  )
}
