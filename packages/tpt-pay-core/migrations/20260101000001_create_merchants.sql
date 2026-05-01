-- Create merchants table
-- Merchants are the businesses (operators/users) that use TPT Pay.

CREATE TABLE merchants (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT        NOT NULL,
    email       TEXT        NOT NULL,
    status      TEXT        NOT NULL DEFAULT 'pending'
                            CHECK (status IN ('pending', 'under_review', 'active', 'suspended')),
    kyc_status  TEXT        NOT NULL DEFAULT 'not_started'
                            CHECK (kyc_status IN ('not_started', 'pending', 'approved', 'rejected')),
    metadata    JSONB,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT merchants_email_unique UNIQUE (email)
);

CREATE INDEX merchants_status_idx ON merchants (status);

-- Prevent UPDATE to immutable fields
CREATE OR REPLACE FUNCTION merchants_updated_at()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    NEW.updated_at = NOW();
    -- id and created_at are immutable
    IF NEW.id <> OLD.id THEN
        RAISE EXCEPTION 'merchants.id is immutable';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER merchants_updated_at_trigger
    BEFORE UPDATE ON merchants
    FOR EACH ROW EXECUTE FUNCTION merchants_updated_at();
