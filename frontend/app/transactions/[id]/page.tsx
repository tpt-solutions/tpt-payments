// tpt-payments/frontend/app/transactions/[id]/page.tsx
"use client"

import { useCallback, useEffect, useRef, useState } from "react"
import { format } from "date-fns"
import { ArrowLeft, Clock, CreditCard, RefreshCw, CheckCircle, DollarSign, FileText, User } from "lucide-react"
import { useParams, useRouter } from "next/navigation"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { Badge } from "@/components/ui/badge"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Separator } from "@/components/ui/separator"
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { STATUS_LABELS, formatMinor, minorDigits, type IntentStatus, type PaymentIntent } from "@/lib/payments"

interface Refund {
  id: string
  amount: number
  currency: string
  status: "pending" | "succeeded" | "failed"
  reason?: string
  created: number
}

interface TimelineEvent {
  id: string
  icon: "created" | "capture" | "refund" | "cancel"
  description: string
  timestamp: number // Unix seconds
  amount?: number
  currency: string
}

const BADGE_CLASS: Record<IntentStatus, string> = {
  succeeded: "bg-emerald-500/10 text-emerald-500",
  requires_capture: "bg-sky-500/10 text-sky-500",
  processing: "bg-amber-500/10 text-amber-500",
  requires_action: "bg-amber-500/10 text-amber-500",
  requires_payment_method: "bg-slate-500/10 text-slate-500",
  requires_confirmation: "bg-slate-500/10 text-slate-500",
  canceled: "bg-rose-500/10 text-rose-500",
}

const ICONS = {
  created: FileText,
  capture: DollarSign,
  refund: RefreshCw,
  cancel: CheckCircle,
} as const

// The API does not serve an event history for an intent, so the timeline is built from what the
// intent and its refunds record: when it was created, captured, refunded, or canceled.
function buildTimeline(intent: PaymentIntent, refunds: Refund[]): TimelineEvent[] {
  const events: TimelineEvent[] = [
    { id: "created", icon: "created", description: "Payment intent created", timestamp: intent.created, currency: intent.currency },
  ]
  if (intent.amount_captured && intent.amount_captured > 0) {
    events.push({
      id: "captured", icon: "capture", description: "Payment captured (capture time is not recorded; shown at creation)",
      timestamp: intent.created, amount: intent.amount_captured, currency: intent.currency,
    })
  }
  for (const r of refunds) {
    events.push({
      id: r.id, icon: "refund",
      description: r.status === "succeeded" ? "Refund succeeded" : r.status === "failed" ? "Refund failed" : "Refund pending",
      timestamp: r.created, amount: r.amount, currency: r.currency,
    })
  }
  if (intent.canceled_at) {
    events.push({ id: "canceled", icon: "cancel", description: "Payment canceled", timestamp: intent.canceled_at, currency: intent.currency })
  }
  return events.sort((a, b) => a.timestamp - b.timestamp)
}

