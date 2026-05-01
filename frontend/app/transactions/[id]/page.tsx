// tpt-payments/frontend/app/transactions/[id]/page.tsx
"use client"

import { useState } from "react"
import { format } from "date-fns"
import { ArrowLeft, Clock, CreditCard, RefreshCw, AlertCircle, CheckCircle, ArrowRight, DollarSign, FileText, User } from "lucide-react"
import { useRouter } from "next/navigation"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { Badge } from "@/components/ui/badge"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Separator } from "@/components/ui/separator"
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"

interface TimelineEvent {
  id: string
  type: string
  status: string
  timestamp: number
  description: string
  amount?: number
}

interface Transaction {
  id: string
  amount: number
  currency: string
  status: string
  created: number
  description: string
  customer: string
  payment_method: string
  metadata: Record<string, string>
  timeline: TimelineEvent[]
}

const mockTransaction: Transaction = {
  id: "pi_01h7nq3qz8k7x9v5b2n4m6l8",
  amount: 12500,
  currency: "usd",
  status: "succeeded",
  created: Date.now() - 3600000 * 2,
  description: "Premium plan subscription",
  customer: "cus_abc123def456",
  payment_method: "card_xyz789",
  metadata: {
    order_id: "ORD-2026-00142",
    customer_email: "customer@example.com",
    ip_address: "192.168.1.100",
    user_agent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36",
  },
  timeline: [
    {
      id: "evt_1",
      type: "created",
      status: "success",
      timestamp: Date.now() - 3600000 * 2,
      description: "Payment intent created",
    },
    {
      id: "evt_2",
      type: "confirm",
      status: "success",
      timestamp: Date.now() - 3600000 * 2 + 15000,
      description: "Payment method confirmed",
    },
    {
      id: "evt_3",
      type: "authorize",
      status: "success",
      timestamp: Date.now() - 3600000 * 2 + 25000,
      description: "Authorization successful",
      amount: 12500,
    },
    {
      id: "evt_4",
      type: "capture",
      status: "success",
      timestamp: Date.now() - 3600000 * 2 + 35000,
      description: "Payment captured successfully",
      amount: 12500,
    },
  ],
}

