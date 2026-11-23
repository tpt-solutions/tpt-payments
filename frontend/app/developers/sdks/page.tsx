// tpt-payments/frontend/app/developers/sdks/page.tsx
import { Metadata } from 'next';

export const metadata: Metadata = {
  title: 'SDKs & Code Samples - TPT Pay Developer Portal',
  description: 'JavaScript, Python, and curl examples for the payment intent and refund endpoints',
};

// Every sample below matches the published SDK surface in packages/tpt-pay (JavaScript) and
// packages/python (Python), and the routes served by packages/tpt-pay-core.
const codeSamples = [
  {
    title: 'JavaScript/TypeScript',
    package: '@tpt/pay',
    install: 'npm install @tpt/pay',
    examples: [
      {
        label: 'Create a Payment Intent',
        code: `import TPTPay from '@tpt/pay';

const tpt = new TPTPay({ apiKey: process.env.TPT_SECRET_KEY! }); // sk_test_... or sk_live_...

const paymentIntent = await tpt.paymentIntents.create({
  amount: 2000,        // minor units: 2000 = $20.00
  currency: 'usd',
  capture_method: 'manual',
});

console.log(paymentIntent.id);
console.log(paymentIntent.status);`,
      },
      {
        label: 'Confirm a Payment Intent',
        code: `const paymentIntent = await tpt.paymentIntents.confirm('pi_3MtwB2eZvKYlo2C2eZvKYlo', {
  payment_method: 'pm_card_visa',
});

if (paymentIntent.status === 'requires_capture') {
  await tpt.paymentIntents.capture(paymentIntent.id);
}`,
      },
      {
        label: 'Create a Refund',
        code: `const refund = await tpt.refunds.create({
  payment_intent: 'pi_3MtwB2eZvKYlo2C2eZvKYlo',
  amount: 1000, // partial refund, in minor units
});

console.log(refund.status);`,
      },
      {
        label: 'Webhook Signature Verification',
        code: `const { constructEvent, SignatureVerificationError } = require('@tpt/pay/webhooks');
const express = require('express');
const app = express();

app.post('/webhook', express.raw({ type: 'application/json' }), (req, res) => {
  let event;
  try {
    // Pass the raw body. Parsed JSON will not verify.
    event = constructEvent(req.body, req.headers['tpt-signature'], process.env.WEBHOOK_SECRET);
  } catch (err) {
    if (err instanceof SignatureVerificationError) return res.status(400).end();
    throw err;
  }

  switch (event.type) {
    case 'payment_intent.succeeded':
      console.log('Payment succeeded:', event.data.object.id);
      break;
    default:
      console.log(\`Unhandled event type \${event.type}\`);
  }

  res.json({ received: true });
});`,
      },
    ],
  },
  {
    title: 'Python',
    package: 'tpt-payments',
    install: 'pip install tpt-payments',
    examples: [
      {
        label: 'Create and Confirm a Payment Intent',
        code: `import asyncio
from tpt_payments import TPTPay

async def main():
    async with TPTPay("sk_test_...") as client:
        intent = await client.payment_intents.create(
            amount=2000,
            currency="usd",
            capture_method="manual",
        )
        intent = await client.payment_intents.confirm(
            intent["id"], payment_method="pm_card_visa"
        )
        print(intent["id"], intent["status"])

asyncio.run(main())`,
      },
      {
        label: 'Create a Refund',
        code: `async with TPTPay("sk_test_...") as client:
    refund = await client.refunds.create(
        payment_intent="pi_3MtwB2eZvKYlo2C2eZvKYlo",
        amount=1000,
    )
    print(refund["status"])`,
      },
      {
        label: 'Webhook Signature Verification',
        code: `from tpt_payments import construct_event, SignatureVerificationError

try:
    event = construct_event(raw_body, request.headers["TPT-Signature"], WEBHOOK_SECRET)
except SignatureVerificationError:
    return 400  # pass the raw bytes, not re-serialized JSON`,
      },
    ],
  },
  {
    title: 'cURL',
    examples: [
      {
        label: 'Create a Payment Intent',
        code: `curl -X POST https://api.tpt-payments.com/v1/payment_intents \\
  -H "Authorization: Bearer sk_test_..." \\
  -H "Content-Type: application/json" \\
  -d '{
    "amount": 2000,
    "currency": "usd",
    "capture_method": "manual"
  }'`,
      },
      {
        label: 'Retrieve a Payment Intent',
        code: `curl -X GET https://api.tpt-payments.com/v1/payment_intents/pi_3MtwB2eZvKYlo2C2eZvKYlo \\
  -H "Authorization: Bearer sk_test_..."`,
      },
      {
        label: 'Create a Refund',
        code: `curl -X POST https://api.tpt-payments.com/v1/refunds \\
  -H "Authorization: Bearer sk_test_..." \\
  -H "Content-Type: application/json" \\
  -d '{
    "payment_intent": "pi_3MtwB2eZvKYlo2C2eZvKYlo",
    "amount": 1000
  }'`,
      },
    ],
  },
];

export default function SDKsPage() {
  return (
    <div className="min-h-screen bg-gray-50">
      <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-12">
        <div className="mb-8">
          <h1 className="text-4xl font-bold text-gray-900">SDKs & Code Samples</h1>
          <p className="mt-4 text-lg text-gray-600">
            JavaScript, Python, and curl examples for payment intents, refunds, and webhooks. Subscriptions
            and invoices are not served yet.
          </p>
        </div>

        <div className="space-y-12">
          {codeSamples.map((sdk) => (
            <div key={sdk.title} className="bg-white rounded-lg shadow overflow-hidden">
              <div className="px-6 py-4 bg-gray-50 border-b">
                <h2 className="text-2xl font-bold text-gray-900">{sdk.title}</h2>
                {sdk.package && (
                  <div className="mt-2 flex items-center gap-4">
                    <code className="text-sm bg-gray-100 px-3 py-1 rounded">{sdk.install}</code>
                  </div>
                )}
              </div>
              <div className="p-6 space-y-6">
                {sdk.examples.map((example, idx) => (
                  <div key={idx}>
                    <h3 className="text-lg font-semibold text-gray-800 mb-3">{example.label}</h3>
                    <pre className="bg-gray-900 text-gray-100 p-4 rounded-md overflow-x-auto text-sm">
                      {example.code}
                    </pre>
                  </div>
                ))}
              </div>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
