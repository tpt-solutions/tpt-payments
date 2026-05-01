// tpt-payments/frontend/app/api-keys/page.tsx
"use client"

import { useState } from "react"
import { Copy, Plus, Trash2, Eye, EyeOff, RefreshCw } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Badge } from "@/components/ui/badge"
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Label } from "@/components/ui/label"
import { Switch } from "@/components/ui/switch"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { toast } from "@/components/ui/use-toast"

interface ApiKey {
  id: string
  name: string
  key: string
  type: "publishable" | "secret"
  mode: "test" | "live"
  created: number
  lastUsed: number | null
  revoked: boolean
}

const mockKeys: ApiKey[] = [
  {
    id: "key_01h7nq3qz8k7x9v5b2n4m6l8",
    name: "Production Key",
    key: "sk_live_abc123def456ghi789",
    type: "secret",
    mode: "live",
    created: Date.now() - 86400000 * 30,
    lastUsed: Date.now() - 3600000,
    revoked: false,
  },
  {
    id: "key_01h7nq5ab1c2d3e4f5g6h7i8",
    name: "Test Key",
    key: "pk_test_xyz789abc123def456",
    type: "publishable",
    mode: "test",
    created: Date.now() - 86400000 * 30,
    lastUsed: Date.now() - 86400000,
    revoked: false,
  },
]

