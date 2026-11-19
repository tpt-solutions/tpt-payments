-- Webhook delivery queue (transactional outbox).
--
-- A delivery row is inserted in the same database transaction as the state change that
-- produced the event, so an event is never lost and never sent for a change that did not
-- commit. A worker claims due rows with FOR UPDATE SKIP LOCKED, so several workers can run.
--
-- payload is TEXT, not JSONB: the exact bytes stored are the exact bytes signed and sent.

ALTER TABLE webhook_deliveries
    ALTER COLUMN payload TYPE TEXT USING payload::text,
    ADD COLUMN IF NOT EXISTS merchant_id UUID REFERENCES merchants(id),
    ADD COLUMN IF NOT EXISTS livemode    BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS failed_at   TIMESTAMPTZ;

-- A delivery is pending while neither delivered nor failed.
DROP INDEX IF EXISTS webhook_deliveries_next_attempt_at_idx;
CREATE INDEX webhook_deliveries_pending_idx
    ON webhook_deliveries (next_attempt_at)
    WHERE delivered_at IS NULL AND failed_at IS NULL;

-- One delivery per (endpoint, event): enqueueing the same event twice is a no-op.
CREATE UNIQUE INDEX webhook_deliveries_endpoint_event_uniq
    ON webhook_deliveries (endpoint_id, event_id);
