-- Create payment_intents table.
--
-- A payment intent represents a single customer payment attempt.
-- State machine: requires_payment_method → requires_confirmation →
--                requires_capture (if manual) → processing → succeeded
--             or canceled at any pre-processing state.

CREATE TABLE payment_intents (
    id                  TEXT        PRIMARY KEY,  -- 'pi_' + random
    merchant_id         UUID        NOT NULL REFERENCES merchants(id),
    amount              BIGINT      NOT NULL CHECK (amount > 0),
    currency            CHAR(3)     NOT NULL,
    status              TEXT        NOT NULL DEFAULT 'requires_payment_method'
                                    CHECK (status IN (
                                        'requires_payment_method',
                                        'requires_confirmation',
                                        'requires_action',
                                        'processing',
                                        'requires_capture',
                                        'succeeded',
                                        'canceled'
                                    )),
    capture_method      TEXT        NOT NULL DEFAULT 'automatic'
                                    CHECK (capture_method IN ('automatic', 'manual')),
    confirmation_method TEXT        NOT NULL DEFAULT 'automatic'
                                    CHECK (confirmation_method IN ('automatic', 'manual')),
    -- Card data reference (tokenized by adapter — never raw PAN)
    payment_method_id   TEXT,
    -- Adapter-level charge reference (e.g. sponsor bank charge ID)
    adapter_charge_id   TEXT,
    adapter_response    JSONB,
    -- Cancellation
    canceled_at         TIMESTAMPTZ,
    cancellation_reason TEXT,
    -- Ledger reference (set after successful capture)
    ledger_txn_id       UUID        REFERENCES ledger_transactions(id),
    metadata            JSONB,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX payment_intents_merchant_id_idx    ON payment_intents (merchant_id);
CREATE INDEX payment_intents_status_idx         ON payment_intents (status);
CREATE INDEX payment_intents_created_at_idx     ON payment_intents (created_at DESC);

CREATE OR REPLACE FUNCTION payment_intents_updated_at()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$;

CREATE TRIGGER payment_intents_updated_at_trigger
    BEFORE UPDATE ON payment_intents
    FOR EACH ROW EXECUTE FUNCTION payment_intents_updated_at();
