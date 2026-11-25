<?php

declare(strict_types=1);

namespace TPTPay;

/**
 * Verifies webhook signatures. Pass the raw request body, before any JSON decoding.
 */
final class Webhook
{
    /** The header that carries the signature. */
    public const SIGNATURE_HEADER = 'TPT-Signature';

    /** How old a signed timestamp may be, in seconds, when the caller does not say. */
    public const DEFAULT_TOLERANCE = 300;

    /** How far in the future a timestamp may be, in seconds. */
    private const MAX_CLOCK_SKEW = 60;

    /**
     * Throws a WebhookSignatureException unless the header matches the body and secret, and the
     * timestamp is recent. Any v1 signature that matches is accepted, so both secrets work during rotation.
     */
    public static function verify(string $payload, string $header, string $secret, ?int $tolerance = null, ?int $now = null): void
    {
        [$ts, $signatures] = self::parseHeader($header);
        $expected = self::computeSignature($payload, $ts, $secret);
        $matched = false;
        foreach ($signatures as $sig) {
            if (hash_equals($expected, $sig)) {
                $matched = true;
            }
        }
        if (!$matched) {
            throw new WebhookSignatureException('webhook signature verification failed');
        }

        $now ??= time();
        $issued = (int) $ts;
        if ($issued > $now + self::MAX_CLOCK_SKEW) {
            throw new WebhookSignatureException('webhook timestamp is in the future');
        }
        $tolerance ??= self::DEFAULT_TOLERANCE;
        if ($tolerance <= 0) {
            $tolerance = self::DEFAULT_TOLERANCE;
        }
        if ($now - $issued > $tolerance) {
            throw new WebhookSignatureException('webhook timestamp is too old');
        }
    }

    /** A TPT-Signature header for a payload, for tests of a receiver. */
    public static function sign(string $payload, string $secret, ?int $at = null): string
    {
        $ts = (string) ($at ?? time());
        return 't=' . $ts . ',v1=' . self::computeSignature($payload, $ts, $secret);
    }

    private static function computeSignature(string $payload, string $ts, string $secret): string
    {
        return hash_hmac('sha256', $ts . '.' . $payload, $secret);
    }

    /** @return array{0: string, 1: list<string>} the timestamp and every v1 signature */
    private static function parseHeader(string $header): array
    {
        $ts = '';
        $signatures = [];
        foreach (explode(',', $header) as $part) {
            $pair = explode('=', trim($part), 2);
            if (count($pair) !== 2) {
                continue;
            }
            if ($pair[0] === 't') {
                $ts = $pair[1];
            } elseif ($pair[0] === 'v1') {
                $signatures[] = $pair[1];
            }
        }
        if ($ts === '' || $signatures === [] || !ctype_digit($ts)) {
            throw new WebhookSignatureException('webhook signature header is malformed');
        }
        return [$ts, $signatures];
    }
}
