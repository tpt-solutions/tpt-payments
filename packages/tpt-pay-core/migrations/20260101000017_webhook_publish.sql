-- Tracks which deliveries have been published to the JetStream stream. The relay publishes
-- rows where published_at IS NULL and sets it afterwards. A redelivery after a crash is
-- suppressed by the stream's message-ID deduplication (the delivery id is the message id).
ALTER TABLE webhook_deliveries
    ADD COLUMN IF NOT EXISTS published_at TIMESTAMPTZ;

-- Rows already finished before this migration must not be published now.
UPDATE webhook_deliveries
SET published_at = COALESCE(delivered_at, failed_at, NOW())
WHERE published_at IS NULL
  AND (delivered_at IS NOT NULL OR failed_at IS NOT NULL);

CREATE INDEX IF NOT EXISTS webhook_deliveries_unpublished_idx
    ON webhook_deliveries (created_at)
    WHERE published_at IS NULL;
