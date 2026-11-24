# Migrating from Stripe to TPT Pay

Status: **partial**. TPT Pay covers the payment-intent, refund and webhook-endpoint core of Stripe's
API. It does not yet cover subscriptions, invoices, customers, disputes or payouts, and it is not a
drop-in replacement for Stripe's official SDKs. This guide says what maps across, what differs, and
what to change. The route-level coverage is measured by `packages/tpt-pay-core/cmd/compatreport`
(13 of 27 reference endpoints served at the time of writing).

Written for: developers who already integrate with Stripe's REST API and want to try or move a
payment-intent flow to TPT Pay.

## What maps across

| Stripe | TPT Pay | Notes |
|---|---|---|
| `POST /v1/payment_intents` | same | Amounts are integers in minor units. |
| `GET /v1/payment_intents/{id}` | same | |
| `GET /v1/payment_intents` | same | Keyset pages: `limit`, `starting_after`, `status`. |
| `POST /v1/payment_intents/{id}/confirm` | same | Any valid key may confirm (checkout needs this). |
| `POST /v1/payment_intents/{id}/capture` | same | Partial capture is supported. |
| `POST /v1/payment_intents/{id}/cancel` | same | |
| `POST /v1/refunds` | same | Partial and full refunds. |
| `GET /v1/refunds/{id}` | same | |
| `GET /v1/refunds` | same | Filters: `status`, `payment_intent`. |
| `POST /v1/webhook_endpoints`, `GET`, `DELETE` | same | Secrets are returned once, at creation. |
| `POST /v1/api_keys` | same | Keys are created in the caller's mode. |
| `Idempotency-Key` header | same | Keys expire after 24 hours. The same key with a different request is refused with 400. |

Authentication is `Authorization: Bearer <key>`. Test keys start with `sk_test_` or `pk_test_`;
publishable keys (`pk_`) may retrieve and confirm intents only. List and write routes need a secret
key.

## What differs

**Webhook signatures.** The header is `TPT-Signature`, not `Stripe-Signature`. The scheme is the same
HMAC-SHA256 over `timestamp.payload`, with `t=` and `v1=` components. Verify with the TPT SDK or the
core's verifier rather than Stripe's library, which will not recognise the header name.

**Event names.** The core sends five event types today: `payment_intent.created`,
`payment_intent.succeeded`, `payment_intent.canceled`, `payment_intent.payment_failed` (sent when an
authorization is declined; the intent returns to `requires_payment_method`) and `charge.refunded`.
Stripe uses the same names for these.

Events for disputes, subscriptions, invoices and customers do not exist yet.

**Error codes.** Parameter and idempotency errors are 400, as Stripe returns them. A payment blocked by
a merchant rule returns 402 with code `payment_blocked`. Rate limits return 429 with `Retry-After`.
A request body over 1 MB returns 413.

**Rules and dry runs.** Merchant rules (`POLICY_FILE`) have no Stripe equivalent. `POST
/v1/policy/evaluate` shows what each rule would do to a payment without creating it.

**Pagination.** Lists return `object: "list"`, `data`, `has_more` and `url`, as in Stripe. Page
size is 1 to 100 (default 10). Cursors are object IDs, as in Stripe.

## Changing the base URL

The TPT SDKs take the API base URL as an option, for self-hosted deployments and local testing:

```js
const { TPTPay } = require('@tpt/pay');
const pay = new TPTPay({ apiKey: process.env.TPT_SECRET_KEY, apiBaseUrl: 'https://pay.example.com/v1' });
```

```python
from tpt_payments import TPTPay
pay = TPTPay(api_key=os.environ["TPT_SECRET_KEY"], api_base="https://pay.example.com/v1")
```

**Stripe's official SDKs cannot yet be pointed at TPT Pay.** Their host or base-URL override would
send requests to a server whose responses and webhook headers do not match Stripe's, and this is not
tested. Until the Stripe-compatibility test suite is in place, migrate to the TPT SDKs or call the REST
API directly.

## Suggested migration order

1. Create a test key and run your payment-intent flow against a self-hosted or sandbox instance
   (`README.md` has the test cards and magic payment methods; `pm_test_*` names work too).
2. Switch the webhook receiver to verify `TPT-Signature`.
3. Move refunds, then list endpoints if you use them.
4. Keep Stripe for anything in the "not covered" list below until TPT Pay supports it.

Subscribe only to the event types the core sends (listed under "What differs").

## Not covered yet

Subscriptions, invoices, customers, disputes, payouts, Connect, test clocks, and Stripe's
`/v1/webhook_endpoints/{id}/test`. These are tracked in `todo.md` (sections 9 and 10).
