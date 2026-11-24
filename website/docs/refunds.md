---
sidebar_position: 2
---

# Refunds

Refunds return money for a captured payment.

## Create

`POST /v1/refunds` needs a secret key.

```bash
curl -s -X POST http://localhost:8080/v1/refunds \
  -H "Authorization: Bearer sk_test_..." \
  -H "Content-Type: application/json" \
  -d '{"payment_intent":"pi_...","amount":500,"reason":"requested_by_customer"}'
```

- Omit `amount` to refund the whole remaining balance.
- `reason` is one of `duplicate`, `fraudulent` or `requested_by_customer`.
- You may refund in several parts. The total refunded can never exceed what was captured. Concurrent refunds
  are serialized, so two requests cannot both spend the same balance.

## Status

A refund is `pending` while the network call is in flight, then `succeeded` or `failed`. A pending refund
counts against the refundable balance. A refund whose network outcome is unknown stays `pending` until it is
reconciled.

On success the merchant's balance is reversed in the ledger, and `charge.refunded` is sent.

## Retrieve and list

- `GET /v1/refunds/{id}` returns one refund in the key's merchant and mode.
- `GET /v1/refunds` lists refunds newest first, with `limit` (1 to 100), `starting_after`, `status`
  (`pending`, `succeeded` or `failed`) and `payment_intent` (only that intent's refunds). It needs a secret key.
