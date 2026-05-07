// tpt-payments/frontend/app/developers/webhooks/page.tsx
import { Metadata } from 'next';

export const metadata: Metadata = {
  title: 'Webhook Events - TPT Pay Developer Portal',
  description: 'Complete catalog of webhook event types with example payloads',
};

const webhookEvents = [
  {
    category: 'Payment Intents',
    events: [
      {
        type: 'payment_intent.created',
        description: 'Occurs when a new payment intent is created.',
        example: {
          id: 'evt_3MtwB2eZvKYlo2C2eZvKYlo',
          object: 'event',
          type: 'payment_intent.created',
          created: 1700000000,
          data: {
            object: {
              id: 'pi_3MtwB2eZvKYlo2C2eZvKYlo',
              object: 'payment_intent',
              amount: 2000,
              currency: 'usd',
              status: 'requires_payment_method',
              created: 1700000000,
            },
          },
        },
      },
      {
        type: 'payment_intent.succeeded',
        description: 'Occurs when a payment intent succeeds.',
        example: {
          id: 'evt_3MtwB2eZvKYlo2C2eZvKYlo',
          object: 'event',
          type: 'payment_intent.succeeded',
          created: 1700000100,
          data: {
            object: {
              id: 'pi_3MtwB2eZvKYlo2C2eZvKYlo',
              object: 'payment_intent',
              amount: 2000,
              currency: 'usd',
              status: 'succeeded',
              created: 1700000000,
            },
          },
        },
      },
      {
        type: 'payment_intent.canceled',
        description: 'Occurs when a payment intent is canceled.',
        example: {
          id: 'evt_3MtwB2eZvKYlo2C2eZvKYlo',
          object: 'event',
          type: 'payment_intent.canceled',
          created: 1700000200,
          data: {
            object: {
              id: 'pi_3MtwB2eZvKYlo2C2eZvKYlo',
              object: 'payment_intent',
              amount: 2000,
              currency: 'usd',
              status: 'canceled',
              created: 1700000000,
            },
          },
        },
      },
      {
        type: 'payment_intent.fraud_flagged',
        description: 'Occurs when a payment intent is flagged by fraud detection.',
        example: {
          id: 'evt_3MtwB2eZvKYlo2C2eZvKYlo',
          object: 'event',
          type: 'payment_intent.fraud_flagged',
          created: 1700000300,
          data: {
            object: {
              id: 'pi_3MtwB2eZvKYlo2C2eZvKYlo',
              object: 'payment_intent',
              amount: 2000,
              currency: 'usd',
              status: 'requires_payment_method',
              fraud_reason: 'card_testing',
              fraud_score: 85,
            },
          },
        },
      },
    ],
  },
  {
    category: 'Refunds',
    events: [
      {
        type: 'refund.created',
        description: 'Occurs when a refund is created.',
        example: {
          id: 'evt_3MtwB2eZvKYlo2C2eZvKYlo',
          object: 'event',
          type: 'refund.created',
          created: 1700000400,
          data: {
            object: {
              id: 're_3MtwB2eZvKYlo2C2eZvKYlo',
              object: 'refund',
              amount: 2000,
              status: 'pending',
              payment_intent: 'pi_3MtwB2eZvKYlo2C2eZvKYlo',
            },
          },
        },
      },
      {
        type: 'refund.succeeded',
        description: 'Occurs when a refund succeeds.',
        example: {
          id: 'evt_3MtwB2eZvKYlo2C2eZvKYlo',
          object: 'event',
          type: 'refund.succeeded',
          created: 1700000500,
          data: {
            object: {
              id: 're_3MtwB2eZvKYlo2C2eZvKYlo',
              object: 'refund',
              amount: 2000,
              status: 'succeeded',
              payment_intent: 'pi_3MtwB2eZvKYlo2C2eZvKYlo',
            },
          },
        },
      },
    ],
  },
  {
    category: 'Subscriptions',
    events: [
      {
        type: 'subscription.created',
        description: 'Occurs when a subscription is created.',
        example: {
          id: 'evt_3MtwB2eZvKYlo2C2eZvKYlo',
          object: 'event',
          type: 'subscription.created',
          created: 1700000600,
          data: {
            object: {
              id: 'sub_3MtwB2eZvKYlo2C2eZvKYlo',
              object: 'subscription',
              status: 'trialing',
              current_period_start: 1700000600,
              current_period_end: 1700691200,
            },
          },
        },
      },
      {
        type: 'subscription.updated',
        description: 'Occurs when a subscription is updated.',
        example: {
          id: 'evt_3MtwB2eZvKYlo2C2eZvKYlo',
          object: 'event',
          type: 'subscription.updated',
          created: 1700000700,
          data: {
            object: {
              id: 'sub_3MtwB2eZvKYlo2C2eZvKYlo',
              object: 'subscription',
              status: 'active',
              current_period_start: 1700000600,
              current_period_end: 1700691200,
            },
          },
        },
      },
      {
        type: 'subscription.deleted',
        description: 'Occurs when a subscription is canceled.',
        example: {
          id: 'evt_3MtwB2eZvKYlo2C2eZvKYlo',
          object: 'event',
          type: 'subscription.deleted',
          created: 1700000800,
          data: {
            object: {
              id: 'sub_3MtwB2eZvKYlo2C2eZvKYlo',
              object: 'subscription',
              status: 'canceled',
              canceled_at: 1700000800,
            },
          },
        },
      },
      {
        type: 'subscription.trial_will_end',
        description: 'Occurs 3 days before a subscription trial ends.',
        example: {
          id: 'evt_3MtwB2eZvKYlo2C2eZvKYlo',
          object: 'event',
          type: 'subscription.trial_will_end',
          created: 1700688200,
          data: {
            object: {
              id: 'sub_3MtwB2eZvKYlo2C2eZvKYlo',
              object: 'subscription',
              status: 'trialing',
              trial_end: 1700691200,
            },
          },
        },
      },
    ],
  },
  {
    category: 'Disputes',
    events: [
      {
        type: 'charge.dispute.created',
        description: 'Occurs when a dispute is created.',
        example: {
          id: 'evt_3MtwB2eZvKYlo2C2eZvKYlo',
          object: 'event',
          type: 'charge.dispute.created',
          created: 1700000900,
          data: {
            object: {
              id: 'dp_3MtwB2eZvKYlo2C2eZvKYlo',
              object: 'dispute',
              amount: 2000,
              status: 'needs_response',
              reason: 'fraudulent',
              payment_intent: 'pi_3MtwB2eZvKYlo2C2eZvKYlo',
            },
          },
        },
      },
    ],
  },
];

