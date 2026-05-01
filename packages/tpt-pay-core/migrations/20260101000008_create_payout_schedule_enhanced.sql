-- Migration: Enhanced payout schedule + reconciliation tables
-- Created: 2026-01-01

-- Add custom schedule columns to payout_schedules
ALTER TABLE payout_schedules
    ADD COLUMN IF NOT EXISTS custom_interval_days INTEGER,
    ADD COLUMN IF NOT EXISTS custom_weekdays INTEGER[] DEFAULT '{}';

-- Add retry tracking to payouts
ALTER TABLE payouts
    ADD COLUMN IF NOT EXISTS retry_count INTEGER DEFAULT 0,
    ADD COLUMN IF NOT EXISTS external_id VARCHAR(255);

-- Create payout reconciliation entries table
CREATE TABLE IF NOT EXISTS payout_reconciliation_entries (
    id                  VARCHAR(64) PRIMARY KEY,
    merchant_id         VARCHAR(64) NOT NULL,
    payout_id           VARCHAR(64),
    bank_transaction_id VARCHAR(255) NOT NULL,
    amount              BIGINT NOT NULL DEFAULT 0,
    currency            VARCHAR(3) NOT NULL,
    status              VARCHAR(32) NOT NULL, -- matched, unmatched, discrepancy
    type                VARCHAR(32) NOT NULL DEFAULT 'payout',
    matched_at          TIMESTAMPTZ,
    discrepancy_reason  TEXT,
    bank_posted_at      BIGINT NOT NULL,
    created_at          BIGINT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_payout_recon_merchant ON payout_reconciliation_entries(merchant_id);
CREATE INDEX IF NOT EXISTS idx_payout_recon_payout_id ON payout_reconciliation_entries(payout_id);
CREATE INDEX IF NOT EXISTS idx_payout_recon_bank_tx ON payout_reconciliation_entries(bank_transaction_id);
CREATE INDEX IF NOT EXISTS idx_payout_recon_date ON payout_reconciliation_entries(bank_posted_at);

-- Create payout reconciliation reports table
CREATE TABLE IF NOT EXISTS payout_reconciliation_reports (
    date        DATE PRIMARY KEY,
    report      JSONB NOT NULL,
    created_at  BIGINT NOT NULL,
    updated_at  BIGINT NOT NULL
);

-- Create payout failure notifications log
CREATE TABLE IF NOT EXISTS payout_failure_notifications (
    id              VARCHAR(64) PRIMARY KEY,
    payout_id       VARCHAR(64) NOT NULL,
    event_type      VARCHAR(64) NOT NULL,
    recipient       VARCHAR(255) NOT NULL,
    status          VARCHAR(32) NOT NULL, -- pending, sent, failed
    sent_at         BIGINT,
    created_at      BIGINT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_payout_notif_payout_id ON payout_failure_notifications(payout_id);
