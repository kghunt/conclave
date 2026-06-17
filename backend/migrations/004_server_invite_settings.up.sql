ALTER TABLE servers
  ADD COLUMN member_invites_enabled    BOOLEAN NOT NULL DEFAULT true,
  ADD COLUMN member_invite_expiry_days INT     NOT NULL DEFAULT 7;
