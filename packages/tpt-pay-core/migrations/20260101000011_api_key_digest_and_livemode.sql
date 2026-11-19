-- API key lookup by digest, and mode-scoped payment intents.
--
-- 1. Keys are 192-bit random values, so a plain SHA-256 digest is sufficient to store
--    them safely (a leaked digest cannot be inverted). bcrypt was slow and was run
--    against every candidate row on every request. Lookup is now a unique-index hit.
--    Existing rows were bcrypt-hashed and cannot authenticate after this change; this
--    is acceptable because the platform is pre-release, and such keys must be recreated.
-- 2. payment_intents.livemode records which mode created the intent, so test and live
--    data are isolated.

DROP INDEX IF EXISTS api_keys_key_prefix_idx;

ALTER TABLE api_keys
    ADD COLUMN IF NOT EXISTS key_digest TEXT;

DELETE FROM api_keys WHERE key_digest IS NULL;

ALTER TABLE api_keys
    ALTER COLUMN key_digest SET NOT NULL,
    DROP COLUMN key_hash;

CREATE UNIQUE INDEX api_keys_key_digest_uniq ON api_keys (key_digest);

ALTER TABLE payment_intents
    ADD COLUMN IF NOT EXISTS livemode BOOLEAN NOT NULL DEFAULT FALSE;

CREATE INDEX IF NOT EXISTS payment_intents_livemode_idx ON payment_intents (merchant_id, livemode);
