// tpt-payments/frontend/app/developers/webhooks/page.tsx
import { Metadata } from 'next';

export const metadata: Metadata = {
  title: 'Webhook Events - TPT Pay Developer Portal',
  description: 'Catalog of the webhook event types the platform emits, with example payloads',
};

// These are the event types in packages/tpt-pay-core/internal/webhook (KnownEventTypes).
// Subscriptions, invoices, customers and disputes are not emitted yet, so they are not listed.
const EVENT_ID = 'evt_3MtwB2eZvKYlo2C2eZvKYlo';
const PI_ID = 'pi_3MtwB2eZvKYlo2C2eZvKYlo';

function paymentIntentEvent(type: string, status: string, created: number, extra: Record<string, unknown> = {}) {
  return {
    type,
    example: {
      id: EVENT_ID,
      object: 'event',
      type,
      created,
      data: {
        object: {
          id: PI_ID,
          object: 'payment_intent',
          amount: 2000,
          currency: 'usd',
          status,
          ...extra,
        },
      },
    },
  };
}

const webhookEvents = [
  {
    category: 'Payment Intents',
    events: [
      {
        ...paymentIntentEvent('payment_intent.created', 'requires_payment_method', 1700000000),
        description: 'A payment intent was created.',
      },
      {
        ...paymentIntentEvent('payment_intent.succeeded', 'succeeded', 1700000100),
        description: 'A payment intent was captured or authorized in full.',
      },
      {
        ...paymentIntentEvent('payment_intent.payment_failed', 'requires_payment_method', 1700000150, {
          last_payment_error: { code: 'card_declined', message: 'Your card was declined.' },
        }),
        description: 'A payment attempt was declined. The intent can be retried.',
      },
      {
        ...paymentIntentEvent('payment_intent.canceled', 'canceled', 1700000200),
        description: 'A payment intent was canceled.',
      },
    ],
  },
  {
    category: 'Refunds',
    events: [
      {
        type: 'charge.refunded',
        description: 'A refund was recorded against a payment. The name matches Stripe so existing receivers work.',
        example: {
          id: EVENT_ID,
          object: 'event',
          type: 'charge.refunded',
          created: 1700000400,
          data: {
            object: {
              id: 'rf_3MtwB2eZvKYlo2C2eZvKYlo',
              object: 'refund',
              amount: 1000,
              currency: 'usd',
              payment_intent: PI_ID,
              status: 'succeeded',
            },
          },
        },
      },
    ],
  },
];

function EventCard({ event }: { event: { type: string; description: string; example: object } }) {
  return (
    <div className="bg-white rounded-lg shadow border">
      <div className="px-4 py-3 border-b">
        <code className="text-sm font-mono text-gray-900">{event.type}</code>
      </div>
      <div className="p-4">
        <p className="text-sm text-gray-600 mb-3">{event.description}</p>
        <details className="mt-3">
          <summary className="cursor-pointer text-sm font-medium text-blue-600 hover:text-blue-800">
            View example payload
          </summary>
          <pre className="mt-2 bg-gray-900 text-gray-100 p-4 rounded overflow-x-auto text-xs">
            {JSON.stringify(event.example, null, 2)}
          </pre>
        </details>
      </div>
    </div>
  );
}

export default function WebhookEventsPage() {
  return (
    <div className="min-h-screen bg-gray-50">
      <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-12">
        <div className="mb-8">
          <h1 className="text-4xl font-bold text-gray-900">Webhook Event Catalog</h1>
          <p className="mt-4 text-lg text-gray-600">
            The event types the platform sends today, with example payloads.
          </p>
        </div>

        <div className="mb-8 bg-blue-50 border border-blue-200 rounded-lg p-6">
          <h2 className="text-lg font-semibold text-blue-900 mb-3">Verifying signatures</h2>
          <p className="text-sm text-blue-800 mb-3">
            Each delivery carries a <code>TPT-Signature</code> header: a timestamp and an HMAC-SHA256 over the
            timestamp and the raw body, keyed by the endpoint secret. Verify the raw bytes before parsing.
          </p>
          <pre className="bg-blue-900 text-blue-100 p-4 rounded overflow-x-auto text-xs">
{`import { constructEvent } from '@tpt/pay/webhooks';

const event = constructEvent(rawBody, req.headers['tpt-signature'], webhookSecret);`}
          </pre>
        </div>

        <div className="space-y-8">
          {webhookEvents.map((category) => (
            <div key={category.category}>
              <h2 className="text-2xl font-bold text-gray-900 mb-4">{category.category}</h2>
              <div className="grid gap-4">
                {category.events.map((event) => (
                  <EventCard key={event.type} event={event} />
                ))}
              </div>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
