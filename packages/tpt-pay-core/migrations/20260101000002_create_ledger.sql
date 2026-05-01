-- Create double-entry financial ledger tables.
--
-- Design principles:
--   - ledger_entries is append-only. No UPDATE or DELETE is ever permitted.
--   - Every financial operation is represented as a transaction with two or more
--     entries that sum to zero (debits = credits).
--   - The integrity check query in internal/ledger/ledger.go verifies this invariant.

CREATE TABLE accounts (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    merchant_id UUID        NOT NULL REFERENCES merchants(id),
    currency    CHAR(3)     NOT NULL,  -- ISO 4217 (e.g. 'USD', 'EUR')
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT accounts_merchant_currency_unique UNIQUE (merchant_id, currency)
);

CREATE INDEX accounts_merchant_id_idx ON accounts (merchant_id);

CREATE TABLE ledger_transactions (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    type            TEXT        NOT NULL,  -- 'payment_intent', 'refund', 'payout', 'fee'
    status          TEXT        NOT NULL,
    reference_id    TEXT,                  -- external reference (payment_intent id, etc.)
    metadata        JSONB,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX ledger_transactions_reference_id_idx ON ledger_transactions (reference_id);
CREATE INDEX ledger_transactions_type_status_idx  ON ledger_transactions (type, status);

CREATE TABLE ledger_entries (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    transaction_id  UUID        NOT NULL REFERENCES ledger_transactions(id),
    account_id      UUID        NOT NULL REFERENCES accounts(id),
    -- Positive = credit (money flows INTO account).
    -- Negative = debit  (money flows OUT of account).
    -- Always stored in smallest currency unit (cents for USD, etc.).
    amount          BIGINT      NOT NULL CHECK (amount <> 0),
    currency        CHAR(3)     NOT NULL,
    description     TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX ledger_entries_transaction_id_idx ON ledger_entries (transaction_id);
CREATE INDEX ledger_entries_account_id_idx     ON ledger_entries (account_id);
CREATE INDEX ledger_entries_created_at_idx     ON ledger_entries (created_at);

-- Enforce append-only: forbid UPDATE and DELETE on ledger_entries.
CREATE OR REPLACE FUNCTION ledger_entries_immutable()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'ledger_entries is append-only: % on row % is forbidden',
        TG_OP, OLD.id;
END;
$$;

CREATE TRIGGER ledger_entries_no_update
    BEFORE UPDATE ON ledger_entries
    FOR EACH ROW EXECUTE FUNCTION ledger_entries_immutable();

CREATE TRIGGER ledger_entries_no_delete
    BEFORE DELETE ON ledger_entries
    FOR EACH ROW EXECUTE FUNCTION ledger_entries_immutable();

-- Ledger reconciliation view: running balance per account.
-- Used by the integrity check job.
CREATE VIEW account_balances AS
    SELECT
        e.account_id,
        a.merchant_id,
        e.currency,
        SUM(e.amount) AS balance
    FROM ledger_entries e
    JOIN accounts a ON a.id = e.account_id
    GROUP BY e.account_id, a.merchant_id, e.currency;
