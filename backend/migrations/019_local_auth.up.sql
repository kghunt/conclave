-- Allow Google-only accounts to coexist with local-auth accounts
ALTER TABLE users ALTER COLUMN google_id DROP NOT NULL;

-- Local auth fields
ALTER TABLE users
    ADD COLUMN username      TEXT UNIQUE,
    ADD COLUMN password_hash TEXT;

-- Admin-generated registration invite codes
CREATE TABLE registration_invites (
    id          UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    code        TEXT        UNIQUE NOT NULL,
    created_by  UUID        REFERENCES users(id) ON DELETE SET NULL,
    max_uses    INT,
    use_count   INT         NOT NULL DEFAULT 0,
    expires_at  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Auth configuration defaults (ON CONFLICT = safe to re-run)
INSERT INTO settings (key, value) VALUES
    ('google_auth_enabled',  'true'),
    ('local_auth_enabled',   'true'),
    ('registration_mode',    'invite')
ON CONFLICT (key) DO NOTHING;
