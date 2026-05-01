-- Idempotency key store.
--
-- When a request arrives with an Idempotency-Key header, we:
--   1. Lock the key row (advisory lock via key hash).
--   2. If a completed response exists, return it directly.
--   3. If in-flight (locked_at set, completed_at null), return 409.
--   4. Otherwise process and store the response.
--
-- The request_checksum ensures that a key can't be reused with a different
-- request body (Stripe behaviour).

CREATE TABLE idempotency_keys (
    id               UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    idempotency_key  TEXT        NOT NULL,
    merchant_id      UUID        NOT NULL REFERENCES merchants(id),
    request_path     TEXT        NOT NULL,
    request_checksum TEXT        NOT NULL,   -- SHA-256 of request body
    response_status  INT,
    response_body    JSONB,
    locked_at        TIMESTAMPTZ,            -- set when request starts processing
    completed_at     TIMESTAMPTZ,            -- set when response is stored
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT idempotency_keys_merchant_key_unique UNIQUE (merchant_id, idempotency_key)
);

CREATE INDEX idempotency_keys_merchant_id_idx ON idempotency_keys (merchant_id);
-- Expire old keys after 24 hours (cleaned up by a background job).
CREATE INDEX idempotency_keys_created_at_idx  ON idempotency_keys (created_at);
