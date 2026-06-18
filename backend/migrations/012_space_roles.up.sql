CREATE TABLE space_roles (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  server_id   UUID NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
  name        TEXT NOT NULL,
  color       TEXT NOT NULL DEFAULT '',
  is_everyone BOOLEAN NOT NULL DEFAULT FALSE,
  position    INT NOT NULL DEFAULT 0,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (server_id, name)
);

CREATE TABLE space_role_members (
  server_id UUID NOT NULL,
  user_id   UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  role_id   UUID NOT NULL REFERENCES space_roles(id) ON DELETE CASCADE,
  PRIMARY KEY (server_id, user_id, role_id)
);

CREATE TABLE channel_role_permissions (
  channel_id UUID NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
  role_id    UUID NOT NULL REFERENCES space_roles(id) ON DELETE CASCADE,
  can_view   BOOLEAN NOT NULL DEFAULT TRUE,
  can_write  BOOLEAN NOT NULL DEFAULT TRUE,
  PRIMARY KEY (channel_id, role_id)
);

-- Create the implicit "everyone" role for all existing servers
INSERT INTO space_roles (server_id, name, is_everyone, position)
SELECT id, 'everyone', TRUE, 0 FROM servers;
