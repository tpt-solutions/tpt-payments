# Subscription billing example

Express endpoints that start, read and cancel TPT Pay subscriptions, and a verified webhook receiver for
subscription events. Subscriptions are test mode only for now, so use a test key.

```bash
npm install
TPT_SECRET_KEY=sk_test_... TPT_WEBHOOK_SECRET=whsec_... npm start
```

- `POST /api/subscriptions` with `{"plan": "pro", "payment_method": "pm_card_visa", "customer_ref": "c_1"}`.
  The plan table in `server.js` sets the price; the browser never sends an amount.
- `GET /api/subscriptions/:id`
- `POST /api/subscriptions/:id/cancel` with `{"at_period_end": true}` to cancel at the end of the paid period.
- `POST /webhook` receives `subscription.created`, `subscription.past_due` and `subscription.canceled`. The
  signature is checked against the raw body before anything runs.

A handler that throws answers `500`, so TPT Pay sends the event again.

Tests (no network): `npm test`
