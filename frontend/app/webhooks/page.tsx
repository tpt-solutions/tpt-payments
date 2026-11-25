// tpt-payments/frontend/app/webhooks/page.tsx
"use client"

import { useEffect, useState } from "react"
import { format } from "date-fns"
import { Plus, Trash2, Copy, Clock } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Badge } from "@/components/ui/badge"
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog"
import { Label } from "@/components/ui/label"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Checkbox } from "@/components/ui/checkbox"
import { ScrollArea } from "@/components/ui/scroll-area"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { toast } from "@/components/ui/use-toast"

// The shape the core API returns for a webhook endpoint. The signing secret is not in it; it is
// returned once, by the create call.
interface WebhookEndpoint {
  id: string
  url: string
  enabled_events: string[]
  status: "enabled" | "disabled"
  livemode: boolean
  secret_prefix: string
  created: number
}

interface Delivery {
  id: string
  endpoint: string
  event_type: string
  event_id: string
  status: string
  attempts: number
  last_http_status: number | null
  last_error: string
  created: number
}

interface ListResponse<T> {
  object: "list"
  data: T[]
  has_more: boolean
}

// The event types the core service emits (packages/tpt-pay-core/internal/webhook). Events the
// core does not emit are not offered, so an endpoint cannot subscribe to nothing.
const availableEvents = [
  "payment_intent.created",
  "payment_intent.succeeded",
  "payment_intent.payment_failed",
  "payment_intent.canceled",
  "charge.refunded",
]

async function readJson<T>(res: Response): Promise<T> {
  const body = await res.json().catch(() => ({}))
  if (!res.ok) throw new Error(body.error?.message || body.error || `request failed (${res.status})`)
  return body as T
}

// Load functions for the webhooks page. Effects call the fetchers and set state in callbacks only.
async function fetchEndpoints(): Promise<ListResponse<WebhookEndpoint>> {
  return readJson<ListResponse<WebhookEndpoint>>(await fetch("/api/tpt/webhook_endpoints?limit=100"))
}

async function fetchDeliveries(): Promise<ListResponse<Delivery>> {
  return readJson<ListResponse<Delivery>>(await fetch("/api/tpt/webhook_deliveries?limit=50"))
}

