// tpt-payments/frontend/app/settings/page.tsx
"use client"

import { useState } from "react"
import { Save, Bell, Mail, Smartphone, Globe, Shield, RefreshCw } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Switch } from "@/components/ui/switch"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Textarea } from "@/components/ui/textarea"
import { Separator } from "@/components/ui/separator"
import { toast } from "@/components/ui/use-toast"

interface MerchantSettings {
  name: string
  email: string
  phone: string
  website: string
  description: string
  notifications: {
    payment_success: boolean
    payment_failure: boolean
    refund_issued: boolean
    subscription_canceled: boolean
    daily_summary: boolean
  }
}

const initialSettings: MerchantSettings = {
  name: "Demo Merchant Inc.",
  email: "admin@example.com",
  phone: "+1 (555) 123-4567",
  website: "https://example.com",
  description: "Online retail business selling premium products",
  notifications: {
    payment_success: true,
    payment_failure: true,
    refund_issued: false,
    subscription_canceled: true,
    daily_summary: false,
  },
}

export default function SettingsPage() {
  const [settings, setSettings] = useState<MerchantSettings>(initialSettings)
  const [isSaving, setIsSaving] = useState(false)

  const updateSetting = <K extends keyof MerchantSettings>(key: K, value: MerchantSettings[K]) => {
    setSettings(prev => ({ ...prev, [key]: value }))
  }

  const updateNotification = (key: keyof MerchantSettings["notifications"], value: boolean) => {
    setSettings(prev => ({
      ...prev,
      notifications: {
        ...prev.notifications,
        [key]: value,
      },
    }))
  }

  const saveSettings = async () => {
    setIsSaving(true)
    // API call would go here
    setTimeout(() => {
      setIsSaving(false)
      toast({
        description: "Settings saved successfully",
      })
    }, 1000)
  }

  return (
    <div className="p-6 space-y-6">
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold tracking-tight">Merchant Settings</h1>
          <p className="text-muted-foreground">Manage your account settings and preferences</p>
        </div>
        <Button onClick={saveSettings} disabled={isSaving}>
          <Save className="h-4 w-4 mr-2" />
          {isSaving ? "Saving..." : "Save Changes"}
        </Button>
      </div>

      <Tabs defaultValue="profile">
        <TabsList>
          <TabsTrigger value="profile">Profile</TabsTrigger>
          <TabsTrigger value="notifications">Notifications</TabsTrigger>
          <TabsTrigger value="security">Security</TabsTrigger>
        </TabsList>

        <TabsContent value="profile" className="mt-4 space-y-6">
          <Card className="p-6">
            <h3 className="text-lg font-semibold mb-4">Business Information</h3>
            <div className="grid gap-4 md:grid-cols-2">
              <div className="space-y-2">
                <Label htmlFor="name">Business Name</Label>
                <Input
                  id="name"
                  value={settings.name}
                  onChange={(e) => updateSetting("name", e.target.value)}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="email">Contact Email</Label>
                <Input
                  id="email"
                  type="email"
                  value={settings.email}
                  onChange={(e) => updateSetting("email", e.target.value)}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="phone">Phone Number</Label>
                <Input
                  id="phone"
                  value={settings.phone}
                  onChange={(e) => updateSetting("phone", e.target.value)}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="website">Website</Label>
                <Input
                  id="website"
                  value={settings.website}
                  onChange={(e) => updateSetting("website", e.target.value)}
                />
              </div>
            </div>
            <div className="mt-4 space-y-2">
              <Label htmlFor="description">Business Description</Label>
              <Textarea
                id="description"
                value={settings.description}
                onChange={(e) => updateSetting("description", e.target.value)}
                rows={3}
              />
            </div>
          </Card>
        </TabsContent>

        <TabsContent value="notifications" className="mt-4 space-y-6">
          <Card className="p-6">
            <h3 className="text-lg font-semibold mb-4">Email Notifications</h3>
            <p className="text-sm text-muted-foreground mb-4">
              Choose which events you want to receive email notifications for
            </p>
            <div className="space-y-4">
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-3">
                  <div className="p-2 bg-emerald-100 dark:bg-emerald-900/30 rounded-full">
                    <Bell className="h-4 w-4 text-emerald-500" />
                  </div>
                  <div>
                    <p className="font-medium">Successful Payments</p>
                    <p className="text-sm text-muted-foreground">Receive email when a payment succeeds</p>
                  </div>
                </div>
                <Switch
                  checked={settings.notifications.payment_success}
                  onCheckedChange={(checked) => updateNotification("payment_success", checked as boolean)}
                />
              </div>
              <Separator />
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-3">
                  <div className="p-2 bg-rose-100 dark:bg-rose-900/30 rounded-full">
                    <Bell className="h-4 w-4 text-rose-500" />
                  </div>
                  <div>
                    <p className="font-medium">Failed Payments</p>
                    <p className="text-sm text-muted-foreground">Receive email when a payment fails</p>
                  </div>
                </div>
                <Switch
                  checked={settings.notifications.payment_failure}
                  onCheckedChange={(checked) => updateNotification("payment_failure", checked as boolean)}
                />
              </div>
              <Separator />
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-3">
                  <div className="p-2 bg-amber-100 dark:bg-amber-900/30 rounded-full">
                    <RefreshCw className="h-4 w-4 text-amber-500" />
                  </div>
                  <div>
                    <p className="font-medium">Refunds Issued</p>
                    <p className="text-sm text-muted-foreground">Receive email when a refund is processed</p>
                  </div>
                </div>
                <Switch
                  checked={settings.notifications.refund_issued}
                  onCheckedChange={(checked) => updateNotification("refund_issued", checked as boolean)}
                />
              </div>
              <Separator />
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-3">
                  <div className="p-2 bg-slate-100 dark:bg-slate-800 rounded-full">
                    <RefreshCw className="h-4 w-4 text-slate-500" />
                  </div>
                  <div>
                    <p className="font-medium">Subscription Cancelled</p>
                    <p className="text-sm text-muted-foreground">Receive email when a subscription is cancelled</p>
                  </div>
                </div>
                <Switch
                  checked={settings.notifications.subscription_canceled}
                  onCheckedChange={(checked) => updateNotification("subscription_canceled", checked as boolean)}
                />
              </div>
              <Separator />
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-3">
                  <div className="p-2 bg-blue-100 dark:bg-blue-900/30 rounded-full">
                    <Mail className="h-4 w-4 text-blue-500" />
                  </div>
                  <div>
                    <p className="font-medium">Daily Summary</p>
                    <p className="text-sm text-muted-foreground">Receive daily transaction summary email</p>
                  </div>
                </div>
                <Switch
                  checked={settings.notifications.daily_summary}
                  onCheckedChange={(checked) => updateNotification("daily_summary", checked as boolean)}
                />
              </div>
            </div>
          </Card>
        </TabsContent>

        <TabsContent value="security" className="mt-4 space-y-6">
          <Card className="p-6">
            <h3 className="text-lg font-semibold mb-4">Security Settings</h3>
            <div className="space-y-4">
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-3">
                  <div className="p-2 bg-emerald-100 dark:bg-emerald-900/30 rounded-full">
                    <Smartphone className="h-4 w-4 text-emerald-500" />
                  </div>
                  <div>
                    <p className="font-medium">Two-Factor Authentication</p>
                    <p className="text-sm text-muted-foreground">Add an extra layer of security to your account</p>
                  </div>
                </div>
                <Button variant="secondary">Manage 2FA</Button>
              </div>
              <Separator />
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-3">
                  <div className="p-2 bg-blue-100 dark:bg-blue-900/30 rounded-full">
                    <Shield className="h-4 w-4 text-blue-500" />
                  </div>
                  <div>
                    <p className="font-medium">Change Password</p>
                    <p className="text-sm text-muted-foreground">Update your account password</p>
                  </div>
                </div>
                <Button variant="secondary">Change Password</Button>
              </div>
            </div>
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  )
}