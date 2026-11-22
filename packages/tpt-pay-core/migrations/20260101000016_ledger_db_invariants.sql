-- Database-level ledger invariants.
--
-- The application validates every posting (internal/ledger.ValidateEntries). These triggers
-- make the same rules hold for any writer, including manual SQL and future code paths:
--
--   1. Balanced: the entries of a transaction sum to zero. Checked at COMMIT (deferred), so
--      a transaction's entries can be inserted one at a time inside a single transaction.
--   2. Currency matches account: an entry's currency is the account's currency.
--   3. No TRUNCATE: ledger_entries and ledger_transactions cannot be emptied in one statement.
--      (Row-level UPDATE and DELETE are already forbidden by the ledger_entries triggers.)
--
-- Transactions with no entries are not caught by (1); the application requires at least two
-- entries before it writes anything.

-- 1. Balanced transactions, enforced at commit.
CREATE OR REPLACE FUNCTION ledger_check_transaction_balanced()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
DECLARE
    unbalanced_sum BIGINT;
BEGIN
    SELECT SUM(amount) INTO unbalanced_sum
    FROM ledger_entries
    WHERE transaction_id = NEW.transaction_id;

    IF unbalanced_sum IS DISTINCT FROM 0 THEN
        RAISE EXCEPTION 'ledger transaction % is not balanced (sum %)',
            NEW.transaction_id, unbalanced_sum
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN NULL;
END;
$$;

CREATE CONSTRAINT TRIGGER ledger_entries_balanced
    AFTER INSERT ON ledger_entries
    DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW EXECUTE FUNCTION ledger_check_transaction_balanced();

-- 2. Entry currency must match the account it posts to.
CREATE OR REPLACE FUNCTION ledger_check_entry_currency()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
DECLARE
    account_currency CHAR(3);
BEGIN
    SELECT currency INTO account_currency FROM accounts WHERE id = NEW.account_id;
    IF account_currency IS DISTINCT FROM NEW.currency THEN
        RAISE EXCEPTION 'ledger entry currency % does not match account currency %',
            NEW.currency, account_currency
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER ledger_entries_currency_matches_account
    BEFORE INSERT ON ledger_entries
    FOR EACH ROW EXECUTE FUNCTION ledger_check_entry_currency();

-- 3. Statement-level TRUNCATE guard. The row-level function above reads OLD, which does not
-- exist for a statement-level trigger, so this uses its own function.
CREATE OR REPLACE FUNCTION ledger_forbid_truncate()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'the ledger is append-only: TRUNCATE on % is forbidden', TG_TABLE_NAME;
END;
$$;

CREATE TRIGGER ledger_entries_no_truncate
    BEFORE TRUNCATE ON ledger_entries
    FOR EACH STATEMENT EXECUTE FUNCTION ledger_forbid_truncate();

CREATE TRIGGER ledger_transactions_no_truncate
    BEFORE TRUNCATE ON ledger_transactions
    FOR EACH STATEMENT EXECUTE FUNCTION ledger_forbid_truncate();
