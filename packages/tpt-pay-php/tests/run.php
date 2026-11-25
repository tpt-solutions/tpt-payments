<?php

// Unit tests for the TPT Pay PHP client. They need no network and no PHPUnit:
//   php tests/run.php
// The live smoke test (tests/smoke.php) runs only when TPT_PAY_TEST_URL and TPT_PAY_TEST_KEY are set.

declare(strict_types=1);

spl_autoload_register(static function (string $class): void {
    $prefix = 'TPTPay\\';
    if (!str_starts_with($class, $prefix)) {
        return;
    }
    $file = __DIR__ . '/../src/' . str_replace('\\', '/', substr($class, strlen($prefix))) . '.php';
    if (is_file($file)) {
        require $file;
    }
});

use TPTPay\Client;
use TPTPay\Error;
use TPTPay\Webhook;
use TPTPay\WebhookSignatureException;

$failures = 0;
$passes = 0;

function check(bool $ok, string $name): void
{
    global $failures, $passes;
    if ($ok) {
        $passes++;
        return;
    }
    $failures++;
    fwrite(STDERR, "FAIL: $name\n");
}

function throwsError(callable $fn, callable $matches): bool
{
    try {
        $fn();
    } catch (\Throwable $e) {
        return $matches($e);
    }
    return false;
}

// Keys.
check(throwsError(fn () => new Client('rk_test_abc'), fn ($e) => $e instanceof \InvalidArgumentException), 'a key that is neither secret nor publishable is refused');
check((new Client('pk_test_abc')) instanceof Client, 'a publishable key is accepted');

// A publishable key is refused for server calls before any request. The base URL is unreachable, so a
// network attempt would show up as a connection error instead of 403.
$pk = new Client('pk_test_abc', ['baseUrl' => 'http://127.0.0.1:1', 'maxRetries' => 0]);
check(throwsError(
    fn () => $pk->paymentIntents->create(['amount' => 1000, 'currency' => 'usd']),
    fn ($e) => $e instanceof Error && $e->status === 403 && $e->errorCode === 'permission_denied',
), 'a publishable key cannot create a payment intent (refused locally, 403)');

// An unreachable API is a connection error after the retries.
$unreachable = new Client('sk_test_abc', ['baseUrl' => 'http://127.0.0.1:1', 'maxRetries' => 1, 'retryBaseMs' => 1]);
check(throwsError(
    fn () => $unreachable->paymentIntents->retrieve('pi_1'),
    fn ($e) => $e instanceof Error && $e->isConnectionError(),
), 'an unreachable API is reported as a connection error');

// Error helpers.
$decline = new Error('Your card was declined.', 402, 'card_error', 'card_declined', '', 'req_1');
check($decline->isCardError() && !$decline->isNotFound(), 'a card decline is a card error');
$missing = new Error('No such refund', 404, 'invalid_request_error', 'resource_missing');
check($missing->isNotFound() && !$missing->isCardError(), 'a missing object is not found');

// Webhooks. The vector was computed independently: HMAC-SHA256 of "1700000000." . payload, key = secret.
$secret = 'whsec_test_secret';
$payload = '{"id":"evt_1","type":"payment_intent.succeeded"}';
$at = 1700000000;
$header = Webhook::sign($payload, $secret, $at);
check(str_ends_with($header, '22d8dc182f5c588077683bd378368e29765bfe5df2560112b4f28fef120f0e31'), 'the signature matches the independent vector');

$now = $at + 10;
$ok = true;
try {
    Webhook::verify($payload, $header, $secret, null, $now);
} catch (\Throwable) {
    $ok = false;
}
check($ok, 'a fresh, correctly signed event verifies');

$rejects = static function (callable $fn): bool {
    try {
        $fn();
    } catch (WebhookSignatureException) {
        return true;
    }
    return false;
};
check($rejects(fn () => Webhook::verify('{"id":"evt_2"}', $header, $secret, null, $now)), 'a changed body does not verify');
check($rejects(fn () => Webhook::verify($payload, $header, 'whsec_other', null, $now)), 'the wrong secret does not verify');
check($rejects(fn () => Webhook::verify($payload, $header, $secret, null, $at + 600)), 'a timestamp older than the tolerance is refused');
check($rejects(fn () => Webhook::verify($payload, $header, $secret, null, $at - 600)), 'a timestamp in the future is refused');
check($rejects(fn () => Webhook::verify($payload, 'garbage', $secret, null, $now)), 'a malformed header is refused');

$oldSig = Webhook::sign($payload, 'whsec_old', $at);
$newSig = Webhook::sign($payload, 'whsec_new', $at);
$both = explode(',', $oldSig)[0] . ',' . explode(',', $oldSig)[1] . ',' . explode(',', $newSig)[1];
$rotation = true;
try {
    Webhook::verify($payload, $both, 'whsec_new', null, $at);
} catch (\Throwable) {
    $rotation = false;
}
check($rotation, 'during rotation the new secret verifies when both signatures are present');

// Retries against a real HTTP server: two 503s, then success. Every attempt must reuse one idempotency key.
$fixtures = __DIR__ . '/fixtures';
$port = 18931;
$out = sys_get_temp_dir() . '/tpt-pay-php-flaky.out';
@unlink($fixtures . '/flaky-count.txt');
@unlink($fixtures . '/flaky-keys.log');
$proc = proc_open(
    [PHP_BINARY, '-S', "127.0.0.1:$port", '-t', $fixtures, $fixtures . '/flaky.php'],
    [1 => ['file', $out, 'w'], 2 => ['file', $out, 'a']],
    $pipes,
);
$ready = false;
for ($i = 0; $i < 100 && !$ready; $i++) {
    $sock = @fsockopen('127.0.0.1', $port);
    if ($sock !== false) {
        fclose($sock);
        $ready = true;
    } else {
        usleep(50000);
    }
}
if (!$ready) {
    fwrite(STDERR, "FAIL: fixture server did not start\n");
    $failures++;
} else {
    try {
        $retrying = new Client('sk_test_abc', ['baseUrl' => "http://127.0.0.1:$port", 'maxRetries' => 2, 'retryBaseMs' => 1]);
        $intent = $retrying->paymentIntents->create(['amount' => 1000, 'currency' => 'usd']);
        check(($intent['id'] ?? '') === 'pi_1', 'a create that fails twice with 503 succeeds on the third attempt');
        $keys = array_values(array_filter(explode("\n", (string) @file_get_contents($fixtures . '/flaky-keys.log'))));
        check(count($keys) === 3, 'three attempts were made');
        check(count(array_unique($keys)) === 1 && $keys[0] !== '', 'every attempt used the same idempotency key');
    } catch (\Throwable $e) {
        check(false, 'retrying create threw: ' . $e->getMessage());
    } finally {
        proc_terminate($proc);
        proc_close($proc);
        @unlink($fixtures . '/flaky-count.txt');
        @unlink($fixtures . '/flaky-keys.log');
        @unlink($out);
    }
}

echo "passed: $passes, failed: $failures\n";
exit($failures === 0 ? 0 : 1);
