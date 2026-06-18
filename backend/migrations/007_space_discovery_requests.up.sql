ALTER TABLE servers ADD COLUMN show_in_discovery BOOLEAN NOT NULL DEFAULT FALSE;

CREATE TABLE join_requests (
  id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
  server_id  UUID        NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
  user_id    UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  status     TEXT        NOT NULL DEFAULT 'pending',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE(server_id, user_id)
);
