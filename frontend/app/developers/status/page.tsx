// tpt-payments/frontend/app/developers/status/page.tsx
import { Metadata } from 'next';

export const metadata: Metadata = {
  title: 'Status - TPT Pay Developer Portal',
  description: 'Real-time uptime status and incident history',
};

const currentStatus = 'operational'; // operational, degraded, partial_outage, major_outage

const services = [
  { name: 'API (Production)', status: 'operational', uptime: '99.99%' },
  { name: 'API (Sandbox)', status: 'operational', uptime: '99.97%' },
  { name: 'Dashboard', status: 'operational', uptime: '99.95%' },
  { name: 'Webhook Delivery', status: 'operational', uptime: '99.98%' },
  { name: 'Payment Processing', status: 'operational', uptime: '99.99%' },
  { name: 'ACH Payouts', status: 'degraded', uptime: '99.90%' },
];

const incidents = [
  {
    id: 'inc_001',
    title: 'Delayed Webhook Deliveries',
    status: 'resolved',
    severity: 'minor',
    created: '2024-06-18T10:30:00Z',
    resolved: '2024-06-18T12:45:00Z',
    updates: [
      { time: '2024-06-18T10:30:00Z', message: 'We are investigating delayed webhook deliveries in the US region.' },
      { time: '2024-06-18T11:15:00Z', message: 'The issue has been identified as a network routing problem with our US data center.' },
      { time: '2024-06-18T12:45:00Z', message: 'Webhook delivery has been restored and is now operating normally. All backed-up events have been delivered.' },
    ],
  },
  {
    id: 'inc_002',
    title: 'API Rate Limiting Issues',
    status: 'resolved',
    severity: 'minor',
    created: '2024-06-10T08:00:00Z',
    resolved: '2024-06-10T09:30:00Z',
    updates: [
      { time: '2024-06-10T08:00:00Z', message: 'Some merchants may be experiencing stricter rate limiting than expected.' },
      { time: '2024-06-10T09:30:00Z', message: 'Rate limiting rules have been adjusted and are now operating as expected.' },
    ],
  },
  {
    id: 'inc_003',
    title: 'Sandbox Environment Maintenance',
    status: 'scheduled',
    severity: 'none',
    created: '2024-06-25T02:00:00Z',
    resolved: '',
    updates: [
      { time: '2024-06-20T10:00:00Z', message: 'Scheduled maintenance for the sandbox environment. Expected downtime: 30 minutes.' },
    ],
  },
];

function StatusBadge({ status }: { status: string }) {
  const colors: Record<string, string> = {
    operational: 'bg-green-100 text-green-800',
    degraded: 'bg-yellow-100 text-yellow-800',
    partial_outage: 'bg-orange-100 text-orange-800',
    major_outage: 'bg-red-100 text-red-800',
    resolved: 'bg-green-100 text-green-800',
    scheduled: 'bg-blue-100 text-blue-800',
  };
  return (
    <span className={`inline-block px-3 py-1 text-xs font-bold rounded-full ${colors[status] || colors.operational}`}>
      {status.replace('_', ' ').toUpperCase()}
    </span>
  );
}

function SeverityBadge({ severity }: { severity: string }) {
  const colors: Record<string, string> = {
    none: 'bg-gray-100 text-gray-800',
    minor: 'bg-yellow-100 text-yellow-800',
    major: 'bg-orange-100 text-orange-800',
    critical: 'bg-red-100 text-red-800',
  };
  return (
    <span className={`inline-block px-2 py-1 text-xs font-medium rounded ${colors[severity] || colors.none}`}>
      {severity.toUpperCase()}
    </span>
  );
}

export default function StatusPage() {
  return (
    <div className="min-h-screen bg-gray-50">
      <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-12">
        <div className="mb-8">
          <h1 className="text-4xl font-bold text-gray-900">Status Page</h1>
          <p className="mt-4 text-lg text-gray-600">
            Real-time uptime status and incident history.
          </p>
        </div>

        {/* Current Status */}
        <div className="bg-white rounded-lg shadow p-6 mb-8">
          <div className="flex items-center justify-between mb-6">
            <h2 className="text-2xl font-bold text-gray-900">Current Status</h2>
            <StatusBadge status={currentStatus} />
          </div>
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
            {services.map((service) => (
              <div key={service.name} className="border rounded-lg p-4">
                <div className="flex items-center justify-between mb-2">
                  <span className="text-sm font-medium text-gray-900">{service.name}</span>
                  <StatusBadge status={service.status} />
                </div>
                <p className="text-xs text-gray-500">30-day uptime: <span className="font-medium">{service.uptime}</span></p>
              </div>
            ))}
          </div>
        </div>

        {/* Incident History */}
        <div className="bg-white rounded-lg shadow p-6">
          <h2 className="text-2xl font-bold text-gray-900 mb-6">Incident History</h2>
          <div className="space-y-6">
            {incidents.map((incident) => (
              <div key={incident.id} className="border rounded-lg overflow-hidden">
                <div className="px-6 py-4 bg-gray-50 border-b">
                  <div className="flex items-center justify-between">
                    <h3 className="text-lg font-semibold text-gray-900">{incident.title}</h3>
                    <div className="flex items-center gap-2">
                      <SeverityBadge severity={incident.severity} />
                      <StatusBadge status={incident.status} />
                    </div>
                  </div>
                  <p className="text-xs text-gray-500 mt-1">
                    Created: {new Date(incident.created).toLocaleString()}
                    {incident.resolved && <> • Resolved: {new Date(incident.resolved).toLocaleString()}</>}
                  </p>
                </div>
                <div className="p-6">
                  <div className="space-y-4">
                    {incident.updates.map((update, idx) => (
                      <div key={idx} className="flex gap-4">
                        <div className="flex-shrink-0 w-24 text-xs text-gray-500">
                          {new Date(update.time).toLocaleTimeString()}
                        </div>
                        <div className="flex-1 border-l-2 border-gray-200 pl-4">
                          <p className="text-sm text-gray-700">{update.message}</p>
                        </div>
                      </div>
                    ))}
                  </div>
                </div>
              </div>
            ))}
          </div>
        </div>

        {/* Subscribe to Updates */}
        <div className="mt-8 bg-blue-50 border border-blue-200 rounded-lg p-6">
          <h3 className="text-lg font-semibold text-blue-900 mb-3">Subscribe to Status Updates</h3>
          <p className="text-sm text-blue-800 mb-4">
            Get notified about incidents and maintenance windows via email or webhook.
          </p>
          <div className="flex gap-3">
            <button className="px-4 py-2 bg-blue-600 text-white rounded-md hover:bg-blue-700 text-sm">
              Subscribe via Email
            </button>
            <button className="px-4 py-2 bg-white text-blue-600 border border-blue-300 rounded-md hover:bg-blue-50 text-sm">
              Subscribe via Webhook
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}