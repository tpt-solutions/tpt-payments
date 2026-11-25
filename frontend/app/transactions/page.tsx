// tpt-payments/frontend/app/transactions/page.tsx
"use client"

import { useEffect, useState } from "react"
import { format } from "date-fns"
import { Search, ChevronLeft, ChevronRight, Eye } from "lucide-react"
import { useRouter } from "next/navigation"
import { Input } from "@/components/ui/input"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Badge } from "@/components/ui/badge"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { STATUS_LABELS, formatMinor, type IntentStatus, type PaymentIntent } from "@/lib/payments"

interface ListResponse {
  object: "list"
  data: PaymentIntent[]
  has_more: boolean
}

const PAGE_SIZE = 10

const BADGE_CLASS: Record<IntentStatus, string> = {
  succeeded: "bg-emerald-500/10 text-emerald-500 hover:bg-emerald-500/20",
  requires_capture: "bg-sky-500/10 text-sky-500 hover:bg-sky-500/20",
  processing: "bg-amber-500/10 text-amber-500 hover:bg-amber-500/20",
  requires_action: "bg-amber-500/10 text-amber-500 hover:bg-amber-500/20",
  requires_payment_method: "bg-slate-500/10 text-slate-500 hover:bg-slate-500/20",
  requires_confirmation: "bg-slate-500/10 text-slate-500 hover:bg-slate-500/20",
  canceled: "bg-rose-500/10 text-rose-500 hover:bg-rose-500/20",
}

export default function TransactionsPage() {
  const router = useRouter()
  const [status, setStatus] = useState<string>("all")
  const [search, setSearch] = useState("")
  // cursors[i] is the starting_after value for page i+1; the first page has none.
  const [cursors, setCursors] = useState<(string | null)[]>([null])
  // Each result is tagged with the request it answers. A result is current only while its tag
  // matches the request on screen, so loading and errors are derived, not set inside the effect.
  const requestKey = `${status}|${cursors.join(",")}`
  const [result, setResult] = useState<{ key: string; rows: PaymentIntent[]; hasMore: boolean; error: string | null } | null>(null)
  const current = result?.key === requestKey ? result : null
  const loading = current === null
  const rows = current?.rows ?? []
  const hasMore = current?.hasMore ?? false
  const error = current?.error ?? null

  const pageIndex = cursors.length - 1

  useEffect(() => {
    const controller = new AbortController()
    const params = new URLSearchParams({ limit: String(PAGE_SIZE) })
    if (status !== "all") params.set("status", status)
    const cursor = cursors[cursors.length - 1]
    if (cursor) params.set("starting_after", cursor)

    fetch(`/api/tpt/payment_intents?${params}`, { signal: controller.signal })
      .then(async (res) => {
        const body = await res.json().catch(() => ({}))
        if (!res.ok) throw new Error(body.error || `request failed (${res.status})`)
        return body as ListResponse
      })
      .then((body) => setResult({ key: requestKey, rows: body.data, hasMore: body.has_more, error: null }))
      .catch((err) => {
        if (err.name === "AbortError") return
        setResult({ key: requestKey, rows: [], hasMore: false, error: err.message })
      })
    return () => controller.abort()
  }, [status, cursors, requestKey])

  // The API pages by cursor and filters by status. Free-text search can only narrow the page
  // that is loaded, so it is labelled that way.
  const visible = search
    ? rows.filter((tx) =>
        tx.id.toLowerCase().includes(search.toLowerCase()) ||
        (tx.metadata?.order_id ?? "").toLowerCase().includes(search.toLowerCase()))
    : rows

  const changeFilter = (value: string) => {
    setStatus(value)
    setCursors([null])
  }

  const next = () => {
    const last = rows[rows.length - 1]
    if (last) setCursors((c) => [...c, last.id])
  }

  const prev = () => setCursors((c) => (c.length > 1 ? c.slice(0, -1) : c))

  return (
    <div className="p-6 space-y-6">
      <div className="flex flex-col gap-4">
        <div>
          <h1 className="text-2xl font-bold tracking-tight">Transactions</h1>
          <p className="text-muted-foreground">Payment intents, newest first</p>
        </div>

        <div className="flex flex-col sm:flex-row gap-4">
          <div className="relative flex-1">
            <Search className="absolute left-2.5 top-2.5 h-4 w-4 text-muted-foreground" />
            <Input
              placeholder="Filter this page by ID or order ID..."
              className="pl-8"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
            />
          </div>
          <Select value={status} onValueChange={changeFilter}>
            <SelectTrigger className="w-full sm:w-[220px]">
              <SelectValue placeholder="Filter by status" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">All statuses</SelectItem>
              {(Object.keys(STATUS_LABELS) as IntentStatus[]).map((s) => (
                <SelectItem key={s} value={s}>{STATUS_LABELS[s]}</SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      </div>

      {error && (
        <div role="alert" className="rounded-md border border-rose-500/30 bg-rose-500/10 p-3 text-sm text-rose-500">
          Could not load transactions: {error}
        </div>
      )}

      <Card>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>ID</TableHead>
              <TableHead>Order</TableHead>
              <TableHead>Amount</TableHead>
              <TableHead>Status</TableHead>
              <TableHead>Created</TableHead>
              <TableHead className="w-[70px]"></TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {visible.map((tx) => (
              <TableRow key={tx.id} className="group">
                <TableCell className="font-mono text-sm">{tx.id}</TableCell>
                <TableCell>{tx.metadata?.order_id || "—"}</TableCell>
                <TableCell className="font-medium">{formatMinor(tx.amount, tx.currency)}</TableCell>
                <TableCell>
                  <Badge variant="secondary" className={BADGE_CLASS[tx.status]}>
                    {STATUS_LABELS[tx.status]}
                  </Badge>
                </TableCell>
                <TableCell className="text-muted-foreground text-sm">
                  {format(tx.created * 1000, "MMM d, HH:mm")}
                </TableCell>
                <TableCell>
                  <Button
                    variant="ghost"
                    size="icon"
                    className="opacity-0 group-hover:opacity-100"
                    aria-label={`View ${tx.id}`}
                    onClick={() => router.push(`/transactions/${tx.id}`)}
                  >
                    <Eye className="h-4 w-4" />
                  </Button>
                </TableCell>
              </TableRow>
            ))}
            {!loading && visible.length === 0 && (
              <TableRow>
                <TableCell colSpan={6} className="h-24 text-center text-muted-foreground">
                  {error ? "Unavailable" : "No transactions found"}
                </TableCell>
              </TableRow>
            )}
            {loading && (
              <TableRow>
                <TableCell colSpan={6} className="h-24 text-center text-muted-foreground">
                  Loading…
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>

        <div className="flex items-center justify-between px-4 py-4 border-t">
          <div className="text-sm text-muted-foreground">
            {rows.length > 0 ? `${rows.length} on this page` : " "}
          </div>
          <div className="flex items-center gap-2">
            <Button variant="ghost" size="icon" disabled={pageIndex === 0 || loading} onClick={prev} aria-label="Previous page">
              <ChevronLeft className="h-4 w-4" />
            </Button>
            <div className="text-sm font-medium">Page {pageIndex + 1}</div>
            <Button variant="ghost" size="icon" disabled={!hasMore || loading} onClick={next} aria-label="Next page">
              <ChevronRight className="h-4 w-4" />
            </Button>
          </div>
        </div>
      </Card>
    </div>
  )
}
