<?php

declare(strict_types=1);

namespace TPTPay;

use TPTPay\Resources\PaymentIntents;
use TPTPay\Resources\Refunds;
use TPTPay\Resources\Subscriptions;

/**
 * Client for the TPT Pay API.
 *
 * A secret key (sk_...) may call every endpoint; keep it on the server. A publishable key (pk_...)
 * may retrieve and confirm payment intents, which is what a browser checkout needs. Other calls are
 * refused before any request is sent.
 *
 * Every write carries an Idempotency-Key. Pass your own to make a retry safe across process restarts;
 * otherwise one is generated for the call and reused across the client's own retries.
 */
final class Client
{
    public const VERSION = '0.1.0';
    public const DEFAULT_BASE_URL = 'https://api.tpt-payments.com/v1';

    private const RETRYABLE_STATUS = [409, 429, 500, 502, 503, 504];
    private const MAX_BACKOFF_MS = 8000;
    private const MAX_RETRY_AFTER_SECONDS = 60;

    public readonly PaymentIntents $paymentIntents;
    public readonly Refunds $refunds;
    public readonly Subscriptions $subscriptions;

    private readonly string $baseUrl;
    private readonly int $timeoutSeconds;
    private readonly int $maxRetries;
    private readonly int $retryBaseMs;

    /**
     * @param array{baseUrl?: string, timeoutSeconds?: int, maxRetries?: int, retryBaseMs?: int} $options
     */
    public function __construct(private readonly string $apiKey, array $options = [])
    {
        if (!str_starts_with($apiKey, 'sk_') && !str_starts_with($apiKey, 'pk_')) {
            throw new \InvalidArgumentException('apiKey must be a secret (sk_...) or publishable (pk_...) key');
        }
        $this->baseUrl = rtrim($options['baseUrl'] ?? self::DEFAULT_BASE_URL, '/');
        $this->timeoutSeconds = $options['timeoutSeconds'] ?? 30;
        $this->maxRetries = $options['maxRetries'] ?? 2;
        $this->retryBaseMs = $options['retryBaseMs'] ?? 500;
        if ($this->maxRetries < 0) {
            throw new \InvalidArgumentException('maxRetries must be zero or more');
        }
        $this->paymentIntents = new PaymentIntents($this);
        $this->refunds = new Refunds($this);
        $this->subscriptions = new Subscriptions($this);
    }

    /**
     * Sends a request and returns the decoded JSON body (an empty array for an empty body).
     *
     * @param array<string, scalar> $query
     * @param array<string, mixed>|null $body
     * @return array<string, mixed>
     * @internal used by the resource classes
     */
    public function request(
        string $method,
        string $path,
        array $query = [],
        ?array $body = null,
        bool $secret = false,
        ?string $idempotencyKey = null,
    ): array {
        if ($secret && !str_starts_with($this->apiKey, 'sk_')) {
            throw new Error(
                sprintf('%s %s requires a secret key (sk_...). Publishable keys cannot perform this operation.', $method, $path),
                403,
                'invalid_request_error',
                'permission_denied',
            );
        }

        $payload = $body === null ? null : json_encode($body, JSON_THROW_ON_ERROR);
        $idem = $idempotencyKey;
        if ($method !== 'GET' && $idem === null) {
            $idem = bin2hex(random_bytes(16));
        }
        $url = $this->baseUrl . $path . ($query === [] ? '' : '?' . http_build_query($query));

        $attempts = $this->maxRetries + 1;
        for ($attempt = 0; $attempt < $attempts; $attempt++) {
            $last = $attempt === $attempts - 1;
            $result = $this->send($method, $url, $payload, $idem);

            if ($result['error'] !== null) {
                if ($last) {
                    throw new Error('could not reach the TPT Pay API: ' . $result['error'], 0, 'api_connection_error');
                }
                $this->sleepMs($this->backoffMs($attempt, null));
                continue;
            }

            $status = $result['status'];
            if ($status >= 200 && $status < 300) {
                if ($result['body'] === '') {
                    return [];
                }
                $decoded = json_decode($result['body'], true);
                if (!is_array($decoded)) {
                    throw new Error('the API returned a body that is not a JSON object', $status);
                }
                return $decoded;
            }

            $error = self::errorFrom($status, $result['body'], $result['headers']['x-request-id'] ?? '');
            if (in_array($status, self::RETRYABLE_STATUS, true) && !$last) {
                $this->sleepMs($this->backoffMs($attempt, $this->retryAfterSeconds($result['headers']['retry-after'] ?? null)));
                continue;
            }
            throw $error;
        }
        throw new Error('request failed'); // unreachable: the loop returns or throws on its last attempt
    }

