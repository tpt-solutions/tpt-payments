# go-minimal

A small TPT Pay integration in Go. It uses only the standard library and shows the two parts every
integration needs: creating a payment from your server, and verifying webhooks before trusting them.

## Run it

Start TPT Pay with `make quickstart` at the repository root, then:

```bash
cd examples/go-minimal
export TPT_SECRET_KEY=sk_test_...          # printed by make quickstart
export TPT_WEBHOOK_SECRET=whsec_...        # returned once when you create an endpoint
go run .
```

Create a payment:

```bash
curl -s -X POST http://localhost:4000/checkout \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: order-1001" \
  -d '{"amount":1000,"currency":"usd"}'
```

Receive events: register an endpoint for `http://localhost:4000/webhooks` (see the quickstart), then send a
signed test event:

```bash
tpt trigger payment_intent.succeeded --forward-to http://localhost:4000/webhooks --secret $TPT_WEBHOOK_SECRET
```

## Tests

```bash
go test ./...
```

The tests cover the signature rules: a valid signature, a wrong secret, a tampered body, a secret rotation
(two `v1` values), stale and future timestamps, a zero tolerance (which must not disable the check), and
malformed headers.

## Notes

- The signature is computed over the **raw** body, so the handler reads the bytes before parsing them.
- The secret is the whole `whsec_...` string, used as the HMAC key.
- The example does not store `TPT-Event-Id`. A real integration should record it and skip repeats, because
  deliveries are at least once.
