# @tpt/pay (JavaScript / TypeScript)

TypeScript client for the TPT Pay API, plus webhook signature verification. Works in Node 18 or
newer. Ships CommonJS and ESM builds with types.

> **Status: alpha.** Tests pass (jest, 25 cases). The API it calls is the `packages/tpt-pay-core`
> service, which serves payment intents, refunds, and webhooks. The `subscriptions` and `invoices`
> namespaces are typed for the planned API; the core service does not serve those routes yet.

## Install

```bash
npm install @tpt/pay   # not yet published; install from this directory for now
```

## Quick start

```ts
import TPTPay from '@tpt/pay';

const tpt = new TPTPay({ apiKey: process.env.TPT_SECRET_KEY! });

const intent = await tpt.paymentIntents.create({
  amount: 1000, // minor units: 1000 = $10.00
  currency: 'usd',
  capture_method: 'manual',
});
await tpt.paymentIntents.confirm(intent.id, { payment_method: 'pm_card_visa' });
const captured = await tpt.paymentIntents.capture(intent.id);
console.log(captured.status);
```

Write calls send an `Idempotency-Key`. A key generated for one call is reused across that call's
retries. You can pass your own in the request options.

## Options

| Option              | Default                            | Meaning                                              |
|---------------------|------------------------------------|------------------------------------------------------|
| `apiKey`            | required                           | Secret (`sk_...`) or publishable (`pk_...`) key. Publishable keys can only retrieve and confirm intents. |
| `apiBaseUrl`        | `https://api.tpt-payments.com/v1`  | Point at a self-hosted deployment or a local server. |
| `timeoutMs`         | `30000`                            | Per-attempt timeout.                                 |
| `maxRetries`        | `2`                                | Retries for connection errors and 409, 429, 5xx.     |
| `retryBaseDelayMs`  | `500`                              | Base backoff, doubled per attempt, with jitter.      |

## Errors

Failed calls throw `TPTPayError` with `code`, `param`, `requestId`, and `status`.
Signature failures throw `SignatureVerificationError`.

## Webhooks

Pass the raw body (a string or bytes), not parsed JSON:

```ts
import { constructEvent, SignatureVerificationError } from '@tpt/pay/webhooks';

try {
  const event = constructEvent(rawBody, req.headers['tpt-signature'] as string, WEBHOOK_SECRET);
} catch (err) {
  if (err instanceof SignatureVerificationError) return res.status(400).end();
  throw err;
}
```

The header is `TPT-Signature`. The default tolerance is 300 seconds.

## Development

```bash
npm ci
npm test        # jest
npm run build   # tsup: CJS and ESM with types
npm run lint
```