export default function TransactionDetailPage({ params }: { params: { id: string } }) {
  const router = useRouter()
  const [refundAmount, setRefundAmount] = useState(`${mockTransaction.amount / 100}`)
  const [refundReason, setRefundReason] = useState("")
  const [isRefunding, setIsRefunding] = useState(false)

  const getStatusBadge = (status: string) => {
    const variants: Record<string, string> = {
      succeeded: "bg-emerald-500/10 text-emerald-500",
      processing: "bg-amber-500/10 text-amber-500",
      failed: "bg-rose-500/10 text-rose-500",
      refunded: "bg-slate-500/10 text-slate-500",
    }
    return variants[status] || ""
  }

  const getTimelineIcon = (type: string) => {
    switch (type) {
      case "created": return FileText
      case "confirm": return CreditCard
      case "authorize": return CheckCircle
      case "capture": return DollarSign
      case "refund": return RefreshCw
      default: return Clock
    }
  }

  const handleRefund = async () => {
    setIsRefunding(true)
    // API call would go here
    setTimeout(() => {
      setIsRefunding(false)
      router.refresh()
    }, 2000)
  }

  return (
    <div className="p-6 space-y-6">
      <div className="flex items-center gap-4">
        <Button
          variant="ghost"
          size="icon"
          onClick={() => router.back()}
          aria-label="Go back to transactions list"
        >
          <ArrowLeft className="h-4 w-4" />
        </Button>
        <div className="flex-1">
          <div className="flex items-center gap-3">
            <h1 className="text-2xl font-bold tracking-tight">{mockTransaction.id}</h1>
            <Badge variant="secondary" className={getStatusBadge(mockTransaction.status)}>
              {mockTransaction.status}
            </Badge>
          </div>
          <p className="text-muted-foreground">
            Created {format(mockTransaction.created, "MMMM d, yyyy 'at' HH:mm")}
          </p>
        </div>

        <Dialog>
          <DialogTrigger asChild>
            <Button variant="destructive">
              <RefreshCw className="h-4 w-4 mr-2" />
              Refund
            </Button>
          </DialogTrigger>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>Issue Refund</DialogTitle>
            </DialogHeader>
            <div className="space-y-4 py-4">
              <div className="space-y-2">
                <Label htmlFor="amount">Refund Amount</Label>
                <Input
                  id="amount"
                  type="number"
                  step="0.01"
                  min="0.01"
                  max={mockTransaction.amount / 100}
                  value={refundAmount}
                  onChange={(e) => setRefundAmount(e.target.value)}
                  aria-describedby="amount-description"
                />
                <p id="amount-description" className="text-sm text-muted-foreground">
                  Maximum: ${(mockTransaction.amount / 100).toFixed(2)}
                </p>
              </div>
              <div className="space-y-2">
                <Label htmlFor="reason">Reason (optional)</Label>
                <Textarea
                  id="reason"
                  placeholder="Reason for refund..."
                  value={refundReason}
                  onChange={(e) => setRefundReason(e.target.value)}
                />
              </div>
              <Button 
                className="w-full" 
                variant="destructive"
                disabled={isRefunding || !refundAmount}
                onClick={handleRefund}
              >
                {isRefunding ? "Processing refund..." : `Refund $${parseFloat(refundAmount).toFixed(2)}`}
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
              <p className="text-2xl font-bold">${(mockTransaction.amount / 100).toFixed(2)}</p>
            </div>
            <div>
              <p className="text-sm text-muted-foreground">Currency</p>
              <p className="text-2xl font-bold uppercase">{mockTransaction.currency}</p>
            </div>
            <div>
              <p className="text-sm text-muted-foreground">Description</p>
              <p className="font-medium">{mockTransaction.description}</p>
            </div>
            <div>
              <p className="text-sm text-muted-foreground">Customer</p>
              <p className="font-medium font-mono">{mockTransaction.customer}</p>
            </div>
          </div>
        </Card>

        <Card className="p-6">
          <h3 className="font-semibold mb-3">Summary</h3>
          <div className="space-y-3">
            <div className="flex justify-between">
              <span className="text-muted-foreground">Captured</span>
              <span className="font-medium">${(mockTransaction.amount / 100).toFixed(2)}</span>
            </div>
            <div className="flex justify-between">
              <span className="text-muted-foreground">Refunded</span>
              <span className="font-medium">$0.00</span>
            </div>
            <Separator />
            <div className="flex justify-between">
              <span className="font-medium">Net</span>
              <span className="font-bold">${(mockTransaction.amount / 100).toFixed(2)}</span>
            </div>
          </div>
        </Card>
      </div>

      <Tabs defaultValue="timeline">
        <TabsList>
          <TabsTrigger value="timeline">Timeline</TabsTrigger>
          <TabsTrigger value="metadata">Metadata</TabsTrigger>
          <TabsTrigger value="related">Related Objects</TabsTrigger>
        </TabsList>

        <TabsContent value="timeline" className="mt-4">
          <Card className="p-6">
            <div className="relative">
              {mockTransaction.timeline.map((event, index) => {
                const Icon = getTimelineIcon(event.type)
                return (
                  <div key={event.id} className="flex gap-4 pb-8 last:pb-0">
                    <div className="relative">
                      <div className="flex items-center justify-center w-10 h-10 rounded-full bg-muted">
                        <Icon className="h-5 w-5" aria-hidden="true" />
                      </div>
                      {index < mockTransaction.timeline.length - 1 && (
                        <div className="absolute left-1/2 top-10 w-0.5 h-full bg-border -translate-x-1/2" aria-hidden="true" />
                      )}
                    </div>
                    <div className="flex-1 pt-1">
                      <div className="flex items-center justify-between">
                        <p className="font-medium">{event.description}</p>
                        <p className="text-sm text-muted-foreground">
                          {format(event.timestamp, "HH:mm:ss")}
                        </p>
                      </div>
                      {event.amount && (
                        <p className="text-sm text-muted-foreground mt-1">
                          ${(event.amount / 100).toFixed(2)}
                        </p>
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
            <div className="grid gap-4 md:grid-cols-2">
              {Object.entries(mockTransaction.metadata).map(([key, value]) => (
                <div key={key}>
                  <p className="text-sm font-medium text-muted-foreground capitalize">
                    {key.replace(/_/g, " ")}
                  </p>
                  <p className="mt-1 break-all">{value}</p>
                </div>
              ))}
            </div>
          </Card>
        </TabsContent>

        <TabsContent value="related" className="mt-4">
          <Card className="p-6">
            <div className="space-y-4">
              <div className="flex items-center justify-between p-3 rounded-lg hover:bg-muted/50 cursor-pointer">
                <div className="flex items-center gap-3">
                  <CreditCard className="h-5 w-5 text-muted-foreground" />
                  <div>
                    <p className="font-medium">Payment Method</p>
                    <p className="text-sm text-muted-foreground font-mono">{mockTransaction.payment_method}</p>
                  </div>
                </div>
                <ArrowRight className="h-4 w-4 text-muted-foreground" />
              </div>
              <Separator />
              <div className="flex items-center justify-between p-3 rounded-lg hover:bg-muted/50 cursor-pointer">
                <div className="flex items-center gap-3">
                  <User className="h-5 w-5 text-muted-foreground" />
                  <div>
                    <p className="font-medium">Customer</p>
                    <p className="text-sm text-muted-foreground font-mono">{mockTransaction.customer}</p>
                  </div>
                </div>
                <ArrowRight className="h-4 w-4 text-muted-foreground" />
              </div>
            </div>
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  )
}