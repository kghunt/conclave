DELETE FROM settings WHERE key IN ('google_auth_enabled', 'local_auth_enabled', 'registration_mode');
DROP TABLE IF EXISTS registration_invites;
ALTER TABLE users DROP COLUMN IF EXISTS password_hash;
ALTER TABLE users DROP COLUMN IF EXISTS username;
ALTER TABLE users ALTER COLUMN google_id SET NOT NULL;
