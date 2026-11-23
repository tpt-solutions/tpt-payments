# Node webhook receiver

Verifies and handles TPT Pay webhook deliveries with `@tpt/pay`, using only Node's `http` module.

- The signature is checked over the raw body before parsing (`constructEvent` from `@tpt/pay/webhooks`).
- A failed signature returns 400 with no reason in the body. A handler error returns 500, so the sender retries.
- Request bodies over 1 MB are refused with 413.

## Run

```bash
npm install
WEBHOOK_SECRET=whsec_... PORT=4242 npm start
```

Point an endpoint at `http://localhost:4242/webhook` (for example with the `tpt` CLI's `listen` command once it exists).

## Test

```bash
npm test
```

The tests sign payloads the same way the server does and cover: a valid delivery, a tampered body,
the wrong secret, a missing header, a stale timestamp, a handler failure, and a GET request.
