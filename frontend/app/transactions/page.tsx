// tpt-payments/frontend/app/transactions/page.tsx
"use client"

import { useState, useMemo } from "react"
import { format } from "date-fns"
import { Search, Filter, ChevronLeft, ChevronRight, ArrowUpDown, Eye } from "lucide-react"
import { useRouter } from "next/navigation"
import { Input } from "@/components/ui/input"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Badge } from "@/components/ui/badge"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"

interface Transaction {
  id: string
  amount: number
  currency: string
  status: "succeeded" | "processing" | "failed" | "refunded"
  created: number
  description: string
  customer: string
  payment_method: string
}

const mockTransactions: Transaction[] = Array.from({ length: 50 }, (_, i) => ({
  id: `pi_${Math.random().toString(36).substring(2, 14)}`,
  amount: Math.floor(Math.random() * 25000) + 500,
  currency: "usd",
  status: ["succeeded", "processing", "failed", "refunded"][Math.floor(Math.random() * 4)] as Transaction["status"],
  created: Date.now() - i * 3600000 * Math.floor(Math.random() * 48),
  description: ["Product purchase", "Subscription renewal", "Donation", "Service payment"][Math.floor(Math.random() * 4)],
  customer: `cus_${Math.random().toString(36).substring(2, 10)}`,
  payment_method: ["card", "bank_transfer"][Math.floor(Math.random() * 2)],
}))

const ITEMS_PER_PAGE = 10

