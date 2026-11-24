---
sidebar_position: 3
---

# Webhooks

TPT Pay sends an HTTP `POST` to your endpoint when something happens to a payment or refund.

## Events

| Event | Sent when |
|---|---|
| `payment_intent.created` | An intent is created. |
| `payment_intent.succeeded` | An intent is captured. |
| `payment_intent.payment_failed` | An authorization is declined. |
| `payment_intent.canceled` | An intent is canceled. |
| `charge.refunded` | A refund succeeds. |

Only these are listed when you subscribe (`enabled_events`). Use `"*"` for all of them.

## Endpoints

`POST /v1/webhook_endpoints` with `url` and `enabled_events`. A live endpoint must use `https` and a public
address. A test endpoint may use `http://localhost`. The response includes the signing secret once.

## Signatures

Each delivery has a `TPT-Signature` header:

```
TPT-Signature: t=1700000000,v1=<hex HMAC-SHA256>
```

The signature is the HMAC-SHA256 of `"<t>.<raw body>"`. The key is the endpoint's whole secret string, including
the `whsec_` prefix. To verify:

1. Read the **raw** request body. Do not parse it first.
2. Compute the HMAC of `t + "." + body`.
3. Compare it with each `v1` value in constant time. Several `v1` values may appear while a secret is being
   rotated, and any match is valid.
4. Reject timestamps more than five minutes old (the default tolerance), or more than a minute in the future.

The SDKs do this for you: `@tpt/pay/webhooks` for Node and `tpt_payments.webhooks` for Python. Runnable
receivers are in `examples/node-webhook-receiver`, `examples/python-webhook-receiver` and `examples/go-minimal`.

## Delivery and retries

Deliveries are at least once. Use the `TPT-Event-Id` header to ignore duplicates.

A failed delivery (any non-2xx response, or no answer within the timeout) is retried. The default policy is up
to 15 attempts within 72 hours. Each delivery is logged. You can list recent deliveries with
`GET /v1/webhook_deliveries` and send one again with `POST /v1/webhook_deliveries/{id}/replay`.

Redirects are not followed. Destinations are checked when the connection is made, so a hostname that resolves
to a private address is refused.

## Test a receiver

Send a signed sample event with the CLI:

```bash
tpt trigger payment_intent.succeeded --forward-to http://localhost:4000/webhooks --secret whsec_...
```
