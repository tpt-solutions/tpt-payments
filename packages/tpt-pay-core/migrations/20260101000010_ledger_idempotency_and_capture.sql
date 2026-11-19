-- Ledger posting idempotency and captured-amount tracking.
--
-- 1. A payment's ledger transaction may be posted at most once. The unique index is
--    the backstop behind the status compare-and-set in the payment service.
-- 2. payment_intents.amount_captured records what was actually captured, which can be
--    less than the authorized amount (partial capture).

DROP INDEX IF EXISTS ledger_transactions_reference_id_idx;

CREATE UNIQUE INDEX ledger_transactions_type_reference_uniq
    ON ledger_transactions (type, reference_id);

ALTER TABLE payment_intents
    ADD COLUMN IF NOT EXISTS amount_captured BIGINT CHECK (amount_captured IS NULL OR amount_captured >= 0);
