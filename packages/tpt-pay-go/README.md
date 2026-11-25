# tpt-pay-go

Go client for the TPT Pay API. Standard library only.

```go
client, err := tptpay.New(os.Getenv("TPT_SECRET_KEY"))
pi, err := client.PaymentIntents.Create(ctx, tptpay.CreatePaymentIntentParams{Amount: 2500, Currency: "usd"})
```

- A secret key (`sk_...`) may call everything; a publishable key (`pk_...`) may retrieve and confirm payment intents.
- Writes carry an `Idempotency-Key`. Pass `tptpay.WithIdempotencyKey` to make a retry safe across restarts.
- Retries connection failures and 409, 429 and 5xx responses, honoring `Retry-After`.
- `VerifyWebhook` checks the `TPT-Signature` header against the raw body.

Tests: `go test ./...`. The smoke test runs against a server when `TPT_PAY_TEST_URL` and
`TPT_PAY_TEST_KEY` are set.
