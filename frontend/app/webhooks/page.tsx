// tpt-payments/frontend/app/webhooks/page.tsx
"use client"

import { useState } from "react"
import { Plus, Trash2, RefreshCw, Copy, ExternalLink, Eye, Clock } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Badge } from "@/components/ui/badge"
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog"
import { Label } from "@/components/ui/label"
import { Switch } from "@/components/ui/switch"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Checkbox } from "@/components/ui/checkbox"
import { ScrollArea } from "@/components/ui/scroll-area"
import { Separator } from "@/components/ui/separator"
import { toast } from "@/components/ui/use-toast"

interface WebhookEndpoint {
  id: string
  url: string
  description: string
  enabled: boolean
  events: string[]
  created: number
  secret: string
}

const mockEndpoints: WebhookEndpoint[] = [
  {
    id: "we_01h7nq3qz8k7x9v5b2n4m6l8",
    url: "https://api.example.com/webhooks/payments",
    description: "Production payment events",
    enabled: true,
    events: ["payment_intent.succeeded", "payment_intent.failed", "charge.refunded"],
    created: Date.now() - 86400000 * 30,
    secret: "whsec_abc123def456ghi789jkl012",
  },
]

const availableEvents = [
  "payment_intent.created",
  "payment_intent.succeeded",
  "payment_intent.failed",
  "payment_intent.canceled",
  "charge.refunded",
  "charge.refund.updated",
  "invoice.created",
  "invoice.paid",
  "invoice.payment_failed",
  "subscription.created",
  "subscription.updated",
  "subscription.deleted",
  "customer.created",
  "customer.updated",
]

