-- Webhook endpoints and delivery log.
--
-- Signing: HMAC-SHA256(timestamp + "." + payload_bytes) using the endpoint's
-- secret. Header format: TPT-Signature: t=<unix>,v1=<hex_sig>
-- This is intentionally compatible with Stripe's webhook signature scheme.

CREATE TABLE webhook_endpoints (
    id              TEXT        PRIMARY KEY,  -- 'we_' + random
    merchant_id     UUID        NOT NULL REFERENCES merchants(id),
    url             TEXT        NOT NULL,
    enabled_events  TEXT[]      NOT NULL DEFAULT '{"*"}',  -- '*' means all events
    secret_hash     TEXT        NOT NULL,     -- bcrypt hash of the signing secret
    secret_prefix   TEXT        NOT NULL,     -- first 8 chars, for display only
    status          TEXT        NOT NULL DEFAULT 'enabled'
                                CHECK (status IN ('enabled', 'disabled')),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX webhook_endpoints_merchant_id_idx ON webhook_endpoints (merchant_id);

CREATE TABLE webhook_deliveries (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    endpoint_id     TEXT        NOT NULL REFERENCES webhook_endpoints(id),
    event_type      TEXT        NOT NULL,
    event_id        TEXT        NOT NULL,     -- idempotent — same event can be redelivered
    payload         JSONB       NOT NULL,
    attempt_count   INT         NOT NULL DEFAULT 0,
    last_http_status INT,
    last_error      TEXT,
    next_attempt_at TIMESTAMPTZ,
    delivered_at    TIMESTAMPTZ,             -- null until first successful delivery
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX webhook_deliveries_endpoint_id_idx      ON webhook_deliveries (endpoint_id);
CREATE INDEX webhook_deliveries_next_attempt_at_idx  ON webhook_deliveries (next_attempt_at)
    WHERE delivered_at IS NULL;
CREATE INDEX webhook_deliveries_event_id_idx         ON webhook_deliveries (event_id);

CREATE OR REPLACE FUNCTION webhook_endpoints_updated_at()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$;

CREATE TRIGGER webhook_endpoints_updated_at_trigger
    BEFORE UPDATE ON webhook_endpoints
    FOR EACH ROW EXECUTE FUNCTION webhook_endpoints_updated_at();
