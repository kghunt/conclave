CREATE TABLE server_bans (
  id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
  server_id  UUID        NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
  user_id    UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  banned_by  UUID        NOT NULL REFERENCES users(id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE(server_id, user_id)
);

ALTER TABLE users ADD COLUMN instance_banned BOOLEAN NOT NULL DEFAULT FALSE;
