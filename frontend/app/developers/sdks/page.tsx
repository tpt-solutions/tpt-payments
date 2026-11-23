// tpt-payments/frontend/app/developers/sdks/page.tsx
import { Metadata } from 'next';

export const metadata: Metadata = {
  title: 'SDKs & Code Samples - TPT Pay Developer Portal',
  description: 'JavaScript, Python, and curl examples for all API endpoints',
};

const codeSamples = [
  {
    title: 'JavaScript/TypeScript',
    package: '@tpt/pay',
    install: 'npm install @tpt/pay',
    examples: [
      {
        label: 'Create a Payment Intent',
        code: `import TPTPay from '@tpt/pay';

const tpt = new TPTPay('pk_test_...');

const paymentIntent = await tpt.paymentIntents.create({
  amount: 2000,
  currency: 'usd',
  payment_method_types: ['card'],
  metadata: {
    order_id: '12345'
  }
});

console.log(paymentIntent.id);
console.log(paymentIntent.client_secret);`,
      },
      {
        label: 'Confirm a Payment Intent',
        code: `const paymentIntent = await tpt.paymentIntents.confirm(
  'pi_3MtwB2eZvKYlo2C2eZvKYlo',
  {
    payment_method: 'pm_card_visa',
  }
);

if (paymentIntent.status === 'succeeded') {
  console.log('Payment succeeded!');
}`,
      },
      {
        label: 'Create a Refund',
        code: `const refund = await tpt.refunds.create({
  payment_intent: 'pi_3MtwB2eZvKYlo2C2eZvKYlo',
  amount: 1000, // Partial refund
});

console.log(refund.status);`,
      },
      {
        label: 'Webhook Signature Verification',
        code: `const express = require('express');
const app = express();

app.post('/webhook', express.raw({type: 'application/json'}), (req, res) => {
  const sig = req.headers['stripe-signature'];
  const event = tpt.webhooks.constructEvent(
    req.body,
    sig,
    'whsec_...'
  );

  // Handle the event
  switch (event.type) {
    case 'payment_intent.succeeded':
      const paymentIntent = event.data.object;
      console.log('Payment succeeded:', paymentIntent.id);
      break;
    default:
      console.log(\`Unhandled event type \${event.type}\`);
  }

  res.json({received: true});
});`,
      },
    ],
  },
  {
    title: 'Python',
    package: 'tpt-pay',
    install: 'pip install tpt-pay',
    examples: [
      {
        label: 'Create a Payment Intent',
        code: `import tpt_pay

tpt = tpt_pay.TPTPay('pk_test_...')

payment_intent = tpt.PaymentIntent.create(
    amount=2000,
    currency='usd',
    payment_method_types=['card'],
    metadata={'order_id': '12345'}
)

print(payment_intent.id)
print(payment_intent.client_secret)`,
      },
      {
        label: 'Confirm a Payment Intent',
        code: `payment_intent = tpt.PaymentIntent.confirm(
    'pi_3MtwB2eZvKYlo2C2eZvKYlo',
    payment_method='pm_card_visa'
)

if payment_intent.status == 'succeeded':
    print('Payment succeeded!')`,
      },
      {
        label: 'Async Usage',
        code: `import asyncio
import tpt_pay

async def create_payment():
    tpt = tpt_pay.TPTPay('pk_test_...')
    
    payment_intent = await tpt.PaymentIntent.create_async(
        amount=2000,
        currency='usd'
    )
    
    print(payment_intent.id)

asyncio.run(create_payment())`,
      },
    ],
  },
  {
    title: 'cURL',
    examples: [
      {
        label: 'Create a Payment Intent',
        code: `curl -X POST https://api-sandbox.tpt-payments.com/v1/payment_intents \\
  -H "Authorization: Bearer sk_test_..." \\
  -H "Content-Type: application/json" \\
  -d '{
    "amount": 2000,
    "currency": "usd",
    "payment_method_types": ["card"]
  }'`,
      },
      {
        label: 'Retrieve a Payment Intent',
        code: `curl -X GET https://api-sandbox.tpt-payments.com/v1/payment_intents/pi_3MtwB2eZvKYlo2C2eZvKYlo \\
  -H "Authorization: Bearer sk_test_..."`,
      },
      {
        label: 'Create a Refund',
        code: `curl -X POST https://api-sandbox.tpt-payments.com/v1/refunds \\
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
            JavaScript, Python, and curl examples for all API endpoints.
          </p>
        </div>

        <div className="space-y-12">
          {codeSamples.map((sdk) => (
            <div key={sdk.title} className="bg-white rounded-lg shadow overflow-hidden">
              <div className="px-6 py-4 bg-gray-50 border-b">
                <h2 className="text-2xl font-bold text-gray-900">{sdk.title}</h2>
                {sdk.package && (
                  <div className="mt-2 flex items-center gap-4">
                    <code className="text-sm bg-gray-100 px-3 py-1 rounded">
                      {sdk.install}
                    </code>
                  </div>
                )}
              </div>
              <div className="p-6 space-y-6">
                {sdk.examples.map((example, idx) => (
                  <div key={idx}>
                    <h3 className="text-lg font-semibold text-gray-800 mb-3">
                      {example.label}
                    </h3>
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