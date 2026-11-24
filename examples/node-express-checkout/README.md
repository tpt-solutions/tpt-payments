# Express checkout

A checkout backend with [Express](https://expressjs.com) and `@tpt/pay`:

- `POST /api/checkout` creates a payment intent (`{"amount": 2000, "currency": "eur", "order_id": "o_9"}`). The amount is in minor units and must be a positive integer.
- `POST /api/checkout/:id/confirm` attaches a payment method and authorizes (`{"payment_method": "pm_card_visa"}` in the sandbox).
- `GET /api/checkout/:id` returns the intent's current status.
- `POST /webhook` verifies a signed delivery over the raw body (`constructEvent`) before parsing it.

A card decline returns its status and code (for example 402 `card_declined`) so the page can show it. Any other upstream failure returns a generic 502 and does not expose the upstream message.

## Run

```bash
npm install
TPT_SECRET_KEY=sk_test_... TPT_WEBHOOK_SECRET=whsec_... PORT=3000 npm start
```

Use a test key (`sk_test_`) while developing. The tests do not need one.

## Test

```bash
npm test
```

The tests run the Express app against a fake client, so they make no network calls. They cover the
checkout and confirm routes, input validation, a decline, an upstream failure, a signed webhook, a
tampered body, a wrong signature, and a missing header.
