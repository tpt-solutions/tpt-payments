<?php

declare(strict_types=1);

namespace TPTPay\Resources;

use TPTPay\Client;

/** Payment intents. Retrieve and confirm also work with a publishable key. */
final class PaymentIntents
{
    public function __construct(private readonly Client $client)
    {
    }

    /**
     * @param array{amount: int, currency: string, capture_method?: string, confirmation_method?: string, metadata?: array<string, string>} $params
     * @return array<string, mixed>
     */
    public function create(array $params, ?string $idempotencyKey = null): array
    {
        return $this->client->request('POST', '/payment_intents', [], $params, true, $idempotencyKey);
    }

    /** @return array<string, mixed> */
    public function retrieve(string $id): array
    {
        return $this->client->request('GET', '/payment_intents/' . rawurlencode($id));
    }

    /** @return array<string, mixed> */
    public function confirm(string $id, string $paymentMethod, ?string $idempotencyKey = null): array
    {
        return $this->client->request('POST', '/payment_intents/' . rawurlencode($id) . '/confirm', [], ['payment_method' => $paymentMethod], false, $idempotencyKey);
    }

    /** Captures an authorized payment. Pass 0 to capture the full amount. @return array<string, mixed> */
    public function capture(string $id, int $amountToCapture = 0, ?string $idempotencyKey = null): array
    {
        $body = $amountToCapture > 0 ? ['amount_to_capture' => $amountToCapture] : [];
        return $this->client->request('POST', '/payment_intents/' . rawurlencode($id) . '/capture', [], $body, true, $idempotencyKey);
    }

    /** @return array<string, mixed> */
    public function cancel(string $id, ?string $idempotencyKey = null): array
    {
        return $this->client->request('POST', '/payment_intents/' . rawurlencode($id) . '/cancel', [], [], true, $idempotencyKey);
    }
}
