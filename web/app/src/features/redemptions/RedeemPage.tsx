import * as React from "react"
import { zodResolver } from "@hookform/resolvers/zod"
import { useMutation, useQuery } from "@tanstack/react-query"
import { CheckCircle2, Clock3, Mail, MessageCircle, RotateCcw, TicketCheck } from "lucide-react"
import { useForm } from "react-hook-form"
import { toast } from "sonner"
import { z } from "zod"

import { fetchRedemptionStatus, submitRedemptionApplication } from "@/api/endpoints"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/components/ui/form"
import { Input } from "@/components/ui/input"
import { Textarea } from "@/components/ui/textarea"

const STORAGE_KEY = "carpool-notify:redemption-token"
const EMAIL_PATTERN = /^[^@\s]+@[^@\s]+\.[^@\s]+$/

const schema = z.object({
  customer_email: z
    .string()
    .trim()
    .min(1, "请填写邮箱")
    .regex(EMAIL_PATTERN, "邮箱格式不正确")
    .max(254, "邮箱太长"),
  redeem_code: z.string().trim().min(1, "请填写兑换码").max(120, "兑换码太长"),
  customer_contact: z.string().trim().min(1, "请填写微信号或 QQ 号").max(80, "联系方式太长"),
  request_note: z.string().trim().max(500, "备注最多 500 个字"),
})

type FormValues = z.infer<typeof schema>

function readStoredToken() {
  try {
    return window.localStorage.getItem(STORAGE_KEY) ?? ""
  } catch {
    return ""
  }
}

function writeStoredToken(token: string) {
  try {
    if (token) {
      window.localStorage.setItem(STORAGE_KEY, token)
    } else {
      window.localStorage.removeItem(STORAGE_KEY)
    }
  } catch {
    // localStorage may be unavailable in private browsing.
  }
}

