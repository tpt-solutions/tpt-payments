<?php

// Live smoke test against a running TPT Pay server. It runs only when TPT_PAY_TEST_URL and
// TPT_PAY_TEST_KEY (a test secret key) are set, so it never runs by accident:
//   TPT_PAY_TEST_URL=http://localhost:8080/v1 TPT_PAY_TEST_KEY=sk_test_... php tests/smoke.php

declare(strict_types=1);

spl_autoload_register(static function (string $class): void {
    $prefix = 'TPTPay\\';
    if (str_starts_with($class, $prefix)) {
        $file = __DIR__ . '/../src/' . str_replace('\\', '/', substr($class, strlen($prefix))) . '.php';
        if (is_file($file)) {
            require $file;
        }
    }
});

use TPTPay\Client;
use TPTPay\Error;

$url = getenv('TPT_PAY_TEST_URL') ?: '';
$key = getenv('TPT_PAY_TEST_KEY') ?: '';
if ($url === '' || $key === '') {
    echo "skipped: set TPT_PAY_TEST_URL and TPT_PAY_TEST_KEY to run against a server\n";
    exit(0);
}

$client = new Client($key, ['baseUrl' => $url]);

$intent = $client->paymentIntents->create(['amount' => 2500, 'currency' => 'usd', 'capture_method' => 'automatic']);
$intent = $client->paymentIntents->confirm($intent['id'], 'pm_card_visa');
if (($intent['status'] ?? '') !== 'succeeded') {
    fwrite(STDERR, 'FAIL: confirm status is ' . ($intent['status'] ?? 'missing') . "\n");
    exit(1);
}

$refund = $client->refunds->create(['payment_intent' => $intent['id'], 'amount' => 500, 'reason' => 'requested_by_customer']);
$fetched = $client->refunds->retrieve($refund['id']);
if (($fetched['amount'] ?? 0) !== 500) {
    fwrite(STDERR, "FAIL: refund amount\n");
    exit(1);
}

$subscription = $client->subscriptions->create(['amount' => 1500, 'currency' => 'usd', 'interval' => 'month', 'payment_method' => 'pm_card_visa']);
if (($subscription['status'] ?? '') !== 'active') {
    fwrite(STDERR, 'FAIL: subscription status is ' . ($subscription['status'] ?? 'missing') . "\n");
    exit(1);
}
$canceled = $client->subscriptions->cancel($subscription['id']);
if (($canceled['status'] ?? '') !== 'canceled') {
    fwrite(STDERR, "FAIL: cancel\n");
    exit(1);
}

try {
    $client->paymentIntents->retrieve('pi_does_not_exist');
    fwrite(STDERR, "FAIL: a missing intent did not raise an error\n");
    exit(1);
} catch (Error $e) {
    if (!$e->isNotFound()) {
        fwrite(STDERR, "FAIL: missing intent is not reported as not found\n");
        exit(1);
    }
}

echo "smoke test passed\n";
