-- Dashboard users: people who sign in to the merchant dashboard. They are stored here, not in the
-- frontend, so that lockout and the replay guard for second factors are shared by every server
-- instance. A password is stored as a PBKDF2-SHA256 hash; the TOTP secret is encrypted with the
-- service's secret box and is never returned by any API.
CREATE TABLE dashboard_users (
    id                   TEXT        PRIMARY KEY,                -- 'usr_' + random
    merchant_id          UUID        NOT NULL REFERENCES merchants(id),
    email                TEXT        NOT NULL,
    name                 TEXT        NOT NULL,
    password_hash        TEXT        NOT NULL,
    totp_ciphertext      TEXT,                                   -- NULL until two-factor is enrolled
    totp_last_step       BIGINT      NOT NULL DEFAULT -1,        -- a code's time step is used once
    password_failures    INT         NOT NULL DEFAULT 0 CHECK (password_failures >= 0),
    password_last_failure_at TIMESTAMPTZ,
    password_locked_until TIMESTAMPTZ,
    totp_failures        INT         NOT NULL DEFAULT 0 CHECK (totp_failures >= 0),
    totp_last_failure_at TIMESTAMPTZ,
    totp_locked_until    TIMESTAMPTZ,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (email = lower(email))
);

CREATE UNIQUE INDEX dashboard_users_email_idx ON dashboard_users (email);
CREATE INDEX dashboard_users_merchant_idx ON dashboard_users (merchant_id);
