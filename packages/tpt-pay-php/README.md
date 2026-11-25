# tpt-solutions/tpt-pay (PHP)

PHP client for the TPT Pay API. It needs PHP 8.1 or later with the `curl`, `json` and `hash` extensions, and nothing else.

```php
use TPTPay\Client;

$client = new Client(getenv('TPT_SECRET_KEY'));
$intent = $client->paymentIntents->create(['amount' => 2500, 'currency' => 'usd'], idempotencyKey: 'order-42');
```

- A secret key (`sk_...`) may call every endpoint; a publishable key (`pk_...`) may retrieve and confirm payment intents.
- Writes carry an `Idempotency-Key`. Pass your own to make a retry safe across process restarts.
- Retries connection failures and 409, 429 and 5xx responses, honoring `Retry-After`.
- Errors are `TPTPay\Error`. `isCardError()`, `isNotFound()` and `isConnectionError()` tell the common cases apart.
- `TPTPay\Webhook::verify($rawBody, $header, $secret)` checks the `TPT-Signature` header. Pass the body before any JSON decoding.

## Tests

```bash
php tests/run.php                       # unit tests; a local fixture server, no external network
TPT_PAY_TEST_URL=http://localhost:8080/v1 TPT_PAY_TEST_KEY=sk_test_... php tests/smoke.php
```

The unit tests include a retry check against PHP's built-in server. The smoke test runs only when both variables are
set. The SDK is hand-written, not generated from the OpenAPI document.
