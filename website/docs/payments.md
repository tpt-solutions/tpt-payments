---
sidebar_position: 1
---

# Payments

A **payment intent** is one attempt to take money. It moves through a fixed set of statuses.

| Status | Meaning |
|---|---|
| `requires_payment_method` | Created, or declined and open to another method. |
| `requires_confirmation` | Has a method, not yet confirmed. |
| `processing` | The card network is being asked. An unknown outcome also stays here until reconciled. |
| `requires_capture` | Authorized, waiting for capture (manual capture only). |
| `succeeded` | Captured. |
| `canceled` | Canceled before capture. |

## Create

`POST /v1/payment_intents` needs a secret key.

- `amount` (integer, minor units), `currency` (ISO 4217, lower or upper case).
- `capture_method`: `automatic` (default) or `manual`.
- `metadata`: up to a set number of keys, with length limits. Invalid values get a `400` with the
  offending parameter named.

Zero-decimal currencies (such as JPY) take the amount without minor units.

## Confirm

`POST /v1/payment_intents/{id}/confirm` attaches a payment method and asks the network to authorize. A
publishable key may confirm, which is what a browser checkout needs.

A decline leaves the intent at `requires_payment_method`, so the customer can try another method. It also
sends `payment_intent.payment_failed`.

## Capture and cancel

`POST /v1/payment_intents/{id}/capture` takes an optional `amount_to_capture`. It cannot be more than the
authorized amount, and the captured amount is posted to the ledger.

`POST /v1/payment_intents/{id}/cancel` cancels an intent that is not yet captured.

## Idempotency

Send an `Idempotency-Key` header on any write. A retry with the same key and the same request returns the
stored response, marked `Idempotent-Replayed: true`. Reusing a key with a different request is a `400`.
A request that is still running gets a `409`. Keys expire after 24 hours.

## Errors

Errors are JSON with `type`, `code`, `message` and, for parameter errors, `param`. Invalid parameters are
`400`. A payment blocked by a merchant rule is `402` with code `payment_blocked`. Rate limits are `429`
with `Retry-After`. Request bodies over 1 MB are `413`.
