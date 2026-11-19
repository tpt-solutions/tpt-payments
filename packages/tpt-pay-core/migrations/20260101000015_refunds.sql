-- Refunds against captured payment intents.
--
-- A refund moves through pending → succeeded | failed. A pending refund counts against the
-- refundable balance, so concurrent refunds cannot exceed what was captured. A refund that
-- ends in an unknown network outcome stays pending for reconciliation.

CREATE TABLE refunds (
    id                 TEXT        PRIMARY KEY,           -- 're_' + random
    payment_intent_id  TEXT        NOT NULL REFERENCES payment_intents(id),
    merchant_id        UUID        NOT NULL REFERENCES merchants(id),
    livemode           BOOLEAN     NOT NULL DEFAULT FALSE,
    amount             BIGINT      NOT NULL CHECK (amount > 0),
    currency           CHAR(3)     NOT NULL,
    status             TEXT        NOT NULL
                                   CHECK (status IN ('pending', 'succeeded', 'failed')),
    reason             TEXT,
    adapter_refund_id  TEXT,
    ledger_txn_id      UUID        REFERENCES ledger_transactions(id),
    failure_message    TEXT,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX refunds_payment_intent_id_idx ON refunds (payment_intent_id);
CREATE INDEX refunds_merchant_livemode_idx ON refunds (merchant_id, livemode);
