-- Webhook endpoint secrets: encrypted at rest, and scoped to a mode.
--
-- secret_hash previously held the raw signing secret (the column name was wrong and the
-- code said "for now"). Signing needs the raw secret, so a hash cannot be used. The
-- column now holds an AES-256-GCM ciphertext (internal/secretbox). Rows written under the
-- old scheme contain plaintext that the new code cannot open, so those endpoints are
-- disabled and must be recreated.

ALTER TABLE webhook_endpoints RENAME COLUMN secret_hash TO secret_ciphertext;
ALTER TABLE webhook_endpoints ADD COLUMN IF NOT EXISTS livemode BOOLEAN NOT NULL DEFAULT FALSE;

UPDATE webhook_endpoints SET status = 'disabled' WHERE secret_ciphertext NOT LIKE 'v1:%';

CREATE INDEX IF NOT EXISTS webhook_endpoints_livemode_idx ON webhook_endpoints (merchant_id, livemode);
