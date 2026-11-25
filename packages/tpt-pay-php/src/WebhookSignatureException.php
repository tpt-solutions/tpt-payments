<?php

declare(strict_types=1);

namespace TPTPay;

/** A webhook did not verify. Do not process the event. */
final class WebhookSignatureException extends \RuntimeException
{
}