export default function TransactionsPage() {
  const router = useRouter()
  const [search, setSearch] = useState("")
  const [statusFilter, setStatusFilter] = useState<string>("all")
  const [page, setPage] = useState(1)
  const [sortField, setSortField] = useState<keyof Transaction>("created")
  const [sortDirection, setSortDirection] = useState<"asc" | "desc">("desc")

  const filteredTransactions = useMemo(() => {
    let filtered = [...mockTransactions]

    if (search) {
      const searchLower = search.toLowerCase()
      filtered = filtered.filter(tx =>
        tx.id.toLowerCase().includes(searchLower) ||
        tx.description.toLowerCase().includes(searchLower) ||
        tx.customer.toLowerCase().includes(searchLower)
      )
    }

    if (statusFilter !== "all") {
      filtered = filtered.filter(tx => tx.status === statusFilter)
    }

    filtered.sort((a, b) => {
      const aVal = a[sortField]
      const bVal = b[sortField]
      const modifier = sortDirection === "asc" ? 1 : -1
      
      if (typeof aVal === "string" && typeof bVal === "string") {
        return aVal.localeCompare(bVal) * modifier
      }
      
      return ((aVal as number) - (bVal as number)) * modifier
    })

    return filtered
  }, [search, statusFilter, sortField, sortDirection])

  const paginatedTransactions = useMemo(() => {
    const start = (page - 1) * ITEMS_PER_PAGE
    return filteredTransactions.slice(start, start + ITEMS_PER_PAGE)
  }, [filteredTransactions, page])

  const totalPages = Math.ceil(filteredTransactions.length / ITEMS_PER_PAGE)

  const handleSort = (field: keyof Transaction) => {
    if (sortField === field) {
      setSortDirection(sortDirection === "asc" ? "desc" : "asc")
    } else {
      setSortField(field)
      setSortDirection("desc")
    }
  }

  const getStatusBadge = (status: Transaction["status"]) => {
    const variants: Record<Transaction["status"], string> = {
      succeeded: "bg-emerald-500/10 text-emerald-500 hover:bg-emerald-500/20",
      processing: "bg-amber-500/10 text-amber-500 hover:bg-amber-500/20",
      failed: "bg-rose-500/10 text-rose-500 hover:bg-rose-500/20",
      refunded: "bg-slate-500/10 text-slate-500 hover:bg-slate-500/20",
    }
    return variants[status]
  }

  return (
    <div className="p-6 space-y-6">
      <div className="flex flex-col gap-4">
        <div>
          <h1 className="text-2xl font-bold tracking-tight">Transactions</h1>
          <p className="text-muted-foreground">View and manage all payment transactions</p>
        </div>

        <div className="flex flex-col sm:flex-row gap-4">
          <div className="relative flex-1">
            <Search className="absolute left-2.5 top-2.5 h-4 w-4 text-muted-foreground" />
            <Input
              placeholder="Search by ID, description, or customer..."
              className="pl-8"
              value={search}
              onChange={(e) => {
                setSearch(e.target.value)
                setPage(1)
              }}
            />
          </div>
          <Select value={statusFilter} onValueChange={(v) => { setStatusFilter(v); setPage(1) }}>
            <SelectTrigger className="w-full sm:w-[180px]">
              <SelectValue placeholder="Filter by status" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">All statuses</SelectItem>
              <SelectItem value="succeeded">Succeeded</SelectItem>
              <SelectItem value="processing">Processing</SelectItem>
              <SelectItem value="failed">Failed</SelectItem>
              <SelectItem value="refunded">Refunded</SelectItem>
            </SelectContent>
          </Select>
        </div>
      </div>

      <Card>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="cursor-pointer" onClick={() => handleSort("id")}>
                <div className="flex items-center gap-1">
                  ID
                  <ArrowUpDown className="h-3.5 w-3.5 opacity-50" />
                </div>
              </TableHead>
              <TableHead>Description</TableHead>
              <TableHead className="cursor-pointer" onClick={() => handleSort("amount")}>
                <div className="flex items-center gap-1">
                  Amount
                  <ArrowUpDown className="h-3.5 w-3.5 opacity-50" />
                </div>
              </TableHead>
              <TableHead>Status</TableHead>
              <TableHead className="cursor-pointer" onClick={() => handleSort("created")}>
                <div className="flex items-center gap-1">
                  Created
                  <ArrowUpDown className="h-3.5 w-3.5 opacity-50" />
                </div>
              </TableHead>
              <TableHead className="w-[70px]"></TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {paginatedTransactions.map((tx) => (
              <TableRow key={tx.id} className="group">
                <TableCell className="font-mono text-sm">{tx.id}</TableCell>
                <TableCell>{tx.description}</TableCell>
                <TableCell className="font-medium">
                  ${(tx.amount / 100).toFixed(2)}
                </TableCell>
                <TableCell>
                  <Badge variant="secondary" className={getStatusBadge(tx.status)}>
                    {tx.status}
                  </Badge>
                </TableCell>
                <TableCell className="text-muted-foreground text-sm">
                  {format(tx.created, "MMM d, HH:mm")}
                </TableCell>
                <TableCell>
                  <Button
                    variant="ghost"
                    size="icon"
                    className="opacity-0 group-hover:opacity-100"
                    onClick={() => router.push(`/transactions/${tx.id}`)}
                  >
                    <Eye className="h-4 w-4" />
                  </Button>
                </TableCell>
              </TableRow>
            ))}
            {paginatedTransactions.length === 0 && (
              <TableRow>
                <TableCell colSpan={6} className="h-24 text-center text-muted-foreground">
                  No transactions found
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>

        <div className="flex items-center justify-between px-4 py-4 border-t">
          <div className="text-sm text-muted-foreground">
            Showing {((page - 1) * ITEMS_PER_PAGE) + 1}–{Math.min(page * ITEMS_PER_PAGE, filteredTransactions.length)} of {filteredTransactions.length}
          </div>
          <div className="flex items-center gap-2">
            <Button
              variant="ghost"
              size="icon"
              disabled={page === 1}
              onClick={() => setPage(p => p - 1)}
            >
              <ChevronLeft className="h-4 w-4" />
            </Button>
            <div className="text-sm font-medium">
              Page {page} of {totalPages}
            </div>
            <Button
              variant="ghost"
              size="icon"
              disabled={page === totalPages}
              onClick={() => setPage(p => p + 1)}
            >
              <ChevronRight className="h-4 w-4" />
            </Button>
          </div>
        </div>
      </Card>
    </div>
  )
}