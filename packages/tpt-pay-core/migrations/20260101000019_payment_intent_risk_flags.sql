-- Merchant policy flags on a payment intent: the IDs of the flag rules that matched when the
-- intent was created. A flag does not change how the payment is processed; it is recorded so
-- that a person can review it. Blocked payments are refused and never stored.
ALTER TABLE payment_intents
    ADD COLUMN risk_flags TEXT[] NOT NULL DEFAULT '{}';
