// tpt-payments/frontend/app/developers/webhook-tester/_client/WebhookTesterClient.tsx
'use client';

import { useState } from 'react';

// The event types the core service emits (packages/tpt-pay-core/internal/webhook).
const eventTypes = [
  'payment_intent.created',
  'payment_intent.succeeded',
  'payment_intent.payment_failed',
  'payment_intent.canceled',
  'payment_intent.fraud_flagged',
  'charge.refunded',
];

export default function WebhookTesterPage() {
  const [selectedEvent, setSelectedEvent] = useState(eventTypes[1]);
  const [webhookUrl, setWebhookUrl] = useState('');
  const [sending, setSending] = useState(false);
  const [logs, setLogs] = useState<Array<{ id: string; event: string; status: string; timestamp: string }>>([]);

  const handleSendTest = async () => {
    if (!webhookUrl) return;

    setSending(true);
    const logEntry = {
      id: Math.random().toString(36).substring(7),
      event: selectedEvent,
      status: 'sending',
      timestamp: new Date().toLocaleTimeString(),
    };
    setLogs((prev) => [logEntry, ...prev]);

    // Simulate webhook send
    setTimeout(() => {
      setLogs((prev) =>
        prev.map((log) =>
          log.id === logEntry.id
            ? { ...log, status: Math.random() > 0.2 ? 'delivered' : 'failed' }
            : log
        )
      );
      setSending(false);
    }, 1500);
  };

  const getStatusColor = (status: string) => {
    switch (status) {
      case 'delivered': return 'bg-green-100 text-green-800';
      case 'failed': return 'bg-red-100 text-red-800';
      case 'sending': return 'bg-yellow-100 text-yellow-800';
      default: return 'bg-gray-100 text-gray-800';
    }
  };

  return (
    <div className="min-h-screen bg-gray-50">
      <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-12">
        <div className="mb-8">
          <h1 className="text-4xl font-bold text-gray-900">Webhook Testing Tool</h1>
          <p className="mt-4 text-lg text-gray-600">
            Send test events and inspect webhook delivery in real-time.
          </p>
        </div>

        <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
          {/* Send Test Event */}
          <div className="bg-white rounded-lg shadow p-6">
            <h2 className="text-xl font-bold text-gray-900 mb-6">Send Test Event</h2>

            <div className="space-y-4">
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-2">
                  Webhook Endpoint URL
                </label>
                <input
                  type="url"
                  value={webhookUrl}
                  onChange={(e) => setWebhookUrl(e.target.value)}
                  className="w-full px-3 py-2 border border-gray-300 rounded-md"
                  placeholder="https://your-app.com/webhook"
                />
              </div>

              <div>
                <label className="block text-sm font-medium text-gray-700 mb-2">
                  Event Type
                </label>
                <select
                  value={selectedEvent}
                  onChange={(e) => setSelectedEvent(e.target.value)}
                  className="w-full px-3 py-2 border border-gray-300 rounded-md"
                >
                  {eventTypes.map((event) => (
                    <option key={event} value={event}>
                      {event}
                    </option>
                  ))}
                </select>
              </div>

              <div className="bg-gray-50 p-4 rounded-md">
                <h3 className="text-sm font-medium text-gray-700 mb-2">Example Payload</h3>
                <pre className="text-xs bg-gray-900 text-gray-100 p-3 rounded overflow-x-auto">
                  {JSON.stringify(
                    {
                      id: 'evt_test_' + Math.random().toString(36).substring(7),
                      object: 'event',
                      type: selectedEvent,
                      created: Math.floor(Date.now() / 1000),
                      data: {
                        object: {
                          id: 'pi_test_...',
                          object: 'payment_intent',
                          amount: 2000,
                          currency: 'usd',
                          status: 'succeeded',
                        },
                      },
                    },
                    null,
                    2
                  )}
                </pre>
              </div>

              <button
                onClick={handleSendTest}
                disabled={sending || !webhookUrl}
                className="w-full px-4 py-2 bg-blue-600 text-white rounded-md hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed"
              >
                {sending ? 'Sending...' : 'Send Test Event'}
              </button>
            </div>
          </div>

          {/* Delivery Logs */}
          <div className="bg-white rounded-lg shadow p-6">
            <h2 className="text-xl font-bold text-gray-900 mb-6">Delivery Logs</h2>

            {logs.length === 0 ? (
              <p className="text-gray-500 text-center py-8">
                No webhook deliveries yet. Send a test event to see logs here.
              </p>
            ) : (
              <div className="space-y-3 max-h-96 overflow-y-auto">
                {logs.map((log) => (
                  <div key={log.id} className="border rounded-lg p-4">
                    <div className="flex items-center justify-between mb-2">
                      <code className="text-sm font-medium text-gray-900">{log.event}</code>
                      <span className={`px-2 py-1 text-xs font-medium rounded-full ${getStatusColor(log.status)}`}>
                        {log.status}
                      </span>
                    </div>
                    <div className="flex items-center justify-between text-xs text-gray-500">
                      <span>ID: {log.id}</span>
                      <span>{log.timestamp}</span>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>
        </div>

        {/* Info Box */}
        <div className="mt-8 bg-blue-50 border border-blue-200 rounded-lg p-6">
          <h3 className="text-lg font-semibold text-blue-900 mb-3">How to Test Webhooks</h3>
          <ol className="list-decimal list-inside space-y-2 text-sm text-blue-800">
            <li>Create a webhook endpoint in your dashboard or via API.</li>
            <li>Make sure your endpoint is publicly accessible (use a tool like ngrok for local testing).</li>
            <li>Enter your webhook URL above and select an event type.</li>
            <li>Click "Send Test Event" to trigger a test webhook.</li>
            <li>Verify your endpoint receives and processes the event correctly.</li>
            <li>Check the signature using your webhook secret for verification.</li>
          </ol>
        </div>
      </div>
    </div>
  );
}