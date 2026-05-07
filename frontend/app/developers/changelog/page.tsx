// tpt-payments/frontend/app/developers/changelog/page.tsx
import { Metadata } from 'next';

export const metadata: Metadata = {
  title: 'Changelog - TPT Pay Developer Portal',
  description: 'API version history and platform updates',
};

const changelogEntries = [
  {
    version: '2024-06-20',
    date: 'June 20, 2024',
    changes: [
      { type: 'added', description: 'Added support for multi-currency transactions' },
      { type: 'added', description: 'New payout schedules (daily, weekly, custom)' },
      { type: 'changed', description: 'Updated API version to match Stripe 2024-06-20' },
      { type: 'fixed', description: 'Fixed edge case in subscription proration calculation' },
    ],
  },
  {
    version: '2024-04-15',
    date: 'April 15, 2024',
    changes: [
      { type: 'added', description: 'New fraud scoring engine with ML-ready hooks' },
      { type: 'added', description: 'Connected account dashboard access (Phase 2)' },
      { type: 'deprecated', description: 'Old webhook endpoint creation API will be removed in v2024-12-31' },
    ],
  },
  {
    version: '2024-01-10',
    date: 'January 10, 2024',
    changes: [
      { type: 'added', description: 'Python SDK with async support released' },
      { type: 'added', description: 'Scheduled report emails (configurable)' },
      { type: 'changed', description: 'Improved webhook delivery retry policy (72-hour window)' },
    ],
  },
  {
    version: '2023-11-05',
    date: 'November 5, 2023',
    changes: [
      { type: 'added', description: 'Initial public release of TPT Pay MVP' },
      { type: 'added', description: 'Payment Intents API (create, confirm, capture, cancel)' },
      { type: 'added', description: 'Refunds API with partial refund support' },
      { type: 'added', description: 'Subscriptions and billing engine' },
      { type: 'added', description: 'Invoicing with hosted payment pages' },
      { type: 'added', description: 'JavaScript SDK (@tpt/pay) published to npm' },
      { type: 'added', description: 'Merchant dashboard v1 (Next.js)' },
    ],
  },
];

function ChangeBadge({ type }: { type: string }) {
  const colors: Record<string, string> = {
    added: 'bg-green-100 text-green-800',
    changed: 'bg-blue-100 text-blue-800',
    fixed: 'bg-yellow-100 text-yellow-800',
    deprecated: 'bg-red-100 text-red-800',
    removed: 'bg-gray-100 text-gray-800',
  };
  return (
    <span className={`inline-block px-2 py-1 text-xs font-bold rounded ${colors[type] || colors.added}`}>
      {type.toUpperCase()}
    </span>
  );
}

export default function ChangelogPage() {
  return (
    <div className="min-h-screen bg-gray-50">
      <div className="max-w-4xl mx-auto px-4 sm:px-6 lg:px-8 py-12">
        <div className="mb-8">
          <h1 className="text-4xl font-bold text-gray-900">Changelog</h1>
          <p className="mt-4 text-lg text-gray-600">
            API version history and platform updates.
          </p>
        </div>

        <div className="space-y-8">
          {changelogEntries.map((entry) => (
            <div key={entry.version} className="bg-white rounded-lg shadow overflow-hidden">
              <div className="px-6 py-4 bg-gray-50 border-b flex items-center justify-between">
                <div>
                  <h2 className="text-xl font-bold text-gray-900">Version {entry.version}</h2>
                  <p className="text-sm text-gray-600">{entry.date}</p>
                </div>
                <code className="bg-blue-50 text-blue-700 px-3 py-1 rounded text-sm">
                  API v{entry.version}
                </code>
              </div>
              <div className="p-6">
                <ul className="space-y-3">
                  {entry.changes.map((change, idx) => (
                    <li key={idx} className="flex items-start gap-3">
                      <ChangeBadge type={change.type} />
                      <span className="text-gray-700">{change.description}</span>
                    </li>
                  ))}
                </ul>
              </div>
            </div>
          ))}
        </div>

        {/* API Versioning Info */}
        <div className="mt-12 bg-blue-50 border border-blue-200 rounded-lg p-6">
          <h3 className="text-lg font-semibold text-blue-900 mb-3">API Versioning</h3>
          <p className="text-sm text-blue-800 mb-3">
            TPT Pay uses date-based API versioning (YYYY-MM-DD format) to mirror Stripe's approach.
          </p>
          <p className="text-sm text-blue-800 mb-3">
            Specify the API version by setting the <code className="bg-blue-100 px-1 rounded">Stripe-Version</code> header in your requests.
          </p>
          <pre className="bg-blue-900 text-blue-100 p-3 rounded text-xs overflow-x-auto">
{`curl https://api.tptpayments.io/v1/payment_intents \\
  -H "Stripe-Version: 2024-06-20" \\
  -H "Authorization: Bearer sk_test_..."`}
          </pre>
        </div>
      </div>
    </div>
  );
}