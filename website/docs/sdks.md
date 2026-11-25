---
sidebar_position: 1
---

# SDKs and tools

The SDKs are in this repository. They are not yet published to npm or PyPI.

## JavaScript and TypeScript: `@tpt/pay`

```js
const { TPTPay } = require('@tpt/pay');

const pay = new TPTPay({
  apiKey: process.env.TPT_SECRET_KEY,
  apiBaseUrl: 'http://localhost:8080/v1', // your server; the default is the hosted API
});
```

The client retries connection failures and retryable responses (409, 429, 5xx) with backoff, and sends an
idempotency key with each retry. When the server sends `Retry-After` (seconds or an HTTP date), the client waits
that long instead, up to 60 seconds. Errors are `TPTPayError`, with `code`, `param` and `requestId`.

## Python: `tpt-payments`

```python
import os
from tpt_payments import TPTPay

pay = TPTPay(os.environ["TPT_SECRET_KEY"], api_base="http://localhost:8080/v1")
```

The Python client takes secret keys only.

## Go: `tpt-pay-go`

```go
import tptpay "github.com/tpt-solutions/tpt-payments/packages/tpt-pay-go"

client, err := tptpay.New(os.Getenv("TPT_SECRET_KEY"), tptpay.WithBaseURL("http://localhost:8080/v1"))

pi, err := client.PaymentIntents.Create(ctx, tptpay.CreatePaymentIntentParams{Amount: 2500, Currency: "usd"},
    tptpay.WithIdempotencyKey("order-42"))

// In the webhook handler, before decoding the body:
err = tptpay.VerifyWebhook(body, r.Header.Get(tptpay.SignatureHeader), secret, 0)
```

The Go client uses only the standard library. Errors are `*tptpay.Error`, and `IsCardError`, `IsNotFound` and
`IsConnectionError` tell the common cases apart. It is written by hand, not generated from the OpenAPI document.

## Command line: `tpt`

Build it from `packages/tpt-pay-core/cmd/tpt`. Commands:

- `tpt login`: save a profile (the file is written with mode 0600; plain `http` only for localhost).
- `tpt keys create`: create an API key.
- `tpt trigger EVENT --forward-to URL --secret whsec_...`: send a signed test event.
- `tpt events`: list the event types it can send.
- `tpt logs` and `tpt replay DELIVERY_ID`: inspect and resend deliveries.

`tpt listen`, which forwards live deliveries to a local machine, is not available yet.

## Starter project

`npx create-tpt-pay-app` makes a Node project with a checkout endpoint and a verified webhook receiver. It
is not published yet, so run it from the repository with the scaffolder's path.
