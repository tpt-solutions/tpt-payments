<?php

// Test fixture for the retry tests, served by PHP's built-in server. The first two requests fail
// with 503; the third succeeds. Every attempt's Idempotency-Key is appended to keys.log, so the test
// can check that a retry reuses the key.

declare(strict_types=1);

$dir = __DIR__;
$countFile = $dir . '/flaky-count.txt';
$count = is_file($countFile) ? (int) file_get_contents($countFile) : 0;
file_put_contents($countFile, (string) ($count + 1));
file_put_contents($dir . '/flaky-keys.log', ($_SERVER['HTTP_IDEMPOTENCY_KEY'] ?? '') . "\n", FILE_APPEND);

header('Content-Type: application/json');
if ($count < 2) {
    http_response_code(503);
    echo '{"error":{"type":"api_error","message":"busy"}}';
    return;
}
echo '{"id":"pi_1","object":"payment_intent","status":"requires_payment_method"}';
