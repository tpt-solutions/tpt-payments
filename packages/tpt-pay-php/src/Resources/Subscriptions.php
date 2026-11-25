<?php

declare(strict_types=1);

namespace TPTPay\Resources;

use TPTPay\Client;

/** Subscriptions. Test mode only until a live billing policy is decided. */
final class Subscriptions
{
    public function __construct(private readonly Client $client)
    {
    }

    /**
     * Starts a subscription and charges its first period before it returns.
     *
     * @param array{amount: int, currency: string, interval: string, payment_method: string, metadata?: array<string, string>} $params
     * @return array<string, mixed>
     */
    public function create(array $params, ?string $idempotencyKey = null): array
    {
        return $this->client->request('POST', '/subscriptions', [], $params, true, $idempotencyKey);
    }

    /** @return array<string, mixed> */
    public function retrieve(string $id): array
    {
        return $this->client->request('GET', '/subscriptions/' . rawurlencode($id), [], null, true);
    }

    /**
     * One page of subscriptions, newest first. Pass startingAfter with the last ID of a page for the next.
     *
     * @return array<string, mixed>
     */
    public function list(int $limit = 10, ?string $startingAfter = null): array
    {
        $query = ['limit' => $limit];
        if ($startingAfter !== null) {
            $query['starting_after'] = $startingAfter;
        }
        return $this->client->request('GET', '/subscriptions', $query, null, true);
    }

    /** Cancels now, or at the end of the paid period when $atPeriodEnd is true. @return array<string, mixed> */
    public function cancel(string $id, bool $atPeriodEnd = false, ?string $idempotencyKey = null): array
    {
        return $this->client->request(
            'POST',
            '/subscriptions/' . rawurlencode($id) . '/cancel',
            [],
            ['at_period_end' => $atPeriodEnd],
            true,
            $idempotencyKey,
        );
    }
}