export default function ApiKeysPage() {
  const [keys, setKeys] = useState<ApiKey[]>(mockKeys)
  const [revealedKeys, setRevealedKeys] = useState<Set<string>>(new Set())
  const [newKeyName, setNewKeyName] = useState("")
  const [newKeyType, setNewKeyType] = useState<"publishable" | "secret">("secret")
  const [newKeyMode, setNewKeyMode] = useState<"test" | "live">("test")
  const [isCreating, setIsCreating] = useState(false)
  const [activeTab, setActiveTab] = useState<string>("all")

  const filteredKeys = keys.filter(key => {
    if (activeTab === "all") return true
    if (activeTab === "live") return key.mode === "live"
    if (activeTab === "test") return key.mode === "test"
    return true
  })

  const copyToClipboard = async (key: string) => {
    await navigator.clipboard.writeText(key)
    toast({
      description: "API key copied to clipboard",
    })
  }

  const toggleReveal = (id: string) => {
    setRevealedKeys(prev => {
      const next = new Set(prev)
      if (next.has(id)) {
        next.delete(id)
      } else {
        next.add(id)
      }
      return next
    })
  }

  const revokeKey = (id: string) => {
    setKeys(prev => prev.map(k => k.id === id ? { ...k, revoked: true } : k))
  }

  const createKey = async () => {
    setIsCreating(true)
    // API call would go here
    setTimeout(() => {
      const newKey: ApiKey = {
        id: `key_${Math.random().toString(36).substring(2, 14)}`,
        name: newKeyName,
        key: newKeyType === "secret" 
          ? `sk_${newKeyMode}_${Math.random().toString(36).substring(2, 24)}`
          : `pk_${newKeyMode}_${Math.random().toString(36).substring(2, 24)}`,
        type: newKeyType,
        mode: newKeyMode,
        created: Date.now(),
        lastUsed: null,
        revoked: false,
      }
      setKeys(prev => [newKey, ...prev])
      setNewKeyName("")
      setIsCreating(false)
    }, 1000)
  }

  const maskKey = (key: string) => {
    const prefix = key.slice(0, 10)
    const suffix = key.slice(-4)
    return `${prefix}••••••••${suffix}`
  }

  return (
    <div className="p-6 space-y-6">
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold tracking-tight">API Keys</h1>
          <p className="text-muted-foreground">Manage your publishable and secret API keys</p>
        </div>

        <Dialog>
          <DialogTrigger asChild>
            <Button>
              <Plus className="h-4 w-4 mr-2" />
              Create API Key
            </Button>
          </DialogTrigger>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>Create API Key</DialogTitle>
            </DialogHeader>
            <div className="space-y-4 py-4">
              <div className="space-y-2">
                <Label htmlFor="name">Key Name</Label>
                <Input
                  id="name"
                  placeholder="Production server key"
                  value={newKeyName}
                  onChange={(e) => setNewKeyName(e.target.value)}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="type">Key Type</Label>
                <Select value={newKeyType} onValueChange={(v: any) => setNewKeyType(v)}>
                  <SelectTrigger id="type">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="secret">Secret Key</SelectItem>
                    <SelectItem value="publishable">Publishable Key</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-2">
                <Label htmlFor="mode">Environment</Label>
                <Select value={newKeyMode} onValueChange={(v: any) => setNewKeyMode(v)}>
                  <SelectTrigger id="mode">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="test">Test Mode</SelectItem>
                    <SelectItem value="live">Live Mode</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <Button 
                className="w-full" 
                disabled={isCreating || !newKeyName}
                onClick={createKey}
              >
                {isCreating ? "Creating key..." : "Create API Key"}
              </Button>
            </div>
          </DialogContent>
        </Dialog>
      </div>

      <Tabs defaultValue="all" value={activeTab} onValueChange={setActiveTab}>
        <TabsList>
          <TabsTrigger value="all">All Keys</TabsTrigger>
          <TabsTrigger value="live">Live Mode</TabsTrigger>
          <TabsTrigger value="test">Test Mode</TabsTrigger>
        </TabsList>

        <TabsContent value={activeTab} className="mt-4">
          <Card>
            <div className="divide-y">
              {filteredKeys.map((key) => (
                <div key={key.id} className="p-4">
                  <div className="flex flex-col sm:flex-row sm:items-center gap-4">
                    <div className="flex-1 min-w-0">
                      <div className="flex items-center gap-3 mb-1">
                        <p className="font-medium">{key.name}</p>
                        <Badge variant="secondary" className={key.mode === "live" ? "bg-rose-500/10 text-rose-500" : "bg-amber-500/10 text-amber-500"}>
                          {key.mode}
                        </Badge>
                        <Badge variant="secondary">
                          {key.type}
                        </Badge>
                        {key.revoked && (
                          <Badge variant="destructive">Revoked</Badge>
                        )}
                      </div>
                      <div className="flex items-center gap-2">
                        <code className="text-sm font-mono bg-muted px-2 py-1 rounded flex-1 truncate">
                          {revealedKeys.has(key.id) ? key.key : maskKey(key.key)}
                        </code>
                        <Button
                          variant="ghost"
                          size="icon"
                          onClick={() => toggleReveal(key.id)}
                          disabled={key.revoked}
                          aria-label={revealedKeys.has(key.id) ? "Hide key" : "Reveal key"}
                        >
                          {revealedKeys.has(key.id) ? (
                            <EyeOff className="h-4 w-4" />
                          ) : (
                            <Eye className="h-4 w-4" />
                          )}
                        </Button>
                        <Button
                          variant="ghost"
                          size="icon"
                          onClick={() => copyToClipboard(key.key)}
                          disabled={key.revoked}
                          aria-label="Copy to clipboard"
                        >
                          <Copy className="h-4 w-4" />
                        </Button>
                        <Button
                          variant="ghost"
                          size="icon"
                          onClick={() => revokeKey(key.id)}
                          disabled={key.revoked}
                          aria-label="Revoke key"
                          className="text-rose-500 hover:text-rose-600 hover:bg-rose-500/10"
                        >
                          <Trash2 className="h-4 w-4" />
                        </Button>
                      </div>
                    </div>
                  </div>
                  <div className="mt-2 text-sm text-muted-foreground">
                    Created {new Date(key.created).toLocaleDateString()}
                    {key.lastUsed && ` • Last used ${new Date(key.lastUsed).toLocaleDateString()}`}
                  </div>
                </div>
              ))}
              {filteredKeys.length === 0 && (
                <div className="p-8 text-center text-muted-foreground">
                  No API keys found
                </div>
              )}
            </div>
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  )
}