export function RedeemPage() {
  const [trackingToken, setTrackingToken] = React.useState(readStoredToken)

  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: {
      customer_email: "",
      redeem_code: "",
      customer_contact: "",
      request_note: "",
    },
  })

  const statusQuery = useQuery({
    queryKey: ["public-redemption-status", trackingToken],
    queryFn: () => fetchRedemptionStatus(trackingToken),
    enabled: trackingToken !== "",
    refetchInterval: (query) => (query.state.data?.status === "pending" ? 5_000 : false),
    retry: false,
  })

  const submitMutation = useMutation({
    mutationFn: (values: FormValues) =>
      submitRedemptionApplication({
        customer_email: values.customer_email.trim(),
        customer_contact: values.customer_contact.trim(),
        redeem_code: values.redeem_code.trim(),
        request_note: values.request_note.trim(),
      }),
    onSuccess: (result) => {
      setTrackingToken(result.tracking_token)
      writeStoredToken(result.tracking_token)
      form.reset()
      toast.success(result.message ?? "申请已提交")
    },
    onError: (error: Error) => toast.error(error.message),
  })

  const resetApplication = () => {
    setTrackingToken("")
    writeStoredToken("")
    form.reset()
  }

  const status = statusQuery.data?.status ?? (trackingToken ? "pending" : null)
  const invited = status === "invited"

  return (
    <main className="min-h-dvh bg-background px-4 py-8 text-foreground sm:px-6">
      <div className="mx-auto flex min-h-[calc(100dvh-4rem)] w-full max-w-5xl items-center">
        <div className="grid w-full gap-5 lg:grid-cols-[0.92fr_1.08fr]">
          <section className="flex flex-col justify-between rounded-xl border bg-card p-6 shadow-sm animate-fade-up">
            <div>
              <div className="inline-flex items-center gap-2 rounded-md border bg-muted/50 px-2.5 py-1 text-xs font-medium text-muted-foreground">
                <TicketCheck className="size-3.5" />
                Carpool Notify
              </div>
              <h1 className="mt-6 text-3xl font-semibold tracking-tight sm:text-4xl">
                兑换加入空间
              </h1>
              <p className="mt-3 max-w-md text-sm leading-relaxed text-muted-foreground">
                填写购买时使用的信息后，系统会把申请同步到管理后台。
              </p>
            </div>

            <div className="mt-8 grid gap-3 text-sm">
              <div className="flex items-center gap-3 rounded-lg bg-muted/45 p-3">
                <Mail className="size-4 shrink-0 text-brand" />
                <span>邀请会发送到你填写的邮箱。</span>
              </div>
              <div className="flex items-center gap-3 rounded-lg bg-muted/45 p-3">
                <MessageCircle className="size-4 shrink-0 text-success" />
                <span>微信号或 QQ 号用于核对订单。</span>
              </div>
            </div>
          </section>

          <Card className="p-6 shadow-sm animate-fade-up [animation-delay:80ms]">
            {trackingToken ? (
              <div className="flex min-h-[31rem] flex-col justify-between">
                <div>
                  <Badge variant={invited ? "success" : "brand"}>
                    {invited ? (
                      <CheckCircle2 className="size-3.5" />
                    ) : (
                      <Clock3 className="size-3.5" />
                    )}
                    {invited ? "已发送邀请" : "待处理"}
                  </Badge>
                  <h2 className="mt-5 text-2xl font-semibold tracking-tight">
                    {invited
                      ? "已成功发送邀请，请在邮箱中点击确认加入空间"
                      : "申请已提交，请耐心等待 1-2 分钟"}
                  </h2>
                  <p className="mt-3 text-sm leading-relaxed text-muted-foreground">
                    {invited
                      ? "如果收件箱没看到，可以检查垃圾邮件或稍等邮箱同步。"
                      : "页面会自动刷新状态，处理完成后这里会同步更新。"}
                  </p>
                </div>

                <div className="mt-8 grid gap-3 rounded-lg border bg-muted/35 p-4 text-sm">
                  <div className="flex items-center justify-between gap-4">
                    <span className="text-muted-foreground">申请邮箱</span>
                    <span className="truncate font-mono">
                      {statusQuery.data?.customer_email || "加载中"}
                    </span>
                  </div>
                  <div className="flex items-center justify-between gap-4">
                    <span className="text-muted-foreground">提交时间</span>
                    <span className="tabular-nums">
                      {statusQuery.data?.created_at_label || "加载中"}
                    </span>
                  </div>
                  {invited ? (
                    <div className="flex items-center justify-between gap-4">
                      <span className="text-muted-foreground">邀请时间</span>
                      <span className="tabular-nums">
                        {statusQuery.data?.invited_at_label || "刚刚"}
                      </span>
                    </div>
                  ) : null}
                </div>

                <div className="mt-6 flex flex-wrap items-center gap-2">
                  <Button
                    type="button"
                    variant="outline"
                    disabled={statusQuery.isFetching}
                    onClick={() => void statusQuery.refetch()}
                  >
                    <RotateCcw data-slot="icon" />
                    刷新状态
                  </Button>
                  <Button type="button" variant="ghost" onClick={resetApplication}>
                    重新填写
                  </Button>
                </div>
              </div>
            ) : (
              <Form {...form}>
                <form
                  onSubmit={form.handleSubmit((values) => submitMutation.mutate(values))}
                  className="grid gap-5"
                >
                  <div>
                    <Badge variant="brand">兑换申请</Badge>
                    <h2 className="mt-4 text-2xl font-semibold tracking-tight">填写申请信息</h2>
                  </div>

                  <FormField
                    control={form.control}
                    name="customer_email"
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>邮箱</FormLabel>
                        <FormControl>
                          <Input
                            type="email"
                            autoComplete="email"
                            placeholder="name@example.com"
                            {...field}
                          />
                        </FormControl>
                        <FormDescription>后续空间邀请会发送到这个邮箱。</FormDescription>
                        <FormMessage />
                      </FormItem>
                    )}
                  />

                  <FormField
                    control={form.control}
                    name="redeem_code"
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>兑换码</FormLabel>
                        <FormControl>
                          <Input autoComplete="off" placeholder="输入兑换码" {...field} />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />

                  <FormField
                    control={form.control}
                    name="customer_contact"
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>微信号 / QQ号</FormLabel>
                        <FormControl>
                          <Input autoComplete="off" placeholder="方便核对订单" {...field} />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />

                  <FormField
                    control={form.control}
                    name="request_note"
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>备注</FormLabel>
                        <FormControl>
                          <Textarea rows={3} placeholder="可选" {...field} />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />

                  <Button type="submit" disabled={submitMutation.isPending}>
                    {submitMutation.isPending ? "提交中..." : "提交申请"}
                  </Button>
                </form>
              </Form>
            )}
          </Card>
        </div>
      </div>
    </main>
  )
}
