<?php

declare(strict_types=1);

namespace TPTPay\Resources;

use TPTPay\Client;

/** Refunds of captured payments. */
final class Refunds
{
    public function __construct(private readonly Client $client)
    {
    }

    /**
     * Refunds all or part of a captured payment.
     *
     * @param array{payment_intent: string, amount?: int, reason?: string} $params
     * @return array<string, mixed>
     */
    public function create(array $params, ?string $idempotencyKey = null): array
    {
        return $this->client->request('POST', '/refunds', [], $params, true, $idempotencyKey);
    }

    /** @return array<string, mixed> */
    public function retrieve(string $id): array
    {
        return $this->client->request('GET', '/refunds/' . rawurlencode($id), [], null, true);
    }
}