export default function WebhooksPage() {
  const [endpoints, setEndpoints] = useState<WebhookEndpoint[]>([])
  const [loadError, setLoadError] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)

  const [deliveries, setDeliveries] = useState<Delivery[] | null>(null)
  const [deliveriesError, setDeliveriesError] = useState<string | null>(null)
  const [activeTab, setActiveTab] = useState<string>("endpoints")

  const [dialogOpen, setDialogOpen] = useState(false)
  const [newUrl, setNewUrl] = useState("")
  const [selectedEvents, setSelectedEvents] = useState<Set<string>>(new Set())
  const [formError, setFormError] = useState<string | null>(null)
  const [isCreating, setIsCreating] = useState(false)
  // The signing secret, shown once after creation and never again.
  const [newSecret, setNewSecret] = useState<{ url: string; secret: string } | null>(null)

  // Used by handlers after a change.
  const refreshEndpoints = async () => {
    try {
      setEndpoints((await fetchEndpoints()).data)
      setLoadError(null)
    } catch (err) {
      setLoadError(err instanceof Error ? err.message : "could not load endpoints")
    }
  }

  useEffect(() => {
    let active = true
    fetchEndpoints()
      .then(
        (body) => { if (active) { setEndpoints(body.data); setLoadError(null) } },
        (err) => { if (active) setLoadError(err instanceof Error ? err.message : "could not load endpoints") },
      )
      .finally(() => { if (active) setLoading(false) })
    return () => { active = false }
  }, [])

  useEffect(() => {
    if (activeTab !== "logs") return
    let active = true
    fetchDeliveries()
      .then(
        (body) => { if (active) { setDeliveries(body.data); setDeliveriesError(null) } },
        (err) => {
          if (!active) return
          setDeliveriesError(err instanceof Error ? err.message : "could not load deliveries")
          setDeliveries([])
        },
      )
    return () => { active = false }
  }, [activeTab])

  const toggleEvent = (event: string) => {
    setSelectedEvents((prev) => {
      const next = new Set(prev)
      if (next.has(event)) next.delete(event)
      else next.add(event)
      return next
    })
  }

  const createEndpoint = async () => {
    setFormError(null)
    if (!/^https:\/\/|^http:\/\/(localhost|127\.0\.0\.1)/.test(newUrl)) {
      setFormError("The URL must start with https:// (http:// is allowed only for localhost in test mode).")
      return
    }
    setIsCreating(true)
    try {
      const body = await readJson<WebhookEndpoint & { secret: string }>(
        await fetch("/api/tpt/webhook_endpoints", {
          method: "POST",
          headers: { "Content-Type": "application/json", "Idempotency-Key": crypto.randomUUID() },
          body: JSON.stringify({ url: newUrl, enabled_events: Array.from(selectedEvents) }),
        }),
      )
      setNewSecret({ url: body.url, secret: body.secret })
      setNewUrl("")
      setSelectedEvents(new Set())
      setDialogOpen(false)
      await refreshEndpoints()
    } catch (err) {
      setFormError(err instanceof Error ? err.message : "could not create the endpoint")
    } finally {
      setIsCreating(false)
    }
  }

  const disableEndpoint = async (id: string) => {
    try {
      await readJson(await fetch(`/api/tpt/webhook_endpoint?id=${encodeURIComponent(id)}`, { method: "DELETE" }))
      toast({ description: "Endpoint disabled. Its delivery history is kept." })
      await refreshEndpoints()
    } catch (err) {
      toast({ description: err instanceof Error ? err.message : "could not disable the endpoint" })
    }
  }

  // Queues a delivery to be sent again, then refreshes the log.
  const replayDelivery = async (id: string) => {
    try {
      const res = await fetch(`/api/tpt/webhook_delivery_replay?id=${encodeURIComponent(id)}`, { method: "POST" })
      const body = await res.json().catch(() => ({}))
      if (!res.ok) throw new Error(body.error || `request failed (${res.status})`)
      toast({ description: "Queued for another attempt." })
      setDeliveries(await fetchDeliveries().then((b) => b.data).catch(() => deliveries))
    } catch (err) {
      toast({ description: err instanceof Error ? err.message : "could not replay the delivery" })
    }
  }

  const copySecret = async (text: string) => {
    await navigator.clipboard.writeText(text)
    toast({ description: "Copied to clipboard" })
  }

  return (
    <div className="p-6 space-y-6">
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold tracking-tight">Webhook Endpoints</h1>
          <p className="text-muted-foreground">Where TPT Pay sends event notifications</p>
        </div>

        <Dialog open={dialogOpen} onOpenChange={(open) => { setDialogOpen(open); setFormError(null) }}>
          <DialogTrigger asChild>
            <Button>
              <Plus className="h-4 w-4 mr-2" />
              Add endpoint
            </Button>
          </DialogTrigger>
          <DialogContent className="max-w-lg">
            <DialogHeader>
              <DialogTitle>Add webhook endpoint</DialogTitle>
            </DialogHeader>
            <div className="space-y-4 py-4">
              <div className="space-y-2">
                <Label htmlFor="url">Endpoint URL</Label>
                <Input id="url" placeholder="https://your-domain.com/webhook" value={newUrl} onChange={(e) => setNewUrl(e.target.value)} />
              </div>
              <div className="space-y-2">
                <Label>Events to send (none selected means all)</Label>
                <ScrollArea className="h-[200px] border rounded-md p-4">
                  <div className="space-y-3">
                    {availableEvents.map((event) => (
                      <div key={event} className="flex items-center gap-3">
                        <Checkbox id={`event-${event}`} checked={selectedEvents.has(event)} onCheckedChange={() => toggleEvent(event)} />
                        <Label htmlFor={`event-${event}`} className="font-mono text-sm">{event}</Label>
                      </div>
                    ))}
                  </div>
                </ScrollArea>
              </div>
              {formError && <div role="alert" className="text-sm text-rose-500">{formError}</div>}
              <Button className="w-full" disabled={isCreating || !newUrl} onClick={createEndpoint}>
                {isCreating ? "Creating endpoint…" : "Create endpoint"}
              </Button>
            </div>
          </DialogContent>
        </Dialog>
      </div>

      {newSecret && (
        <div role="status" className="rounded-md border border-amber-500/40 bg-amber-500/10 p-4 space-y-2">
          <p className="font-medium">Copy the signing secret now. It is shown only once.</p>
          <p className="text-sm text-muted-foreground">Endpoint: <code className="font-mono">{newSecret.url}</code></p>
          <div className="flex items-center gap-2">
            <code className="text-sm font-mono break-all">{newSecret.secret}</code>
            <Button variant="ghost" size="icon" aria-label="Copy signing secret" onClick={() => copySecret(newSecret.secret)}>
              <Copy className="h-3.5 w-3.5" />
            </Button>
          </div>
          <Button variant="outline" size="sm" onClick={() => setNewSecret(null)}>I have copied it</Button>
        </div>
      )}

      <Tabs defaultValue="endpoints" value={activeTab} onValueChange={setActiveTab}>
        <TabsList>
          <TabsTrigger value="endpoints">Endpoints</TabsTrigger>
          <TabsTrigger value="logs">Delivery logs</TabsTrigger>
        </TabsList>

        <TabsContent value="endpoints" className="mt-4">
          {loadError && <div role="alert" className="mb-4 text-sm text-rose-500">Could not load endpoints: {loadError}</div>}
          <div className="space-y-4">
            {endpoints.map((endpoint) => (
              <Card key={endpoint.id} className="p-6">
                <div className="flex flex-col sm:flex-row sm:items-start sm:justify-between gap-4">
                  <div className="flex-1 space-y-3">
                    <div className="flex items-center gap-3">
                      <code className="font-mono text-sm">{endpoint.url}</code>
                      <Badge variant={endpoint.status === "enabled" ? "default" : "secondary"}>
                        {endpoint.status === "enabled" ? "Enabled" : "Disabled"}
                      </Badge>
                      {!endpoint.livemode && <Badge variant="outline">test</Badge>}
                    </div>
                    <div className="flex flex-wrap gap-2">
                      {endpoint.enabled_events.length === 0 ? (
                        <Badge variant="secondary" className="text-xs">all events</Badge>
                      ) : (
                        endpoint.enabled_events.map((event) => (
                          <Badge key={event} variant="secondary" className="text-xs">{event}</Badge>
                        ))
                      )}
                    </div>
                    <p className="text-sm text-muted-foreground font-mono">Signing secret {endpoint.secret_prefix}…</p>
                    <p className="text-xs text-muted-foreground">Created {format(endpoint.created * 1000, "MMM d, yyyy")}</p>
                  </div>
                  {endpoint.status === "enabled" && (
                    <Button
                      variant="ghost"
                      className="text-rose-500 hover:text-rose-600 hover:bg-rose-500/10"
                      onClick={() => disableEndpoint(endpoint.id)}
                    >
                      <Trash2 className="h-4 w-4 mr-2" />
                      Disable
                    </Button>
                  )}
                </div>
              </Card>
            ))}
            {!loading && !loadError && endpoints.length === 0 && (
              <Card className="p-8 text-center text-muted-foreground">No webhook endpoints configured</Card>
            )}
          </div>
        </TabsContent>

        <TabsContent value="logs" className="mt-4">
          <Card className="p-6">
            {deliveriesError && <p role="alert" className="mb-4 text-sm text-rose-500">Could not load deliveries: {deliveriesError}</p>}
            {deliveries === null ? (
              <p className="text-center text-muted-foreground py-8">Loading…</p>
            ) : deliveries.length === 0 ? (
              <div className="text-center text-muted-foreground py-8">
                <Clock className="h-12 w-12 mx-auto mb-4 opacity-50" />
                <p>No deliveries yet</p>
              </div>
            ) : (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>When</TableHead>
                    <TableHead>Event</TableHead>
                    <TableHead>Status</TableHead>
                    <TableHead>HTTP</TableHead>
                    <TableHead>Attempts</TableHead>
                    <TableHead>Last error</TableHead>
                    <TableHead className="w-[90px]"></TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {deliveries.map((d) => (
                    <TableRow key={d.id}>
                      <TableCell className="text-sm text-muted-foreground">{format(d.created * 1000, "MMM d, HH:mm")}</TableCell>
                      <TableCell className="font-mono text-sm">{d.event_type}</TableCell>
                      <TableCell>{d.status}</TableCell>
                      <TableCell>{d.last_http_status ?? "—"}</TableCell>
                      <TableCell>{d.attempts}</TableCell>
                      <TableCell className="text-sm text-muted-foreground">{d.last_error || "—"}</TableCell>
                      <TableCell>
                        <Button variant="outline" size="sm" onClick={() => replayDelivery(d.id)}>Replay</Button>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            )}
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  )
}
