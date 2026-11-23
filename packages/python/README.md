# tpt-payments (Python)

Async Python client for the TPT Pay API, plus webhook signature verification.

> **Status: alpha.** Tests pass (`pytest`, 23 cases). The API it calls is the `packages/tpt-pay-core`
> service, which serves payment intents, refunds, and webhooks. Subscriptions and invoices have
> client methods, but the core service does not serve those routes yet.

## Install

```bash
pip install tpt-payments   # not yet published; install from this directory for now
pip install -e ".[dev]"    # development install, with pytest and pytest-asyncio
```

Requires Python 3.10 or newer.

## Quick start

```python
import asyncio
from tpt_payments import TPTPay

async def main():
    async with TPTPay("sk_test_...") as client:
        intent = await client.payment_intents.create(
            amount=1000,            # minor units, e.g. 1000 = $10.00
            currency="usd",
            capture_method="manual",
        )
        intent = await client.payment_intents.confirm(
            intent["id"], payment_method="pm_card_visa"
        )
        captured = await client.payment_intents.capture(intent["id"])
        print(captured["status"])

asyncio.run(main())
```

Write calls (`create`, `confirm`, `capture`, `cancel`, refunds) send an `Idempotency-Key`. A key
generated for one call is reused across that call's retries. You can pass your own with
`idempotency_key=`.

## Configuration

| Argument      | Default                              | Meaning                                              |
|---------------|--------------------------------------|------------------------------------------------------|
| `api_key`     | required                             | Secret key, `sk_test_...` or `sk_live_...`. Publishable keys are refused. |
| `api_base`    | `https://api.tpt-payments.com/v1`    | Point at a self-hosted deployment or a local server. |
| `timeout`     | `30` seconds                         | Per-attempt timeout.                                 |
| `max_retries` | `2`                                  | Retries for connection errors and 409, 429, 5xx.     |

## Errors

All errors subclass `tpt_payments.TPTError`. The API error types are `AuthenticationError`,
`PermissionDeniedError`, `InvalidRequestError`, `CardError` (with `code`), and `APIError`. Each
carries `request_id` for support requests. `APIConnectionError` means the request may or may not
have been processed. Retry it with the same idempotency key.

## Webhooks

Verify the raw request body, not a re-serialized copy:

```python
from tpt_payments import construct_event, SignatureVerificationError

try:
    event = construct_event(raw_body, request.headers["TPT-Signature"], WEBHOOK_SECRET)
except SignatureVerificationError:
    return 400
```

The signature header is `TPT-Signature`. Timestamps outside the tolerance window (300 seconds by
default) and future timestamps are rejected.

## Development

```bash
pip install -e ".[dev]"
pytest
```