export default function WebhooksPage() {
  const [endpoints, setEndpoints] = useState<WebhookEndpoint[]>(mockEndpoints)
  const [newUrl, setNewUrl] = useState("")
  const [newDescription, setNewDescription] = useState("")
  const [selectedEvents, setSelectedEvents] = useState<Set<string>>(new Set())
  const [isCreating, setIsCreating] = useState(false)
  const [revealedSecrets, setRevealedSecrets] = useState<Set<string>>(new Set())
  const [activeTab, setActiveTab] = useState<string>("endpoints")

  const toggleEnabled = (id: string) => {
    setEndpoints(prev => prev.map(e => e.id === id ? { ...e, enabled: !e.enabled } : e))
  }

  const toggleEvent = (event: string) => {
    setSelectedEvents(prev => {
      const next = new Set(prev)
      if (next.has(event)) {
        next.delete(event)
      } else {
        next.add(event)
      }
      return next
    })
  }

  const deleteEndpoint = (id: string) => {
    setEndpoints(prev => prev.filter(e => e.id !== id))
  }

  const toggleRevealSecret = (id: string) => {
    setRevealedSecrets(prev => {
      const next = new Set(prev)
      if (next.has(id)) {
        next.delete(id)
      } else {
        next.add(id)
      }
      return next
    })
  }

  const copyToClipboard = async (text: string) => {
    await navigator.clipboard.writeText(text)
    toast({
      description: "Copied to clipboard",
    })
  }

  const createEndpoint = async () => {
    setIsCreating(true)
    setTimeout(() => {
      const newEndpoint: WebhookEndpoint = {
        id: `we_${Math.random().toString(36).substring(2, 14)}`,
        url: newUrl,
        description: newDescription,
        enabled: true,
        events: Array.from(selectedEvents),
        created: Date.now(),
        secret: `whsec_${Math.random().toString(36).substring(2, 28)}`,
      }
      setEndpoints(prev => [newEndpoint, ...prev])
      setNewUrl("")
      setNewDescription("")
      setSelectedEvents(new Set())
      setIsCreating(false)
    }, 1000)
  }

  return (
    <div className="p-6 space-y-6">
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold tracking-tight">Webhook Endpoints</h1>
          <p className="text-muted-foreground">Configure endpoints to receive event notifications</p>
        </div>

        <Dialog>
          <DialogTrigger asChild>
            <Button>
              <Plus className="h-4 w-4 mr-2" />
              Add Endpoint
            </Button>
          </DialogTrigger>
          <DialogContent className="max-w-lg">
            <DialogHeader>
              <DialogTitle>Add Webhook Endpoint</DialogTitle>
            </DialogHeader>
            <div className="space-y-4 py-4">
              <div className="space-y-2">
                <Label htmlFor="url">Endpoint URL</Label>
                <Input
                  id="url"
                  placeholder="https://your-domain.com/webhook"
                  value={newUrl}
                  onChange={(e) => setNewUrl(e.target.value)}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="description">Description</Label>
                <Input
                  id="description"
                  placeholder="Production payment events"
                  value={newDescription}
                  onChange={(e) => setNewDescription(e.target.value)}
                />
              </div>
              <div className="space-y-2">
                <Label>Events to send</Label>
                <ScrollArea className="h-[200px] border rounded-md p-4">
                  <div className="space-y-3">
                    {availableEvents.map((event) => (
                      <div key={event} className="flex items-center gap-3">
                        <Checkbox
                          id={`event-${event}`}
                          checked={selectedEvents.has(event)}
                          onCheckedChange={() => toggleEvent(event)}
                        />
                        <Label htmlFor={`event-${event}`} className="font-mono text-sm">
                          {event}
                        </Label>
                      </div>
                    ))}
                  </div>
                </ScrollArea>
              </div>
              <Button 
                className="w-full" 
                disabled={isCreating || !newUrl}
                onClick={createEndpoint}
              >
                {isCreating ? "Creating endpoint..." : "Create Endpoint"}
              </Button>
            </div>
          </DialogContent>
        </Dialog>
      </div>

      <Tabs defaultValue="endpoints" value={activeTab} onValueChange={setActiveTab}>
        <TabsList>
          <TabsTrigger value="endpoints">Endpoints</TabsTrigger>
          <TabsTrigger value="logs">Delivery Logs</TabsTrigger>
        </TabsList>

        <TabsContent value="endpoints" className="mt-4">
          <div className="space-y-4">
            {endpoints.map((endpoint) => (
              <Card key={endpoint.id} className="p-6">
                <div className="flex flex-col sm:flex-row sm:items-start sm:justify-between gap-4">
                  <div className="flex-1 space-y-3">
                    <div className="flex items-center gap-3">
                      <code className="font-mono text-sm">{endpoint.url}</code>
                      <Badge variant={endpoint.enabled ? "default" : "secondary"}>
                        {endpoint.enabled ? "Enabled" : "Disabled"}
                      </Badge>
                    </div>
                    {endpoint.description && (
                      <p className="text-sm text-muted-foreground">{endpoint.description}</p>
                    )}
                    <div className="flex flex-wrap gap-2">
                      {endpoint.events.slice(0, 3).map((event) => (
                        <Badge key={event} variant="secondary" className="text-xs">
                          {event}
                        </Badge>
                      ))}
                      {endpoint.events.length > 3 && (
                        <Badge variant="secondary" className="text-xs">
                          +{endpoint.events.length - 3} more
                        </Badge>
                      )}
                    </div>
                    <div className="flex items-center gap-2">
                      <Badge variant="outline" className="font-mono">
                        Signing secret:
                      </Badge>
                      <code className="text-sm font-mono">
                        {revealedSecrets.has(endpoint.id) ? endpoint.secret : "whsec_••••••••••••"}
                      </code>
                      <Button
                        variant="ghost"
                        size="icon"
                        className="h-6 w-6"
                        onClick={() => toggleRevealSecret(endpoint.id)}
                      >
                        <Eye className="h-3.5 w-3.5" />
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon"
                        className="h-6 w-6"
                        onClick={() => copyToClipboard(endpoint.secret)}
                      >
                        <Copy className="h-3.5 w-3.5" />
                      </Button>
                    </div>
                    <p className="text-xs text-muted-foreground">
                      Created {new Date(endpoint.created).toLocaleDateString()}
                    </p>
                  </div>
                  <div className="flex items-center gap-2">
                    <Switch
                      checked={endpoint.enabled}
                      onCheckedChange={() => toggleEnabled(endpoint.id)}
                      aria-label="Toggle endpoint"
                    />
                    <Button
                      variant="ghost"
                      size="icon"
                      onClick={() => deleteEndpoint(endpoint.id)}
                      className="text-rose-500 hover:text-rose-600 hover:bg-rose-500/10"
                    >
                      <Trash2 className="h-4 w-4" />
                    </Button>
                  </div>
                </div>
              </Card>
            ))}
            {endpoints.length === 0 && (
              <Card className="p-8 text-center text-muted-foreground">
                No webhook endpoints configured
              </Card>
            )}
          </div>
        </TabsContent>

        <TabsContent value="logs" className="mt-4">
          <Card className="p-6">
            <div className="text-center text-muted-foreground py-8">
              <Clock className="h-12 w-12 mx-auto mb-4 opacity-50" />
              <p>Delivery logs will appear here</p>
            </div>
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  )
}