    /**
     * @return array{status: int, body: string, headers: array<string, string>, error: ?string}
     */
    private function send(string $method, string $url, ?string $payload, ?string $idempotencyKey): array
    {
        $headers = [
            'Authorization: Bearer ' . $this->apiKey,
            'Accept: application/json',
            'User-Agent: tpt-pay-php/' . self::VERSION,
        ];
        if ($payload !== null) {
            $headers[] = 'Content-Type: application/json';
        }
        if ($idempotencyKey !== null) {
            $headers[] = 'Idempotency-Key: ' . $idempotencyKey;
        }

        $ch = curl_init($url);
        if ($ch === false) {
            return ['status' => 0, 'body' => '', 'headers' => [], 'error' => 'curl could not start'];
        }
        $responseHeaders = [];
        curl_setopt_array($ch, [
            CURLOPT_CUSTOMREQUEST => $method,
            CURLOPT_HTTPHEADER => $headers,
            CURLOPT_RETURNTRANSFER => true,
            CURLOPT_TIMEOUT => $this->timeoutSeconds,
            CURLOPT_FOLLOWLOCATION => false,
            CURLOPT_HEADERFUNCTION => static function ($ch, string $line) use (&$responseHeaders): int {
                $parts = explode(':', $line, 2);
                if (count($parts) === 2) {
                    $responseHeaders[strtolower(trim($parts[0]))] = trim($parts[1]);
                }
                return strlen($line);
            },
        ]);
        if ($payload !== null) {
            curl_setopt($ch, CURLOPT_POSTFIELDS, $payload);
        }
        $body = curl_exec($ch);
        if ($body === false) {
            $message = curl_error($ch);
            curl_close($ch);
            return ['status' => 0, 'body' => '', 'headers' => [], 'error' => $message];
        }
        $status = (int) curl_getinfo($ch, CURLINFO_RESPONSE_CODE);
        curl_close($ch);
        return ['status' => $status, 'body' => (string) $body, 'headers' => $responseHeaders, 'error' => null];
    }

    /** Returns the exponential backoff with jitter, or the server's Retry-After (capped). */
    private function backoffMs(int $attempt, ?int $retryAfterSeconds): int
    {
        if ($retryAfterSeconds !== null) {
            return min($retryAfterSeconds, self::MAX_RETRY_AFTER_SECONDS) * 1000;
        }
        $base = min($this->retryBaseMs * (2 ** $attempt), self::MAX_BACKOFF_MS);
        return (int) ($base * (0.5 + mt_rand() / mt_getrandmax() / 2));
    }

    /** Reads Retry-After as delay-seconds or an HTTP date. Null when absent or unusable. */
    private function retryAfterSeconds(?string $value): ?int
    {
        if ($value === null || $value === '') {
            return null;
        }
        if (ctype_digit($value)) {
            return (int) $value;
        }
        $when = strtotime($value);
        if ($when === false) {
            return null;
        }
        return max(0, $when - time());
    }

    private function sleepMs(int $ms): void
    {
        if ($ms > 0) {
            usleep($ms * 1000);
        }
    }

    private static function errorFrom(int $status, string $body, string $requestId): Error
    {
        $decoded = json_decode($body, true);
        $inner = is_array($decoded) && isset($decoded['error']) && is_array($decoded['error']) ? $decoded['error'] : null;
        if ($inner !== null && isset($inner['message']) && is_string($inner['message'])) {
            return new Error(
                $inner['message'],
                $status,
                (string) ($inner['type'] ?? 'api_error'),
                (string) ($inner['code'] ?? ''),
                (string) ($inner['param'] ?? ''),
                $requestId,
            );
        }
        $text = match (true) {
            $status === 404 => 'Not Found',
            $status === 401 => 'Unauthorized',
            default => 'the API returned an error',
        };
        return new Error($text, $status, 'api_error', '', '', $requestId);
    }
}
