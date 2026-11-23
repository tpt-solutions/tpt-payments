// tpt-payments/frontend/app/dashboard/page.tsx
"use client"

import { useEffect, useState } from "react"
import { LineChart, Line, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer, AreaChart, Area } from "recharts"
import { format } from "date-fns"
import { CreditCard, ArrowUpRight, ArrowDownRight, Clock, Activity } from "lucide-react"
import { Card } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import { useAuth } from "@/components/auth-provider"

interface Transaction {
  id: string
  amount: number
  status: string
  created: number
  description: string
}

interface VolumeData {
  date: string
  volume: number
  count: number
}

export default function DashboardPage() {
  const { user } = useAuth()
  const [transactions, setTransactions] = useState<Transaction[]>([])
  const [volumeData, setVolumeData] = useState<VolumeData[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    const fetchData = async () => {
      try {
        // Mock data - replace with actual API calls
        const mockVolume = Array.from({ length: 7 }, (_, i) => ({
          date: format(Date.now() - (6 - i) * 86400000, "EEE"),
          volume: Math.floor(Math.random() * 50000) + 10000,
          count: Math.floor(Math.random() * 200) + 50,
        }))

        const mockTransactions = Array.from({ length: 5 }, () => ({
          id: `pi_${Math.random().toString(36).substring(2, 10)}`,
          amount: Math.floor(Math.random() * 10000) + 500,
          status: ["succeeded", "processing", "failed"][Math.floor(Math.random() * 3)],
          created: Date.now() - Math.floor(Math.random() * 86400000),
          description: "Payment from customer",
        }))

        setVolumeData(mockVolume)
        setTransactions(mockTransactions)
      } finally {
        setLoading(false)
      }
    }

    fetchData()
  }, [])

  const totalVolume = volumeData.reduce((sum, d) => sum + d.volume, 0)
  const totalTransactions = volumeData.reduce((sum, d) => sum + d.count, 0)
  const successRate = 97.2 // Mock value

  if (loading) {
    return (
      <div className="flex items-center justify-center min-h-[60vh]">
        <div className="animate-pulse text-muted-foreground">Loading dashboard...</div>
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
        <Button>View Transactions</Button>
      </div>

      {/* Stats Cards */}
      <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
        <Card className="p-6">
          <div className="flex items-center justify-between">
            <div className="space-y-1">
              <p className="text-sm text-muted-foreground">Total Volume</p>
              <p className="text-2xl font-bold">${(totalVolume / 100).toLocaleString()}</p>
            </div>
            <div className="p-2 bg-primary/10 rounded-full">
              <CreditCard className="h-5 w-5 text-primary" />
            </div>
          </div>
          <div className="flex items-center text-sm mt-2">
            <ArrowUpRight className="h-4 w-4 text-emerald-500 mr-1" />
            <span className="text-emerald-500 font-medium">+12.5%</span>
            <span className="text-muted-foreground ml-1">vs last week</span>
          </div>
        </Card>

        <Card className="p-6">
          <div className="flex items-center justify-between">
            <div className="space-y-1">
              <p className="text-sm text-muted-foreground">Transactions</p>
              <p className="text-2xl font-bold">{totalTransactions.toLocaleString()}</p>
            </div>
            <div className="p-2 bg-primary/10 rounded-full">
              <Activity className="h-5 w-5 text-primary" />
            </div>
          </div>
          <div className="flex items-center text-sm mt-2">
            <ArrowUpRight className="h-4 w-4 text-emerald-500 mr-1" />
            <span className="text-emerald-500 font-medium">+8.2%</span>
            <span className="text-muted-foreground ml-1">vs last week</span>
          </div>
        </Card>

        <Card className="p-6">
          <div className="flex items-center justify-between">
            <div className="space-y-1">
              <p className="text-sm text-muted-foreground">Success Rate</p>
              <p className="text-2xl font-bold">{successRate}%</p>
            </div>
            <div className="p-2 bg-emerald-100 dark:bg-emerald-900/30 rounded-full">
              <ArrowUpRight className="h-5 w-5 text-emerald-500" />
            </div>
          </div>
          <div className="flex items-center text-sm mt-2">
            <ArrowUpRight className="h-4 w-4 text-emerald-500 mr-1" />
            <span className="text-emerald-500 font-medium">+0.3%</span>
            <span className="text-muted-foreground ml-1">vs last week</span>
          </div>
        </Card>

        <Card className="p-6">
          <div className="flex items-center justify-between">
            <div className="space-y-1">
              <p className="text-sm text-muted-foreground">Pending</p>
              <p className="text-2xl font-bold">12</p>
            </div>
            <div className="p-2 bg-amber-100 dark:bg-amber-900/30 rounded-full">
              <Clock className="h-5 w-5 text-amber-500" />
            </div>
          </div>
          <div className="flex items-center text-sm mt-2">
            <ArrowDownRight className="h-4 w-4 text-rose-500 mr-1" />
            <span className="text-rose-500 font-medium">-3</span>
            <span className="text-muted-foreground ml-1">vs last week</span>
          </div>
        </Card>
      </div>

      {/* Volume Chart */}
      <Card className="p-6">
        <h3 className="text-lg font-semibold mb-4">Transaction Volume</h3>
        <div className="h-[300px]">
          <ResponsiveContainer width="100%" height="100%">
            <AreaChart data={volumeData}>
              <defs>
                <linearGradient id="colorVolume" x1="0" y1="0" x2="0" y2="1">
                  <stop offset="5%" stopColor="hsl(var(--primary))" stopOpacity={0.3} />
                  <stop offset="95%" stopColor="hsl(var(--primary))" stopOpacity={0} />
                </linearGradient>
              </defs>
              <CartesianGrid strokeDasharray="3 3" className="stroke-muted" />
              <XAxis 
                dataKey="date" 
                className="text-xs"
                tick={{ fill: "hsl(var(--muted-foreground))" }}
              />
              <YAxis 
                className="text-xs"
                tickFormatter={(value) => `$${(value / 100).toFixed(0)}`}
                tick={{ fill: "hsl(var(--muted-foreground))" }}
              />
              <Tooltip
                contentStyle={{
                  backgroundColor: "hsl(var(--card))",
                  border: "1px solid hsl(var(--border))",
                  borderRadius: "var(--radius)",
                }}
                formatter={(value) => [`$${(Number(value) / 100).toLocaleString()}`, "Volume"]}
              />
              <Area
                type="monotone"
                dataKey="volume"
                stroke="hsl(var(--primary))"
                strokeWidth={2}
                fill="url(#colorVolume)"
              />
            </AreaChart>
          </ResponsiveContainer>
        </div>
      </Card>

      {/* Recent Activity */}
      <Card className="p-6">
        <div className="flex items-center justify-between mb-4">
          <h3 className="text-lg font-semibold">Recent Activity</h3>
          <Button variant="ghost" size="sm">View All</Button>
        </div>
        <div className="divide-y">
          {transactions.map((tx) => (
            <div key={tx.id} className="py-4 flex items-center justify-between hover:bg-muted/50 px-2 -mx-2 rounded cursor-pointer">
              <div className="space-y-1">
                <p className="font-medium">{tx.description}</p>
                <p className="text-sm text-muted-foreground flex items-center gap-2">
                  <span className="font-mono">{tx.id}</span>
                  <span>•</span>
                  <span>{format(tx.created, "MMM d, HH:mm")}</span>
                </p>
              </div>
              <div className="text-right">
                <p className="font-medium">${(tx.amount / 100).toFixed(2)}</p>
                <p className={`text-sm ${
                  tx.status === "succeeded" ? "text-emerald-500" :
                  tx.status === "processing" ? "text-amber-500" :
                  "text-rose-500"
                }`}>
                  {tx.status}
                </p>
              </div>
            </div>
          ))}
        </div>
      </Card>
    </div>
  )
}