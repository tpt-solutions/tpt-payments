-- API key management.
--
-- Raw keys are never stored. We store a bcrypt hash and the first 8 characters
-- (prefix) for fast lookup without exposing the full key.
--
-- Key format:
--   Publishable test:  pk_test_<random>
--   Publishable live:  pk_live_<random>
--   Secret test:       sk_test_<random>
--   Secret live:       sk_live_<random>

CREATE TABLE api_keys (
    id            TEXT        PRIMARY KEY,  -- 'key_' + random
    merchant_id   UUID        NOT NULL REFERENCES merchants(id),
    key_hash      TEXT        NOT NULL,     -- bcrypt hash of full key
    key_prefix    TEXT        NOT NULL,     -- first 12 chars for lookup (e.g. 'sk_test_abc1')
    type          TEXT        NOT NULL CHECK (type IN ('publishable', 'secret')),
    mode          TEXT        NOT NULL CHECK (mode IN ('test', 'live')),
    name          TEXT,                     -- human label (e.g. "Production backend")
    last_used_at  TIMESTAMPTZ,
    revoked_at    TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX api_keys_merchant_id_idx ON api_keys (merchant_id);
CREATE INDEX api_keys_key_prefix_idx  ON api_keys (key_prefix)
    WHERE revoked_at IS NULL;
