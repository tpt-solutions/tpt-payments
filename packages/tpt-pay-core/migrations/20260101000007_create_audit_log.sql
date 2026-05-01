-- Immutable audit log for sensitive operations.
-- Every state-changing API call is recorded here for compliance and debugging.

CREATE TABLE audit_log (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    merchant_id UUID        REFERENCES merchants(id),
    actor_type  TEXT        NOT NULL,   -- 'api_key', 'system', 'admin'
    actor_id    TEXT,                   -- api_key id or system component name
    action      TEXT        NOT NULL,   -- e.g. 'payment_intent.create'
    resource_type TEXT      NOT NULL,   -- e.g. 'payment_intent'
    resource_id TEXT,
    request_id  TEXT,                   -- correlation ID from X-Request-ID header
    ip_address  INET,
    metadata    JSONB,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX audit_log_merchant_id_idx  ON audit_log (merchant_id);
CREATE INDEX audit_log_resource_idx     ON audit_log (resource_type, resource_id);
CREATE INDEX audit_log_created_at_idx   ON audit_log (created_at DESC);

-- Audit log is also append-only.
CREATE OR REPLACE FUNCTION audit_log_immutable()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'audit_log is append-only: % is forbidden', TG_OP;
END;
$$;

CREATE TRIGGER audit_log_no_update
    BEFORE UPDATE ON audit_log
    FOR EACH ROW EXECUTE FUNCTION audit_log_immutable();

CREATE TRIGGER audit_log_no_delete
    BEFORE DELETE ON audit_log
    FOR EACH ROW EXECUTE FUNCTION audit_log_immutable();
