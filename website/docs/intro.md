---
slug: /
sidebar_position: 1
---

# TPT Pay

TPT Pay is an open-source payments API. It takes payments with payment intents, refunds them, and sends
signed webhooks. Its shapes are modelled on Stripe's, so most concepts carry over; the differences are in the
[migration guide](./migration/stripe). You can self-host it, and you can try it end to end against the built-in sandbox.

:::warning Pre-release
This is pre-release software. Each capability is marked in the repository's status table
(`README.md`). Check that table before you rely on something in production. Subscriptions, invoices,
customers, disputes and payouts are not built yet.
:::

## What works today

- Payment intents: create, confirm, capture (automatic or manual, full or partial), cancel, list, retrieve.
- Refunds: full and partial, with a row lock so concurrent refunds cannot exceed the captured amount.
- Webhook endpoints, signed deliveries with retries, a delivery log, and replay.
- Idempotency keys on every write, API keys with secret and publishable types, and test and live modes.
- A sandbox card adapter with scripted outcomes for tests.

Start with the [quickstart](./quickstart).
