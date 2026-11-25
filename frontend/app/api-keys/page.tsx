// tpt-payments/frontend/app/api-keys/page.tsx
"use client"

import { useCallback, useEffect, useState } from "react"
import { format } from "date-fns"
import { Copy, Plus, Trash2 } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Badge } from "@/components/ui/badge"
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Label } from "@/components/ui/label"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { toast } from "@/components/ui/use-toast"

// The shape the core API returns for a key. The key itself is not in it: the create call returns
// the secret once.
interface ApiKey {
  id: string
  object: "api_key"
  name: string
  type: "publishable" | "secret"
  mode: "test" | "live"
  prefix: string
  status: "active" | "revoked"
  created: number
  last_used?: number
  revoked?: number
}

interface ListResponse {
  object: "list"
  data: ApiKey[]
  has_more: boolean
}

// Loads the merchant's keys. Effects call this and set state only in its callbacks.
async function fetchKeys(): Promise<ApiKey[]> {
  const res = await fetch("/api/tpt/api_keys?limit=100")
  const body = await res.json().catch(() => ({}))
  if (!res.ok) throw new Error(body.error || `request failed (${res.status})`)
  return (body as ListResponse).data
}

export default function ApiKeysPage() {
  const [keys, setKeys] = useState<ApiKey[]>([])
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState<string | null>(null)

  const [dialogOpen, setDialogOpen] = useState(false)
  const [newKeyName, setNewKeyName] = useState("")
  const [newKeyType, setNewKeyType] = useState<"publishable" | "secret">("secret")
  const [formError, setFormError] = useState<string | null>(null)
  const [isCreating, setIsCreating] = useState(false)
  // The new key's secret, shown once and never again.
  const [newSecret, setNewSecret] = useState<{ name: string; secret: string } | null>(null)

  // Used by handlers after a change. Effects use the fetch directly (see below).
  const refreshKeys = async () => {
    try {
      setKeys(await fetchKeys())
      setLoadError(null)
    } catch (err) {
      setLoadError(err instanceof Error ? err.message : "could not load keys")
    }
  }

  useEffect(() => {
    let active = true
    fetchKeys()
      .then(
        (data) => { if (active) { setKeys(data); setLoadError(null) } },
        (err) => { if (active) setLoadError(err instanceof Error ? err.message : "could not load keys") },
      )
      .finally(() => { if (active) setLoading(false) })
    return () => { active = false }
  }, [])

  const createKey = async () => {
    setFormError(null)
    setIsCreating(true)
    try {
      const res = await fetch("/api/tpt/api_keys", {
        method: "POST",
        headers: { "Content-Type": "application/json", "Idempotency-Key": crypto.randomUUID() },
        body: JSON.stringify({ type: newKeyType, name: newKeyName.trim() }),
      })
      const body = await res.json().catch(() => ({}))
      if (!res.ok) throw new Error(body.error?.message || body.error || `request failed (${res.status})`)
      setNewSecret({ name: body.name, secret: body.secret })
      setNewKeyName("")
      setDialogOpen(false)
      await refreshKeys()
    } catch (err) {
      setFormError(err instanceof Error ? err.message : "could not create the key")
    } finally {
      setIsCreating(false)
    }
  }

  const revokeKey = async (id: string) => {
    try {
      const res = await fetch(`/api/tpt/api_key?id=${encodeURIComponent(id)}`, { method: "DELETE" })
      const body = await res.json().catch(() => ({}))
      if (!res.ok) throw new Error(body.error?.message || body.error || `request failed (${res.status})`)
      toast({ description: "Key revoked. Requests with it will now be refused." })
      await refreshKeys()
    } catch (err) {
      toast({ description: err instanceof Error ? err.message : "could not revoke the key" })
    }
  }

  const copySecret = async (text: string) => {
    await navigator.clipboard.writeText(text)
    toast({ description: "API key copied to clipboard" })
  }

  return (
    <div className="p-6 space-y-6">
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold tracking-tight">API Keys</h1>
          <p className="text-muted-foreground">
            Keys work only in the mode they were created in. Test keys cannot move live money.
          </p>
        </div>

        <Dialog open={dialogOpen} onOpenChange={(open) => { setDialogOpen(open); setFormError(null) }}>
          <DialogTrigger asChild>
            <Button>
              <Plus className="h-4 w-4 mr-2" />
              Create key
            </Button>
          </DialogTrigger>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>Create API key</DialogTitle>
            </DialogHeader>
            <div className="space-y-4 py-4">
              <div className="space-y-2">
                <Label htmlFor="key-name">Name</Label>
                <Input id="key-name" placeholder="Production backend" maxLength={100} value={newKeyName} onChange={(e) => setNewKeyName(e.target.value)} />
              </div>
              <div className="space-y-2">
                <Label htmlFor="key-type">Type</Label>
                <Select value={newKeyType} onValueChange={(v) => setNewKeyType(v as "publishable" | "secret")}>
                  <SelectTrigger id="key-type"><SelectValue /></SelectTrigger>
                  <SelectContent>
                    <SelectItem value="secret">Secret (server-side only)</SelectItem>
                    <SelectItem value="publishable">Publishable (browser checkout)</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              {formError && <div role="alert" className="text-sm text-rose-500">{formError}</div>}
              <Button className="w-full" disabled={isCreating || newKeyName.trim().length === 0} onClick={createKey}>
                {isCreating ? "Creating key…" : "Create key"}
              </Button>
            </div>
          </DialogContent>
        </Dialog>
      </div>

      {newSecret && (
        <div role="status" className="rounded-md border border-amber-500/40 bg-amber-500/10 p-4 space-y-2">
          <p className="font-medium">Copy the key for “{newSecret.name}” now. It is shown only once.</p>
          <div className="flex items-center gap-2">
            <code className="text-sm font-mono break-all">{newSecret.secret}</code>
            <Button variant="ghost" size="icon" aria-label="Copy key" onClick={() => copySecret(newSecret.secret)}>
              <Copy className="h-3.5 w-3.5" />
            </Button>
          </div>
          <Button variant="outline" size="sm" onClick={() => setNewSecret(null)}>I have copied it</Button>
        </div>
      )}

      {loadError && <div role="alert" className="text-sm text-rose-500">Could not load keys: {loadError}</div>}

      <Card>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Name</TableHead>
              <TableHead>Key</TableHead>
              <TableHead>Type</TableHead>
              <TableHead>Mode</TableHead>
              <TableHead>Created</TableHead>
              <TableHead>Last used</TableHead>
              <TableHead>Status</TableHead>
              <TableHead className="w-[70px]"></TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {keys.map((key) => (
              <TableRow key={key.id}>
                <TableCell className="font-medium">{key.name || "—"}</TableCell>
                <TableCell className="font-mono text-sm">{key.prefix}…</TableCell>
                <TableCell>{key.type}</TableCell>
                <TableCell>
                  <Badge variant={key.mode === "live" ? "default" : "outline"}>{key.mode}</Badge>
                </TableCell>
                <TableCell className="text-sm text-muted-foreground">{format(key.created * 1000, "MMM d, yyyy")}</TableCell>
                <TableCell className="text-sm text-muted-foreground">
                  {key.last_used ? format(key.last_used * 1000, "MMM d, HH:mm") : "Never"}
                </TableCell>
                <TableCell>
                  <Badge variant={key.status === "active" ? "secondary" : "outline"}>{key.status}</Badge>
                </TableCell>
                <TableCell>
                  {key.status === "active" && (
                    <Button
                      variant="ghost"
                      size="icon"
                      aria-label={`Revoke ${key.name || key.prefix}`}
                      className="text-rose-500 hover:text-rose-600 hover:bg-rose-500/10"
                      onClick={() => revokeKey(key.id)}
                    >
                      <Trash2 className="h-4 w-4" />
                    </Button>
                  )}
                </TableCell>
              </TableRow>
            ))}
            {!loading && !loadError && keys.length === 0 && (
              <TableRow>
                <TableCell colSpan={8} className="h-24 text-center text-muted-foreground">No API keys yet</TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
      </Card>
    </div>
  )
}
