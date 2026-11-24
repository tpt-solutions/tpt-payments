---
sidebar_position: 2
---

# Quickstart: your first payment

You need Docker with Compose. From the repository root:

```bash
make quickstart
```

This copies the example environment file if you have none, starts PostgreSQL, Redis, NATS, Vault and the
server, waits for `http://localhost:8080/health`, creates a sandbox merchant, and prints a **test** secret
key (`sk_test_...`). It never prints a live key.

Use that key below.

## 1. Create a payment intent

Amounts are integers in the currency's minor units, so `1000` is 10.00 USD.

```bash
curl -s -X POST http://localhost:8080/v1/payment_intents \
  -H "Authorization: Bearer sk_test_..." \
  -H "Content-Type: application/json" \
  -d '{"amount":1000,"currency":"usd"}'
```

## 2. Confirm it with a sandbox card

Use the `id` from the response (it starts with `pi_`).

```bash
curl -s -X POST http://localhost:8080/v1/payment_intents/pi_.../confirm \
  -H "Authorization: Bearer sk_test_..." \
  -H "Content-Type: application/json" \
  -d '{"payment_method":"pm_card_visa"}'
```

The status becomes `succeeded`. Other sandbox payment methods are listed in [Sandbox](./sandbox).

## 3. Capture later (optional)

Create the intent with `"capture_method":"manual"`. Confirming it leaves it at `requires_capture`. Then
capture all of it, or part of it:

```bash
curl -s -X POST http://localhost:8080/v1/payment_intents/pi_.../capture \
  -H "Authorization: Bearer sk_test_..." \
  -H "Content-Type: application/json" \
  -d '{"amount_to_capture":1000}'
```

## 4. Receive webhooks

Register an endpoint. Test keys may use `http://localhost`. The response includes the signing secret
(`whsec_...`) once, so keep it.

```bash
curl -s -X POST http://localhost:8080/v1/webhook_endpoints \
  -H "Authorization: Bearer sk_test_..." \
  -H "Content-Type: application/json" \
  -d '{"url":"http://localhost:4000/webhooks","enabled_events":["payment_intent.succeeded"]}'
```

To test your receiver without making a payment, send a signed test event with the `tpt` CLI:

```bash
tpt trigger payment_intent.succeeded --forward-to http://localhost:4000/webhooks --secret whsec_...
```

Verification examples are in [Webhooks](./webhooks).
