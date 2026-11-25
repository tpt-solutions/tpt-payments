// tpt-payments/frontend/app/dashboard/page.tsx
"use client"

import { useEffect, useMemo, useState } from "react"
import { useRouter } from "next/navigation"
import { AreaChart, Area, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer } from "recharts"
import { format, startOfDay, subDays } from "date-fns"
import { CreditCard, Clock, Activity, ArrowUpRight } from "lucide-react"
import { Card } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { useAuth } from "@/components/auth-provider"
import { STATUS_LABELS, formatMinor, minorDigits, type IntentStatus, type PaymentIntent } from "@/lib/payments"

interface ListResponse {
  object: "list"
  data: PaymentIntent[]
  has_more: boolean
}

// The window the overview is computed from. The API pages by cursor; the overview reads one page
// and says so, rather than pretending to cover all history.
const WINDOW = 100

// What an intent has captured: the recorded amount, or the full amount once it has succeeded.
function captured(i: PaymentIntent): number {
  return i.amount_captured ?? (i.status === "succeeded" ? i.amount : 0)
}

export default function DashboardPage() {
  const { user } = useAuth()
  const router = useRouter()
  const [intents, setIntents] = useState<PaymentIntent[]>([])
  const [hasMore, setHasMore] = useState(false)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    fetch(`/api/tpt/payment_intents?limit=${WINDOW}`)
      .then(async (res) => {
        const body = await res.json().catch(() => ({}))
        if (!res.ok) throw new Error(body.error || `request failed (${res.status})`)
        return body as ListResponse
      })
      .then((body) => {
        setIntents(body.data)
        setHasMore(body.has_more)
      })
      .catch((err) => setError(err.message))
      .finally(() => setLoading(false))
  }, [])

  // Everything is computed in the currency most of the window uses, so no amounts of different
  // currencies are added together.
  const currency = useMemo(() => {
    const counts = new Map<string, number>()
    for (const i of intents) counts.set(i.currency, (counts.get(i.currency) ?? 0) + 1)
    let best = "usd"
    let max = 0
    for (const [c, n] of counts) if (n > max) { best = c; max = n }
    return best
  }, [intents])
  const inCurrency = useMemo(() => intents.filter((i) => i.currency === currency), [intents, currency])

  const volume = inCurrency.reduce((sum, i) => sum + captured(i), 0)
  const succeeded = inCurrency.filter((i) => i.status === "succeeded").length
  const canceled = inCurrency.filter((i) => i.status === "canceled").length
  const finished = succeeded + canceled
  const pending = inCurrency.filter((i) => ["processing", "requires_action", "requires_capture"].includes(i.status)).length

  // Captured volume per day for the last seven days, in local time.
  const daily = useMemo(() => {
    const days = Array.from({ length: 7 }, (_, k) => startOfDay(subDays(new Date(), 6 - k)))
    return days.map((day) => {
      const next = day.getTime() + 86_400_000
      const inDay = inCurrency.filter((i) => {
        const t = i.created * 1000
        return t >= day.getTime() && t < next
      })
      return {
        date: format(day, "EEE"),
        volume: inDay.reduce((sum, i) => sum + captured(i), 0),
        count: inDay.length,
      }
    })
  }, [inCurrency])

  const digits = minorDigits(currency)
  const majorTick = (value: number) => (value / 10 ** digits).toFixed(0)

  if (loading) {
    return (
      <div className="flex items-center justify-center min-h-[60vh]">
        <div className="animate-pulse text-muted-foreground">Loading dashboard…</div>
      </div>
    )
  }

  return (
    <div className="p-6 space-y-6">
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold tracking-tight">Dashboard</h1>
          <p className="text-muted-foreground">Welcome back, {user?.name}</p>
        </div>
        <Button onClick={() => router.push("/transactions")}>View transactions</Button>
      </div>

      {error && <div role="alert" className="rounded-md border border-rose-500/30 bg-rose-500/10 p-3 text-sm text-rose-500">Could not load payments: {error}</div>}

      <p className="text-sm text-muted-foreground">
        Based on the latest {intents.length}{hasMore ? "+" : ""} payment intents in {currency.toUpperCase()}.
        {intents.length !== inCurrency.length && " Other currencies are not included."}
      </p>

      <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
        <Stat icon={CreditCard} label="Captured volume" value={formatMinor(volume, currency)} />
        <Stat icon={Activity} label="Payment intents" value={inCurrency.length.toLocaleString()} />
        <Stat
          icon={ArrowUpRight}
          label="Succeeded share"
          value={finished === 0 ? "—" : `${Math.round((succeeded / finished) * 1000) / 10}%`}
          hint={finished === 0 ? "No finished payments yet" : `of ${finished} finished (succeeded or canceled)`}
        />
        <Stat icon={Clock} label="Awaiting action" value={pending.toString()} hint="processing, authorized or needs action" />
      </div>

      <Card className="p-6">
        <h3 className="text-lg font-semibold mb-4">Captured volume, last 7 days</h3>
        <div className="h-[300px]">
          <ResponsiveContainer width="100%" height="100%">
            <AreaChart data={daily}>
              <defs>
                <linearGradient id="colorVolume" x1="0" y1="0" x2="0" y2="1">
                  <stop offset="5%" stopColor="hsl(var(--primary))" stopOpacity={0.3} />
                  <stop offset="95%" stopColor="hsl(var(--primary))" stopOpacity={0} />
                </linearGradient>
              </defs>
              <CartesianGrid strokeDasharray="3 3" className="stroke-muted" />
              <XAxis dataKey="date" tick={{ fill: "hsl(var(--muted-foreground))" }} />
              <YAxis tickFormatter={majorTick} tick={{ fill: "hsl(var(--muted-foreground))" }} />
              <Tooltip
                contentStyle={{ backgroundColor: "hsl(var(--card))", border: "1px solid hsl(var(--border))", borderRadius: "var(--radius)" }}
                formatter={(value) => [formatMinor(Number(value), currency), "Captured"]}
              />
              <Area type="monotone" dataKey="volume" stroke="hsl(var(--primary))" strokeWidth={2} fill="url(#colorVolume)" />
            </AreaChart>
          </ResponsiveContainer>
        </div>
      </Card>

      <Card className="p-6">
        <div className="flex items-center justify-between mb-4">
          <h3 className="text-lg font-semibold">Recent activity</h3>
          <Button variant="ghost" size="sm" onClick={() => router.push("/transactions")}>View all</Button>
        </div>
        {intents.length === 0 ? (
          <p className="text-muted-foreground py-4">No payments yet. Create one with the quickstart in the developer docs.</p>
        ) : (
          <div className="divide-y">
            {intents.slice(0, 5).map((tx) => (
              <button
                key={tx.id}
                type="button"
                onClick={() => router.push(`/transactions/${tx.id}`)}
                className="w-full py-4 flex items-center justify-between hover:bg-muted/50 px-2 -mx-2 rounded text-left"
              >
                <div className="space-y-1">
                  <p className="font-medium">{tx.metadata?.order_id ? `Order ${tx.metadata.order_id}` : "Payment"}</p>
                  <p className="text-sm text-muted-foreground flex items-center gap-2">
                    <span className="font-mono">{tx.id}</span>
                    <span>•</span>
                    <span>{format(tx.created * 1000, "MMM d, HH:mm")}</span>
                  </p>
                </div>
                <div className="text-right space-y-1">
                  <p className="font-medium">{formatMinor(tx.amount, tx.currency)}</p>
                  <Badge variant="outline">{STATUS_LABELS[tx.status as IntentStatus] ?? tx.status}</Badge>
                </div>
              </button>
            ))}
          </div>
        )}
      </Card>
    </div>
  )
}

function Stat({ icon: Icon, label, value, hint }: { icon: React.ElementType; label: string; value: string; hint?: string }) {
  return (
    <Card className="p-6">
      <div className="flex items-center justify-between">
        <div className="space-y-1">
          <p className="text-sm text-muted-foreground">{label}</p>
          <p className="text-2xl font-bold">{value}</p>
        </div>
        <div className="p-2 bg-primary/10 rounded-full">
          <Icon className="h-5 w-5 text-primary" />
        </div>
      </div>
      {hint && <p className="text-xs text-muted-foreground mt-2">{hint}</p>}
    </Card>
  )
}