export default function TransactionDetailPage() {
  const router = useRouter()
  const params = useParams<{ id: string }>()
  const id = params.id

  const [intent, setIntent] = useState<PaymentIntent | null>(null)
  const [refunds, setRefunds] = useState<Refund[]>([])
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState<string | null>(null)

  const [refundAmount, setRefundAmount] = useState("")
  const [refundReason, setRefundReason] = useState("")
  const [refundError, setRefundError] = useState<string | null>(null)
  const [isRefunding, setIsRefunding] = useState(false)
  const [dialogOpen, setDialogOpen] = useState(false)
  // One key per refund being attempted: a retry after a network error reuses it, so it cannot
  // refund twice. A new key is made once a refund succeeds.
  const idempotencyKey = useRef<string | null>(null)

  const load = useCallback(async () => {
    setLoadError(null)
    try {
      const [iRes, rRes] = await Promise.all([
        fetch(`/api/tpt/payment_intent?id=${encodeURIComponent(id)}`),
        fetch(`/api/tpt/refunds?payment_intent=${encodeURIComponent(id)}`),
      ])
      const iBody = await iRes.json().catch(() => ({}))
      if (!iRes.ok) throw new Error(iBody.error || `request failed (${iRes.status})`)
      const rBody = await rRes.json().catch(() => ({ data: [] }))
      setIntent(iBody as PaymentIntent)
      setRefunds(rRes.ok ? (rBody.data as Refund[]) : [])
    } catch (err) {
      setLoadError(err instanceof Error ? err.message : "could not load the transaction")
    } finally {
      setLoading(false)
    }
  }, [id])

  useEffect(() => {
    load()
  }, [load])

  if (loading) {
    return <div className="p-6 text-muted-foreground">Loading transaction…</div>
  }
  if (loadError || !intent) {
    return (
      <div className="p-6 space-y-4">
        <Button variant="ghost" onClick={() => router.back()}>
          <ArrowLeft className="h-4 w-4 mr-2" /> Back
        </Button>
        <div role="alert" className="rounded-md border border-rose-500/30 bg-rose-500/10 p-3 text-sm text-rose-500">
          Could not load this transaction: {loadError ?? "not found"}
        </div>
      </div>
    )
  }

  const digits = minorDigits(intent.currency)
  const captured = intent.amount_captured ?? (intent.status === "succeeded" ? intent.amount : 0)
  const refunded = refunds.filter((r) => r.status === "succeeded").reduce((sum, r) => sum + r.amount, 0)
  const net = captured - refunded
  const canRefund = intent.status === "succeeded" && net > 0
  const timeline = buildTimeline(intent, refunds)

  const openRefund = (open: boolean) => {
    setDialogOpen(open)
    if (open) {
      setRefundAmount((net / 10 ** digits).toFixed(digits))
      setRefundReason("")
      setRefundError(null)
      idempotencyKey.current = idempotencyKey.current ?? crypto.randomUUID()
    }
  }

  const submitRefund = async () => {
    const major = Number(refundAmount)
    const minor = Math.round(major * 10 ** digits)
    if (!Number.isFinite(major) || minor <= 0) {
      setRefundError("Enter an amount greater than zero.")
      return
    }
    if (minor > net) {
      setRefundError(`The most that can be refunded is ${formatMinor(net, intent.currency)}.`)
      return
    }
    setIsRefunding(true)
    setRefundError(null)
    try {
      const res = await fetch("/api/tpt/refunds", {
        method: "POST",
        headers: { "Content-Type": "application/json", "Idempotency-Key": idempotencyKey.current ?? crypto.randomUUID() },
        body: JSON.stringify({ payment_intent: intent.id, amount: minor, reason: refundReason || undefined }),
      })
      const body = await res.json().catch(() => ({}))
      if (!res.ok) throw new Error(body.error?.message || body.error || `refund failed (${res.status})`)
      idempotencyKey.current = null
      setDialogOpen(false)
      await load()
    } catch (err) {
      setRefundError(err instanceof Error ? err.message : "refund failed")
    } finally {
      setIsRefunding(false)
    }
  }

  return (
    <div className="p-6 space-y-6">
      <div className="flex items-center gap-4">
        <Button variant="ghost" size="icon" onClick={() => router.back()} aria-label="Go back to transactions list">
          <ArrowLeft className="h-4 w-4" />
        </Button>
        <div className="flex-1">
          <div className="flex items-center gap-3">
            <h1 className="text-2xl font-bold tracking-tight font-mono">{intent.id}</h1>
            <Badge variant="secondary" className={BADGE_CLASS[intent.status]}>
              {STATUS_LABELS[intent.status]}
            </Badge>
            {!intent.livemode && <Badge variant="outline">test</Badge>}
          </div>
          <p className="text-muted-foreground">
            Created {format(intent.created * 1000, "MMMM d, yyyy 'at' HH:mm")}
          </p>
        </div>
        <Dialog open={dialogOpen} onOpenChange={openRefund}>
          <DialogTrigger asChild>
            <Button variant="destructive" disabled={!canRefund}>
              <RefreshCw className="h-4 w-4 mr-2" />
              Refund
            </Button>
          </DialogTrigger>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>Issue refund</DialogTitle>
            </DialogHeader>
            <div className="space-y-4 py-4">
              <div className="space-y-2">
                <Label htmlFor="amount">Refund amount ({intent.currency.toUpperCase()})</Label>
                <Input
                  id="amount"
                  type="number"
                  step={10 ** -digits}
                  min={10 ** -digits}
                  max={net / 10 ** digits}
                  value={refundAmount}
                  onChange={(e) => setRefundAmount(e.target.value)}
                  aria-describedby="amount-description"
                />
                <p id="amount-description" className="text-sm text-muted-foreground">
                  Maximum: {formatMinor(net, intent.currency)}
                </p>
              </div>
              <div className="space-y-2">
                <Label htmlFor="reason">Reason (optional)</Label>
                <Textarea id="reason" placeholder="Reason for refund..." value={refundReason} onChange={(e) => setRefundReason(e.target.value)} />
              </div>
              {refundError && (
                <div role="alert" className="text-sm text-rose-500">{refundError}</div>
              )}
              <Button className="w-full" variant="destructive" disabled={isRefunding} onClick={submitRefund}>
                {isRefunding ? "Refunding…" : "Issue refund"}
              </Button>
            </div>
          </DialogContent>
        </Dialog>
      </div>

      <div className="grid gap-6 md:grid-cols-3">
        <Card className="p-6 md:col-span-2">
          <div className="grid gap-4 md:grid-cols-2">
            <div>
              <p className="text-sm text-muted-foreground">Amount</p>
              <p className="text-2xl font-bold">{formatMinor(intent.amount, intent.currency)}</p>
            </div>
            <div>
              <p className="text-sm text-muted-foreground">Order</p>
              <p className="font-medium">{intent.metadata?.order_id || "—"}</p>
            </div>
            <div>
              <p className="text-sm text-muted-foreground">Capture method</p>
              <p className="font-medium">{intent.capture_method ?? "—"}</p>
            </div>
            <div>
              <p className="text-sm text-muted-foreground">Mode</p>
              <p className="font-medium">{intent.livemode ? "Live" : "Test"}</p>
            </div>
          </div>
        </Card>
        <Card className="p-6">
          <h3 className="font-semibold mb-3">Summary</h3>
          <div className="space-y-3">
            <div className="flex justify-between">
              <span className="text-muted-foreground">Captured</span>
              <span className="font-medium">{formatMinor(captured, intent.currency)}</span>
            </div>
            <div className="flex justify-between">
              <span className="text-muted-foreground">Refunded</span>
              <span className="font-medium">{formatMinor(refunded, intent.currency)}</span>
            </div>
            <Separator />
            <div className="flex justify-between">
              <span className="font-medium">Net</span>
              <span className="font-bold">{formatMinor(net, intent.currency)}</span>
            </div>
          </div>
        </Card>
      </div>

      <Tabs defaultValue="timeline">
        <TabsList>
          <TabsTrigger value="timeline">Timeline</TabsTrigger>
          <TabsTrigger value="metadata">Metadata</TabsTrigger>
          <TabsTrigger value="related">Related objects</TabsTrigger>
        </TabsList>

        <TabsContent value="timeline" className="mt-4">
          <Card className="p-6">
            <div className="relative">
              {timeline.map((event, index) => {
                const Icon = ICONS[event.icon] ?? Clock
                return (
                  <div key={event.id} className="flex gap-4 pb-8 last:pb-0">
                    <div className="relative">
                      <div className="flex items-center justify-center w-10 h-10 rounded-full bg-muted">
                        <Icon className="h-5 w-5" aria-hidden="true" />
                      </div>
                      {index < timeline.length - 1 && (
                        <div className="absolute left-1/2 top-10 w-0.5 h-full bg-border -translate-x-1/2" aria-hidden="true" />
                      )}
                    </div>
                    <div className="flex-1 pt-1">
                      <div className="flex items-center justify-between">
                        <p className="font-medium">{event.description}</p>
                        <p className="text-sm text-muted-foreground">{format(event.timestamp * 1000, "MMM d, HH:mm:ss")}</p>
                      </div>
                      {event.amount !== undefined && (
                        <p className="text-sm text-muted-foreground mt-1">{formatMinor(event.amount, event.currency)}</p>
                      )}
                    </div>
                  </div>
                )
              })}
            </div>
          </Card>
        </TabsContent>

        <TabsContent value="metadata" className="mt-4">
          <Card className="p-6">
            {Object.keys(intent.metadata ?? {}).length === 0 ? (
              <p className="text-muted-foreground">No metadata on this payment intent.</p>
            ) : (
              <div className="grid gap-4 md:grid-cols-2">
                {Object.entries(intent.metadata ?? {}).map(([key, value]) => (
                  <div key={key}>
                    <p className="text-sm font-medium text-muted-foreground capitalize">{key.replace(/_/g, " ")}</p>
                    <p className="mt-1 break-all">{value}</p>
                  </div>
                ))}
              </div>
            )}
          </Card>
        </TabsContent>

        <TabsContent value="related" className="mt-4">
          <Card className="p-6">
            <div className="space-y-4">
              {intent.payment_method && (
                <div className="flex items-center gap-3 p-3">
                  <CreditCard className="h-5 w-5 text-muted-foreground" />
                  <div>
                    <p className="font-medium">Payment method</p>
                    <p className="text-sm text-muted-foreground font-mono">{intent.payment_method}</p>
                  </div>
                </div>
              )}
              {intent.ledger_transaction_id && (
                <>
                  {intent.payment_method && <Separator />}
                  <div className="flex items-center gap-3 p-3">
                    <User className="h-5 w-5 text-muted-foreground" />
                    <div>
                      <p className="font-medium">Ledger transaction</p>
                      <p className="text-sm text-muted-foreground font-mono">{intent.ledger_transaction_id}</p>
                    </div>
                  </div>
                </>
              )}
              {refunds.length > 0 && (
                <>
                  <Separator />
                  <div className="space-y-2 p-3">
                    <p className="font-medium">Refunds</p>
                    {refunds.map((r) => (
                      <p key={r.id} className="text-sm text-muted-foreground font-mono">
                        {r.id} · {formatMinor(r.amount, r.currency)} · {r.status}
                      </p>
                    ))}
                  </div>
                </>
              )}
            </div>
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  )
}
