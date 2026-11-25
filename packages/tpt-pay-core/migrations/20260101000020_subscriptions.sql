-- Subscriptions: a fixed amount charged to a payment method at the start of each period.
-- A renewal is a payment intent tagged with the subscription and the period start (see
-- internal/subscription). A failed renewal moves the subscription to past_due and is retried on
-- a fixed schedule; after the last retry the subscription is canceled.
CREATE TABLE subscriptions (
    id                   TEXT        PRIMARY KEY,               -- 'sub_' + random
    merchant_id          UUID        NOT NULL REFERENCES merchants(id),
    status               TEXT        NOT NULL CHECK (status IN ('active', 'past_due', 'canceled')),
    amount               BIGINT      NOT NULL CHECK (amount > 0),
    currency             CHAR(3)     NOT NULL,
    billing_interval     TEXT        NOT NULL CHECK (billing_interval IN ('week', 'month', 'year')),
    payment_method_id    TEXT        NOT NULL,
    current_period_start TIMESTAMPTZ NOT NULL,
    current_period_end   TIMESTAMPTZ NOT NULL,
    next_attempt_at      TIMESTAMPTZ NOT NULL,                  -- when the next charge is due
    retry_count          INT         NOT NULL DEFAULT 0 CHECK (retry_count >= 0),
    cancel_at_period_end BOOLEAN     NOT NULL DEFAULT FALSE,
    cancel_reason        TEXT,
    canceled_at          TIMESTAMPTZ,
    livemode             BOOLEAN     NOT NULL,
    metadata             JSONB,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (current_period_end > current_period_start),
    CHECK ((status = 'canceled') = (canceled_at IS NOT NULL))
);

CREATE INDEX subscriptions_due_idx ON subscriptions (next_attempt_at) WHERE status <> 'canceled';
CREATE INDEX subscriptions_merchant_idx ON subscriptions (merchant_id, livemode, created_at DESC, id DESC);
