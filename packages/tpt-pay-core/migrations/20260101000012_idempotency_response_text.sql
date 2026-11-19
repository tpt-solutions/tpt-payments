-- Store idempotent responses as exact text, not JSONB.
--
-- JSONB normalises whitespace and key order, so a replayed response could differ
-- byte-for-byte from the original. Clients comparing responses must see the original
-- bytes.
ALTER TABLE idempotency_keys
    ALTER COLUMN response_body TYPE TEXT USING response_body::text;
