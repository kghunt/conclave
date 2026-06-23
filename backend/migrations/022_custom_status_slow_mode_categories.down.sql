ALTER TABLE users    DROP COLUMN IF EXISTS custom_status;
ALTER TABLE channels DROP COLUMN IF EXISTS slow_mode_seconds;
ALTER TABLE channels DROP COLUMN IF EXISTS category;
