-- Merkle checkpoints for ledger audit periods. One row per exported period, recorded when an
-- audit pack is exported with --record. A period's root must not change once recorded: if a
-- later export of the same period gives a different root, the ledger changed after the period
-- was closed, and that is an incident. Rows are append-only.
CREATE TABLE ledger_checkpoints (
    period_start      TIMESTAMPTZ NOT NULL,
    period_end        TIMESTAMPTZ NOT NULL,
    merkle_root       TEXT        NOT NULL CHECK (merkle_root ~ '^[0-9a-f]{64}$'),
    transaction_count INT         NOT NULL CHECK (transaction_count >= 0),
    recorded_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (period_start, period_end),
    CHECK (period_end > period_start)
);

CREATE OR REPLACE FUNCTION ledger_checkpoints_append_only()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'ledger_checkpoints is append-only';
END;
$$;

CREATE TRIGGER ledger_checkpoints_no_update_delete
    BEFORE UPDATE OR DELETE ON ledger_checkpoints
    FOR EACH ROW EXECUTE FUNCTION ledger_checkpoints_append_only();
