<?php

declare(strict_types=1);

namespace TPTPay;

/**
 * An error returned by the API, or a failure to reach it.
 */
final class Error extends \RuntimeException
{
    public function __construct(
        string $message,
        public readonly int $status = 0,
        public readonly string $type = 'api_error',
        public readonly string $errorCode = '',
        public readonly string $param = '',
        public readonly string $requestId = '',
    ) {
        parent::__construct($message);
    }

    /** The payment was declined by the card or the network. */
    public function isCardError(): bool
    {
        return $this->type === 'card_error';
    }

    /** The object does not exist, or is not visible to the caller. */
    public function isNotFound(): bool
    {
        return $this->status === 404;
    }

    /** The API could not be reached after the retries. */
    public function isConnectionError(): bool
    {
        return $this->type === 'api_connection_error';
    }
}