function EventCard({ event }: { event: any }) {
  return (
    <div className="border rounded-lg overflow-hidden">
      <div className="bg-gray-50 px-4 py-3 border-b">
        <code className="text-sm font-semibold text-blue-700">{event.type}</code>
      </div>
      <div className="p-4">
        <p className="text-sm text-gray-600 mb-3">{event.description}</p>
        <details className="mt-3">
          <summary className="cursor-pointer text-sm font-medium text-blue-600 hover:text-blue-800">
            View Example Payload
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
            Complete catalog of all webhook event types with example payloads.
          </p>
        </div>

        {/* Quick Reference */}
        <div className="mb-8 bg-blue-50 border border-blue-200 rounded-lg p-6">
          <h2 className="text-lg font-semibold text-blue-900 mb-3">Webhook Signature Verification</h2>
          <p className="text-sm text-blue-800 mb-3">
            All webhook payloads are signed with a HMAC-SHA256 signature using your webhook endpoint's secret.
          </p>
          <pre className="bg-blue-900 text-blue-100 p-4 rounded overflow-x-auto text-xs">
{`// JavaScript example
const stripe = require('@tpt/pay');

const event = stripe.webhooks.constructEvent(
  payload,
  signature,
  webhookSecret
);`}
          </pre>
        </div>

        {/* Event Categories */}
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