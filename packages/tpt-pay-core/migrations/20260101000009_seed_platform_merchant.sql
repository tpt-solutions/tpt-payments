-- Seed the platform clearing merchant.
--
-- Captured funds are posted against a platform-level clearing account owned by this
-- merchant (see internal/payment/service.go: PlatformMerchantID). Without this row the
-- accounts.merchant_id foreign key fails and every capture would post nothing.
-- Idempotent: safe to run on a database that already has the row.

INSERT INTO merchants (id, name, email, status, kyc_status)
VALUES (
    '00000000-0000-0000-0000-000000000000',
    'TPT Platform (clearing)',
    'clearing@platform.invalid',
    'active',
    'approved'
)
ON CONFLICT (id) DO NOTHING;
