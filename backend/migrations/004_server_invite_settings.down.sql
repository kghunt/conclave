ALTER TABLE servers
  DROP COLUMN IF EXISTS member_invites_enabled,
  DROP COLUMN IF EXISTS member_invite_expiry_days;
